package plugin

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapEventType(t *testing.T) {
	tests := []struct {
		name      string
		eventType servercontrol.PluginEventType
		want      proto.EventType
	}{
		{"pre_start", servercontrol.PluginEventServerPreStart, proto.EventType_EVENT_TYPE_SERVER_PRE_START},
		{"post_start", servercontrol.PluginEventServerPostStart, proto.EventType_EVENT_TYPE_SERVER_POST_START},
		{"pre_stop", servercontrol.PluginEventServerPreStop, proto.EventType_EVENT_TYPE_SERVER_PRE_STOP},
		{"post_stop", servercontrol.PluginEventServerPostStop, proto.EventType_EVENT_TYPE_SERVER_POST_STOP},
		{"pre_restart", servercontrol.PluginEventServerPreRestart, proto.EventType_EVENT_TYPE_SERVER_PRE_RESTART},
		{"post_restart", servercontrol.PluginEventServerPostRestart, proto.EventType_EVENT_TYPE_SERVER_POST_RESTART},
		{"pre_install", servercontrol.PluginEventServerPreInstall, proto.EventType_EVENT_TYPE_SERVER_PRE_INSTALL},
		{"post_install", servercontrol.PluginEventServerPostInstall, proto.EventType_EVENT_TYPE_SERVER_POST_INSTALL},
		{"pre_update", servercontrol.PluginEventServerPreUpdate, proto.EventType_EVENT_TYPE_SERVER_PRE_UPDATE},
		{"post_update", servercontrol.PluginEventServerPostUpdate, proto.EventType_EVENT_TYPE_SERVER_POST_UPDATE},
		{"pre_reinstall", servercontrol.PluginEventServerPreReinstall, proto.EventType_EVENT_TYPE_SERVER_PRE_REINSTALL},
		{"post_reinstall", servercontrol.PluginEventServerPostReinstall, proto.EventType_EVENT_TYPE_SERVER_POST_REINSTALL},
		{"pre_delete", servercontrol.PluginEventServerPreDelete, proto.EventType_EVENT_TYPE_SERVER_PRE_DELETE},
		{"post_delete", servercontrol.PluginEventServerPostDelete, proto.EventType_EVENT_TYPE_SERVER_POST_DELETE},
		{"created", servercontrol.PluginEventServerCreated, proto.EventType_EVENT_TYPE_SERVER_CREATED},
		{"updated", servercontrol.PluginEventServerUpdated, proto.EventType_EVENT_TYPE_SERVER_UPDATED},
		{"deleted", servercontrol.PluginEventServerDeleted, proto.EventType_EVENT_TYPE_SERVER_DELETED},
		{"unknown", servercontrol.PluginEventType(999), proto.EventType_EVENT_TYPE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mapEventType(tt.eventType))
		})
	}
}

func TestServerControlAdapter_DispatchServerEvent(t *testing.T) {
	server := &domain.Server{ID: 1, DSID: 10}

	t.Run("nil_adapter_returns_empty_result", func(t *testing.T) {
		// ARRANGE
		var adapter *ServerControlAdapter

		// ACT
		result := adapter.DispatchServerEvent(context.Background(), servercontrol.PluginEventServerPreStart, server, nil)

		// ASSERT
		require.NotNil(t, result, "nil adapter must return an empty result, not nil")
		assert.False(t, result.Cancelled)
	})

	t.Run("nil_dispatcher_returns_empty_result", func(t *testing.T) {
		// ARRANGE
		adapter := NewServerControlAdapter(nil)

		// ACT
		result := adapter.DispatchServerEvent(context.Background(), servercontrol.PluginEventServerPreStart, server, nil)

		// ASSERT
		require.NotNil(t, result, "nil dispatcher must return an empty result, not nil")
		assert.False(t, result.Cancelled)
	})

	t.Run("no_subscribers_not_cancelled", func(t *testing.T) {
		// ARRANGE
		adapter := NewServerControlAdapter(NewDispatcher(newDispatcherTestManager(), discardLogger()))

		// ACT
		result := adapter.DispatchServerEvent(context.Background(), servercontrol.PluginEventServerPreStart, server, nil)

		// ASSERT
		require.NotNil(t, result)
		assert.False(t, result.Cancelled)
	})

	t.Run("cancellation_is_propagated", func(t *testing.T) {
		// ARRANGE
		manager := newDispatcherTestManager()
		manager.plugins["plugin-a"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-a"},
			Enabled: true,
			Instance: &mockPluginService{
				getSubscribedEventsFunc: func(
					_ context.Context, _ *proto.GetSubscribedEventsRequest,
				) (*proto.GetSubscribedEventsResponse, error) {
					return &proto.GetSubscribedEventsResponse{
						Events: []proto.EventType{proto.EventType_EVENT_TYPE_SERVER_PRE_START},
					}, nil
				},
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					return &proto.EventResult{
						Handled:      true,
						ShouldCancel: true,
						Message:      new("nope"),
					}, nil
				},
			},
		}
		dispatcher := NewDispatcher(manager, discardLogger())
		require.NoError(t, dispatcher.RefreshSubscriptions(context.Background()))
		adapter := NewServerControlAdapter(dispatcher)

		// ACT
		result := adapter.DispatchServerEvent(context.Background(), servercontrol.PluginEventServerPreStart, server, nil)

		// ASSERT
		require.NotNil(t, result)
		assert.True(t, result.Cancelled, "plugin cancellation must reach the servercontrol caller")
		assert.Equal(t, "plugin-a", result.CancelledBy)
		assert.Equal(t, "nope", result.CancelMessage)
	})
}

func TestServerControlAdapter_DispatchServerEventAsync(t *testing.T) {
	server := &domain.Server{ID: 1, DSID: 10}

	t.Run("nil_adapter_is_noop", func(t *testing.T) {
		var adapter *ServerControlAdapter

		assert.NotPanics(t, func() {
			adapter.DispatchServerEventAsync(context.Background(), servercontrol.PluginEventServerPostStart, server, nil)
		})
	})

	t.Run("nil_dispatcher_is_noop", func(t *testing.T) {
		adapter := NewServerControlAdapter(nil)

		assert.NotPanics(t, func() {
			adapter.DispatchServerEventAsync(context.Background(), servercontrol.PluginEventServerPostStart, server, nil)
		})
	})

	t.Run("dispatches_to_dispatcher", func(t *testing.T) {
		adapter := NewServerControlAdapter(NewDispatcher(newDispatcherTestManager(), discardLogger()))

		assert.NotPanics(t, func() {
			adapter.DispatchServerEventAsync(context.Background(), servercontrol.PluginEventServerPostStart, server, nil)
		})
	})
}

func TestServerControlAdapter_DispatchServerEventsAsync(t *testing.T) {
	server := &domain.Server{ID: 1, DSID: 10}
	eventTypes := []servercontrol.PluginEventType{
		servercontrol.PluginEventServerPostStop,
		servercontrol.PluginEventServerPostDelete,
		servercontrol.PluginEventServerPostInstall,
	}

	t.Run("nil_adapter_is_noop", func(t *testing.T) {
		var adapter *ServerControlAdapter

		assert.NotPanics(t, func() {
			adapter.DispatchServerEventsAsync(context.Background(), eventTypes, server, nil)
		})
	})

	t.Run("nil_dispatcher_is_noop", func(t *testing.T) {
		adapter := NewServerControlAdapter(nil)

		assert.NotPanics(t, func() {
			adapter.DispatchServerEventsAsync(context.Background(), eventTypes, server, nil)
		})
	})

	t.Run("dispatches_batch_to_dispatcher", func(t *testing.T) {
		adapter := NewServerControlAdapter(NewDispatcher(newDispatcherTestManager(), discardLogger()))

		assert.NotPanics(t, func() {
			adapter.DispatchServerEventsAsync(context.Background(), eventTypes, server, nil)
		})
	})
}
