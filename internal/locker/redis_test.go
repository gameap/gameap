// RedisLocker integration tests rely on a reachable Redis. They self-skip when
// TEST_REDIS_ADDR is not set, mirroring internal/cache/redis_test.go. We do not
// add miniredis as a dev dependency — keeping behavior identical to production.

package locker_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/locker"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const redisTestLockKeyPrefix = "gameap:lock:test:"

func setupRedisLocker(t *testing.T) (*locker.RedisLocker, *redis.Client) {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("Skipping Redis locker tests because TEST_REDIS_ADDR is not set")
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("TEST_REDIS_PASSWORD"),
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping Redis locker tests because Redis is not available: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
	})

	return locker.NewRedisLocker(client, redisTestLockKeyPrefix), client
}

func cleanupRedisKey(t *testing.T, client *redis.Client, key string) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = client.Del(ctx, redisTestLockKeyPrefix+key).Err()
	})
}

func TestRedisLocker_Acquire(t *testing.T) {
	tests := []struct {
		name      string
		ttl       time.Duration
		wantError string
	}{
		{name: "valid_ttl_acquires", ttl: 1 * time.Second},
		{name: "zero_ttl_rejected", ttl: 0, wantError: "ttl must be positive"},
		{name: "negative_ttl_rejected", ttl: -time.Second, wantError: "ttl must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, client := setupRedisLocker(t)
			key := "acquire-" + tt.name
			cleanupRedisKey(t, client, key)

			lock, err := l.Acquire(context.Background(), key, tt.ttl)
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, lock)

			err = lock.Release(context.Background())
			require.NoError(t, err)
		})
	}
}

func TestRedisLocker_MutualExclusion(t *testing.T) {
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	cleanupRedisKey(t, client, "shared")

	first, err := l.Acquire(ctx, "shared", 5*time.Second)
	require.NoError(t, err)

	_, err = l.Acquire(ctx, "shared", 5*time.Second)
	assert.True(t, errors.Is(err, locker.ErrLocked))

	require.NoError(t, first.Release(ctx))

	second, err := l.Acquire(ctx, "shared", 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, second.Release(ctx))
}

func TestRedisLocker_KeyPrefixApplied(t *testing.T) {
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	cleanupRedisKey(t, client, "prefixed")

	lock, err := l.Acquire(ctx, "prefixed", 5*time.Second)
	require.NoError(t, err)

	exists, err := client.Exists(ctx, redisTestLockKeyPrefix+"prefixed").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)

	require.NoError(t, lock.Release(ctx))
}

func TestRedisLocker_RefreshLostAfterExpiry(t *testing.T) {
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	cleanupRedisKey(t, client, "expired")

	lock, err := l.Acquire(ctx, "expired", 100*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond)

	err = lock.Refresh(ctx, 1*time.Second)
	assert.True(t, errors.Is(err, locker.ErrLockLost))
}
