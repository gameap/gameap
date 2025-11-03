package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisKeyPrefix = "gameap:"

type Redis struct {
	client *redis.Client
}

// NewRedis creates a new Redis cache instance.
func NewRedis(addr string, password string, db int) (*Redis, error) {
	redis.SetLogger(&redisLoggerAdapter{})

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Redis{
		client: client,
	}, nil
}

// NewRedisFromClient creates a Redis cache from an existing client.
func NewRedisFromClient(client *redis.Client) *Redis {
	return &Redis{
		client: client,
	}
}

// Get retrieves a value from cache.
func (r *Redis) Get(ctx context.Context, key string) (any, error) {
	val, err := r.client.Get(ctx, redisKeyPrefix+key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("redis get error: %w", err)
	}

	var result any
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return result, nil
}

// Set stores a value in cache with optional TTL.
func (r *Redis) Set(ctx context.Context, key string, value any, options ...Option) error {
	opts := ApplyOptions(options...)

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	expiration := time.Duration(0)
	if opts.Expiration > 0 {
		expiration = opts.Expiration
	}

	if err := r.client.Set(ctx, redisKeyPrefix+key, data, expiration).Err(); err != nil {
		return fmt.Errorf("redis set error: %w", err)
	}

	return nil
}

// Delete removes a key from cache.
func (r *Redis) Delete(ctx context.Context, key string) error {
	if err := r.client.Del(ctx, redisKeyPrefix+key).Err(); err != nil {
		return fmt.Errorf("redis delete error: %w", err)
	}

	return nil
}

// Clear removes all keys from the current database.
func (r *Redis) Clear(ctx context.Context) error {
	if err := r.client.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("redis clear error: %w", err)
	}

	return nil
}

// DeletePattern removes all keys matching a pattern.
func (r *Redis) DeletePattern(ctx context.Context, pattern string) error {
	iter := r.client.Scan(ctx, 0, redisKeyPrefix+pattern, 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("redis scan error: %w", err)
	}

	if len(keys) > 0 {
		if err := r.client.Del(ctx, keys...).Err(); err != nil {
			return fmt.Errorf("redis delete pattern error: %w", err)
		}
	}

	return nil
}

// Close closes the Redis connection.
func (r *Redis) Close() error {
	return r.client.Close()
}

// GetTyped retrieves a typed value from cache.
func GetTyped[T any](ctx context.Context, cache Cache, key string) (T, error) {
	var zero T

	val, err := cache.Get(ctx, key)
	if err != nil {
		return zero, err
	}

	// Re-marshal and unmarshal to get the correct type
	data, err := json.Marshal(val)
	if err != nil {
		return zero, fmt.Errorf("failed to marshal cached value: %w", err)
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, fmt.Errorf("failed to unmarshal to type: %w", err)
	}

	return result, nil
}

// SetWithTTL is a convenience function for setting with TTL.
func SetWithTTL(ctx context.Context, cache Cache, key string, value any, ttl time.Duration) error {
	return cache.Set(ctx, key, value, WithExpiration(ttl))
}

type redisLoggerAdapter struct{}

func (r redisLoggerAdapter) Printf(ctx context.Context, format string, v ...any) {
	slog.InfoContext(ctx, "redis client: "+fmt.Sprintf(format, v...))
}
