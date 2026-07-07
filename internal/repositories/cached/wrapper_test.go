package cached_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/repositories/cached"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flakyCache fails Get/Set/Delete/Clear on demand, or returns a preset value
// from Get, to exercise the resilient read/invalidation paths that a healthy
// cache never triggers.
type flakyCache struct {
	getErr    error
	getValue  any
	setErr    error
	deleteErr error
	clearErr  error
}

func (c *flakyCache) Get(_ context.Context, _ string) (any, error) {
	if c.getErr != nil {
		return nil, c.getErr
	}
	if c.getValue != nil {
		return c.getValue, nil
	}

	return nil, cache.ErrNotFound
}

func (c *flakyCache) Set(_ context.Context, _ string, _ any, _ ...cache.Option) error {
	return c.setErr
}

func (c *flakyCache) Delete(_ context.Context, _ string) error { return c.deleteErr }
func (c *flakyCache) Clear(_ context.Context) error            { return c.clearErr }

func newWrapper(c cache.Cache) *cached.Wrapper {
	return cached.NewWrapper(c, cached.CacheConfig{TTL: time.Minute})
}

func TestGetOrLoad_returnsLoadedValueWhenCacheWriteFails(t *testing.T) {
	w := newWrapper(&flakyCache{setErr: errors.New("redis down")})

	loaded := 0
	got, err := cached.GetOrLoad(context.Background(), w, "k", func() ([]int, error) {
		loaded++

		return []int{1, 2, 3}, nil
	})

	require.NoError(t, err, "a cache write failure must not fail the read")
	assert.Equal(t, []int{1, 2, 3}, got)
	assert.Equal(t, 1, loaded)
}

func TestGetOrLoad_returnsLoadedValueWhenCacheReadErrors(t *testing.T) {
	w := newWrapper(&flakyCache{getErr: errors.New("redis timeout")})

	got, err := cached.GetOrLoad(context.Background(), w, "k", func() (string, error) {
		return "value", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "value", got)
}

func TestGetOrLoad_propagatesLoaderError(t *testing.T) {
	w := newWrapper(&flakyCache{})
	loaderErr := errors.New("db exploded")

	_, err := cached.GetOrLoad(context.Background(), w, "k", func() (int, error) {
		return 0, loaderErr
	})

	require.ErrorIs(t, err, loaderErr, "a genuine loader error must propagate")
}

func TestGetOrLoad_hitReturnsCachedValue_inMemory(t *testing.T) {
	w := newWrapper(cache.NewInMemory())
	ctx := context.Background()

	first, err := cached.GetOrLoad(ctx, w, "k", func() ([]string, error) {
		return []string{"a"}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, first)

	// Second call must hit the cache and not invoke the loader again.
	second, err := cached.GetOrLoad(ctx, w, "k", func() ([]string, error) {
		return nil, errors.New("loader must not run on a cache hit")
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, second)
}

func TestInvalidateBestEffort_swallowsCacheError(t *testing.T) {
	w := newWrapper(&flakyCache{deleteErr: errors.New("redis down")})

	assert.NotPanics(t, func() {
		w.InvalidateBestEffort(context.Background(), "k1", "k2")
	}, "a failed invalidation after a committed write must not surface")
}

func TestInvalidatePatternBestEffort_swallowsCacheError(t *testing.T) {
	// A non-Redis cache falls back to Clear() for pattern invalidation; forcing
	// Clear to fail exercises the error path.
	w := newWrapper(&flakyCache{clearErr: errors.New("redis down")})

	assert.NotPanics(t, func() {
		w.InvalidatePatternBestEffort(context.Background(), "user:id:*")
	}, "a failed pattern invalidation after a committed write must not surface")
}

// cachedRecord stands in for a domain value that a Redis/SQL cache returns as a
// JSON-decoded generic value rather than the original Go type.
type cachedRecord struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestGetOrLoad_decodesMapValueFromCache(t *testing.T) {
	// A Redis/SQL cache returns the stored value JSON-decoded into a generic
	// map, not the original T. GetOrLoad must convert it back into T via a JSON
	// round-trip and skip the loader entirely.
	c := &flakyCache{getValue: map[string]any{"name": "neo", "age": float64(30)}}
	w := newWrapper(c)

	got, err := cached.GetOrLoad(context.Background(), w, "k", func() (cachedRecord, error) {
		return cachedRecord{}, errors.New("loader must not run when the cached map decodes cleanly")
	})

	require.NoError(t, err)
	assert.Equal(t, cachedRecord{Name: "neo", Age: 30}, got,
		"a map cache value must be decoded into T without reloading")
}

func TestGetOrLoad_reloadsWhenCachedValueCannotDecode(t *testing.T) {
	// A cached value that neither type-asserts nor JSON-decodes into T (here a
	// bare string against a struct) must be treated as a miss and reloaded.
	c := &flakyCache{getValue: "not-a-record-object"}
	w := newWrapper(c)

	loaded := 0
	got, err := cached.GetOrLoad(context.Background(), w, "k", func() (cachedRecord, error) {
		loaded++

		return cachedRecord{Name: "loaded", Age: 7}, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, loaded, "an undecodable cached value must fall back to the loader")
	assert.Equal(t, cachedRecord{Name: "loaded", Age: 7}, got)
}
