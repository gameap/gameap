package application

import (
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/services/pluginssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContainer_PluginSSHIsBuiltOnceUnderConcurrentAccess: lazySSHSessions
// resolves the SSH engine during plugin load, which LoadTransient runs under a
// read lock — concurrent loads reach PluginSSH() at the same time, and two
// engines would leak the loser's connections past Shutdown.
func TestContainer_PluginSSHIsBuiltOnceUnderConcurrentAccess(t *testing.T) {
	t.Parallel()
	c := newWiredContainer(t)

	const goroutines = 16

	services := make([]*pluginssh.Service, goroutines)

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Go(func() {
			services[i] = c.PluginSSH()
		})
	}
	wg.Wait()

	require.NotNil(t, services[0])
	for i := 1; i < goroutines; i++ {
		assert.Same(t, services[0], services[i], "every caller must see the same engine")
	}
}
