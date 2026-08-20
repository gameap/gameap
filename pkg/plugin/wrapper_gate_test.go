package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallFunction_queued_caller_gives_up_at_deadline(t *testing.T) {
	t.Parallel()
	// Guest functions are nil on purpose: a busy-wait failure must return
	// before the request is even marshaled.
	wrapper := &pluginServiceWrapper{gate: make(chan struct{}, 1)}
	wrapper.gate <- struct{}{}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := wrapper.callFunction(ctx, nil, &proto.GetInfoRequest{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPluginBusy)
	assert.Less(t, time.Since(start), time.Second, "the wait must end at the caller deadline")
}

func TestCallFunction_queued_caller_gives_up_on_cancellation(t *testing.T) {
	t.Parallel()
	wrapper := &pluginServiceWrapper{gate: make(chan struct{}, 1)}
	wrapper.gate <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := wrapper.callFunction(ctx, nil, &proto.GetInfoRequest{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPluginBusy)
}

func TestDispatcher_Dispatch_busy_plugin_is_not_disabled(t *testing.T) {
	t.Parallel()
	manager := newDispatcherTestManager()
	plugin := &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "busy"},
		Enabled: true,
		Instance: &mockPluginService{
			handleEventFunc: func(ctx context.Context, _ *proto.Event) (*proto.EventResult, error) {
				<-ctx.Done()

				return nil, errors.Wrapf(ErrPluginBusy, "%s", ctx.Err())
			},
		},
	}
	dispatcher := NewDispatcher(manager, discardLogger())
	dispatcher.callTimeout = 30 * time.Millisecond
	dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] = []*LoadedPlugin{plugin}

	result := dispatcher.Dispatch(context.Background(), &proto.Event{
		Type: proto.EventType_EVENT_TYPE_SERVER_POST_START,
	})

	require.Len(t, result.Errors, 1)
	assert.True(t, plugin.IsEnabled(),
		"a caller that gave up waiting for the call gate must not disable the plugin")
}
