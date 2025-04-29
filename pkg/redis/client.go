package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog/log"
	"go-cloud-camp-2025-test-assignment/config"
)

type Client struct {
	client *redis.Client
	mu     sync.RWMutex
	config config.RedisConfig
}

func New(redisConfig config.RedisConfig) (*Client, error) {
	dialTimeout := 5 * time.Second
	readTimeout := 3 * time.Second
	writeTimeout := 3 * time.Second
	poolSize := 10
	minIdleConns := 2

	client := redis.NewClient(&redis.Options{
		Addr:         redisConfig.Addr,
		Password:     redisConfig.Password,
		DB:           redisConfig.DB,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		PoolSize:     poolSize,
		MinIdleConns: minIdleConns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Client{
		client: client,
		config: redisConfig,
	}, nil
}

func (c *Client) Get() *redis.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client.Close()
}

func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Client) Reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		if err := c.client.Close(); err != nil {
			log.Warn().Err(err).Msg("Error closing Redis connection")
		}
	}

	dialTimeout := 5 * time.Second
	readTimeout := 3 * time.Second
	writeTimeout := 3 * time.Second
	poolSize := 10
	minIdleConns := 2

	client := redis.NewClient(&redis.Options{
		Addr:         c.config.Addr,
		Password:     c.config.Password,
		DB:           c.config.DB,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		PoolSize:     poolSize,
		MinIdleConns: minIdleConns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to reconnect to Redis: %w", err)
	}

	c.client = client
	log.Info().Str("addr", c.config.Addr).Msg("Successfully reconnected to Redis")

	return nil
}

func (c *Client) WithRetry(ctx context.Context, fn func(*redis.Client) error, maxRetries int) error {
	var err error

	for i := 0; i <= maxRetries; i++ {
		client := c.Get()

		err = fn(client)

		if err == nil || !isConnectionError(err) {
			return err
		}

		if i == maxRetries {
			return err
		}

		log.Warn().Err(err).Int("retry", i+1).Int("max_retries", maxRetries).Msg("Redis connection error, trying to reconnect")

		if reconnectErr := c.Reconnect(); reconnectErr != nil {
			log.Error().Err(reconnectErr).Msg("Failed to reconnect to Redis")
		}

		backoff := time.Duration(1<<uint(i)) * 100 * time.Millisecond
		if backoff > 2*time.Second {
			backoff = 2 * time.Second
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}

	return err
}

func (c *Client) ExecLuaScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	var result interface{}

	err := c.WithRetry(ctx, func(client *redis.Client) error {
		var err error
		result, err = client.Eval(ctx, script, keys, args...).Result()
		return err
	}, 3)

	return result, err
}

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, redis.ErrClosed) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled)
}
