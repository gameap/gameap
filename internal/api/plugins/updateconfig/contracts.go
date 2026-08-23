package updateconfig

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

// PluginLoader restarts the plugin with its new configuration and fences
// the operation from the multi-instance reconciler; satisfied by
// *plugin.Loader.
type PluginLoader interface {
	Reload(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, *pkgplugin.LoadedPlugin, error)
	Hold(dbID domain.Uint64ID) func()
}

// DBIDResolver maps the ID a loaded plugin is registered under (its declared
// info ID) to its database ID. Optional: the loader satisfies it.
type DBIDResolver interface {
	GetDBPluginID(managerID string) (domain.Uint64ID, bool)
}
