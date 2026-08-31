package plugin

import (
	"context"
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
