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

// PluginSubscriptionsNotifier keeps the per-instance subscription maps in
// step with the shared permission records: the instance that changed a
// plugin's grants announces it, every instance rebuilds its map. Dispatch
// re-checks the grant per delivery anyway, so this only stops revoked
// plugins from sitting in the map (and restores subscriptions of a plugin
// that was granted listen_events elsewhere).
type PluginSubscriptionsNotifier struct {
	pubsub    pubsub.PubSub
	refresher SubscriptionRefresher
	logger    *slog.Logger
}

func NewPluginSubscriptionsNotifier(ps pubsub.PubSub, refresher SubscriptionRefresher) *PluginSubscriptionsNotifier {
	return &PluginSubscriptionsNotifier{
		pubsub:    ps,
		refresher: refresher,
		logger:    slog.Default(),
	}
}

func (n *PluginSubscriptionsNotifier) Start(ctx context.Context) error {
	return n.pubsub.Subscribe(ctx, channels.PluginSubscriptionsRefresh, n.handleRefresh)
}

// PublishRefresh announces a permission change; pluginID is informational.
func (n *PluginSubscriptionsNotifier) PublishRefresh(ctx context.Context, pluginID uint64) error {
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
	if n.refresher == nil {
		return nil
	}

	payload, err := messages.ParsePayload[messages.PluginSubscriptionsRefreshPayload](msg)
	if err != nil {
		n.logger.Error("failed to parse plugin subscriptions refresh payload",
			slog.String("error", err.Error()),
		)

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
