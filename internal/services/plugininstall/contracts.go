package plugininstall

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

// SubscriptionRefresher rebuilds the plugin event subscription map after
// runtime plugin changes (install, update, uninstall).
type SubscriptionRefresher interface {
	RefreshSubscriptions(ctx context.Context) error
}

// SyncNotifier tells the other panel instances that a plugin changed and clears
// what the local reconciler believed about it. Satisfied by
// *pluginsync.Service.
type SyncNotifier interface {
	Notify(ctx context.Context, pluginID domain.Uint64ID, reason string)
	Forget(pluginID domain.Uint64ID)
}
