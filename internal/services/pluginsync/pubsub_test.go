package pluginsync_test

import (
	"context"
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

// waitForLoad polls until the reconciler has applied the plugin, so the test
// does not depend on how fast the loop goroutine gets scheduled.
func waitForLoad(t *testing.T, h *harness, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(h.loader.loadCalls()) >= want {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("expected %d load calls, got %d", want, len(h.loader.loadCalls()))
}

func TestService_hints(t *testing.T) {
	t.Run("a_hint_triggers_a_pass", func(t *testing.T) {
		bus := memory.New()
		h := newHarness(t, func(d *pluginsync.Deps, _ *pluginsync.Options) {
			d.Bus = bus
		})

		ctx := t.Context()

		require.NoError(t, h.svc.Subscribe(ctx))
		require.NoError(t, h.svc.Start(ctx))
		defer h.svc.Stop()

		// The plugin appears only after the first pass, so the hint is what
		// must bring it in.
		h.writePluginFile(t)
		h.repo.set(activeRow())

		h.svc.Notify(ctx, 1, "install")

		waitForLoad(t, h, 1)
	})

	t.Run("own_hint_still_triggers_a_pass", func(t *testing.T) {
		// Source filtering is deliberately absent: a pass is idempotent, and
		// every replica shares an instance ID when PUBSUB_INSTANCE_ID is unset.
		bus := memory.New()
		h := newHarness(t, func(d *pluginsync.Deps, _ *pluginsync.Options) {
			d.Bus = bus
		})

		ctx := t.Context()

		require.NoError(t, h.svc.Subscribe(ctx))
		require.NoError(t, h.svc.Start(ctx))
		defer h.svc.Stop()

		h.writePluginFile(t)
		h.repo.set(activeRow())

		msg, err := messages.NewMessage(channels.PluginSync, messages.TypePluginSync, messages.PluginSyncPayload{
			PluginID: 1,
			Reason:   "install",
		})
		require.NoError(t, err)
		require.NoError(t, bus.Publish(ctx, channels.PluginSync, msg))

		waitForLoad(t, h, 1)
	})

	t.Run("malformed_payload_leaves_the_subscription_alive", func(t *testing.T) {
		bus := memory.New()
		h := newHarness(t, func(d *pluginsync.Deps, _ *pluginsync.Options) {
			d.Bus = bus
		})

		ctx := t.Context()

		require.NoError(t, h.svc.Subscribe(ctx))
		require.NoError(t, h.svc.Start(ctx))
		defer h.svc.Stop()

		require.NoError(t, bus.Publish(ctx, channels.PluginSync, &pubsub.Message{
			ID:      "broken",
			Channel: channels.PluginSync,
			Type:    messages.TypePluginSync,
			Payload: []byte("not json"),
		}))

		h.writePluginFile(t)
		h.repo.set(activeRow())
		h.svc.Notify(ctx, 1, "install")

		waitForLoad(t, h, 1)
	})

	t.Run("repeated_hints_collapse_into_one_pass", func(t *testing.T) {
		h := newHarness(t)

		for range 100 {
			h.svc.Kick()
		}

		// The wake channel holds one slot, so a hundred hints cost one pass.
		assert.Empty(t, h.loader.loadCalls())
	})

	t.Run("notify_without_a_bus_is_a_no_op", func(t *testing.T) {
		h := newHarness(t)

		h.svc.Notify(context.Background(), 1, "install")
	})
}
