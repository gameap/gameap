package plugin

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

// closingHostLib is a factory-created library that owns per-plugin resources;
// the manager must release it on every path that ends a plugin's life.
type closingHostLib struct {
	closes   *atomic.Int32
	closeErr error
}

func (l *closingHostLib) Instantiate(context.Context, wazero.Runtime) error { return nil }

func (l *closingHostLib) Close(context.Context) error {
	l.closes.Add(1)

	return l.closeErr
}

type closingHostLibFactory struct {
	closes   *atomic.Int32
	closeErr error
}

func (f closingHostLibFactory) Create(uint64) HostLibrary {
	return &closingHostLib{closes: f.closes, closeErr: f.closeErr}
}

func TestHostLibraryCloser_ReleasedOnEveryPluginExit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		run        func(t *testing.T, manager *Manager)
		wantCloses int32
	}{
		{
			name: "failed_load_releases_what_was_built",
			run: func(t *testing.T, manager *Manager) {
				t.Helper()

				// emptyWASMModule has no api-version export, so the load fails
				// after the libraries were already instantiated.
				_, err := manager.LoadTransient(context.Background(), emptyWASMModule, nil, 0)
				require.Error(t, err)
			},
			wantCloses: 1,
		},
		{
			name: "transient_plugin_close",
			run: func(t *testing.T, manager *Manager) {
				t.Helper()

				wasmBytes, err := decompressServerLoggerWASM()
				require.NoError(t, err)

				plugin, err := manager.LoadTransient(context.Background(), wasmBytes, nil, 0)
				require.NoError(t, err)
				require.NoError(t, plugin.Close(context.Background()))
			},
			wantCloses: 1,
		},
		{
			name: "unload",
			run: func(t *testing.T, manager *Manager) {
				t.Helper()

				wasmBytes, err := decompressServerLoggerWASM()
				require.NoError(t, err)

				plugin, err := manager.Load(context.Background(), wasmBytes, nil, 1)
				require.NoError(t, err)
				require.NoError(t, manager.Unload(context.Background(), plugin.Info.Id))
			},
			wantCloses: 1,
		},
		{
			name: "shutdown",
			run: func(t *testing.T, manager *Manager) {
				t.Helper()

				wasmBytes, err := decompressServerLoggerWASM()
				require.NoError(t, err)

				_, err = manager.Load(context.Background(), wasmBytes, nil, 1)
				require.NoError(t, err)
				require.NoError(t, manager.Shutdown(context.Background()))
			},
			wantCloses: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			closes := &atomic.Int32{}
			manager := NewManager(ManagerConfig{
				Libraries:        sharedTestHostLibraries(),
				LibraryFactories: []HostLibraryFactory{closingHostLibFactory{closes: closes}},
			})

			tt.run(t, manager)

			assert.Equal(t, tt.wantCloses, closes.Load())
		})
	}
}

// TestHostLibraryCloser_ClosedWhenALaterFactoryFails: the libraries built so
// far already hold resources for this plugin and nothing else would free them.
func TestHostLibraryCloser_ClosedWhenALaterFactoryFails(t *testing.T) {
	t.Parallel()
	closes := &atomic.Int32{}
	manager := NewManager(ManagerConfig{
		LibraryFactories: []HostLibraryFactory{
			closingHostLibFactory{closes: closes},
			failingHostLibFactory{err: errTestHostLibFactory},
		},
	})

	_, err := manager.LoadTransient(context.Background(), emptyWASMModule, nil, 0)

	require.Error(t, err)
	assert.Equal(t, int32(1), closes.Load())
}

// TestHostLibraryCloser_CloseErrorDoesNotBreakUnload: cleanup failures are
// logged, never surfaced as an unload failure the operator has to act on.
func TestHostLibraryCloser_CloseErrorDoesNotBreakUnload(t *testing.T) {
	t.Parallel()
	closes := &atomic.Int32{}
	manager := NewManager(ManagerConfig{
		Libraries: sharedTestHostLibraries(),
		LibraryFactories: []HostLibraryFactory{
			closingHostLibFactory{closes: closes, closeErr: errors.New("cleanup failed")},
		},
	})

	wasmBytes, err := decompressServerLoggerWASM()
	require.NoError(t, err)

	plugin, err := manager.Load(context.Background(), wasmBytes, nil, 1)
	require.NoError(t, err)

	require.NoError(t, manager.Unload(context.Background(), plugin.Info.Id))
	assert.Equal(t, int32(1), closes.Load())
}
