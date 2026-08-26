// RedisLocker integration tests rely on a reachable Redis. They self-skip when
// TEST_REDIS_ADDR is not set, mirroring internal/cache/redis_test.go. We do not
// add miniredis as a dev dependency — keeping behavior identical to production.

package locker_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/acme/locker"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const redisLockKeyPrefix = "gameap:acme:lock:"

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

	return locker.NewRedisLocker(client), client
}

func cleanupKey(t *testing.T, client *redis.Client, key string) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = client.Del(ctx, redisLockKeyPrefix+key).Err()
	})
}

func TestRedisLocker_Acquire(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			// ARRANGE
			l, client := setupRedisLocker(t)
			key := "acquire-" + tt.name
			cleanupKey(t, client, key)

			// ACT
			lock, err := l.Acquire(context.Background(), key, tt.ttl)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, lock)

			ctx := context.Background()
			exists, err := client.Exists(ctx, redisLockKeyPrefix+key).Result()
			require.NoError(t, err)
			assert.Equal(t, int64(1), exists, "redis key must be created on acquire")

			require.NoError(t, lock.Release(ctx))
		})
	}
}

func TestRedisLocker_AcquireReturnsErrLockedOnCollision(t *testing.T) {
	t.Parallel()

	// ARRANGE
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	key := "collision"
	cleanupKey(t, client, key)

	first, err := l.Acquire(ctx, key, 5*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Release(ctx) })

	// ACT
	_, err = l.Acquire(ctx, key, 5*time.Second)

	// ASSERT
	require.Error(t, err)
	assert.True(t, errors.Is(err, locker.ErrLocked), "second Acquire must return ErrLocked")
}

func TestRedisLocker_ReleaseRemovesKeyOnlyWhenTokenMatches(t *testing.T) {
	t.Parallel()

	// ARRANGE
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	key := "release-token-mismatch"
	cleanupKey(t, client, key)

	lock, err := l.Acquire(ctx, key, 5*time.Second)
	require.NoError(t, err)

	// Overwrite key with a foreign token, simulating a different holder.
	require.NoError(t, client.Set(ctx, redisLockKeyPrefix+key, "foreign-token", 5*time.Second).Err())

	// ACT
	err = lock.Release(ctx)

	// ASSERT
	require.NoError(t, err, "Release must not error when token mismatches")

	val, err := client.Get(ctx, redisLockKeyPrefix+key).Result()
	require.NoError(t, err, "foreign-token entry must remain after our Release")
	assert.Equal(t, "foreign-token", val,
		"Release must not delete a key it does not own (Lua atomic check)")
}

func TestRedisLocker_ReleaseIdempotent(t *testing.T) {
	t.Parallel()

	// ARRANGE
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	key := "release-idempotent"
	cleanupKey(t, client, key)

	lock, err := l.Acquire(ctx, key, 5*time.Second)
	require.NoError(t, err)

	// ACT
	require.NoError(t, lock.Release(ctx))
	err = lock.Release(ctx)

	// ASSERT
	require.NoError(t, err, "double Release must be a silent no-op")
}

func TestRedisLocker_RefreshExtendsTTL(t *testing.T) {
	t.Parallel()

	// ARRANGE
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	key := "refresh-extend"
	cleanupKey(t, client, key)

	lock, err := l.Acquire(ctx, key, 200*time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lock.Release(ctx) })

	// ACT
	require.NoError(t, lock.Refresh(ctx, 5*time.Second))

	// ASSERT
	pttl, err := client.PTTL(ctx, redisLockKeyPrefix+key).Result()
	require.NoError(t, err)
	assert.Greater(t, pttl, 1*time.Second,
		"PTTL after Refresh(5s) must be at least 1s, got %v", pttl)
}

func TestRedisLocker_RefreshRejectsNonPositiveTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ttl  time.Duration
	}{
		{name: "zero_ttl", ttl: 0},
		{name: "negative_ttl", ttl: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			l, client := setupRedisLocker(t)
			ctx := context.Background()
			key := "refresh-bad-ttl-" + tt.name
			cleanupKey(t, client, key)

			lock, err := l.Acquire(ctx, key, 5*time.Second)
			require.NoError(t, err)
			t.Cleanup(func() { _ = lock.Release(ctx) })

			// ACT
			err = lock.Refresh(ctx, tt.ttl)

			// ASSERT
			require.Error(t, err)
			assert.Contains(t, err.Error(), "ttl must be positive")
		})
	}
}

func TestRedisLocker_RefreshAfterReleaseReturnsError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	key := "refresh-after-release"
	cleanupKey(t, client, key)

	lock, err := l.Acquire(ctx, key, 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, lock.Release(ctx))

	// ACT
	err = lock.Refresh(ctx, 1*time.Second)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lock already released")
}

func TestRedisLocker_RefreshReturnsErrLockLostWhenKeyTaken(t *testing.T) {
	t.Parallel()

	// ARRANGE
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	key := "refresh-lost"
	cleanupKey(t, client, key)

	lock, err := l.Acquire(ctx, key, 5*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lock.Release(ctx) })

	// Simulate the lock entry being stolen / expired.
	require.NoError(t, client.Del(ctx, redisLockKeyPrefix+key).Err())

	// ACT
	err = lock.Refresh(ctx, 1*time.Second)

	// ASSERT
	require.Error(t, err)
	assert.True(t, errors.Is(err, locker.ErrLockLost),
		"Refresh against a missing/replaced key must return ErrLockLost; got %v", err)
}

func TestRedisLocker_ConcurrentAcquireOnlyOneSucceeds(t *testing.T) {
	t.Parallel()

	// ARRANGE
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	key := "concurrent-acquire"
	cleanupKey(t, client, key)

	const goroutines = 10
	var wg sync.WaitGroup
	var successes atomic.Int32
	locks := make(chan acmeLock, goroutines)

	// ACT
	for range goroutines {
		wg.Go(func() {
			lock, err := l.Acquire(ctx, key, 5*time.Second)
			if err == nil {
				successes.Add(1)
				locks <- lock
			}
		})
	}

	wg.Wait()
	close(locks)

	// ASSERT
	assert.Equal(t, int32(1), successes.Load(),
		"exactly one of %d concurrent Acquire calls must succeed", goroutines)

	for lock := range locks {
		require.NoError(t, lock.Release(ctx))
	}
}

func TestRedisLocker_LockExpiresNaturallyAfterTTL(t *testing.T) {
	t.Parallel()

	// ARRANGE
	l, client := setupRedisLocker(t)
	ctx := context.Background()
	key := "natural-expiry"
	cleanupKey(t, client, key)

	first, err := l.Acquire(ctx, key, 200*time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Release(ctx) })

	// ACT
	time.Sleep(350 * time.Millisecond)

	second, err := l.Acquire(ctx, key, 1*time.Second)

	// ASSERT
	require.NoError(t, err, "second Acquire must succeed after the first lock's TTL elapses")
	require.NotNil(t, second)
	require.NoError(t, second.Release(ctx))
}

// acmeLock is a local alias for the acme.Lock interface so the test does not
// need to import internal/acme just for the type. The redis_test runs in
// internal/acme/locker_test, which already imports the locker package.
type acmeLock interface {
	Release(ctx context.Context) error
	Refresh(ctx context.Context, ttl time.Duration) error
}
