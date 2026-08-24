package updatepermissions

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

// PluginManager exposes the plugins loaded on this instance; satisfied by
// *plugin.Manager.
type PluginManager interface {
	GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool)
}

// DBIDResolver maps the ID a loaded plugin is registered under (its declared
// info ID) to its database ID and back. Optional: the loader satisfies it.
type DBIDResolver interface {
	GetDBPluginID(managerID string) (domain.Uint64ID, bool)
	GetPluginManagerID(dbID domain.Uint64ID) (string, bool)
}

// SubscriptionsAnnouncer tells the other panel instances to rebuild their
// event subscriptions: the map is per instance, so a listen_events change
// made here would otherwise only take effect there on their next refresh.
// Optional; satisfied by *integration.PluginSubscriptionsNotifier.
type SubscriptionsAnnouncer interface {
	PublishRefresh(ctx context.Context, pluginID uint64) error
}
