package cache_test

// TODO: InMemory.StartCleanup spawns a ticker-driven goroutine with no stop
// channel or context, so every test that calls it leaks one goroutine plus
// one ticker for the lifetime of the test process. Tests in this file run
// the cleanup loop with a short interval and rely on test-process exit to
// reclaim it; cancellation should be added in production.

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemory_StartCleanup_RemovesExpiredEntries(t *testing.T) {
	t.Parallel()

	// ARRANGE
	c := cache.NewInMemory()
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "expired_a", "value_a", cache.WithExpiration(1*time.Millisecond)))
	require.NoError(t, c.Set(ctx, "expired_b", "value_b", cache.WithExpiration(1*time.Millisecond)))

	time.Sleep(10 * time.Millisecond)

	// ACT
	c.StartCleanup(2 * time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	// ASSERT
	_, err := c.Get(ctx, "expired_a")
	assert.ErrorIs(t, err, cache.ErrNotFound, "expired_a must be removed by cleanup")

	_, err = c.Get(ctx, "expired_b")
	assert.ErrorIs(t, err, cache.ErrNotFound, "expired_b must be removed by cleanup")
}

func TestInMemory_StartCleanup_LeavesNonExpiredEntries(t *testing.T) {
	t.Parallel()

	// ARRANGE
	c := cache.NewInMemory()
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "live_long", "still_here", cache.WithExpiration(10*time.Second)))
	require.NoError(t, c.Set(ctx, "no_ttl", "always_here"))

	require.NoError(t, c.Set(ctx, "doomed", "gone", cache.WithExpiration(1*time.Millisecond)))
	time.Sleep(10 * time.Millisecond)

	// ACT
	c.StartCleanup(2 * time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	// ASSERT
	value, err := c.Get(ctx, "live_long")
	require.NoError(t, err)
	assert.Equal(t, "still_here", value, "non-expired entries must survive cleanup")

	value, err = c.Get(ctx, "no_ttl")
	require.NoError(t, err)
	assert.Equal(t, "always_here", value, "entries without expiration must survive cleanup")

	_, err = c.Get(ctx, "doomed")
	assert.ErrorIs(t, err, cache.ErrNotFound, "expired entry must still be removed alongside live ones")
}

func TestInMemory_StartCleanup_RaceFreeUnderConcurrentAccess(t *testing.T) {
	t.Parallel()

	// ARRANGE
	c := cache.NewInMemory()
	ctx := context.Background()

	const writers = 4
	const readers = 4
	const opsPerGoroutine = 200

	c.StartCleanup(1 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	// ACT
	for w := range writers {
		go func(id int) {
			defer wg.Done()
			for i := range opsPerGoroutine {
				key := "writer_" + strconv.Itoa(id) + "_" + strconv.Itoa(i%32)
				err := c.Set(ctx, key, i, cache.WithExpiration(2*time.Millisecond))
				assert.NoError(t, err, "concurrent Set must not fail")
			}
		}(w)
	}

	for r := range readers {
		go func(id int) {
			defer wg.Done()
			for i := range opsPerGoroutine {
				key := "writer_" + strconv.Itoa(id%writers) + "_" + strconv.Itoa(i%32)
				_, _ = c.Get(ctx, key)
			}
		}(r)
	}

	wg.Wait()

	// ASSERT
	// Pure data-race detection: this test passes when run with -race and no
	// race report is produced. The state assertion below just confirms the
	// cache is still usable after the storm of concurrent operations.
	require.NoError(t, c.Set(ctx, "post_storm", "ok"))
	value, err := c.Get(ctx, "post_storm")
	require.NoError(t, err)
	assert.Equal(t, "ok", value, "cache must remain usable after concurrent access")
}
