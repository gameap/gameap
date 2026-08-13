package pluginsync

import (
	"context"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

// Repository reads the desired plugin state. It deliberately exposes no writer:
// the plugins table is global while a load failure is local to one instance, so
// a reconciler that wrote back would let a single unhealthy replica change what
// every other replica runs.
type Repository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Plugin, error)
}

// PluginLoader is the single writer of manager membership. Everything goes
// through it so the reconciler and the admin HTTP handlers serialise on the
// loader's apply lock. Satisfied by *internalplugin.Loader.
type PluginLoader interface {
	LoadWithID(ctx context.Context, filename string, pluginID uint64) (*pkgplugin.LoadedPlugin, error)
	Reload(ctx context.Context, filename string, pluginID uint64) (*pkgplugin.LoadedPlugin, error)
	Unload(ctx context.Context, managerID string) error
	GetPluginManagerID(dbID domain.Uint64ID) (string, bool)
	GetDBPluginID(managerID string) (domain.Uint64ID, bool)
	UnregisterPluginID(dbID domain.Uint64ID)
}

// PluginProvider reports what is currently loaded. Satisfied by
// *pkgplugin.Manager.
type PluginProvider interface {
	GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool)
	GetPlugins() []*pkgplugin.LoadedPlugin
}

// SubscriptionRefresher rebuilds the plugin event subscription map. Satisfied
// by *pkgplugin.Dispatcher.
type SubscriptionRefresher interface {
	RefreshSubscriptions(ctx context.Context) error
}

// ArchiveEvents drops a plugin's archive registrations when it goes away.
// Satisfied by *pluginarchive.Service.
type ArchiveEvents interface {
	RemovePlugin(pluginID uint64)
}

// FileStore is the subset of files.FileManager used to check and repair plugin
// files.
type FileStore interface {
	Exists(ctx context.Context, path string) bool
	Read(ctx context.Context, path string) ([]byte, error)
	Write(ctx context.Context, path string, data []byte) error
}

// StoreDownloader fetches a plugin file from the plugin store. Satisfied by
// *pluginstore.Service.
type StoreDownloader interface {
	DownloadPlugin(ctx context.Context, pluginID string, version string) ([]byte, error)
}

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}
