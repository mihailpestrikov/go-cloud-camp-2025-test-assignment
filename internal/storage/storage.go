package storage

import (
	"context"
)

type Storage interface {
	TakeTokens(ctx context.Context, key string, tokensToTake int, capacity int, refillRate int) (bool, int, error)

	GetClientConfig(ctx context.Context, key string) (capacity int, refillRate int, err error)

	SetClientConfig(ctx context.Context, key string, capacity int, refillRate int) error

	Ping(ctx context.Context) error

	Close() error
}

const (
	RateLimitPrefix = "ratelimit:"
	ConfigPrefix    = "config:"
)

func RateLimitKey(clientID string) string {
	return RateLimitPrefix + clientID
}

func ConfigKey(clientID string) string {
	return RateLimitPrefix + ConfigPrefix + clientID
}
