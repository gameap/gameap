package enrollment

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupKeyManager_Validate_from_cache(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cacheInstance := cache.NewInMemory()
	m := NewSetupKeyManager(cacheInstance, "")

	err := cacheInstance.Set(ctx, SetupKeyCacheKey, "stored-key")
	require.NoError(t, err)

	assert.NoError(t, m.Validate(ctx, "stored-key"))
	assert.ErrorIs(t, m.Validate(ctx, "wrong-key"), ErrInvalidSetupKey)
}

func TestSetupKeyManager_Validate_cache_takes_priority_over_env(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cacheInstance := cache.NewInMemory()
	m := NewSetupKeyManager(cacheInstance, "env-key")

	err := cacheInstance.Set(ctx, SetupKeyCacheKey, "cache-key")
	require.NoError(t, err)

	assert.NoError(t, m.Validate(ctx, "cache-key"))
	assert.ErrorIs(t, m.Validate(ctx, "env-key"), ErrInvalidSetupKey)
}

func TestSetupKeyManager_Validate_env_fallback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cacheInstance := cache.NewInMemory()
	m := NewSetupKeyManager(cacheInstance, "env-key")

	assert.NoError(t, m.Validate(ctx, "env-key"))
	assert.ErrorIs(t, m.Validate(ctx, "wrong-key"), ErrInvalidSetupKey)
}

func TestSetupKeyManager_Validate_not_configured(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cacheInstance := cache.NewInMemory()
	m := NewSetupKeyManager(cacheInstance, "")

	err := m.Validate(ctx, "any-key")
	assert.ErrorIs(t, err, ErrSetupKeyNotConfigured)
}

func TestSetupKeyManager_Generate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cacheInstance := cache.NewInMemory()
	m := NewSetupKeyManager(cacheInstance, "")

	key, err := m.Generate(ctx)
	require.NoError(t, err)
	assert.Len(t, key, setupKeyLength)

	storedKey, err := m.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, key, storedKey)
}

func TestSetupKeyManager_Set_and_Get(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cacheInstance := cache.NewInMemory()
	m := NewSetupKeyManager(cacheInstance, "")

	err := m.Set(ctx, "my-custom-key")
	require.NoError(t, err)

	key, err := m.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "my-custom-key", key)
}

func TestSetupKeyManager_Invalidate_cache_key(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cacheInstance := cache.NewInMemory()
	m := NewSetupKeyManager(cacheInstance, "")

	err := m.Set(ctx, "key-to-invalidate")
	require.NoError(t, err)

	err = m.Invalidate(ctx)
	require.NoError(t, err)

	_, err = m.Get(ctx)
	assert.ErrorIs(t, err, ErrSetupKeyNotConfigured)
}

func TestSetupKeyManager_Invalidate_env_key_burns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cacheInstance := cache.NewInMemory()
	m := NewSetupKeyManager(cacheInstance, "env-key-value")

	key, err := m.Get(ctx)
	require.NoError(t, err)
	assert.Equal(t, "env-key-value", key)

	err = m.Invalidate(ctx)
	require.NoError(t, err)

	_, err = m.Get(ctx)
	assert.ErrorIs(t, err, ErrSetupKeyNotConfigured)
}

func TestSetupKeyManager_Delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cacheInstance := cache.NewInMemory()
	m := NewSetupKeyManager(cacheInstance, "")

	err := m.Set(ctx, "key-to-delete")
	require.NoError(t, err)

	err = m.Delete(ctx)
	require.NoError(t, err)

	_, err = m.Get(ctx)
	assert.ErrorIs(t, err, ErrSetupKeyNotConfigured)
}
