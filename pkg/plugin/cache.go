package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tetratelabs/wazero"
)

// compilationCacheMaxAge is how long compiled code nobody loads stays on disk:
// a disabled plugin, the previous version of an updated one, an upload that
// was validated but never installed.
const compilationCacheMaxAge = 7 * 24 * time.Hour

const (
	// wazeroModulePath is the module wazero looks itself up by in the build
	// info to name its version directory.
	wazeroModulePath = "github.com/tetratelabs/wazero"
	// wazeroCacheDirPrefix starts the per-version directory wazero creates
	// inside a cache directory.
	wazeroCacheDirPrefix = "wazero-"
	// cacheDirPerm keeps compiled code readable and writable by the panel
	// user alone.
	cacheDirPerm fs.FileMode = 0o700
	// unsafeCacheDirPerm are the bits that let someone other than the owner
	// put files into a directory.
	unsafeCacheDirPerm fs.FileMode = 0o022
)

var (
	errCacheDirNotDirectory     = errors.New("not a directory")
	errCacheDirSymlink          = errors.New("a symbolic link")
	errCacheDirWritableByOthers = errors.New("writable by group or others")
)

// compilationCaches hands out the wazero compilation cache a module is
// compiled with. Without a directory every module shares one in-memory cache.
// With one, the compiled code of each module (named by the sha256 of its
// bytes) lives in a subdirectory of its own: wazero fails a compilation
// outright when the cache holds an entry it cannot read or cannot store a new
// one, and only a directory per module lets such an entry, or an obsolete
// module, be deleted without knowing how wazero names its files. Caches are
// never closed — transient modules may still be alive at process exit.
type compilationCaches struct {
	// dir is the cache directory; empty keeps compiled code in memory only.
	dir string
	// shared is the in-memory cache: every module's without a directory, and
	// the fallback for a module whose directory cannot be used. Nil when
	// caching is disabled.
	shared wazero.CompilationCache

	mu sync.Mutex
	// modules are the directory-backed caches this process handed out, by
	// module hash; pruning never deletes a module in use.
	modules map[string]wazero.CompilationCache
}

// newCompilationCaches picks the caches of a manager: none when disabled, in
// memory without a directory, directory-backed otherwise. A directory that
// cannot be used falls back to memory, so a panel whose cache location is
// unusable still loads its plugins.
func newCompilationCaches(cfg ManagerConfig) *compilationCaches {
	if cfg.DisableCompilationCache {
		return &compilationCaches{}
	}

	caches := &compilationCaches{
		shared:  wazero.NewCompilationCache(),
		modules: make(map[string]wazero.CompilationCache),
	}

	if cfg.CompilationCacheDir == "" {
		return caches
	}

	dir, err := prepareCacheDir(cfg.CompilationCacheDir)
	if err != nil {
		slog.Warn("plugin compilation cache directory is unusable, falling back to the in-memory cache",
			slog.String("dir", cfg.CompilationCacheDir),
			slog.String("error", err.Error()),
		)

		return caches
	}

	caches.dir = dir

	slog.Info("plugin compilation cache enabled", slog.String("dir", dir))

	return caches
}

// forModule returns the cache to compile a module with and whether that cache
// keeps the compiled code on disk. The module's directory is touched on every
// use, so pruning sees how long nobody has loaded the module.
func (c *compilationCaches) forModule(hash string) (wazero.CompilationCache, bool) {
	if c.dir == "" {
		return c.shared, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	moduleDir := filepath.Join(c.dir, hash)

	cache, ok := c.modules[hash]
	if !ok {
		var err error

		cache, err = newModuleCache(moduleDir)
		if err != nil {
			slog.Warn("plugin compilation cache is unusable for the module, compiling it in memory",
				slog.String("dir", moduleDir),
				slog.String("error", err.Error()),
			)

			return c.shared, false
		}

		c.modules[hash] = cache
	}

	now := time.Now()
	if err := os.Chtimes(moduleDir, now, now); err != nil {
		slog.Debug("failed to touch the plugin compilation cache of a module",
			slog.String("dir", moduleDir),
			slog.String("error", err.Error()),
		)
	}

	return cache, true
}

// drop forgets a module's cache and deletes the code it holds on disk, so the
// next load of the module starts from an empty directory.
func (c *compilationCaches) drop(hash string) {
	if c.dir == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.modules, hash)

	moduleDir := filepath.Join(c.dir, hash)
	if err := os.RemoveAll(moduleDir); err != nil {
		slog.Warn("failed to delete the plugin compilation cache of a module",
			slog.String("dir", moduleDir),
			slog.String("error", err.Error()),
		)
	}
}

// pruneResult sums what one prune pass deleted.
type pruneResult struct {
	removed int
	freed   int64
}

// prune deletes compiled code of modules this process did not use and nobody
// loaded for maxAge, code of other wazero versions kept next to the current
// one, and the layout panels wrote before the per-module directories (wazero's
// version directories right in the cache directory). Only names the cache
// creates itself are considered, so nothing else in the directory is lost.
func (c *compilationCaches) prune(now time.Time, maxAge time.Duration) (pruneResult, error) {
	var result pruneResult

	if c.dir == "" {
		return result, nil
	}

	c.mu.Lock()
	inUse := make(map[string]struct{}, len(c.modules))
	for hash := range c.modules {
		inUse[hash] = struct{}{}
	}
	c.mu.Unlock()

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return result, errors.Wrap(err, "failed to read the cache directory")
	}

	current := wazeroCacheDirName()

	var failures []error

	for _, entry := range entries {
		// A symbolic link is not a directory entry here; it is never
		// followed and never deleted.
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		path := filepath.Join(c.dir, name)

		switch {
		case isModuleHash(name):
			info, err := entry.Info()
			if err != nil {
				failures = append(failures, errors.Wrapf(err, "failed to stat %s", path))

				continue
			}

			if _, used := inUse[name]; !used && now.Sub(info.ModTime()) > maxAge {
				failures = result.remove(path, failures)

				continue
			}

			failures = result.removeStaleVersions(path, current, failures)
		case strings.HasPrefix(name, wazeroCacheDirPrefix):
			failures = result.remove(path, failures)
		}
	}

	return result, stderrors.Join(failures...)
}

// removeStaleVersions deletes the version directories of other wazero
// releases inside a module directory. Nothing is deleted unless the directory
// of the running version is there: should wazero name it differently, pruning
// must not take it for a stale one.
func (r *pruneResult) removeStaleVersions(moduleDir, current string, failures []error) []error {
	if current == "" {
		return failures
	}

	if info, err := os.Lstat(filepath.Join(moduleDir, current)); err != nil || !info.IsDir() {
		return failures
	}

	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		return append(failures, errors.Wrapf(err, "failed to read %s", moduleDir))
	}

	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == current || !strings.HasPrefix(name, wazeroCacheDirPrefix) {
			continue
		}

		failures = r.remove(filepath.Join(moduleDir, name), failures)
	}

	return failures
}

func (r *pruneResult) remove(dir string, failures []error) []error {
	size := dirSize(dir)

	if err := os.RemoveAll(dir); err != nil {
		return append(failures, errors.Wrapf(err, "failed to delete %s", dir))
	}

	r.removed++
	r.freed += size

	return failures
}

// dirSize sums the regular files under dir; what cannot be read counts as
// nothing, the figure only goes to the log.
func dirSize(dir string) int64 {
	var size int64

	_ = filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return nil //nolint:nilerr // an unreadable entry only leaves the figure short
		}

		if info, infoErr := entry.Info(); infoErr == nil {
			size += info.Size()
		}

		return nil
	})

	return size
}

// PruneCompilationCache deletes compiled code from the cache directory that
// no load of this process used and nobody loaded for a week: disabled
// plugins, previous versions of updated ones, uploads validated but never
// installed, code of other wazero versions. Call it once the startup loads are
// done, so every module that runs has been used by then. Without a cache
// directory it does nothing.
func (m *Manager) PruneCompilationCache() {
	result, err := m.caches.prune(time.Now(), compilationCacheMaxAge)
	if err != nil {
		slog.Warn("failed to prune the plugin compilation cache", slog.String("error", err.Error()))
	}

	if result.removed > 0 {
		slog.Info("plugin compilation cache pruned",
			slog.Int("removed", result.removed),
			slog.Int64("freed_bytes", result.freed),
		)
	}
}

// prepareCacheDir creates the cache directory for the owner alone and checks
// that nobody else can write into it: what it holds runs as native code of
// the panel. The directory itself may be a symbolic link an operator set up.
func prepareCacheDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", errors.Wrap(err, "failed to resolve the directory")
	}

	if err := os.MkdirAll(abs, cacheDirPerm); err != nil {
		return "", errors.Wrap(err, "failed to create the directory")
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", errors.Wrap(err, "failed to stat the directory")
	}

	if err := checkOwnerOnlyDir(abs, info); err != nil {
		return "", err
	}

	return abs, nil
}

// newModuleCache opens the directory-backed cache of one module. The module
// directory is created by the panel, so unlike the cache directory it must
// not be a symbolic link.
func newModuleCache(moduleDir string) (wazero.CompilationCache, error) {
	if err := os.MkdirAll(moduleDir, cacheDirPerm); err != nil {
		return nil, errors.Wrap(err, "failed to create the module directory")
	}

	info, err := os.Lstat(moduleDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to stat the module directory")
	}

	if err := checkOwnerOnlyDir(moduleDir, info); err != nil {
		return nil, err
	}

	cache, err := newWazeroDirCache(moduleDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open the module cache")
	}

	return cache, nil
}

// checkOwnerOnlyDir accepts a directory only its owner can write to. Windows
// does not express permissions in mode bits, so only the type is checked
// there.
func checkOwnerOnlyDir(dir string, info fs.FileInfo) error {
	if info.Mode()&fs.ModeSymlink != 0 {
		return errors.WithMessage(errCacheDirSymlink, dir)
	}

	if !info.IsDir() {
		return errors.WithMessage(errCacheDirNotDirectory, dir)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm()&unsafeCacheDirPerm != 0 {
		return errors.WithMessagef(errCacheDirWritableByOthers, "%s (mode %s)", dir, info.Mode().Perm())
	}

	return nil
}

// moduleHash names a module in the cache directory.
func moduleHash(wasmBytes []byte) string {
	sum := sha256.Sum256(wasmBytes)

	return hex.EncodeToString(sum[:])
}

func isModuleHash(name string) bool {
	if len(name) != sha256.Size*2 {
		return false
	}

	for _, r := range name {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}

	return true
}

// wazeroCacheDirName is the directory the running wazero keeps compiled code
// in inside a cache directory, named after its version and platform the way
// wazero's NewCompilationCacheWithDir and internal/version do; empty when the
// version is unknown.
func wazeroCacheDirName() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	var version string

	for _, dep := range info.Deps {
		if strings.Contains(dep.Path, wazeroModulePath) {
			version = dep.Version
		}
	}

	if version == "" || version == "(devel)" {
		return ""
	}

	return wazeroCacheDirPrefix + version + "-" + runtime.GOARCH + "-" + runtime.GOOS
}
