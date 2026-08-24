package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitTimeout bounds the polling for asynchronous in-memory deliveries.
const waitTimeout = 2 * time.Second

// countingRefresher records how often the local subscription map was rebuilt.
type countingRefresher struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (r *countingRefresher) RefreshSubscriptions(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++

	return r.err
}

func (r *countingRefresher) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.calls
}

func TestPluginSubscriptionsNotifier_refreshes_every_instance(t *testing.T) {
	t.Parallel()

	bus, ctx := setupPubsub(t)

	// Two panel instances share the bus; the change is made on the first.
	local := &countingRefresher{}
	remote := &countingRefresher{}

	publisher := NewPluginSubscriptionsNotifier(bus, local)
	subscriber := NewPluginSubscriptionsNotifier(bus, remote)
	require.NoError(t, subscriber.Start(ctx))

	require.NoError(t, publisher.PublishRefresh(ctx, 42))

	waitFor(t, func() bool { return remote.count() == 1 }, waitTimeout,
		"the instance that did not make the change rebuilds its subscriptions")
}

func TestPluginSubscriptionsNotifier_publishes_the_plugin_id(t *testing.T) {
	t.Parallel()

	bus, ctx := setupPubsub(t)
	rec := subscribeRecorder(ctx, t, bus, channels.PluginSubscriptionsRefresh)

	require.NoError(t, NewPluginSubscriptionsNotifier(bus, nil).PublishRefresh(ctx, 7))

	got := rec.snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, channels.PluginSubscriptionsRefresh, got[0].Channel)
	assert.Equal(t, messages.TypePluginSubscriptionsRefresh, got[0].Type)

	payload, err := messages.ParsePayload[messages.PluginSubscriptionsRefreshPayload](got[0])
	require.NoError(t, err)
	assert.Equal(t, uint64(7), payload.PluginID)
}

func TestPluginSubscriptionsNotifier_tolerates_a_refresh_error(t *testing.T) {
	t.Parallel()

	bus, ctx := setupPubsub(t)

	refresher := &countingRefresher{err: errBoom}
	notifier := NewPluginSubscriptionsNotifier(bus, refresher)
	require.NoError(t, notifier.Start(ctx))

	require.NoError(t, notifier.PublishRefresh(ctx, 1))

	// A failing refresh must not fail the delivery: the bus would otherwise
	// treat the message as poison and retry it.
	waitFor(t, func() bool { return refresher.count() == 1 }, waitTimeout, "the refresh is attempted")
}

func TestPluginSubscriptionsNotifier_ignores_a_malformed_payload(t *testing.T) {
	t.Parallel()

	bus, ctx := setupPubsub(t)

	refresher := &countingRefresher{}
	notifier := NewPluginSubscriptionsNotifier(bus, refresher)
	require.NoError(t, notifier.Start(ctx))

	var delivered atomic.Bool
	require.NoError(t, bus.Subscribe(ctx, channels.PluginSubscriptionsRefresh,
		func(context.Context, *pubsub.Message) error {
			delivered.Store(true)

			return nil
		}))

	require.NoError(t, bus.Publish(ctx, channels.PluginSubscriptionsRefresh, &pubsub.Message{
		Channel: channels.PluginSubscriptionsRefresh,
		Type:    messages.TypePluginSubscriptionsRefresh,
		Payload: []byte(`"not an object"`),
	}))

	waitFor(t, delivered.Load, waitTimeout, "the message reaches the subscribers")
	assert.Equal(t, 0, refresher.count(), "an unreadable announcement rebuilds nothing")
}
