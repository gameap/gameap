package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTyped_RoundTrip_String(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	c := cache.NewInMemory()
	require.NoError(t, c.Set(ctx, "s_key", "hello"))

	// ACT
	got, err := cache.GetTyped[string](ctx, c, "s_key")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "hello", got, "string value must round-trip unchanged")
}

func TestGetTyped_RoundTrip_Int(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	c := cache.NewInMemory()
	require.NoError(t, c.Set(ctx, "i_key", 42))

	// ACT
	got, err := cache.GetTyped[int](ctx, c, "i_key")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, 42, got, "int value must round-trip via JSON re-marshal")
}

func TestGetTyped_RoundTrip_Bool(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	c := cache.NewInMemory()
	require.NoError(t, c.Set(ctx, "b_key", true))

	// ACT
	got, err := cache.GetTyped[bool](ctx, c, "b_key")

	// ASSERT
	require.NoError(t, err)
	assert.True(t, got, "bool value must round-trip unchanged")
}

func TestGetTyped_RoundTrip_Struct(t *testing.T) {
	t.Parallel()

	// ARRANGE
	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	ctx := context.Background()
	c := cache.NewInMemory()
	original := payload{Name: "alice", Count: 7}
	require.NoError(t, c.Set(ctx, "p_key", original))

	// ACT
	got, err := cache.GetTyped[payload](ctx, c, "p_key")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, original.Name, got.Name, "struct Name field must round-trip")
	assert.Equal(t, original.Count, got.Count, "struct Count field must round-trip")
}

func TestGetTyped_RoundTrip_Slice(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	c := cache.NewInMemory()
	original := []string{"alpha", "beta", "gamma"}
	require.NoError(t, c.Set(ctx, "sl_key", original))

	// ACT
	got, err := cache.GetTyped[[]string](ctx, c, "sl_key")

	// ASSERT
	require.NoError(t, err)
	require.Len(t, got, len(original), "slice length must round-trip")
	assert.Equal(t, original, got, "slice contents must round-trip unchanged")
}

func TestGetTyped_PropagatesNotFound(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	c := cache.NewInMemory()

	// ACT
	got, err := cache.GetTyped[string](ctx, c, "absent")

	// ASSERT
	assert.ErrorIs(t, err, cache.ErrNotFound, "GetTyped must surface ErrNotFound from underlying cache")
	assert.Empty(t, got, "GetTyped must return zero value when key is missing")
}

func TestGetTyped_PropagatesUnderlyingCacheError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	fake := &failingCache{getErr: errInjectedGet}

	// ACT
	got, err := cache.GetTyped[string](ctx, fake, "any_key")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected get failure", "GetTyped must propagate underlying Get error")
	assert.Empty(t, got, "GetTyped must return zero value on underlying error")
}

func TestGetTyped_ReturnsUnmarshalErrorOnTypeMismatch(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	c := cache.NewInMemory()
	require.NoError(t, c.Set(ctx, "wrong_type", "not-a-number"))

	// ACT
	got, err := cache.GetTyped[int](ctx, c, "wrong_type")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal to type", "type mismatch must surface unmarshal error")
	assert.Equal(t, 0, got, "GetTyped must return zero value on unmarshal error")
}

func TestSetWithTTL_StoresValueAndExpires(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	c := cache.NewInMemory()
	const ttl = 50 * time.Millisecond

	// ACT
	require.NoError(t, cache.SetWithTTL(ctx, c, "ttl_key", "ttl_value", ttl))

	// ASSERT — value is reachable immediately
	value, err := c.Get(ctx, "ttl_key")
	require.NoError(t, err)
	assert.Equal(t, "ttl_value", value, "value set via SetWithTTL must be retrievable before expiration")

	// ASSERT — value expires after TTL elapses
	time.Sleep(ttl + 30*time.Millisecond)
	value, err = c.Get(ctx, "ttl_key")
	assert.ErrorIs(t, err, cache.ErrNotFound, "value must expire after TTL elapses")
	assert.Nil(t, value, "expired value must be nil")
}

func TestSetWithTTL_PropagatesUnderlyingCacheError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := context.Background()
	fake := &failingCache{setErr: errInjectedSet}

	// ACT
	err := cache.SetWithTTL(ctx, fake, "any_key", "any_value", time.Second)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "injected set failure", "SetWithTTL must propagate underlying Set error")
	assert.Equal(t, int64(1), fake.setCalls.Load(), "SetWithTTL must invoke underlying Set exactly once")
}
