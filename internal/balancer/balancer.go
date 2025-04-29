package balancer

import (
	"context"
	"errors"
	"go-cloud-camp-2025-test-assignment/config"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

type BackendStatus struct {
	URL           *url.URL
	IsAlive       bool
	ActiveConns   int32
	LastChecked   time.Time
	FailureCount  int32
	TotalRequests int64
	FailedReqs    int64
}

type Backend struct {
	URL           *url.URL
	IsAlive       atomic.Bool
	ActiveConns   atomic.Int32
	LastChecked   atomic.Value
	FailureCount  atomic.Int32
	TotalRequests atomic.Int64
	FailedReqs    atomic.Int64
}

func NewBackend(backendURL string) (*Backend, error) {
	u, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}

	b := &Backend{
		URL: u,
	}
	b.IsAlive.Store(true)
	b.LastChecked.Store(time.Now())

	return b, nil
}

func (b *Backend) IncrementActiveConns() {
	b.ActiveConns.Add(1)
}

func (b *Backend) DecrementActiveConns() {
	b.ActiveConns.Add(-1)
}

func (b *Backend) GetActiveConns() int32 {
	return b.ActiveConns.Load()
}

func (b *Backend) MarkUp() {
	b.IsAlive.Store(true)
	b.FailureCount.Store(0)
	b.LastChecked.Store(time.Now())
	log.Info().Str("backend", b.URL.String()).Msg("Backend marked as UP")
}

func (b *Backend) MarkDown() {
	b.IsAlive.Store(false)
	b.LastChecked.Store(time.Now())
	log.Warn().Str("backend", b.URL.String()).Msg("Backend marked as DOWN")
}

func (b *Backend) IsAvailable() bool {
	return b.IsAlive.Load()
}

func (b *Backend) IncrementFailureCount() {
	b.FailureCount.Add(1)
}

func (b *Backend) GetStatus() BackendStatus {
	return BackendStatus{
		URL:           b.URL,
		IsAlive:       b.IsAlive.Load(),
		ActiveConns:   b.ActiveConns.Load(),
		LastChecked:   b.LastChecked.Load().(time.Time),
		FailureCount:  b.FailureCount.Load(),
		TotalRequests: b.TotalRequests.Load(),
		FailedReqs:    b.FailedReqs.Load(),
	}
}

func (b *Backend) RecordRequest(success bool) {
	b.TotalRequests.Add(1)
	if !success {
		b.FailedReqs.Add(1)
	}
}

type Balancer interface {
	NextBackend() (*Backend, error)

	RegisterBackend(backend *Backend)

	RemoveBackend(backend *Backend)

	MarkBackendDown(backend *Backend)

	MarkBackendUp(backend *Backend)

	GetHealthyBackends() []*Backend

	GetAllBackends() []*Backend

	Name() string

	GetStatistics() map[string]BackendStats
}

var (
	ErrNoBackends = errors.New("no backends available")

	ErrNoValidBackends = errors.New("no valid backends in configuration")
)

type BaseBalancer struct {
	backends []*Backend
	mutex    sync.RWMutex
}

type BackendStats struct {
	URL           string  `json:"url"`
	IsAlive       bool    `json:"is_alive"`
	ActiveConns   int32   `json:"active_connections"`
	TotalRequests int64   `json:"total_requests"`
	FailedReqs    int64   `json:"failed_requests"`
	FailureRate   float64 `json:"failure_rate,omitempty"`
}

func NewBaseBalancer(backends []*Backend) *BaseBalancer {
	return &BaseBalancer{
		backends: backends,
	}
}

func (b *BaseBalancer) RegisterBackend(backend *Backend) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, existingBackend := range b.backends {
		if existingBackend.URL.String() == backend.URL.String() {
			log.Warn().Str("url", backend.URL.String()).Msg("Backend already registered")
			return
		}
	}

	b.backends = append(b.backends, backend)
	log.Info().Str("url", backend.URL.String()).Msg("Backend registered")
}

func (b *BaseBalancer) RemoveBackend(backend *Backend) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for i, existingBackend := range b.backends {
		if existingBackend.URL.String() == backend.URL.String() {
			b.backends = append(b.backends[:i], b.backends[i+1:]...)
			log.Info().Str("url", backend.URL.String()).Msg("Backend removed")
			return
		}
	}
}

func (b *BaseBalancer) MarkBackendDown(backend *Backend) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	for _, existingBackend := range b.backends {
		if existingBackend.URL.String() == backend.URL.String() {
			existingBackend.MarkDown()
			return
		}
	}
}

func (b *BaseBalancer) MarkBackendUp(backend *Backend) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	for _, existingBackend := range b.backends {
		if existingBackend.URL.String() == backend.URL.String() {
			existingBackend.MarkUp()
			return
		}
	}
}

func (b *BaseBalancer) GetHealthyBackends() []*Backend {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	var healthy []*Backend
	for _, backend := range b.backends {
		if backend.IsAvailable() {
			healthy = append(healthy, backend)
		}
	}

	return healthy
}

func (b *BaseBalancer) GetAllBackends() []*Backend {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	result := make([]*Backend, len(b.backends))
	copy(result, b.backends)

	return result
}

func BalancerFactory(cfg *config.Config) (Balancer, error) {

	var backends []*Backend
	for _, backendCfg := range cfg.Backends {
		backend, err := NewBackend(backendCfg.URL)
		if err != nil {
			log.Error().Err(err).Str("url", backendCfg.URL).Msg("Failed to create backend")
			continue
		}
		backends = append(backends, backend)
	}

	if len(backends) == 0 {
		log.Error().Msg("No valid backends configured")
		return nil, ErrNoValidBackends
	}

	switch cfg.Balancer.Algorithm {
	case "round_robin":
		return NewRoundRobinBalancer(backends), nil
	case "least_connections":
		return NewLeastConnectionsBalancer(backends), nil
	case "random":
		return NewRandomBalancer(backends), nil
	default:
		log.Warn().Str("algorithm", cfg.Balancer.Algorithm).Msg("Unknown balancing algorithm, using round_robin")
		return NewRoundRobinBalancer(backends), nil
	}
}

func StartHealthChecks(ctx context.Context, balancer Balancer, cfg *config.HealthCheckConfig, healthChecker HealthChecker) {
	if !cfg.Enabled {
		log.Info().Msg("Health checks are disabled")
		return
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	log.Info().Dur("interval", cfg.Interval).Str("path", cfg.Path).Msg("Starting health checks")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping health checks")
			return
		case <-ticker.C:
			backends := balancer.GetAllBackends()
			for _, backend := range backends {
				go func(b *Backend) {
					isHealthy := healthChecker.Check(ctx, b)
					if isHealthy {
						balancer.MarkBackendUp(b)
					} else {
						balancer.MarkBackendDown(b)
					}
				}(backend)
			}
		}
	}
}

func (b *BaseBalancer) GetStatistics() map[string]BackendStats {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	stats := make(map[string]BackendStats)

	for _, backend := range b.backends {
		burl := backend.URL.String()
		totalReqs := backend.TotalRequests.Load()
		failedReqs := backend.FailedReqs.Load()

		var failureRate float64
		if totalReqs > 0 {
			failureRate = float64(failedReqs) / float64(totalReqs) * 100.0
		}

		stats[burl] = BackendStats{
			URL:           burl,
			IsAlive:       backend.IsAvailable(),
			ActiveConns:   backend.GetActiveConns(),
			TotalRequests: totalReqs,
			FailedReqs:    failedReqs,
			FailureRate:   failureRate,
		}
	}

	return stats
}

type HealthChecker interface {
	Check(ctx context.Context, backend *Backend) bool
}
