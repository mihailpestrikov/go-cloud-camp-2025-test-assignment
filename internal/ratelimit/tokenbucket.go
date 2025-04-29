package ratelimit

import (
	"context"
	"errors"
	"sync"
	"time"

	"go-cloud-camp-2025-test-assignment/config"
	"go-cloud-camp-2025-test-assignment/internal/storage"

	"github.com/rs/zerolog/log"
)

var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

type RateLimiter interface {
	Allow(ctx context.Context, clientID string, tokens int) (bool, int, error)

	Close() error
}

type TokenBucketRateLimiter struct {
	storage       storage.Storage
	defaultConfig config.TokenBucketConfig
	ticker        *time.Ticker
	stop          chan struct{}
	clientsMu     sync.RWMutex
	clients       map[string]*clientConfig
}

type clientConfig struct {
	capacity   int
	refillRate int
}

func NewTokenBucketRateLimiter(store storage.Storage, cfg *config.RateLimitConfig) (*TokenBucketRateLimiter, error) {
	limiter := &TokenBucketRateLimiter{
		storage:       store,
		defaultConfig: cfg.Default,
		clients:       make(map[string]*clientConfig),
		stop:          make(chan struct{}),
	}

	if err := store.Ping(context.Background()); err != nil {
		return nil, err
	}

	return limiter, nil
}

func (tb *TokenBucketRateLimiter) Allow(ctx context.Context, clientID string, tokens int) (bool, int, error) {
	if tokens <= 0 {

		return true, 0, nil
	}

	capacity, refillRate, err := tb.getClientConfig(ctx, clientID)
	if err != nil {
		log.Error().Err(err).Str("client_id", clientID).Msg("Failed to get client config")
		return false, 0, err
	}

	allowed, remaining, err := tb.storage.TakeTokens(ctx, clientID, tokens, capacity, refillRate)
	if err != nil {
		log.Error().Err(err).Str("client_id", clientID).Msg("Failed to take tokens")
		return false, 0, err
	}

	if !allowed {
		log.Debug().
			Str("client_id", clientID).
			Int("requested", tokens).
			Int("remaining", remaining).
			Int("capacity", capacity).
			Msg("Rate limit exceeded")
		return false, remaining, ErrRateLimitExceeded
	}

	log.Debug().
		Str("client_id", clientID).
		Int("requested", tokens).
		Int("remaining", remaining).
		Int("capacity", capacity).
		Msg("Request allowed")

	return true, remaining, nil
}

func (tb *TokenBucketRateLimiter) getClientConfig(ctx context.Context, clientID string) (capacity int, refillRate int, err error) {

	tb.clientsMu.RLock()
	if config, ok := tb.clients[clientID]; ok {
		tb.clientsMu.RUnlock()
		return config.capacity, config.refillRate, nil
	}
	tb.clientsMu.RUnlock()

	capacity, refillRate, err = tb.storage.GetClientConfig(ctx, clientID)
	if err != nil {
		return 0, 0, err
	}

	if capacity == 0 || refillRate == 0 {
		capacity = tb.defaultConfig.Capacity
		refillRate = tb.defaultConfig.RefillRate
	}

	tb.clientsMu.Lock()
	tb.clients[clientID] = &clientConfig{
		capacity:   capacity,
		refillRate: refillRate,
	}
	tb.clientsMu.Unlock()

	return capacity, refillRate, nil
}

func (tb *TokenBucketRateLimiter) UpdateClientConfig(ctx context.Context, clientID string, capacity, refillRate int) error {

	if capacity <= 0 || refillRate <= 0 {
		return errors.New("capacity and refillRate must be positive")
	}

	if err := tb.storage.SetClientConfig(ctx, clientID, capacity, refillRate); err != nil {
		return err
	}

	tb.clientsMu.Lock()
	tb.clients[clientID] = &clientConfig{
		capacity:   capacity,
		refillRate: refillRate,
	}
	tb.clientsMu.Unlock()

	log.Info().
		Str("client_id", clientID).
		Int("capacity", capacity).
		Int("refill_rate", refillRate).
		Msg("Client rate limit config updated")

	return nil
}

func (tb *TokenBucketRateLimiter) Close() error {

	if tb.ticker != nil {
		tb.ticker.Stop()
		close(tb.stop)
	}
	return nil
}
