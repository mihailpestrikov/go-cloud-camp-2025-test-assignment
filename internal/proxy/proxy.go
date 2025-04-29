package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"go-cloud-camp-2025-test-assignment/config"
	"go-cloud-camp-2025-test-assignment/internal/balancer"
	"go-cloud-camp-2025-test-assignment/internal/ratelimit"

	"github.com/rs/zerolog/log"
)

type Proxy struct {
	balancer      balancer.Balancer
	rateLimiter   ratelimit.RateLimiter
	errorHandler  ErrorHandler
	config        *config.Config
	requestLogger RequestLogger
}

type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

type RequestLogger func(r *http.Request, backend *balancer.Backend, statusCode int, duration time.Duration, err error)

type ProxyOption func(*Proxy)

func NewProxy(loadBalancer balancer.Balancer, cfg *config.Config, opts ...ProxyOption) *Proxy {
	p := &Proxy{
		balancer: loadBalancer,
		config:   cfg,
		errorHandler: func(w http.ResponseWriter, r *http.Request, err error) {

			log.Error().Err(err).Str("path", r.URL.Path).Msg("Proxy error")
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		},
		requestLogger: func(r *http.Request, backend *balancer.Backend, statusCode int, duration time.Duration, err error) {

			logger := log.With().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remote_addr", r.RemoteAddr).
				Int("status", statusCode).
				Dur("duration", duration)

			if backend != nil {
				logger = logger.Str("backend", backend.URL.String())
			}

			if err != nil {
				l := logger.Err(err).Logger()
				l.Warn().Msg("Proxy request completed with error")
			} else {
				l := logger.Err(err).Logger()
				l.Info().Msg("Proxy request completed")
			}
		},
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

func WithErrorHandler(handler ErrorHandler) ProxyOption {
	return func(p *Proxy) {
		p.errorHandler = handler
	}
}

func WithRequestLogger(logger RequestLogger) ProxyOption {
	return func(p *Proxy) {
		p.requestLogger = logger
	}
}

func WithRateLimiter(limiter ratelimit.RateLimiter) ProxyOption {
	return func(p *Proxy) {
		p.rateLimiter = limiter
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var backend *balancer.Backend
	var statusCode int = http.StatusOK
	var responseErr error

	defer func() {
		p.requestLogger(r, backend, statusCode, time.Since(start), responseErr)
	}()

	if p.rateLimiter != nil {
		clientIP := getClientIP(r)
		allowed, remaining, err := p.rateLimiter.Allow(r.Context(), clientIP, 1)
		if err != nil {
			log.Error().Err(err).Str("client_ip", clientIP).Msg("Rate limiter error")
			statusCode = http.StatusInternalServerError
			p.errorHandler(w, r, err)
			return
		}

		if !allowed {
			log.Warn().Str("client_ip", clientIP).Msg("Rate limit exceeded")
			statusCode = http.StatusTooManyRequests

			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			w.Header().Set("Retry-After", "1")

			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
	}

	backend, err := p.balancer.NextBackend()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get backend")
		statusCode = http.StatusServiceUnavailable
		p.errorHandler(w, r, err)
		return
	}

	backend.IncrementActiveConns()
	defer backend.DecrementActiveConns()

	proxy := httputil.NewSingleHostReverseProxy(backend.URL)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Origin-Host", backend.URL.Host)
		req.Header.Set("X-Proxy", "Go-Load-Balancer")
	}

	proxy.Transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   p.config.Server.Timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error().
			Err(err).
			Str("backend", backend.URL.String()).
			Str("path", r.URL.Path).
			Msg("Backend request failed")

		statusCode = http.StatusBadGateway
		responseErr = err

		backend.RecordRequest(false)
		backend.IncrementFailureCount()

		p.errorHandler(w, r, err)
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		statusCode = resp.StatusCode

		if statusCode >= 500 {
			backend.RecordRequest(false)
		} else {
			backend.RecordRequest(true)
		}

		return nil
	}

	proxy.ServeHTTP(w, r)
}

func getClientIP(r *http.Request) string {

	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor != "" {

		ips := strings.Split(forwardedFor, ",")
		ip := strings.TrimSpace(ips[0])
		return ip
	}

	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {

		return r.RemoteAddr
	}

	return ip
}
