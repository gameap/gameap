package reload

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

// Reloader restarts an installed plugin and reports the resulting record;
// satisfied by *plugin.Loader.
type Reloader interface {
	Reload(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, *pkgplugin.LoadedPlugin, error)
}

// DBIDResolver maps the ID a loaded plugin is registered under (its declared
// info ID) to its database ID; they differ when a store plugin's wasm
// declares another ID. Optional: the loader satisfies it.
type DBIDResolver interface {
	GetDBPluginID(managerID string) (domain.Uint64ID, bool)
}
