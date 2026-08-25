package plugin

import (
	"log/slog"

	"github.com/tetratelabs/wazero"
)

// newCompilationCache picks the compilation cache for a manager: none when
// disabled, in-memory by default, or a directory that survives restarts so
// every panel start does not recompile every module. The cache is never
// closed — transient modules may still be alive at process exit.
func newCompilationCache(cfg ManagerConfig) wazero.CompilationCache {
	if cfg.DisableCompilationCache {
		return nil
	}

	if cfg.CompilationCacheDir == "" {
		return wazero.NewCompilationCache()
	}

	cache, err := wazero.NewCompilationCacheWithDir(cfg.CompilationCacheDir)
	if err != nil {
		slog.Warn("plugin compilation cache directory is unusable, falling back to the in-memory cache",
			slog.String("dir", cfg.CompilationCacheDir),
			slog.String("error", err.Error()),
		)

		return wazero.NewCompilationCache()
	}

	slog.Info("plugin compilation cache enabled", slog.String("dir", cfg.CompilationCacheDir))

	return cache
}
