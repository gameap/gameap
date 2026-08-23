package pluginsync

import (
	"context"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

// Repository reads the desired plugin state. It deliberately exposes no
// writer: the plugins table is global while a load failure is local to one
// instance, so a reconciler that wrote back would let a single unhealthy
// replica change what every other replica runs.
type Repository interface {
	FindAll(
		ctx context.Context,
		order []filters.Sorting,
		pagination *filters.Pagination,
	) ([]domain.Plugin, error)
}

// Loader is the slice of the plugin loader the reconciler drives. Every
// method takes the plugin's lifecycle lock, so the reconciler and the admin
// HTTP handlers serialise there. Satisfied by *internalplugin.Loader.
type Loader interface {
	ApplyRecord(ctx context.Context, plugin *domain.Plugin) (bool, error)
	UnloadRecord(ctx context.Context, dbID domain.Uint64ID, trigger string) (bool, error)
	RuntimeState(dbID domain.Uint64ID) internalplugin.RuntimeState
	GetDBPluginID(managerID string) (domain.Uint64ID, bool)
}

// PluginProvider reports what is currently loaded. Satisfied by
// *pkgplugin.Manager.
type PluginProvider interface {
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

// PassObserver counts reconcile passes for the metrics.
type PassObserver interface {
	SyncPass(result string)
}

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}
