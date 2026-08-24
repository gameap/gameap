package hostlibrary

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingGrantsReader records how often the record was actually read, which
// is the whole point of the cache.
type countingGrantsReader struct {
	mu          sync.Mutex
	permissions []domain.PluginPermission
	err         error
	reads       atomic.Int64
	block       chan struct{}
}

func (r *countingGrantsReader) Grants(_ context.Context, _ uint64) ([]domain.PluginPermission, error) {
	r.reads.Add(1)

	if r.block != nil {
		<-r.block
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.permissions, r.err
}

func (r *countingGrantsReader) set(permissions ...domain.PluginPermission) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.permissions = permissions
}

func newTestCache(source PluginGrantsReader, ttl time.Duration) (*CachedPermissionChecker, *time.Time) {
	clock := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	cache := NewCachedPermissionChecker(source, ttl)
	cache.now = func() time.Time { return clock }

	return cache, &clock
}

func TestCachedPermissionChecker_answers_a_repeated_question_from_the_cache(t *testing.T) {
	t.Parallel()

	source := &countingGrantsReader{permissions: []domain.PluginPermission{domain.PluginPermissionFiles}}
	cache, _ := newTestCache(source, time.Minute)

	for range 5 {
		allowed, err := cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	denied, err := cache.Has(t.Context(), 7, domain.PluginPermissionSecrets)
	require.NoError(t, err)
	assert.False(t, denied, "a permission outside the cached set is refused")

	assert.Equal(t, int64(1), source.reads.Load(), "one record read serves every permission of the plugin")
}

func TestCachedPermissionChecker_reads_again_after_the_ttl(t *testing.T) {
	t.Parallel()

	source := &countingGrantsReader{permissions: []domain.PluginPermission{domain.PluginPermissionFiles}}
	cache, clock := newTestCache(source, time.Minute)

	_, err := cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
	require.NoError(t, err)

	*clock = clock.Add(time.Minute)

	source.set(domain.PluginPermissionSecrets)

	allowed, err := cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
	require.NoError(t, err)
	assert.False(t, allowed, "the expired entry is not reused")
	assert.Equal(t, int64(2), source.reads.Load())
}

func TestCachedPermissionChecker_invalidate_drops_the_entry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		drop func(*CachedPermissionChecker)
	}{
		{
			name: "one_plugin",
			drop: func(c *CachedPermissionChecker) { c.Invalidate(7) },
		},
		{
			name: "every_plugin",
			drop: func(c *CachedPermissionChecker) { c.InvalidateAll() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source := &countingGrantsReader{permissions: []domain.PluginPermission{domain.PluginPermissionFiles}}
			cache, _ := newTestCache(source, time.Hour)

			allowed, err := cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
			require.NoError(t, err)
			require.True(t, allowed)

			source.set(domain.PluginPermissionSecrets)
			tt.drop(cache)

			allowed, err = cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
			require.NoError(t, err)
			assert.False(t, allowed, "the revoked grant is not answered from the cache")
			assert.Equal(t, int64(2), source.reads.Load())
		})
	}
}

func TestCachedPermissionChecker_does_not_cache_an_empty_grant_set(t *testing.T) {
	t.Parallel()

	// A store install saves the plugin record, loads the module and only then
	// writes the manifest's permissions: caching the empty set in between
	// would deny the fresh plugin for a whole TTL.
	source := &countingGrantsReader{}
	cache, _ := newTestCache(source, time.Hour)

	allowed, err := cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
	require.NoError(t, err)
	require.False(t, allowed)

	source.set(domain.PluginPermissionFiles)

	allowed, err = cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
	require.NoError(t, err)
	assert.True(t, allowed, "the grant recorded after the install is seen without waiting for the ttl")
	assert.Equal(t, int64(2), source.reads.Load())
}

func TestCachedPermissionChecker_reads_every_time_when_the_ttl_disables_it(t *testing.T) {
	t.Parallel()

	source := &countingGrantsReader{permissions: []domain.PluginPermission{domain.PluginPermissionFiles}}
	cache, _ := newTestCache(source, 0)

	for range 3 {
		allowed, err := cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	assert.Equal(t, int64(3), source.reads.Load())
}

func TestCachedPermissionChecker_does_not_cache_an_error(t *testing.T) {
	t.Parallel()

	source := &countingGrantsReader{err: errors.New("database is down")}
	cache, _ := newTestCache(source, time.Hour)

	_, err := cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database is down")

	source.mu.Lock()
	source.err = nil
	source.mu.Unlock()
	source.set(domain.PluginPermissionFiles)

	allowed, err := cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestCachedPermissionChecker_denies_a_plugin_without_a_record_id(t *testing.T) {
	t.Parallel()

	source := &countingGrantsReader{permissions: []domain.PluginPermission{domain.PluginPermissionFiles}}
	cache, _ := newTestCache(source, time.Hour)

	allowed, err := cache.Has(t.Context(), 0, domain.PluginPermissionFiles)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, int64(0), source.reads.Load(), "a transient load never reaches the repository")
}

func TestCachedPermissionChecker_collapses_concurrent_misses(t *testing.T) {
	t.Parallel()

	const callers = 16

	source := &countingGrantsReader{
		permissions: []domain.PluginPermission{domain.PluginPermissionListenEvents},
		block:       make(chan struct{}),
	}

	cache, _ := newTestCache(source, time.Hour)

	var (
		wg      sync.WaitGroup
		granted atomic.Int64
	)

	wg.Add(callers)

	for range callers {
		go func() {
			defer wg.Done()

			allowed, err := cache.Has(t.Context(), 7, domain.PluginPermissionListenEvents)
			assert.NoError(t, err)

			if allowed {
				granted.Add(1)
			}
		}()
	}

	// The first caller is inside the blocked read; the rest pile up behind it
	// instead of each starting one of their own.
	require.Eventually(t, func() bool { return source.reads.Load() == 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	close(source.block)
	wg.Wait()

	assert.Equal(t, int64(callers), granted.Load())
	assert.Less(t, source.reads.Load(), int64(callers),
		"a burst of deliveries after an invalidation must not fan out one read per subscriber")
}

func TestCachedPermissionChecker_serves_reads_while_it_is_invalidated(t *testing.T) {
	t.Parallel()

	source := &countingGrantsReader{permissions: []domain.PluginPermission{domain.PluginPermissionFiles}}
	cache, _ := newTestCache(source, time.Hour)

	var wg sync.WaitGroup

	wg.Add(3)

	go func() {
		defer wg.Done()

		for range 200 {
			allowed, err := cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
			assert.NoError(t, err)
			assert.True(t, allowed, "the grant is answered the same whether or not the cache holds it")
		}
	}()

	go func() {
		defer wg.Done()

		for range 200 {
			cache.Invalidate(7)
		}
	}()

	go func() {
		defer wg.Done()

		for range 200 {
			cache.InvalidateAll()
		}
	}()

	wg.Wait()
}

func TestCachedPermissionChecker_returns_a_copy_of_the_cached_grants(t *testing.T) {
	t.Parallel()

	source := &countingGrantsReader{permissions: []domain.PluginPermission{domain.PluginPermissionFiles}}
	cache, _ := newTestCache(source, time.Hour)

	permissions, err := cache.Grants(t.Context(), 7)
	require.NoError(t, err)
	require.Len(t, permissions, 1)

	permissions[0] = domain.PluginPermissionSecrets

	allowed, err := cache.Has(t.Context(), 7, domain.PluginPermissionFiles)
	require.NoError(t, err)
	assert.True(t, allowed, "a caller writing to the returned slice must not corrupt the cache")
}
