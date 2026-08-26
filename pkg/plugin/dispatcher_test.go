package plugin

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newDispatcherTestManager() *Manager {
	return NewManager(ManagerConfig{})
}

func TestNewDispatcher(t *testing.T) {
	t.Parallel()
	t.Run("returns_dispatcher_with_initialized_state", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := newDispatcherTestManager()
		logger := discardLogger()

		// ACT
		dispatcher := NewDispatcher(manager, logger)

		// ASSERT
		require.NotNil(t, dispatcher)
		assert.Same(t, manager, dispatcher.manager)
		assert.Same(t, logger, dispatcher.logger)
		assert.NotNil(t, dispatcher.subscriptions, "subscriptions map must be initialized")
		assert.False(t, dispatcher.subscriptionsOK, "subscriptionsOK must default to false")
	})
}

func TestDispatcher_RefreshSubscriptions(t *testing.T) {
	t.Parallel()
	t.Run("merges_subscriptions_from_multiple_plugins", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		manager := newDispatcherTestManager()
		manager.plugins["plugin-a"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-a"},
			Enabled: true,
			Instance: &mockPluginService{
				getSubscribedEventsFunc: func(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
					return &proto.GetSubscribedEventsResponse{
						Events: []proto.EventType{
							proto.EventType_EVENT_TYPE_SERVER_PRE_START,
							proto.EventType_EVENT_TYPE_SERVER_POST_START,
						},
					}, nil
				},
			},
		}
		manager.plugins["plugin-b"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-b"},
			Enabled: true,
			Instance: &mockPluginService{
				getSubscribedEventsFunc: func(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
					return &proto.GetSubscribedEventsResponse{
						Events: []proto.EventType{
							proto.EventType_EVENT_TYPE_SERVER_PRE_START,
							proto.EventType_EVENT_TYPE_SERVER_POST_STOP,
						},
					}, nil
				},
			},
		}
		dispatcher := NewDispatcher(manager, discardLogger())

		// ACT
		err := dispatcher.RefreshSubscriptions(context.Background())

		// ASSERT
		require.NoError(t, err)
		assert.True(t, dispatcher.subscriptionsOK, "subscriptionsOK must be set after refresh")
		require.Len(t, dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_PRE_START], 2,
			"both plugins must be subscribed to PRE_START")
		require.Len(t, dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START], 1,
			"only plugin-a must be subscribed to POST_START")
		require.Len(t, dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_STOP], 1,
			"only plugin-b must be subscribed to POST_STOP")
	})

	t.Run("skips_disabled_plugins", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := newDispatcherTestManager()
		var enabledCalled, disabledCalled atomic.Bool
		manager.plugins["enabled"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "enabled"},
			Enabled: true,
			Instance: &mockPluginService{
				getSubscribedEventsFunc: func(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
					enabledCalled.Store(true)

					return &proto.GetSubscribedEventsResponse{
						Events: []proto.EventType{proto.EventType_EVENT_TYPE_SERVER_POST_START},
					}, nil
				},
			},
		}
		manager.plugins["disabled"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "disabled"},
			Enabled: false,
			Instance: &mockPluginService{
				getSubscribedEventsFunc: func(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
					disabledCalled.Store(true)

					return &proto.GetSubscribedEventsResponse{
						Events: []proto.EventType{proto.EventType_EVENT_TYPE_SERVER_POST_START},
					}, nil
				},
			},
		}
		dispatcher := NewDispatcher(manager, discardLogger())

		// ACT
		err := dispatcher.RefreshSubscriptions(context.Background())

		// ASSERT
		require.NoError(t, err)
		assert.True(t, enabledCalled.Load(), "enabled plugin must be queried")
		assert.False(t, disabledCalled.Load(), "disabled plugin must not be queried")
		require.Len(t, dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START], 1)
	})

	t.Run("continues_on_plugin_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		manager := newDispatcherTestManager()
		manager.plugins["broken"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "broken"},
			Enabled: true,
			Instance: &mockPluginService{
				getSubscribedEventsFunc: func(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
					return nil, errors.New("boom")
				},
			},
		}
		manager.plugins["working"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "working"},
			Enabled: true,
			Instance: &mockPluginService{
				getSubscribedEventsFunc: func(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
					return &proto.GetSubscribedEventsResponse{
						Events: []proto.EventType{proto.EventType_EVENT_TYPE_SERVER_POST_START},
					}, nil
				},
			},
		}
		dispatcher := NewDispatcher(manager, discardLogger())

		// ACT
		err := dispatcher.RefreshSubscriptions(context.Background())

		// ASSERT
		require.NoError(t, err, "errors from individual plugins must not abort the refresh")
		assert.True(t, dispatcher.subscriptionsOK)
		require.Len(t, dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START], 1,
			"working plugin's subscription must still be recorded")
	})

	t.Run("refresh_replaces_previous_subscriptions", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := newDispatcherTestManager()
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_PRE_START] = []*LoadedPlugin{
			{Info: &proto.PluginInfo{Id: "stale"}, Enabled: true},
		}

		// ACT
		err := dispatcher.RefreshSubscriptions(context.Background())

		// ASSERT
		require.NoError(t, err)
		assert.Empty(t, dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_PRE_START],
			"stale entries must be cleared on refresh")
	})

	t.Run("returns_no_subscriptions_when_no_plugins", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := newDispatcherTestManager()
		dispatcher := NewDispatcher(manager, discardLogger())

		// ACT
		err := dispatcher.RefreshSubscriptions(context.Background())

		// ASSERT
		require.NoError(t, err)
		assert.True(t, dispatcher.subscriptionsOK)
		assert.Empty(t, dispatcher.subscriptions)
	})
}

func TestDispatcher_Dispatch(t *testing.T) {
	t.Parallel()
	t.Run("returns_empty_result_when_no_subscribers", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		dispatcher := NewDispatcher(newDispatcherTestManager(), discardLogger())
		event := &proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_POST_START}

		// ACT
		result := dispatcher.Dispatch(context.Background(), event)

		// ASSERT
		require.NotNil(t, result)
		assert.False(t, result.Cancelled)
		assert.Empty(t, result.HandledBy)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.ModifiedData)
	})

	t.Run("dispatches_to_single_subscriber", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := newDispatcherTestManager()
		var receivedEventType proto.EventType
		manager.plugins["only"] = &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "only"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, event *proto.Event) (*proto.EventResult, error) {
					receivedEventType = event.Type

					return &proto.EventResult{Handled: true}, nil
				},
			},
		}
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] = []*LoadedPlugin{manager.plugins["only"]}
		event := &proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_POST_START}

		// ACT
		result := dispatcher.Dispatch(context.Background(), event)

		// ASSERT
		assert.Equal(t, proto.EventType_EVENT_TYPE_SERVER_POST_START, receivedEventType)
		require.Len(t, result.HandledBy, 1)
		assert.Equal(t, "only", result.HandledBy[0])
		assert.False(t, result.Cancelled)
		assert.Empty(t, result.Errors)
	})

	t.Run("merges_modified_data_across_plugins", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		manager := newDispatcherTestManager()
		pluginA := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-a"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					return &proto.EventResult{
						Handled:      true,
						ModifiedData: map[string]string{"k1": "v1", "shared": "from-a"},
					}, nil
				},
			},
		}
		pluginB := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-b"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					return &proto.EventResult{
						Handled:      true,
						ModifiedData: map[string]string{"k2": "v2", "shared": "from-b"},
					}, nil
				},
			},
		}
		manager.plugins["plugin-a"] = pluginA
		manager.plugins["plugin-b"] = pluginB
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] = []*LoadedPlugin{pluginA, pluginB}

		// ACT
		result := dispatcher.Dispatch(
			context.Background(),
			&proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_POST_START},
		)

		// ASSERT
		require.Len(t, result.HandledBy, 2)
		assert.Equal(t, "v1", result.ModifiedData["k1"])
		assert.Equal(t, "v2", result.ModifiedData["k2"])
		assert.Equal(t, "from-b", result.ModifiedData["shared"], "later plugin must overwrite shared key")
	})

	t.Run("cancellable_event_short_circuits_after_cancel", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := newDispatcherTestManager()
		var calledA, calledB atomic.Bool
		cancelMsg := "blocked by plugin-a"
		pluginA := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-a"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					calledA.Store(true)

					return &proto.EventResult{
						Handled:      true,
						ShouldCancel: true,
						Message:      &cancelMsg,
					}, nil
				},
			},
		}
		pluginB := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-b"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					calledB.Store(true)

					return &proto.EventResult{Handled: true}, nil
				},
			},
		}
		manager.plugins["plugin-a"] = pluginA
		manager.plugins["plugin-b"] = pluginB
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_PRE_START] = []*LoadedPlugin{pluginA, pluginB}

		// ACT
		result := dispatcher.Dispatch(
			context.Background(),
			&proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_PRE_START},
		)

		// ASSERT
		assert.True(t, result.Cancelled, "result must report cancellation")
		assert.Equal(t, "plugin-a", result.CancelledBy)
		assert.Equal(t, cancelMsg, result.CancelMessage)
		assert.True(t, calledA.Load(), "first plugin must be invoked")
		assert.False(t, calledB.Load(), "subsequent plugins must NOT be invoked after cancel")
	})

	t.Run("non_cancellable_event_continues_even_when_plugin_requests_cancel", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		manager := newDispatcherTestManager()
		var calledB atomic.Bool
		pluginA := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-a"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					return &proto.EventResult{
						Handled:      true,
						ShouldCancel: true,
					}, nil
				},
			},
		}
		pluginB := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-b"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					calledB.Store(true)

					return &proto.EventResult{Handled: true}, nil
				},
			},
		}
		manager.plugins["plugin-a"] = pluginA
		manager.plugins["plugin-b"] = pluginB
		dispatcher := NewDispatcher(manager, discardLogger())
		// POST_START is NOT cancellable
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] = []*LoadedPlugin{pluginA, pluginB}

		// ACT
		result := dispatcher.Dispatch(
			context.Background(),
			&proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_POST_START},
		)

		// ASSERT
		assert.False(t, result.Cancelled, "non-cancellable events must not be cancelled")
		assert.True(t, calledB.Load(), "second plugin must still be invoked on non-cancellable events")
		require.Len(t, result.HandledBy, 2)
	})

	t.Run("plugin_error_appended_and_loop_continues", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := newDispatcherTestManager()
		var calledB atomic.Bool
		pluginA := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-a"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					return nil, errors.New("plugin a failed")
				},
			},
		}
		pluginB := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "plugin-b"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					calledB.Store(true)

					return &proto.EventResult{Handled: true}, nil
				},
			},
		}
		manager.plugins["plugin-a"] = pluginA
		manager.plugins["plugin-b"] = pluginB
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] = []*LoadedPlugin{pluginA, pluginB}

		// ACT
		result := dispatcher.Dispatch(
			context.Background(),
			&proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_POST_START},
		)

		// ASSERT
		require.Len(t, result.Errors, 1, "error from plugin-a must be appended")
		assert.Contains(t, result.Errors[0].Error(), "plugin-a", "error must reference offending plugin ID")
		assert.Contains(t, result.Errors[0].Error(), "plugin a failed")
		assert.True(t, calledB.Load(), "loop must continue past errored plugin")
		require.Len(t, result.HandledBy, 1, "only plugin-b should appear in HandledBy")
		assert.Equal(t, "plugin-b", result.HandledBy[0])
	})

	t.Run("disabled_plugin_in_subscriptions_is_skipped", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		manager := newDispatcherTestManager()
		var calledDisabled atomic.Bool
		disabled := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "disabled"},
			Enabled: false,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					calledDisabled.Store(true)

					return &proto.EventResult{Handled: true}, nil
				},
			},
		}
		manager.plugins["disabled"] = disabled
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] = []*LoadedPlugin{disabled}

		// ACT
		result := dispatcher.Dispatch(
			context.Background(),
			&proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_POST_START},
		)

		// ASSERT
		assert.False(t, calledDisabled.Load(), "disabled plugin must not have HandleEvent called")
		assert.Empty(t, result.HandledBy)
		assert.Empty(t, result.Errors)
	})

	t.Run("event_result_without_handled_does_not_appear_in_handled_by", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := newDispatcherTestManager()
		plugin := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "no-op"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					return &proto.EventResult{Handled: false}, nil
				},
			},
		}
		manager.plugins["no-op"] = plugin
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] = []*LoadedPlugin{plugin}

		// ACT
		result := dispatcher.Dispatch(
			context.Background(),
			&proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_POST_START},
		)

		// ASSERT
		assert.Empty(t, result.HandledBy, "Handled=false must not add plugin to HandledBy")
	})
}

func TestDispatcher_DispatchServerEvent(t *testing.T) {
	t.Parallel()
	t.Run("dispatches_event_with_server_payload", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := newDispatcherTestManager()
		var captured *proto.Event
		plugin := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "captor"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, event *proto.Event) (*proto.EventResult, error) {
					captured = event

					return &proto.EventResult{Handled: true}, nil
				},
			},
		}
		manager.plugins["captor"] = plugin
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] = []*LoadedPlugin{plugin}

		server := &domain.Server{
			ID:         42,
			UID:        uuid.New(),
			UUIDShort:  "shortuid",
			Enabled:    true,
			Name:       "my-server",
			GameID:     "cs",
			ServerIP:   "10.0.0.1",
			ServerPort: 27015,
		}
		extra := map[string]string{"foo": "bar"}

		// ACT
		result := dispatcher.DispatchServerEvent(
			context.Background(),
			proto.EventType_EVENT_TYPE_SERVER_POST_START,
			server,
			extra,
		)

		// ASSERT
		require.NotNil(t, captured, "plugin must receive the event")
		assert.Equal(t, proto.EventType_EVENT_TYPE_SERVER_POST_START, captured.Type)
		require.NotNil(t, captured.Context)
		assert.NotEmpty(t, captured.Context.RequestId, "request id must be generated")
		assert.NotZero(t, captured.Timestamp, "timestamp must be set")
		serverEvent := captured.GetServerEvent()
		require.NotNil(t, serverEvent)
		require.NotNil(t, serverEvent.Server)
		assert.Equal(t, uint64(42), serverEvent.Server.Id)
		assert.Equal(t, "my-server", serverEvent.Server.Name)
		assert.Equal(t, "bar", serverEvent.ExtraData["foo"])
		require.Len(t, result.HandledBy, 1)
	})
}

func TestDispatcher_DispatchTaskEvent(t *testing.T) {
	t.Parallel()
	t.Run("dispatches_task_event_with_payload", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		manager := newDispatcherTestManager()
		var captured *proto.Event
		plugin := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "captor"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, event *proto.Event) (*proto.EventResult, error) {
					captured = event

					return &proto.EventResult{Handled: true}, nil
				},
			},
		}
		manager.plugins["captor"] = plugin
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED] = []*LoadedPlugin{plugin}

		serverID := uint(99)
		extra := map[string]string{"k": "v"}

		// ACT
		result := dispatcher.DispatchTaskEvent(
			context.Background(),
			proto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED,
			123, 7, &serverID, "gsstart", "queued", extra,
		)

		// ASSERT
		require.NotNil(t, captured)
		taskEvent := captured.GetTaskEvent()
		require.NotNil(t, taskEvent)
		assert.Equal(t, uint64(123), taskEvent.TaskId)
		assert.Equal(t, uint64(7), taskEvent.NodeId)
		require.NotNil(t, taskEvent.ServerId)
		assert.Equal(t, uint64(99), *taskEvent.ServerId)
		assert.Equal(t, "gsstart", taskEvent.TaskType)
		assert.Equal(t, "queued", taskEvent.Status)
		assert.Equal(t, "v", taskEvent.ExtraData["k"])
		require.Len(t, result.HandledBy, 1)
	})

	t.Run("nil_server_id_results_in_nil_payload_field", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		manager := newDispatcherTestManager()
		var captured *proto.Event
		plugin := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "captor"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, event *proto.Event) (*proto.EventResult, error) {
					captured = event

					return &proto.EventResult{Handled: true}, nil
				},
			},
		}
		manager.plugins["captor"] = plugin
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_DAEMON_TASK_COMPLETED] = []*LoadedPlugin{plugin}

		// ACT
		_ = dispatcher.DispatchTaskEvent(
			context.Background(),
			proto.EventType_EVENT_TYPE_DAEMON_TASK_COMPLETED,
			321, 8, nil, "gsstop", "done", nil,
		)

		// ASSERT
		taskEvent := captured.GetTaskEvent()
		require.NotNil(t, taskEvent)
		assert.Nil(t, taskEvent.ServerId, "nil serverID must yield nil payload field")
	})
}

func TestDispatcher_HasSubscribers(t *testing.T) {
	t.Parallel()
	manager := newDispatcherTestManager()
	enabled := &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "enabled"},
		Enabled: true,
	}
	disabled := &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "disabled"},
		Enabled: false,
	}
	manager.plugins["enabled"] = enabled
	manager.plugins["disabled"] = disabled

	dispatcher := NewDispatcher(manager, discardLogger())
	dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] = []*LoadedPlugin{enabled}
	dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_STOP] = []*LoadedPlugin{disabled}

	tests := []struct {
		name      string
		eventType proto.EventType
		want      bool
	}{
		{
			name:      "no_subscribers_returns_false",
			eventType: proto.EventType_EVENT_TYPE_SERVER_PRE_START,
			want:      false,
		},
		{
			name:      "only_disabled_subscriber_returns_false",
			eventType: proto.EventType_EVENT_TYPE_SERVER_POST_STOP,
			want:      false,
		},
		{
			name:      "enabled_subscriber_returns_true",
			eventType: proto.EventType_EVENT_TYPE_SERVER_POST_START,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := dispatcher.HasSubscribers(tt.eventType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsCancellableEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		eventType proto.EventType
		want      bool
	}{
		{
			name:      "pre_start_is_cancellable",
			eventType: proto.EventType_EVENT_TYPE_SERVER_PRE_START,
			want:      true,
		},
		{
			name:      "pre_stop_is_cancellable",
			eventType: proto.EventType_EVENT_TYPE_SERVER_PRE_STOP,
			want:      true,
		},
		{
			name:      "pre_restart_is_cancellable",
			eventType: proto.EventType_EVENT_TYPE_SERVER_PRE_RESTART,
			want:      true,
		},
		{
			name:      "pre_install_is_cancellable",
			eventType: proto.EventType_EVENT_TYPE_SERVER_PRE_INSTALL,
			want:      true,
		},
		{
			name:      "pre_update_is_cancellable",
			eventType: proto.EventType_EVENT_TYPE_SERVER_PRE_UPDATE,
			want:      true,
		},
		{
			name:      "pre_reinstall_is_cancellable",
			eventType: proto.EventType_EVENT_TYPE_SERVER_PRE_REINSTALL,
			want:      true,
		},
		{
			name:      "pre_delete_is_cancellable",
			eventType: proto.EventType_EVENT_TYPE_SERVER_PRE_DELETE,
			want:      true,
		},
		{
			name:      "post_start_is_not_cancellable",
			eventType: proto.EventType_EVENT_TYPE_SERVER_POST_START,
			want:      false,
		},
		{
			name:      "post_stop_is_not_cancellable",
			eventType: proto.EventType_EVENT_TYPE_SERVER_POST_STOP,
			want:      false,
		},
		{
			name:      "server_created_is_not_cancellable",
			eventType: proto.EventType_EVENT_TYPE_SERVER_CREATED,
			want:      false,
		},
		{
			name:      "daemon_task_created_is_not_cancellable",
			eventType: proto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED,
			want:      false,
		},
		{
			name:      "unspecified_is_not_cancellable",
			eventType: proto.EventType_EVENT_TYPE_UNSPECIFIED,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isCancellableEvent(tt.eventType)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDomainServerToProto(t *testing.T) {
	t.Parallel()
	t.Run("nil_server_returns_nil", func(t *testing.T) {
		t.Parallel()

		// ACT
		got := domainServerToProto(nil)

		// ASSERT
		assert.Nil(t, got)
	})

	t.Run("converts_all_fields_correctly", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		uid := uuid.New()
		queryPort := 27016
		rconPort := 27015
		suUser := "gameap"
		startCmd := "./srcds_run"
		server := &domain.Server{
			ID:            10,
			UID:           uid,
			UUID:          uid,
			UUIDShort:     "abcd1234",
			Enabled:       true,
			Installed:     domain.ServerInstalledStatusInstalled,
			Blocked:       false,
			Name:          "Counter-Strike Server",
			GameID:        "cs",
			DSID:          3,
			GameModID:     7,
			ServerIP:      "10.0.0.5",
			ServerPort:    27015,
			QueryPort:     &queryPort,
			RconPort:      &rconPort,
			Dir:           "/srv/cs",
			SuUser:        &suUser,
			StartCommand:  &startCmd,
			ProcessActive: true,
		}

		// ACT
		got := domainServerToProto(server)

		// ASSERT
		require.NotNil(t, got)
		assert.Equal(t, uint64(10), got.Id)
		assert.Equal(t, uid.String(), got.Uuid)
		assert.Equal(t, "abcd1234", got.UuidShort)
		assert.True(t, got.Enabled)
		assert.False(t, got.Blocked)
		assert.Equal(t, "Counter-Strike Server", got.Name)
		assert.Equal(t, "cs", got.GameId)
		assert.Equal(t, uint64(3), got.DsId)
		assert.Equal(t, uint64(7), got.GameModId)
		assert.Equal(t, "10.0.0.5", got.ServerIp)
		assert.Equal(t, int32(27015), got.ServerPort)
		require.NotNil(t, got.QueryPort)
		assert.Equal(t, int32(27016), *got.QueryPort)
		require.NotNil(t, got.RconPort)
		assert.Equal(t, int32(27015), *got.RconPort)
		assert.Equal(t, "/srv/cs", got.Dir)
		require.NotNil(t, got.SuUser)
		assert.Equal(t, "gameap", *got.SuUser)
		require.NotNil(t, got.StartCommand)
		assert.Equal(t, "./srcds_run", *got.StartCommand)
		assert.True(t, got.ProcessActive)
	})

	t.Run("nil_optional_ports_yield_nil_proto_fields", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		server := &domain.Server{
			ID:         11,
			UID:        uuid.New(),
			Name:       "no-ports-server",
			ServerIP:   "10.0.0.6",
			ServerPort: 27015,
			QueryPort:  nil,
			RconPort:   nil,
		}

		// ACT
		got := domainServerToProto(server)

		// ASSERT
		require.NotNil(t, got)
		assert.Nil(t, got.QueryPort, "nil domain QueryPort must yield nil proto QueryPort")
		assert.Nil(t, got.RconPort, "nil domain RconPort must yield nil proto RconPort")
	})
}
