package pluginsync_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/services/pluginsync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBusEnv(t *testing.T) (*env, *memory.Memory) {
	t.Helper()

	bus := memory.New()
	t.Cleanup(func() { _ = bus.Close() })

	e := newEnv(t, activeRow(1))
	e.service = pluginsync.New(pluginsync.Deps{
		Repo: e.repo, Loader: e.loader, Plugins: e.loader, Subs: e.subs, Archive: e.archive,
		Files: e.files, Store: e.store, Locks: e.locks, Bus: bus, Audit: e.audit, Metrics: e.passes,
		PluginsDir: "plugins",
	}, pluginsync.Options{Clock: e.clock}, slog.New(slog.DiscardHandler))

	return e, bus
}

func TestNotify_publishes_a_hint_and_the_subscriber_reconciles(t *testing.T) {
	t.Parallel()

	e, bus := newBusEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var received *messages.PluginSyncPayload
	require.NoError(t, bus.Subscribe(ctx, channels.PluginSync, func(_ context.Context, msg *pubsub.Message) error {
		payload, err := messages.ParsePayload[messages.PluginSyncPayload](msg)
		if err == nil {
			received = &payload
		}

		return nil
	}))

	require.NoError(t, e.service.Subscribe(ctx))
	require.NoError(t, e.service.Start(ctx))
	t.Cleanup(e.service.Stop)

	require.True(t, e.loader.isRunning(1), "the first pass is synchronous")

	e.repo.put(activeRow(2))
	require.NoError(t, e.files.Write(ctx, "plugins/pc.wasm", []byte(wasmContent)))

	e.service.Notify(ctx, 2, "install")

	assert.Eventually(t, func() bool { return e.loader.isRunning(2) }, 5*time.Second, 10*time.Millisecond)
	require.NotNil(t, received)
	assert.Equal(t, uint64(2), received.PluginID)
	assert.Equal(t, "install", received.Action)
}

func TestSubscribe_tolerates_malformed_hints(t *testing.T) {
	t.Parallel()

	e, bus := newBusEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, e.service.Subscribe(ctx))
	require.NoError(t, e.service.Start(ctx))
	t.Cleanup(e.service.Stop)

	require.NoError(t, bus.Publish(ctx, channels.PluginSync, &pubsub.Message{
		Channel: channels.PluginSync, Type: messages.TypePluginSync, Payload: []byte("not json"),
	}))

	e.repo.put(activeRow(2))
	require.NoError(t, e.files.Write(ctx, "plugins/pc.wasm", []byte(wasmContent)))

	e.service.Notify(ctx, 2, "install")

	assert.Eventually(t, func() bool { return e.loader.isRunning(2) }, 5*time.Second, 10*time.Millisecond,
		"the subscription survived the malformed message")
}

func TestKick_collapses_repeated_hints(t *testing.T) {
	t.Parallel()

	e, _ := newBusEnv(t)

	for range 10 {
		e.service.Kick()
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, e.service.Start(ctx))
	t.Cleanup(e.service.Stop)

	assert.Eventually(t, func() bool { return e.repo.readCount() >= 2 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	assert.LessOrEqual(t, e.repo.readCount(), 3, "ten kicks cost at most one extra pass after the initial one")
}
