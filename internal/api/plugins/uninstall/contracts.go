package uninstall

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

type PluginManager interface {
	GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool)
	Unload(ctx context.Context, pluginID string) error
}

// ManagerIDResolver maps a plugin DB ID to the ID it is registered under in
// the manager (they differ when the wasm's own info ID is not the store ID).
type ManagerIDResolver interface {
	GetPluginManagerID(dbID domain.Uint64ID) (string, bool)
}

// TaskScheduler drops the plugin's scheduled task registrations on uninstall.
type TaskScheduler interface {
	RemovePluginTasks(ctx context.Context, pluginID domain.Uint64ID) (int, error)
}

// ArchiveEvents drops the plugin's archive event registrations on uninstall
// so stale deliveries cannot reach a freshly reinstalled instance.
type ArchiveEvents interface {
	RemovePlugin(pluginID uint64)
}

// RecoveryCanceller drops a pending automatic reload of the plugin; an
// uninstall owns the lifecycle from then on. Satisfied by *plugin.Loader.
type RecoveryCanceller interface {
	Forget(dbID domain.Uint64ID)
}

// PluginPolicyCleaner drops the per-plugin policy state the host libraries
// keep in memory (rate limiter buckets, audit throttle) so an uninstalled
// plugin does not hold it until the panel restarts. Satisfied by
// *hostlibrary.Guard.
type PluginPolicyCleaner interface {
	Forget(pluginID uint64)
}

// PluginStorageCleaner drops the plugin's gameap-storage rows on uninstall.
type PluginStorageCleaner interface {
	DeleteByPlugin(ctx context.Context, pluginID uint64) error
}

// PluginSecretCleaner drops the plugin's encrypted secrets on uninstall.
type PluginSecretCleaner interface {
	DeleteByPlugin(ctx context.Context, pluginID domain.Uint64ID) (int, error)
}
