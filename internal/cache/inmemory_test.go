package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestInMemoryCache(t *testing.T) {
	t.Parallel()

	suite.Run(t, cache.NewCacheSuite(
		func(_ *testing.T) cache.Cache {
			return cache.NewInMemory()
		},
	))
}

func TestInMemoryCacheCleanup(t *testing.T) {
	t.Parallel()

	c := cache.NewInMemory()
	ctx := context.Background()

	t.Run("cleanup_expired", func(t *testing.T) {
		t.Parallel()

		err := c.Set(ctx, "expired_1", "value1", cache.WithExpiration(1*time.Millisecond))
		require.NoError(t, err)

		err = c.Set(ctx, "expired_2", "value2", cache.WithExpiration(1*time.Millisecond))
		require.NoError(t, err)

		err = c.Set(ctx, "not_expired", "value3", cache.WithExpiration(10*time.Second))
		require.NoError(t, err)

		err = c.Set(ctx, "no_expiration", "value4")
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		c.StartCleanup(1 * time.Millisecond)
		time.Sleep(50 * time.Millisecond)

		_, err = c.Get(ctx, "expired_1")
		require.ErrorIs(t, err, cache.ErrNotFound)

		_, err = c.Get(ctx, "expired_2")
		require.ErrorIs(t, err, cache.ErrNotFound)

		value, err := c.Get(ctx, "not_expired")
		require.NoError(t, err)
		require.Equal(t, "value3", value)

		value, err = c.Get(ctx, "no_expiration")
		require.NoError(t, err)
		require.Equal(t, "value4", value)
	})
}

func BenchmarkInMemoryCache_Set(b *testing.B) {
	c := cache.NewInMemory()
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		key := "bench_key_" + string(rune(b.N%1000))
		_ = c.Set(ctx, key, "benchmark_value")
	}
}

func BenchmarkInMemoryCache_Get(b *testing.B) {
	c := cache.NewInMemory()
	ctx := context.Background()

	for i := range 1000 {
		key := "bench_key_" + string(rune(i))
		_ = c.Set(ctx, key, "benchmark_value")
	}

	b.ResetTimer()
	for range b.N {
		key := "bench_key_" + string(rune(b.N%1000))
		_, _ = c.Get(ctx, key)
	}
}

func BenchmarkInMemoryCache_Delete(b *testing.B) {
	c := cache.NewInMemory()
	ctx := context.Background()

	b.ResetTimer()
	for i := range b.N {
		key := "bench_key_" + string(rune(i))
		_ = c.Set(ctx, key, "benchmark_value")
		_ = c.Delete(ctx, key)
	}
}
