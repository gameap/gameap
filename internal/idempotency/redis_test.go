// Redis-backed store tests. They skip when TEST_REDIS_ADDR is not set,
// mirroring internal/locker/redis_test.go.
package idempotency_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/idempotency"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("Skipping Redis idempotency tests because TEST_REDIS_ADDR is not set")
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("TEST_REDIS_PASSWORD"),
	})

	t.Cleanup(func() {
		_ = client.Close()
	})

	if err := client.Ping(t.Context()).Err(); err != nil {
		t.Skipf("Skipping Redis idempotency tests because Redis is not available: %v", err)
	}

	return client
}

// newTestRedisStore isolates every test under its own key prefix.
func newTestRedisStore(t *testing.T, client *redis.Client) (*idempotency.RedisStore, string) {
	t.Helper()

	prefix := "gameap:test:idempotency:" + t.Name() + ":" + strconv.FormatInt(time.Now().UnixNano(), 10) + ":"

	t.Cleanup(func() {
		keys, err := client.Keys(context.Background(), prefix+"*").Result()
		if err == nil && len(keys) > 0 {
			_ = client.Del(context.Background(), keys...).Err()
		}
	})

	return idempotency.NewRedisStore(client, prefix), prefix
}

const redisTestKeyHash = "abc"

func newRedisRecord(userID uint, createdAt time.Time, ttl time.Duration) *domain.IdempotencyKey {
	return &domain.IdempotencyKey{
		UserID:             userID,
		KeyHash:            redisTestKeyHash,
		RequestMethod:      "POST",
		RequestPath:        "/api/servers",
		RequestFingerprint: "fingerprint-" + redisTestKeyHash,
		ResponseStatus:     201,
		ResponseHeaders:    map[string][]string{"Content-Type": {"application/json"}},
		ResponseBody:       []byte(`{"serverId":7}`),
		CreatedAt:          createdAt,
		ExpiresAt:          createdAt.Add(ttl),
	}
}

func TestRedisStore_saves_and_finds_record(t *testing.T) {
	t.Parallel()

	client := newTestRedisClient(t)
	store, prefix := newTestRedisStore(t, client)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Millisecond)

	saved, err := store.Save(ctx, newRedisRecord(3, now, time.Hour))
	require.NoError(t, err)
	require.True(t, saved)

	found, err := store.Find(ctx, 3, "abc", now)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, uint(3), found.UserID)
	assert.Equal(t, "abc", found.KeyHash)
	assert.Equal(t, "POST", found.RequestMethod)
	assert.Equal(t, "/api/servers", found.RequestPath)
	assert.Equal(t, "fingerprint-abc", found.RequestFingerprint)
	assert.Equal(t, 201, found.ResponseStatus)
	assert.Equal(t, map[string][]string{"Content-Type": {"application/json"}}, found.ResponseHeaders)
	assert.JSONEq(t, `{"serverId":7}`, string(found.ResponseBody))
	assert.True(t, now.Equal(found.CreatedAt))
	assert.True(t, now.Add(time.Hour).Equal(found.ExpiresAt))

	ttl, err := client.PTTL(ctx, prefix+"3:abc").Result()
	require.NoError(t, err)
	assert.InDelta(t, time.Hour.Seconds(), ttl.Seconds(), 5)
}

func TestRedisStore_keeps_live_record(t *testing.T) {
	t.Parallel()

	client := newTestRedisClient(t)
	store, _ := newTestRedisStore(t, client)
	ctx := t.Context()
	now := time.Now().UTC()

	saved, err := store.Save(ctx, newRedisRecord(3, now, time.Hour))
	require.NoError(t, err)
	require.True(t, saved)

	second := newRedisRecord(3, now, time.Hour)
	second.ResponseStatus = 500

	saved, err = store.Save(ctx, second)
	require.NoError(t, err)
	assert.False(t, saved)

	found, err := store.Find(ctx, 3, "abc", now)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, 201, found.ResponseStatus)
}

func TestRedisStore_delayed_save_keeps_original_expiration(t *testing.T) {
	t.Parallel()

	client := newTestRedisClient(t)
	store, prefix := newTestRedisStore(t, client)
	now := time.Now().UTC()

	record := newRedisRecord(3, now.Add(-time.Hour), 2*time.Hour)
	saved, err := store.Save(t.Context(), record)
	require.NoError(t, err)
	require.True(t, saved)

	ttl, err := client.PTTL(t.Context(), prefix+"3:abc").Result()
	require.NoError(t, err)
	assert.InDelta(t, time.Hour.Seconds(), ttl.Seconds(), 5)
}

func TestRedisStore_delayed_save_of_expired_record_does_not_block_replacement(t *testing.T) {
	t.Parallel()

	client := newTestRedisClient(t)
	store, _ := newTestRedisStore(t, client)
	ctx := t.Context()
	now := time.Now().UTC()

	saved, err := store.Save(ctx, newRedisRecord(3, now.Add(-2*time.Hour), time.Hour))
	require.NoError(t, err)
	require.True(t, saved)

	found, err := store.Find(ctx, 3, redisTestKeyHash, now)
	require.NoError(t, err)
	require.Nil(t, found)

	replacement := newRedisRecord(3, now, time.Hour)
	replacement.ResponseBody = []byte(`{"serverId":8}`)
	saved, err = store.Save(ctx, replacement)
	require.NoError(t, err)
	require.True(t, saved)

	found, err = store.Find(ctx, 3, redisTestKeyHash, now)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.JSONEq(t, `{"serverId":8}`, string(found.ResponseBody))
}

func TestRedisStore_scopes_keys_by_user(t *testing.T) {
	t.Parallel()

	client := newTestRedisClient(t)
	store, _ := newTestRedisStore(t, client)
	ctx := t.Context()
	now := time.Now().UTC()

	saved, err := store.Save(ctx, newRedisRecord(3, now, time.Hour))
	require.NoError(t, err)
	require.True(t, saved)

	found, err := store.Find(ctx, 4, "abc", now)
	require.NoError(t, err)
	assert.Nil(t, found)

	saved, err = store.Save(ctx, newRedisRecord(4, now, time.Hour))
	require.NoError(t, err)
	assert.True(t, saved)
}

func TestRedisStore_forgets_expired_record(t *testing.T) {
	t.Parallel()

	client := newTestRedisClient(t)
	store, _ := newTestRedisStore(t, client)
	ctx := t.Context()
	now := time.Now().UTC()

	saved, err := store.Save(ctx, newRedisRecord(3, now, 100*time.Millisecond))
	require.NoError(t, err)
	require.True(t, saved)

	require.Eventually(t, func() bool {
		found, findErr := store.Find(ctx, 3, "abc", time.Now())

		return findErr == nil && found == nil
	}, 5*time.Second, 50*time.Millisecond)

	saved, err = store.Save(ctx, newRedisRecord(3, time.Now().UTC(), time.Hour))
	require.NoError(t, err)
	assert.True(t, saved)
}

func TestRedisStore_rejects_record_expiring_before_creation(t *testing.T) {
	t.Parallel()

	client := newTestRedisClient(t)
	store, _ := newTestRedisStore(t, client)
	now := time.Now().UTC()

	saved, err := store.Save(t.Context(), newRedisRecord(3, now, 0))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "idempotency record expires before it is created")
	assert.False(t, saved)
}

//nolint:paralleltest // changes the server-wide maxmemory settings of the test Redis
func TestWarnIfEvictable(t *testing.T) {
	client := newTestRedisClient(t)
	ctx := t.Context()

	original, err := client.ConfigGet(ctx, "maxmemory*").Result()
	if err != nil {
		t.Skipf("Skipping because CONFIG is not available: %v", err)
	}

	t.Cleanup(func() {
		_ = client.ConfigSet(context.Background(), "maxmemory", original["maxmemory"]).Err()
		_ = client.ConfigSet(context.Background(), "maxmemory-policy", original["maxmemory-policy"]).Err()
	})

	tests := []struct {
		name      string
		maxmemory string
		policy    string
		wantWarn  bool
	}{
		{
			name:      "evicting_policy_with_memory_limit",
			maxmemory: "512mb",
			policy:    "allkeys-lru",
			wantWarn:  true,
		},
		{
			name:      "volatile_policy_evicts_keys_with_ttl",
			maxmemory: "512mb",
			policy:    "volatile-lru",
			wantWarn:  true,
		},
		{
			name:      "noeviction_policy",
			maxmemory: "512mb",
			policy:    "noeviction",
			wantWarn:  false,
		},
		{
			name:      "no_memory_limit",
			maxmemory: "0",
			policy:    "allkeys-lru",
			wantWarn:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, client.ConfigSet(ctx, "maxmemory", tt.maxmemory).Err())
			require.NoError(t, client.ConfigSet(ctx, "maxmemory-policy", tt.policy).Err())

			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))

			idempotency.WarnIfEvictable(ctx, client, logger)

			if tt.wantWarn {
				assert.Contains(t, logs.String(), "level=WARN")
				assert.Contains(t, logs.String(), "maxmemory_policy="+tt.policy)
			} else {
				assert.Empty(t, logs.String())
			}
		})
	}
}
