package balancer

import (
	"github.com/rs/zerolog/log"
)

type LeastConnectionsBalancer struct {
	*BaseBalancer
}

func NewLeastConnectionsBalancer(backends []*Backend) *LeastConnectionsBalancer {
	return &LeastConnectionsBalancer{
		BaseBalancer: NewBaseBalancer(backends),
	}
}

func (lb *LeastConnectionsBalancer) NextBackend() (*Backend, error) {
	healthy := lb.GetHealthyBackends()

	if len(healthy) == 0 {
		log.Warn().Msg("No healthy backends available")
		return nil, ErrNoBackends
	}

	var minIdx int
	minConn := healthy[0].GetActiveConns()

	for i := 1; i < len(healthy); i++ {
		conn := healthy[i].GetActiveConns()
		if conn < minConn {
			minConn = conn
			minIdx = i
		}
	}

	backend := healthy[minIdx]
	log.Debug().
		Str("backend", backend.URL.String()).
		Int32("active_connections", minConn).
		Msg("Selected backend using least-connections")

	return backend, nil
}

func (lb *LeastConnectionsBalancer) Name() string {
	return "least_connections"
}
