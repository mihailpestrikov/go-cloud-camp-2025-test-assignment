package ratelimit

import (
	"context"
	"errors"
	"go-cloud-camp-2025-test-assignment/config"
	"testing"
	"time"
)

type MockStorage struct {
	tokens        map[string]int
	configs       map[string]struct{ capacity, refillRate int }
	pingError     error
	takeTokensErr error
	configErr     error
}

func NewMockStorage() *MockStorage {
	return &MockStorage{
		tokens:  make(map[string]int),
		configs: make(map[string]struct{ capacity, refillRate int }),
	}
}

func (m *MockStorage) TakeTokens(ctx context.Context, key string, tokensToTake int, capacity int, refillRate int) (bool, int, error) {
	if m.takeTokensErr != nil {
		return false, 0, m.takeTokensErr
	}

	if _, ok := m.tokens[key]; !ok {
		m.tokens[key] = capacity
	}

	if m.tokens[key] >= tokensToTake {
		m.tokens[key] -= tokensToTake
		return true, m.tokens[key], nil
	}

	return false, m.tokens[key], nil
}

func (m *MockStorage) GetClientConfig(ctx context.Context, key string) (capacity int, refillRate int, err error) {
	if m.configErr != nil {
		return 0, 0, m.configErr
	}

	config, ok := m.configs[key]
	if !ok {
		return 0, 0, nil
	}

	return config.capacity, config.refillRate, nil
}

func (m *MockStorage) SetClientConfig(ctx context.Context, key string, capacity int, refillRate int) error {
	if m.configErr != nil {
		return m.configErr
	}

	m.configs[key] = struct{ capacity, refillRate int }{capacity, refillRate}
	return nil
}

func (m *MockStorage) Ping(ctx context.Context) error {
	return m.pingError
}

func (m *MockStorage) Close() error {
	return nil
}

func TestNewTokenBucketRateLimiter(t *testing.T) {
	tests := []struct {
		name      string
		pingError error
		wantErr   bool
	}{
		{
			name:      "Successful creation",
			pingError: nil,
			wantErr:   false,
		},
		{
			name:      "Ping error",
			pingError: errors.New("ping error"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMockStorage()
			store.pingError = tt.pingError

			cfg := &config.RateLimitConfig{
				Default: config.TokenBucketConfig{
					Capacity:   100,
					RefillRate: 10,
				},
			}

			_, err := NewTokenBucketRateLimiter(store, cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTokenBucketRateLimiter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTokenBucketRateLimiter_Allow(t *testing.T) {
	tests := []struct {
		name              string
		clientID          string
		tokens            int
		initialTokens     int
		capacity          int
		refillRate        int
		expectedAllowed   bool
		expectedRemaining int
		storageError      error
		wantErr           bool
	}{
		{
			name:              "Allow when enough tokens",
			clientID:          "client1",
			tokens:            5,
			initialTokens:     10,
			capacity:          20,
			refillRate:        5,
			expectedAllowed:   true,
			expectedRemaining: 5,
			storageError:      nil,
			wantErr:           false,
		},
		{
			name:              "Zero tokens request always allowed",
			clientID:          "client3",
			tokens:            0,
			initialTokens:     10,
			capacity:          20,
			refillRate:        5,
			expectedAllowed:   true,
			expectedRemaining: 0,
			storageError:      nil,
			wantErr:           false,
		},
		{
			name:              "Storage error",
			clientID:          "client4",
			tokens:            5,
			initialTokens:     10,
			capacity:          20,
			refillRate:        5,
			expectedAllowed:   false,
			expectedRemaining: 0,
			storageError:      errors.New("storage error"),
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMockStorage()

			if tt.capacity > 0 {
				store.configs[tt.clientID] = struct{ capacity, refillRate int }{tt.capacity, tt.refillRate}
				store.tokens[tt.clientID] = tt.initialTokens
			}

			if tt.storageError != nil {
				store.takeTokensErr = tt.storageError
			}

			cfg := &config.RateLimitConfig{
				Default: config.TokenBucketConfig{
					Capacity:   50,
					RefillRate: 10,
				},
			}

			limiter, err := NewTokenBucketRateLimiter(store, cfg)
			if err != nil {
				t.Fatalf("Failed to create rate limiter: %v", err)
			}

			ctx := context.Background()
			allowed, remaining, err := limiter.Allow(ctx, tt.clientID, tt.tokens)

			if (err != nil) != tt.wantErr {
				t.Errorf("Allow() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if allowed != tt.expectedAllowed {
				t.Errorf("Allow() allowed = %v, want %v", allowed, tt.expectedAllowed)
			}

			if !tt.wantErr && tt.tokens > 0 && remaining != tt.expectedRemaining {
				t.Errorf("Allow() remaining = %v, want %v", remaining, tt.expectedRemaining)
			}
		})
	}
}

func TestTokenBucketRateLimiter_getClientConfig(t *testing.T) {
	tests := []struct {
		name               string
		clientID           string
		existingConfig     bool
		storedCapacity     int
		storedRefillRate   int
		defaultCapacity    int
		defaultRefillRate  int
		cachedConfig       bool
		expectedCapacity   int
		expectedRefillRate int
		storageError       error
		wantErr            bool
	}{
		{
			name:               "Get existing config from storage",
			clientID:           "client1",
			existingConfig:     true,
			storedCapacity:     100,
			storedRefillRate:   20,
			defaultCapacity:    50,
			defaultRefillRate:  10,
			cachedConfig:       false,
			expectedCapacity:   100,
			expectedRefillRate: 20,
			storageError:       nil,
			wantErr:            false,
		},
		{
			name:               "Use default config when not in storage",
			clientID:           "client2",
			existingConfig:     false,
			storedCapacity:     0,
			storedRefillRate:   0,
			defaultCapacity:    50,
			defaultRefillRate:  10,
			cachedConfig:       false,
			expectedCapacity:   50,
			expectedRefillRate: 10,
			storageError:       nil,
			wantErr:            false,
		},
		{
			name:               "Use cached config",
			clientID:           "client3",
			existingConfig:     true,
			storedCapacity:     100,
			storedRefillRate:   20,
			defaultCapacity:    50,
			defaultRefillRate:  10,
			cachedConfig:       true,
			expectedCapacity:   100,
			expectedRefillRate: 20,
			storageError:       nil,
			wantErr:            false,
		},
		{
			name:               "Storage error",
			clientID:           "client4",
			existingConfig:     false,
			storedCapacity:     0,
			storedRefillRate:   0,
			defaultCapacity:    50,
			defaultRefillRate:  10,
			cachedConfig:       false,
			expectedCapacity:   0,
			expectedRefillRate: 0,
			storageError:       errors.New("storage error"),
			wantErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMockStorage()

			if tt.existingConfig {
				store.configs[tt.clientID] = struct{ capacity, refillRate int }{tt.storedCapacity, tt.storedRefillRate}
			}

			if tt.storageError != nil {
				store.configErr = tt.storageError
			}

			cfg := &config.RateLimitConfig{
				Default: config.TokenBucketConfig{
					Capacity:   tt.defaultCapacity,
					RefillRate: tt.defaultRefillRate,
				},
			}

			limiter, err := NewTokenBucketRateLimiter(store, cfg)
			if err != nil {
				t.Fatalf("Failed to create rate limiter: %v", err)
			}

			if tt.cachedConfig {
				limiter.clientsMu.Lock()
				limiter.clients[tt.clientID] = &clientConfig{
					capacity:   tt.storedCapacity,
					refillRate: tt.storedRefillRate,
				}
				limiter.clientsMu.Unlock()
			}

			ctx := context.Background()
			capacity, refillRate, err := limiter.getClientConfig(ctx, tt.clientID)

			if (err != nil) != tt.wantErr {
				t.Errorf("getClientConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && capacity != tt.expectedCapacity {
				t.Errorf("getClientConfig() capacity = %v, want %v", capacity, tt.expectedCapacity)
			}

			if !tt.wantErr && refillRate != tt.expectedRefillRate {
				t.Errorf("getClientConfig() refillRate = %v, want %v", refillRate, tt.expectedRefillRate)
			}

			if !tt.wantErr && !tt.cachedConfig {
				limiter.clientsMu.RLock()
				cached, exists := limiter.clients[tt.clientID]
				limiter.clientsMu.RUnlock()

				if !exists {
					t.Errorf("getClientConfig() did not cache the client config")
				} else if cached.capacity != tt.expectedCapacity || cached.refillRate != tt.expectedRefillRate {
					t.Errorf("getClientConfig() cached incorrect values: got capacity=%v, refillRate=%v, want capacity=%v, refillRate=%v",
						cached.capacity, cached.refillRate, tt.expectedCapacity, tt.expectedRefillRate)
				}
			}
		})
	}
}

func TestTokenBucketRateLimiter_UpdateClientConfig(t *testing.T) {
	tests := []struct {
		name       string
		clientID   string
		capacity   int
		refillRate int
		invalids   bool
		storageErr error
		wantErr    bool
	}{
		{
			name:       "Valid config update",
			clientID:   "client1",
			capacity:   200,
			refillRate: 25,
			invalids:   false,
			storageErr: nil,
			wantErr:    false,
		},
		{
			name:       "Invalid config values",
			clientID:   "client2",
			capacity:   0,
			refillRate: -5,
			invalids:   true,
			storageErr: nil,
			wantErr:    true,
		},
		{
			name:       "Storage error",
			clientID:   "client3",
			capacity:   200,
			refillRate: 25,
			invalids:   false,
			storageErr: errors.New("storage error"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMockStorage()

			if tt.storageErr != nil {
				store.configErr = tt.storageErr
			}

			cfg := &config.RateLimitConfig{
				Default: config.TokenBucketConfig{
					Capacity:   50,
					RefillRate: 10,
				},
			}

			limiter, err := NewTokenBucketRateLimiter(store, cfg)
			if err != nil {
				t.Fatalf("Failed to create rate limiter: %v", err)
			}

			ctx := context.Background()
			err = limiter.UpdateClientConfig(ctx, tt.clientID, tt.capacity, tt.refillRate)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateClientConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {

				capacity, refillRate, _ := store.GetClientConfig(ctx, tt.clientID)
				if capacity != tt.capacity || refillRate != tt.refillRate {
					t.Errorf("UpdateClientConfig() stored incorrect values: got capacity=%v, refillRate=%v, want capacity=%v, refillRate=%v",
						capacity, refillRate, tt.capacity, tt.refillRate)
				}

				limiter.clientsMu.RLock()
				cached, exists := limiter.clients[tt.clientID]
				limiter.clientsMu.RUnlock()

				if !exists {
					t.Errorf("UpdateClientConfig() did not cache the client config")
				} else if cached.capacity != tt.capacity || cached.refillRate != tt.refillRate {
					t.Errorf("UpdateClientConfig() cached incorrect values: got capacity=%v, refillRate=%v, want capacity=%v, refillRate=%v",
						cached.capacity, cached.refillRate, tt.capacity, tt.refillRate)
				}
			}
		})
	}
}

func TestTokenBucketRateLimiter_Close(t *testing.T) {
	tests := []struct {
		name      string
		hasTicker bool
		wantErr   bool
	}{
		{
			name:      "Close without ticker",
			hasTicker: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMockStorage()

			cfg := &config.RateLimitConfig{
				Default: config.TokenBucketConfig{
					Capacity:   50,
					RefillRate: 10,
				},
			}

			limiter, err := NewTokenBucketRateLimiter(store, cfg)
			if err != nil {
				t.Fatalf("Failed to create rate limiter: %v", err)
			}

			if tt.hasTicker {
				limiter.ticker = time.NewTicker(1 * time.Hour)
			}

			err = limiter.Close()
			if (err != nil) != tt.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tt.wantErr)
			}

			err = limiter.Close()
			if err != nil {
				t.Errorf("Second Close() call should not return error, got: %v", err)
			}
		})
	}
}
