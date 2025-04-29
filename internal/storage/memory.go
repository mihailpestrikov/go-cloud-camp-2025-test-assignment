package storage

import (
	"context"
	"sync"
	"time"
)

type MemoryStorage struct {
	tokens  map[string]*tokenBucket
	configs map[string]*clientConfig
	mu      sync.RWMutex
}

type tokenBucket struct {
	tokens     int
	lastRefill time.Time
}

type clientConfig struct {
	capacity   int
	refillRate int
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		tokens:  make(map[string]*tokenBucket),
		configs: make(map[string]*clientConfig),
	}
}

func (s *MemoryStorage) TakeTokens(ctx context.Context, key string, tokensToTake int, capacity int, refillRate int) (bool, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bucket, ok := s.tokens[key]
	now := time.Now()

	if !ok {
		bucket = &tokenBucket{
			tokens:     capacity,
			lastRefill: now,
		}
		s.tokens[key] = bucket
	} else {

		elapsed := now.Sub(bucket.lastRefill).Seconds()
		tokensToAdd := int(elapsed * float64(refillRate))

		if tokensToAdd > 0 {
			bucket.tokens = min(capacity, bucket.tokens+tokensToAdd)
			bucket.lastRefill = now
		}
	}

	if bucket.tokens >= tokensToTake {
		bucket.tokens -= tokensToTake
		return true, bucket.tokens, nil
	}

	return false, bucket.tokens, nil
}

func (s *MemoryStorage) GetClientConfig(ctx context.Context, key string) (capacity int, refillRate int, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	config, ok := s.configs[key]
	if !ok {
		return 0, 0, nil
	}

	return config.capacity, config.refillRate, nil
}

func (s *MemoryStorage) SetClientConfig(ctx context.Context, key string, capacity int, refillRate int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if capacity == 0 && refillRate == 0 {
		delete(s.configs, key)
		return nil
	}

	s.configs[key] = &clientConfig{
		capacity:   capacity,
		refillRate: refillRate,
	}

	return nil
}

func (s *MemoryStorage) Ping(ctx context.Context) error {

	return nil
}

func (s *MemoryStorage) Close() error {

	return nil
}
