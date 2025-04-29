package storage

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	redisClient "go-cloud-camp-2025-test-assignment/pkg/redis"
)

type RedisStorage struct {
	client *redisClient.Client
}

func NewRedisStorage(client *redisClient.Client) (*RedisStorage, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}

	if err := client.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("redis connection check failed: %w", err)
	}

	return &RedisStorage{
		client: client,
	}, nil
}

const tokenBucketScript = `
local key = KEYS[1]
local tokensToTake = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local refillRate = tonumber(ARGV[3])
local now = tonumber(ARGV[4])

-- Получаем текущее состояние
local tokens = tonumber(redis.call('HGET', key, 'tokens') or capacity)
local lastRefill = tonumber(redis.call('HGET', key, 'lastRefill') or now)

-- Вычисляем, сколько времени прошло и сколько токенов нужно добавить
local elapsed = math.max(0, now - lastRefill)
local tokensToAdd = math.min(capacity - tokens, math.floor(elapsed * refillRate / 1000))

-- Обновляем количество токенов
tokens = math.min(capacity, tokens + tokensToAdd)

-- Проверяем, можно ли взять токены
local allowed = tokens >= tokensToTake

-- Если можно, уменьшаем количество токенов
if allowed then
	tokens = tokens - tokensToTake
end

-- Сохраняем обновленное состояние
redis.call('HSET', key, 'tokens', tokens)
redis.call('HSET', key, 'lastRefill', now)

return {allowed and 1 or 0, tokens}
`

func (s *RedisStorage) TakeTokens(ctx context.Context, key string, tokensToTake int, capacity int, refillRate int) (bool, int, error) {
	key = RateLimitKey(key)
	now := time.Now().UnixMilli()

	result, err := s.client.ExecLuaScript(
		ctx,
		tokenBucketScript,
		[]string{key},
		tokensToTake, capacity, refillRate, now,
	)
	if err != nil {
		return false, 0, fmt.Errorf("failed to execute token bucket script: %w", err)
	}

	resultArray, ok := result.([]interface{})
	if !ok || len(resultArray) != 2 {
		return false, 0, fmt.Errorf("unexpected result from Redis: %v", result)
	}

	allowed := resultArray[0].(int64) == 1
	tokens := int(resultArray[1].(int64))

	return allowed, tokens, nil
}

func (s *RedisStorage) GetClientConfig(ctx context.Context, key string) (capacity int, refillRate int, err error) {
	configKey := ConfigKey(key)

	var result map[string]string
	err = s.client.WithRetry(ctx, func(client *redis.Client) error {
		var err error
		result, err = client.HGetAll(ctx, configKey).Result()
		return err
	}, 3)

	if err != nil {
		return 0, 0, fmt.Errorf("failed to get client config: %w", err)
	}

	if len(result) == 0 {
		return 0, 0, nil
	}

	capacity, _ = strconv.Atoi(result["capacity"])
	refillRate, _ = strconv.Atoi(result["refillRate"])

	return capacity, refillRate, nil
}

func (s *RedisStorage) SetClientConfig(ctx context.Context, key string, capacity int, refillRate int) error {
	configKey := ConfigKey(key)

	err := s.client.WithRetry(ctx, func(client *redis.Client) error {
		_, err := client.HSet(ctx, configKey, map[string]interface{}{
			"capacity":   capacity,
			"refillRate": refillRate,
		}).Result()
		return err
	}, 3)

	if err != nil {
		return fmt.Errorf("failed to set client config: %w", err)
	}

	return nil
}

func (s *RedisStorage) Ping(ctx context.Context) error {
	return s.client.Ping(ctx)
}

func (s *RedisStorage) Close() error {
	return s.client.Close()
}
