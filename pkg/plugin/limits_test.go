package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryMinOverLimitWASMModule declares an initial memory of 3 pages and
// nothing else.
var memoryMinOverLimitWASMModule = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // header
	0x05, 0x03, 0x01, 0x00, 0x03, // memory: 1 entry, no max, min 3 pages
}

// memoryMaxOverLimitWASMModule declares min 1 page, max 64 pages.
var memoryMaxOverLimitWASMModule = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // header
	0x05, 0x04, 0x01, 0x01, 0x01, 0x40, // memory: 1 entry, with max, min 1, max 64 pages
}

func TestLoadTransient_module_too_large(t *testing.T) {
	t.Parallel()
	manager := NewManager(ManagerConfig{MaxModuleBytes: 4})

	plugin, err := manager.LoadTransient(context.Background(), emptyWASMModule, nil, 0)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModuleTooLarge)
	assert.Contains(t, err.Error(), "8 bytes exceeds the limit of 4 bytes")
	assert.Nil(t, plugin)

	sanitized := SanitizeLoadError(err)
	assert.Contains(t, sanitized.Error(), "plugin module too large")
}

func TestLoadTransient_memory_limit_rejects_initial_memory_over_limit(t *testing.T) {
	t.Parallel()
	manager := NewManager(ManagerConfig{MaxMemoryBytes: 2 * wasmPageSize})

	plugin, err := manager.LoadTransient(context.Background(), memoryMinOverLimitWASMModule, nil, 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "over limit of 2 pages")
	assert.Nil(t, plugin)

	// The decoder's wording names both sizes, so the dry-run can show it.
	assert.Contains(t, SanitizeLoadError(err).Error(), "over limit of 2 pages")
}

func TestLoadTransient_memory_limit_clamps_declared_maximum(t *testing.T) {
	t.Parallel()
	manager := NewManager(ManagerConfig{MaxMemoryBytes: 2 * wasmPageSize})

	plugin, err := manager.LoadTransient(context.Background(), memoryMaxOverLimitWASMModule, nil, 0)

	// Compilation passes (the maximum is clamped); the module then fails
	// API version verification because it exports nothing.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExportNotFound)
	assert.Nil(t, plugin)
}

func TestLoadTransient_memory_limit_not_applied_to_default_config(t *testing.T) {
	t.Parallel()
	manager := NewManager(ManagerConfig{})

	plugin, err := manager.LoadTransient(context.Background(), memoryMinOverLimitWASMModule, nil, 0)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExportNotFound)
	assert.Nil(t, plugin)
}

func TestMemoryLimitPages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		maxBytes uint64
		want     uint32
	}{
		{name: "zero_keeps_default", maxBytes: 0, want: 0},
		{name: "below_one_page_rounds_up_to_one", maxBytes: 1000, want: 1},
		{name: "exact_pages", maxBytes: 256 << 20, want: 4096},
		{name: "rounds_down", maxBytes: 256<<20 + 1, want: 4096},
		{name: "clamped_to_wasm_maximum", maxBytes: 16 << 30, want: wasmMaxPages},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, memoryLimitPages(tt.maxBytes))
		})
	}
}

func TestNewCompilationCache(t *testing.T) {
	t.Parallel()

	t.Run("disabled_is_nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, newCompilationCache(ManagerConfig{DisableCompilationCache: true}))
	})

	t.Run("empty_dir_is_in_memory", func(t *testing.T) {
		t.Parallel()
		assert.NotNil(t, newCompilationCache(ManagerConfig{}))
	})

	t.Run("dir_persists_compiled_modules", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "wasm-cache")
		manager := NewManager(ManagerConfig{CompilationCacheDir: dir})
		t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

		loaded, err := manager.LoadTransient(context.Background(), misbehavingWASM, nil, 0)
		require.NoError(t, err)
		require.NoError(t, loaded.Close(context.Background()))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.NotEmpty(t, entries, "wazero keeps a version-named subdirectory with compiled modules")
	})

	t.Run("unusable_dir_falls_back_to_memory", func(t *testing.T) {
		t.Parallel()
		file := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

		manager := NewManager(ManagerConfig{CompilationCacheDir: file})
		t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

		loaded, err := manager.LoadTransient(context.Background(), misbehavingWASM, nil, 0)
		require.NoError(t, err)
		require.NoError(t, loaded.Close(context.Background()))
	})

	t.Run("no_cache_still_loads", func(t *testing.T) {
		t.Parallel()
		manager := NewManager(ManagerConfig{DisableCompilationCache: true})
		t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

		loaded, err := manager.LoadTransient(context.Background(), misbehavingWASM, nil, 0)
		require.NoError(t, err)
		require.NoError(t, loaded.Close(context.Background()))
	})
}
