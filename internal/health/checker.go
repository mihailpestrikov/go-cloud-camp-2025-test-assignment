package health

import (
	"context"
	"net/http"
	"time"

	"go-cloud-camp-2025-test-assignment/config"
	"go-cloud-camp-2025-test-assignment/internal/balancer"

	"github.com/rs/zerolog/log"
)

type HTTPHealthChecker struct {
	client    *http.Client
	path      string
	timeout   time.Duration
	threshold int
}

func NewHTTPHealthChecker(cfg *config.HealthCheckConfig) *HTTPHealthChecker {

	timeout := cfg.Interval / 2
	if timeout > 5*time.Second {
		timeout = 5 * time.Second
	}

	return &HTTPHealthChecker{
		client: &http.Client{
			Timeout: timeout,

			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		path:      cfg.Path,
		timeout:   timeout,
		threshold: 3,
	}
}

func (hc *HTTPHealthChecker) Check(ctx context.Context, backend *balancer.Backend) bool {
	requestURL := backend.URL.String() + hc.path

	reqCtx, cancel := context.WithTimeout(ctx, hc.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", requestURL, nil)
	if err != nil {
		log.Error().
			Err(err).
			Str("backend", backend.URL.String()).
			Msg("Failed to create health check request")
		return false
	}

	req.Header.Set("User-Agent", "LoadBalancer-HealthCheck/1.0")

	resp, err := hc.client.Do(req)
	if err != nil {
		log.Debug().
			Err(err).
			Str("backend", backend.URL.String()).
			Msg("Health check failed")

		if backend.IsAvailable() {
			backend.IncrementFailureCount()

			if backend.FailureCount.Load() >= int32(hc.threshold) {
				log.Warn().
					Str("backend", backend.URL.String()).
					Int32("failure_count", backend.FailureCount.Load()).
					Int("threshold", hc.threshold).
					Msg("Backend marked as DOWN after exceeding failure threshold")
				return false
			}

			return true
		}
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		if !backend.IsAvailable() {
			log.Info().
				Str("backend", backend.URL.String()).
				Int("status", resp.StatusCode).
				Msg("Backend health check passed, marking as UP")
		}
		return true
	}

	log.Debug().
		Str("backend", backend.URL.String()).
		Int("status", resp.StatusCode).
		Msg("Backend returned error status code")

	if backend.IsAvailable() {
		backend.IncrementFailureCount()

		if backend.FailureCount.Load() >= int32(hc.threshold) {
			log.Warn().
				Str("backend", backend.URL.String()).
				Int("status", resp.StatusCode).
				Int32("failure_count", backend.FailureCount.Load()).
				Int("threshold", hc.threshold).
				Msg("Backend marked as DOWN after exceeding failure threshold")
			return false
		}

		return true
	}

	return false
}
