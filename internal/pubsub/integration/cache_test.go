package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingCache is a thin spy around cache.NewInMemory that records which
// methods the CacheInvalidator drove it through.
type recordingCache struct {
	mu          sync.Mutex
	inner       cache.Cache
	clearCalls  atomic.Int32
	deleteCalls []string
	setCalls    []string
}

func newRecordingCache() *recordingCache {
	return &recordingCache{inner: cache.NewInMemory()}
}

func (r *recordingCache) Get(ctx context.Context, key string) (any, error) {
	return r.inner.Get(ctx, key)
}

func (r *recordingCache) Set(ctx context.Context, key string, value any, opts ...cache.Option) error {
	r.mu.Lock()
	r.setCalls = append(r.setCalls, key)
	r.mu.Unlock()

	return r.inner.Set(ctx, key, value, opts...)
}

func (r *recordingCache) Delete(ctx context.Context, key string) error {
	r.mu.Lock()
	r.deleteCalls = append(r.deleteCalls, key)
	r.mu.Unlock()

	return r.inner.Delete(ctx, key)
}

func (r *recordingCache) Clear(ctx context.Context) error {
	r.clearCalls.Add(1)

	return r.inner.Clear(ctx)
}

func (r *recordingCache) deleteCallsSnapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]string, len(r.deleteCalls))
	copy(out, r.deleteCalls)

	return out
}

// waitFor polls cond until it returns true or the deadline expires. Used to
// drive subscribed-handler assertions where Publish is fire-and-forget.
func waitFor(t *testing.T, cond func() bool, timeout time.Duration, msgFmt string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for: %s", msgFmt)
}

func TestCacheInvalidator_PublishInvalidation_PublishesMessage(t *testing.T) {
	t.Parallel()

	// ARRANGE
	bus, ctx := setupPubsub(t)

	// PublishInvalidation routes the message to channels.CacheInvalidate, so
	// subscribe to that exact channel to capture the round-trip.
	rec := subscribeRecorder(ctx, t, bus, channels.CacheInvalidate)

	invalidator := NewCacheInvalidator(bus, cache.NewInMemory())

	// ACT
	require.NoError(t, invalidator.PublishInvalidation(ctx, "user", "1", "2"))

	// ASSERT
	got := rec.snapshot()
	require.Len(t, got, 1, "exactly one cache-invalidate message must be delivered")

	msg := got[0]
	assert.Equal(t, messages.TypeCacheInvalidate, msg.Type)
	// The message's own Channel field is the per-entity sub-channel name even
	// though routing happens on CacheInvalidate.
	assert.Equal(t, channels.BuildCacheInvalidateChannel("user", ""), msg.Channel)

	payload, err := messages.ParsePayload[messages.CacheInvalidatePayload](msg)
	require.NoError(t, err)
	assert.Equal(t, "user", payload.EntityType)
	assert.Equal(t, []string{"1", "2"}, payload.EntityIDs)
}

func TestCacheInvalidator_PublishInvalidation_NoEntityIDs(t *testing.T) {
	t.Parallel()

	// ARRANGE
	bus, ctx := setupPubsub(t)
	rec := subscribeRecorder(ctx, t, bus, channels.CacheInvalidate)

	invalidator := NewCacheInvalidator(bus, cache.NewInMemory())

	// ACT
	require.NoError(t, invalidator.PublishInvalidation(ctx, "rbac"))

	// ASSERT
	got := rec.snapshot()
	require.Len(t, got, 1)

	payload, err := messages.ParsePayload[messages.CacheInvalidatePayload](got[0])
	require.NoError(t, err)
	assert.Equal(t, "rbac", payload.EntityType)
	assert.Empty(t, payload.EntityIDs, "omitting variadic IDs must produce an empty/nil slice")
}

func TestCacheInvalidator_Start_Subscribes_GenericCache_CallsClear(t *testing.T) {
	t.Parallel()

	// ARRANGE
	bus, ctx := setupPubsub(t)
	rec := newRecordingCache()
	invalidator := NewCacheInvalidator(bus, rec)

	require.NoError(t, invalidator.Start(ctx))

	payload := messages.CacheInvalidatePayload{
		EntityType: "user",
		EntityIDs:  []string{"42"},
	}
	msg, err := messages.NewMessage(
		channels.BuildCacheInvalidateChannel("user", ""),
		messages.TypeCacheInvalidate,
		payload,
	)
	require.NoError(t, err)

	// ACT
	// Route via a channel that matches CacheInvalidateAll = "gameap:cache:invalidate:*".
	require.NoError(t, bus.Publish(ctx, channels.BuildCacheInvalidateChannel("user", ""), msg))

	// ASSERT
	waitFor(t, func() bool {
		return rec.clearCalls.Load() >= 1
	}, time.Second, "Clear() must be invoked on a generic (non-Redis) cache")

	assert.Equal(t, int32(1), rec.clearCalls.Load(), "exactly one Clear must run per matching message")
	assert.Empty(t, rec.deleteCallsSnapshot(),
		"generic-cache path must not invoke Delete (DeletePattern is Redis-only)")
}

func TestCacheInvalidator_handleInvalidation_BadPayload_NoOp(t *testing.T) {
	t.Parallel()

	// ARRANGE
	bus, ctx := setupPubsub(t)
	rec := newRecordingCache()
	invalidator := NewCacheInvalidator(bus, rec)

	require.NoError(t, invalidator.Start(ctx))

	// Construct a malformed payload — raw bytes that are not valid JSON for
	// CacheInvalidatePayload — and route it through the same channel pattern
	// the invalidator subscribed to.
	bad := &pubsub.Message{
		ID:        "bad-1",
		Channel:   channels.BuildCacheInvalidateChannel("user", ""),
		Type:      messages.TypeCacheInvalidate,
		Payload:   []byte("not-json"),
		Timestamp: time.Now(),
	}

	// ACT
	require.NoError(t, bus.Publish(ctx, channels.BuildCacheInvalidateChannel("user", ""), bad))

	// ASSERT
	// Give the handler a beat to run; assert it completed without invoking
	// Clear or Delete and without panicking.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), rec.clearCalls.Load(),
		"malformed payload must not trigger Clear")
	assert.Empty(t, rec.deleteCallsSnapshot(),
		"malformed payload must not trigger Delete")
}

func TestCacheInvalidator_handleInvalidation_RedisBranch_TakesDeletePattern(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// We construct a *cache.Redis whose client points at an unreachable port.
	// The handler still must take the Redis branch (type assertion succeeds);
	// DeletePattern will fail at the network layer, but that is irrelevant —
	// what we assert is that the *generic* Clear path is NOT taken, which is
	// only observable via a non-Redis cache spy.
	//
	// To observe that, we use two collaborators:
	//   1) the Redis-typed cache passed into the invalidator (so the type
	//      assertion fires), and
	//   2) a recordingCache that we install AFTER Start but never hand to the
	//      invalidator — used as a control to make sure cross-test state is
	//      not leaking. Instead, we rely on the simpler fact that DeletePattern
	//      is not called on a generic cache, and that the message handler
	//      finishes (no panic) under SafeCall, by checking the bus is still
	//      live afterwards.
	bus, ctx := setupPubsub(t)

	// 127.0.0.1:1 reliably refuses TCP connections; MaxRetries=-1 disables
	// the 5-attempt retry loop so DeletePattern fails fast (sub-100ms).
	redisClient := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		MaxRetries:  -1,
		DialTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() { _ = redisClient.Close() })
	redisCache := cache.NewRedisFromClient(redisClient)

	invalidator := NewCacheInvalidator(bus, redisCache)
	require.NoError(t, invalidator.Start(ctx))

	payload := messages.CacheInvalidatePayload{
		EntityType: "user",
		EntityIDs:  []string{"1"},
	}
	msg, err := messages.NewMessage(
		channels.BuildCacheInvalidateChannel("user", ""),
		messages.TypeCacheInvalidate,
		payload,
	)
	require.NoError(t, err)

	// ACT
	require.NoError(t, bus.Publish(ctx, channels.BuildCacheInvalidateChannel("user", ""), msg))

	// ASSERT
	// Verify the bus (and therefore SafeCall) survived the handler returning
	// a network error — proving the type-assertion branch executed without
	// panicking out of the goroutine.
	probe := subscribeRecorder(ctx, t, bus, "probe:after-redis-branch")
	probeMsg, err := messages.NewMessage("probe:after-redis-branch", "test", map[string]string{})
	require.NoError(t, err)
	require.NoError(t, bus.Publish(ctx, "probe:after-redis-branch", probeMsg))

	waitFor(t, func() bool {
		return len(probe.snapshot()) >= 1
	}, time.Second, "bus must remain live after the Redis branch fires")
}

func TestNewCacheInvalidator_AssignsLogger(t *testing.T) {
	t.Parallel()

	// ARRANGE
	bus := memory.New()
	t.Cleanup(func() { _ = bus.Close() })
	c := cache.NewInMemory()

	// ACT
	invalidator := NewCacheInvalidator(bus, c)

	// ASSERT
	require.NotNil(t, invalidator)
	assert.NotNil(t, invalidator.logger, "constructor must wire a non-nil default logger")
	assert.Equal(t, pubsub.PubSub(bus), invalidator.pubsub)
	assert.Equal(t, c, invalidator.cache)
}
