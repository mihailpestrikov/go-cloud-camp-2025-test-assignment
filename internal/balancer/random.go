package balancer

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type RandomBalancer struct {
	*BaseBalancer
	rnd sync.Pool
}

func NewRandomBalancer(backends []*Backend) *RandomBalancer {
	return &RandomBalancer{
		BaseBalancer: NewBaseBalancer(backends),
	}
}

func (rb *RandomBalancer) NextBackend() (*Backend, error) {
	healthy := rb.GetHealthyBackends()

	if len(healthy) == 0 {
		log.Warn().Msg("No healthy backends available")
		return nil, ErrNoBackends
	}

	idx := time.Now().UnixNano() % int64(len(healthy))
	backend := healthy[idx]

	log.Debug().Str("backend", backend.URL.String()).Msg("Selected backend using random algorithm")
	return backend, nil
}

func (rb *RandomBalancer) Name() string {
	return "random"
}
