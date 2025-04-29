package balancer

import (
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

type RoundRobinBalancer struct {
	*BaseBalancer
	current atomic.Int64
}

func NewRoundRobinBalancer(backends []*Backend) *RoundRobinBalancer {
	return &RoundRobinBalancer{
		BaseBalancer: NewBaseBalancer(backends),
	}
}

func (rb *RoundRobinBalancer) NextBackend() (*Backend, error) {
	healthy := rb.GetHealthyBackends()

	if len(healthy) == 0 {
		log.Warn().Msg("No healthy backends available")
		return nil, ErrNoBackends
	}

	next := rb.current.Add(1) % int64(len(healthy))
	backend := healthy[next]

	log.Debug().Str("backend", backend.URL.String()).Msg("Selected backend using round-robin")
	return backend, nil
}

func (rb *RoundRobinBalancer) Name() string {
	return "round_robin"
}
