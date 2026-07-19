package plugininstall

import "context"

// SubscriptionRefresher rebuilds the plugin event subscription map after
// runtime plugin changes (install, update, uninstall).
type SubscriptionRefresher interface {
	RefreshSubscriptions(ctx context.Context) error
}
