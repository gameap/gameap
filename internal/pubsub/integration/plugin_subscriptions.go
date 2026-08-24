package integration

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
)

// SubscriptionRefresher rebuilds the event subscriptions of this instance;
// satisfied by *plugin.Dispatcher.
type SubscriptionRefresher interface {
	RefreshSubscriptions(ctx context.Context) error
}

// PermissionCacheInvalidator drops the cached grants of a plugin; satisfied
// by *hostlibrary.CachedPermissionChecker.
type PermissionCacheInvalidator interface {
	Invalidate(pluginID uint64)
}

// NotifierOption tunes a PluginSubscriptionsNotifier.
type NotifierOption func(*PluginSubscriptionsNotifier)

// WithPermissionCache lets the notifier drop the plugin's cached grants
// before anything reads them again.
func WithPermissionCache(cache PermissionCacheInvalidator) NotifierOption {
	return func(n *PluginSubscriptionsNotifier) {
		n.cache = cache
	}
}

// PluginSubscriptionsNotifier keeps every instance in step with the shared
// permission records: the instance that changed a plugin's grants announces
// it, and each instance drops its cached grants and rebuilds its subscription
// map. The cache drop is what makes a revocation effective — the map rebuild
// only stops a revoked plugin from sitting in the map (and restores the
// subscriptions of a plugin that was granted listen_events elsewhere).
type PluginSubscriptionsNotifier struct {
	pubsub    pubsub.PubSub
	refresher SubscriptionRefresher
	cache     PermissionCacheInvalidator
	logger    *slog.Logger
}

func NewPluginSubscriptionsNotifier(
	ps pubsub.PubSub,
	refresher SubscriptionRefresher,
	opts ...NotifierOption,
) *PluginSubscriptionsNotifier {
	n := &PluginSubscriptionsNotifier{
		pubsub:    ps,
		refresher: refresher,
		logger:    slog.Default(),
	}

	for _, opt := range opts {
		opt(n)
	}

	return n
}

func (n *PluginSubscriptionsNotifier) Start(ctx context.Context) error {
	return n.pubsub.Subscribe(ctx, channels.PluginSubscriptionsRefresh, n.handleRefresh)
}

// PublishRefresh announces a permission change. The local cache is dropped
// first: this instance must not depend on the broker echoing the message back
// to its own publisher, nor on it being delivered at all.
func (n *PluginSubscriptionsNotifier) PublishRefresh(ctx context.Context, pluginID uint64) error {
	n.invalidate(pluginID)

	msg, err := messages.NewMessage(
		channels.PluginSubscriptionsRefresh,
		messages.TypePluginSubscriptionsRefresh,
		messages.PluginSubscriptionsRefreshPayload{PluginID: pluginID},
	)
	if err != nil {
		return err
	}

	return n.pubsub.Publish(ctx, channels.PluginSubscriptionsRefresh, msg)
}

func (n *PluginSubscriptionsNotifier) handleRefresh(ctx context.Context, msg *pubsub.Message) error {
	payload, err := messages.ParsePayload[messages.PluginSubscriptionsRefreshPayload](msg)
	if err != nil {
		n.logger.Error("failed to parse plugin subscriptions refresh payload",
			slog.String("error", err.Error()),
		)

		return nil
	}

	// Before the rebuild, not after: building the map consults the grants, so
	// a stale cache would put the map back the way it was.
	n.invalidate(payload.PluginID)

	if n.refresher == nil {
		return nil
	}

	n.logger.Debug("rebuilding plugin event subscriptions after a permission change",
		slog.Uint64("plugin_id", payload.PluginID),
	)

	// The refresh calls every loaded plugin, so it must not inherit the
	// delivery context of the pubsub message.
	if err := n.refresher.RefreshSubscriptions(context.WithoutCancel(ctx)); err != nil {
		n.logger.Error("failed to refresh plugin event subscriptions",
			slog.Uint64("plugin_id", payload.PluginID),
			slog.String("error", err.Error()),
		)
	}

	return nil
}

func (n *PluginSubscriptionsNotifier) invalidate(pluginID uint64) {
	if n.cache == nil {
		return
	}

	n.cache.Invalidate(pluginID)
}
