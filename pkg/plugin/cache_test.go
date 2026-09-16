package plugin

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/experimental"
)

const staleVersionDirName = "wazero-v0.0.1-arch-os"

// cacheEntrySize is the size of every fake compiled-code file the prune tests
// create, so freed bytes can be checked exactly.
const cacheEntrySize = 16

func newDiskManager(t *testing.T, dir string) *Manager {
	t.Helper()

	manager := NewManager(ManagerConfig{CompilationCacheDir: dir})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	return manager
}

func loadAndClose(t *testing.T, manager *Manager, wasm []byte) {
	t.Helper()

	loaded, err := manager.LoadTransient(context.Background(), wasm, nil, 0)
	require.NoError(t, err)
	require.NoError(t, loaded.Close(context.Background()))
}

// cacheEntries lists the files wazero stored for a module under the running
// wazero version. Reading them through wazeroCacheDirName also proves that
// the name pruning relies on is the one wazero actually uses.
func cacheEntries(t *testing.T, dir string, wasm []byte) []string {
	t.Helper()

	versionDir := filepath.Join(dir, moduleHash(wasm), wazeroCacheDirName())

	entries, err := os.ReadDir(versionDir)
	require.NoError(t, err)

	files := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.Type().IsRegular() {
			files = append(files, filepath.Join(versionDir, entry.Name()))
		}
	}

	return files
}

// makeModuleDir creates a module directory holding one fake entry in each of
// the given version directories and dates the module directory to modTime.
func makeModuleDir(t *testing.T, dir, hexDigit string, modTime time.Time, versionDirs ...string) string {
	t.Helper()

	moduleDir := filepath.Join(dir, strings.Repeat(hexDigit, 64))

	for _, versionDir := range versionDirs {
		makeEntryDir(t, filepath.Join(moduleDir, versionDir))
	}

	require.NoError(t, os.Chtimes(moduleDir, modTime, modTime))

	return moduleDir
}

func makeEntryDir(t *testing.T, dir string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entry"), make([]byte, cacheEntrySize), 0o600))
}

func TestNewCompilationCaches(t *testing.T) {
	t.Parallel()

	t.Run("disabled_has_no_cache", func(t *testing.T) {
		t.Parallel()

		caches := newCompilationCaches(ManagerConfig{DisableCompilationCache: true})

		cache, onDisk := caches.forModule(moduleHash(misbehavingWASM))

		assert.Nil(t, cache)
		assert.False(t, onDisk)
	})

	t.Run("empty_dir_is_in_memory", func(t *testing.T) {
		t.Parallel()

		caches := newCompilationCaches(ManagerConfig{})

		cache, onDisk := caches.forModule(moduleHash(misbehavingWASM))

		require.NotNil(t, cache)
		assert.Same(t, caches.shared, cache)
		assert.False(t, onDisk)
		assert.Empty(t, caches.dir)
	})

	t.Run("dir_is_created_for_the_owner_alone", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "nested", "wasm-cache")

		caches := newCompilationCaches(ManagerConfig{CompilationCacheDir: dir})

		assert.Equal(t, dir, caches.dir)

		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
		}
	})

	t.Run("file_instead_of_dir_falls_back_to_memory", func(t *testing.T) {
		t.Parallel()

		file := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

		caches := newCompilationCaches(ManagerConfig{CompilationCacheDir: file})

		assert.Empty(t, caches.dir)
		assert.NotNil(t, caches.shared)

		loadAndClose(t, newDiskManager(t, file), misbehavingWASM)
	})

	t.Run("dir_writable_by_others_falls_back_to_memory", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("windows permissions are not expressed in mode bits")
		}

		dir := filepath.Join(t.TempDir(), "wasm-cache")
		require.NoError(t, os.Mkdir(dir, 0o700))
		require.NoError(t, os.Chmod(dir, 0o777))

		caches := newCompilationCaches(ManagerConfig{CompilationCacheDir: dir})

		assert.Empty(t, caches.dir)
		assert.NotNil(t, caches.shared)
	})

	t.Run("no_cache_still_loads", func(t *testing.T) {
		t.Parallel()

		manager := NewManager(ManagerConfig{DisableCompilationCache: true})
		t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

		loadAndClose(t, manager, misbehavingWASM)
	})
}

func TestManagerCompilationCacheOnDisk(t *testing.T) {
	t.Parallel()

	t.Run("module_gets_a_directory_of_its_own", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		loadAndClose(t, newDiskManager(t, dir), misbehavingWASM)

		require.NotEmpty(t, wazeroCacheDirName())
		require.Len(t, cacheEntries(t, dir, misbehavingWASM), 1)
	})

	t.Run("second_manager_reads_the_entry_from_disk", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		loadAndClose(t, newDiskManager(t, dir), misbehavingWASM)

		entries := cacheEntries(t, dir, misbehavingWASM)
		require.Len(t, entries, 1)

		before, err := os.Stat(entries[0])
		require.NoError(t, err)

		loadAndClose(t, newDiskManager(t, dir), misbehavingWASM)

		after, err := os.Stat(entries[0])
		require.NoError(t, err)
		assert.True(t, os.SameFile(before, after), "a cache hit must not compile and store the module again")
	})

	t.Run("unreadable_entry_is_dropped_and_rebuilt", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		loadAndClose(t, newDiskManager(t, dir), misbehavingWASM)

		entries := cacheEntries(t, dir, misbehavingWASM)
		require.Len(t, entries, 1)

		garbage := bytes.Repeat([]byte("x"), 128)
		require.NoError(t, os.WriteFile(entries[0], garbage, 0o600))

		loadAndClose(t, newDiskManager(t, dir), misbehavingWASM)

		assert.NoDirExists(t, filepath.Join(dir, moduleHash(misbehavingWASM)),
			"the module directory holding the unreadable entry must be dropped")

		loadAndClose(t, newDiskManager(t, dir), misbehavingWASM)

		rebuilt := cacheEntries(t, dir, misbehavingWASM)
		require.Len(t, rebuilt, 1)

		content, err := os.ReadFile(rebuilt[0])
		require.NoError(t, err)
		assert.NotEqual(t, garbage, content)
	})

	t.Run("store_failure_compiles_in_memory", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("needs a directory the test user cannot write to")
		}

		dir := t.TempDir()
		loadAndClose(t, newDiskManager(t, dir), misbehavingWASM)

		entries := cacheEntries(t, dir, misbehavingWASM)
		require.Len(t, entries, 1)
		require.NoError(t, os.Remove(entries[0]))

		versionDir := filepath.Dir(entries[0])
		require.NoError(t, os.Chmod(versionDir, 0o500))
		t.Cleanup(func() { _ = os.Chmod(versionDir, 0o700) })

		loadAndClose(t, newDiskManager(t, dir), misbehavingWASM)

		assert.NoDirExists(t, filepath.Join(dir, moduleHash(misbehavingWASM)),
			"the module directory that refused the entry must be dropped")
	})

	t.Run("module_dir_that_is_a_file_compiles_in_memory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		blocker := filepath.Join(dir, moduleHash(misbehavingWASM))
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

		loadAndClose(t, newDiskManager(t, dir), misbehavingWASM)

		assert.FileExists(t, blocker)
	})

	t.Run("module_dir_symlink_is_not_used", func(t *testing.T) {
		t.Parallel()

		if runtime.GOOS == "windows" {
			t.Skip("symbolic links need extra privileges on windows")
		}

		dir := t.TempDir()
		target := t.TempDir()
		require.NoError(t, os.Symlink(target, filepath.Join(dir, moduleHash(misbehavingWASM))))

		loadAndClose(t, newDiskManager(t, dir), misbehavingWASM)

		entries, err := os.ReadDir(target)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("concurrent_loads_share_one_entry", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		manager := newDiskManager(t, dir)

		var wg sync.WaitGroup

		for range 8 {
			wg.Go(func() {
				loaded, err := manager.LoadTransient(context.Background(), misbehavingWASM, nil, 0)
				if assert.NoError(t, err) {
					assert.NoError(t, loaded.Close(context.Background()))
				}
			})
		}

		wg.Wait()

		require.Len(t, cacheEntries(t, dir, misbehavingWASM), 1)
	})

	t.Run("invalid_module_reports_the_compile_error", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		invalid := []byte("not a wasm module")

		plugin, err := newDiskManager(t, dir).LoadTransient(context.Background(), invalid, nil, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to compile WASM module")
		assert.Nil(t, plugin)
		assert.NoDirExists(t, filepath.Join(dir, moduleHash(invalid)))
	})
}

func TestCompilationCachesPrune(t *testing.T) {
	t.Parallel()

	current := wazeroCacheDirName()
	require.NotEmpty(t, current)

	now := time.Now()
	old := now.Add(-2 * compilationCacheMaxAge)

	t.Run("deletes_only_code_nobody_needs", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		caches := newCompilationCaches(ManagerConfig{CompilationCacheDir: dir})
		require.Equal(t, dir, caches.dir)

		stale := makeModuleDir(t, dir, "a", old, current)
		fresh := makeModuleDir(t, dir, "b", now.Add(-24*time.Hour), current, staleVersionDirName)
		used := makeModuleDir(t, dir, "c", old, current)
		withoutCurrent := makeModuleDir(t, dir, "d", now.Add(-time.Hour), staleVersionDirName)
		caches.modules[filepath.Base(used)] = caches.shared

		legacy := filepath.Join(dir, staleVersionDirName)
		makeEntryDir(t, legacy)

		foreign := filepath.Join(dir, "keep-me")
		makeEntryDir(t, foreign)
		require.NoError(t, os.Chtimes(foreign, old, old))

		hashNamedFile := filepath.Join(dir, strings.Repeat("e", 64))
		require.NoError(t, os.WriteFile(hashNamedFile, []byte("x"), 0o600))
		require.NoError(t, os.Chtimes(hashNamedFile, old, old))

		var link, linkTarget string
		if runtime.GOOS != "windows" {
			linkTarget = makeModuleDir(t, t.TempDir(), "f", old, current)
			link = filepath.Join(dir, filepath.Base(linkTarget))
			require.NoError(t, os.Symlink(linkTarget, link))
		}

		result, err := caches.prune(now, compilationCacheMaxAge)

		require.NoError(t, err)
		assert.Equal(t, pruneResult{removed: 3, freed: 3 * cacheEntrySize}, result)
		assert.NoDirExists(t, stale)
		assert.DirExists(t, filepath.Join(fresh, current))
		assert.NoDirExists(t, filepath.Join(fresh, staleVersionDirName))
		assert.DirExists(t, filepath.Join(used, current))
		assert.DirExists(t, filepath.Join(withoutCurrent, staleVersionDirName),
			"without the running version's directory nothing may be taken for stale")
		assert.NoDirExists(t, legacy)
		assert.DirExists(t, foreign)
		assert.FileExists(t, hashNamedFile)

		if link != "" {
			_, err := os.Lstat(link)
			require.NoError(t, err)
			assert.DirExists(t, filepath.Join(linkTarget, current))
		}
	})

	t.Run("in_memory_cache_prunes_nothing", func(t *testing.T) {
		t.Parallel()

		caches := newCompilationCaches(ManagerConfig{})

		result, err := caches.prune(now, compilationCacheMaxAge)

		require.NoError(t, err)
		assert.Equal(t, pruneResult{}, result)
	})

	t.Run("manager_keeps_the_modules_it_loaded", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		manager := newDiskManager(t, dir)
		loadAndClose(t, manager, misbehavingWASM)

		loadedDir := filepath.Join(dir, moduleHash(misbehavingWASM))
		require.NoError(t, os.Chtimes(loadedDir, old, old))

		stale := makeModuleDir(t, dir, "a", old, current)

		manager.PruneCompilationCache()

		assert.DirExists(t, filepath.Join(loadedDir, current))
		assert.NoDirExists(t, stale)
	})
}

func TestManagerCompileContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		workers int
		want    int
	}{
		{name: "zero_uses_every_cpu", workers: 0, want: runtime.GOMAXPROCS(0)},
		{name: "negative_uses_every_cpu", workers: -1, want: runtime.GOMAXPROCS(0)},
		{name: "explicit_count", workers: 3, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			manager := NewManager(ManagerConfig{CompileWorkers: tt.workers, DisableCompilationCache: true})

			ctx := manager.compileContext(context.Background())

			assert.Equal(t, tt.want, experimental.GetCompilationWorkers(ctx))
		})
	}
}

func TestIsModuleHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "module_hash", in: moduleHash([]byte("module")), want: true},
		{name: "upper_case", in: strings.ToUpper(moduleHash([]byte("module"))), want: false},
		{name: "too_short", in: strings.Repeat("a", 63), want: false},
		{name: "not_hex", in: strings.Repeat("g", 64), want: false},
		{name: "version_dir", in: staleVersionDirName, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, isModuleHash(tt.in))
		})
	}
}

func TestWazeroCacheDirNameFor(t *testing.T) {
	t.Parallel()

	platform := "-" + runtime.GOARCH + "-" + runtime.GOOS

	tests := []struct {
		name string
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{
			name: "pinned_dependency",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
				Deps: []*debug.Module{{Path: wazeroModulePath, Version: "v1.12.0"}},
			},
			ok:   true,
			want: "wazero-v1.12.0" + platform,
		},
		{
			name: "devel_dependency_falls_back_to_the_main_module",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "v1.13.0"},
				Deps: []*debug.Module{{Path: wazeroModulePath, Version: "(devel)"}},
			},
			ok:   true,
			want: "wazero-v1.13.0" + platform,
		},
		{
			name: "no_version_anywhere_is_dev",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "(devel)"},
				Deps: []*debug.Module{{Path: "github.com/other/module", Version: "v9.9.9"}},
			},
			ok:   true,
			want: "wazero-dev" + platform,
		},
		{
			name: "no_build_info_is_dev",
			want: "wazero-dev" + platform,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, wazeroCacheDirNameFor(tt.info, tt.ok))
		})
	}
}
