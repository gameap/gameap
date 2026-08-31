package plugin

import (
	"context"
	"sync"

	"github.com/tetratelabs/wazero"
)

const (
	wasmPageSize = 65536
	// wasmMaxPages is the largest memory a 32-bit wasm module can address;
	// wazero panics on a larger limit.
	wasmMaxPages = 65536
)

// wazero fills its own version cache while the first runtime is built
// (internal/version.GetWazeroVersion writes an unsynchronised package
// variable), so two plugins loading at once race on it. Serialising creation
// costs nothing after the first call and keeps the whole path race-free.
var runtimeCreateMu sync.Mutex

func newWazeroRuntime(ctx context.Context, cfg wazero.RuntimeConfig) wazero.Runtime {
	runtimeCreateMu.Lock()
	defer runtimeCreateMu.Unlock()

	return wazero.NewRuntimeWithConfig(ctx, cfg)
}

// runtimeConfig builds the wazero configuration shared by every plugin
// runtime of this manager.
func (m *Manager) runtimeConfig() wazero.RuntimeConfig {
	// CloseOnContextDone lets call deadlines interrupt guest execution;
	// without it a runaway plugin blocks its caller forever.
	cfg := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)

	if m.cache != nil {
		cfg = cfg.WithCompilationCache(m.cache)
	}

	if pages := memoryLimitPages(m.config.MaxMemoryBytes); pages > 0 {
		cfg = cfg.WithMemoryLimitPages(pages)
	}

	return cfg
}

// memoryLimitPages converts the configured byte cap to wasm pages; 0 keeps
// the wazero default (4 GiB), any other value is at least one page so a
// tiny cap never silently means "unlimited". A module declaring a larger
// maximum is clamped to the limit; only a module whose initial memory
// already exceeds it fails to load.
func memoryLimitPages(maxBytes uint64) uint32 {
	if maxBytes == 0 {
		return 0
	}

	pages := min(max(maxBytes/wasmPageSize, 1), wasmMaxPages)

	return uint32(pages)
}
