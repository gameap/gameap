package plugin

import (
	"context"

	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

type LoaderManager interface {
	Load(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error)
	LoadTransient(
		ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64,
	) (*pkgplugin.LoadedPlugin, error)
	Unload(ctx context.Context, pluginID string) error
	GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool)
	GetPlugins() []*pkgplugin.LoadedPlugin
	Shutdown(ctx context.Context) error
}

// SubscriptionRefresher rebuilds the event subscriptions after a plugin was
// reloaded; satisfied by *pkgplugin.Dispatcher.
type SubscriptionRefresher interface {
	RefreshSubscriptions(ctx context.Context) error
}
