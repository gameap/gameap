package plugin

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tetratelabs/wazero"
)

// wazero populates its version cache without synchronisation while the first
// runtime is built, so concurrent creation trips the race detector. The guard
// lives in newWazeroRuntime; this test only fails once someone calls
// wazero.NewRuntimeWithConfig directly again, and only under -race.
func TestNewWazeroRuntimeSurvivesConcurrentCreation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			r := newWazeroRuntime(ctx, wazero.NewRuntimeConfig())
			assert.NoError(t, r.Close(ctx))
		})
	}

	wg.Wait()
}

// Directory caches read the same unsynchronised version cache as runtimes, so
// both constructors share the guard; like the test above, this one only fails
// under -race once a caller bypasses newWazeroDirCache.
func TestNewWazeroDirCacheSurvivesConcurrentCreation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Go(func() {
			cache, err := newWazeroDirCache(filepath.Join(dir, strconv.Itoa(i)))
			if !assert.NoError(t, err) {
				return
			}

			r := newWazeroRuntime(ctx, wazero.NewRuntimeConfig().WithCompilationCache(cache))
			assert.NoError(t, r.Close(ctx))
		})
	}

	wg.Wait()
}
