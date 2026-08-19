package pluginssh

import (
	"context"
	"net"
	"net/netip"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

// PluginProvider resolves a loaded plugin by its manager registry ID.
// Satisfied by *pkgplugin.Manager.
type PluginProvider interface {
	GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool)
}

// ManagerIDResolver maps a plugin DB ID to the ID it is registered under in
// the manager. Satisfied by *internalplugin.Loader.
type ManagerIDResolver interface {
	GetPluginManagerID(dbID domain.Uint64ID) (string, bool)
}

// netResolver and netDialer are seams: the tests drive DNS answers and the
// socket without touching the network.
type netResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type netDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}
