package plugin

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDispatcher_Dispatch_call_timeout_disables_plugin(t *testing.T) {
	manager := newDispatcherTestManager()
	plugin := &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "slowpoke"},
		Enabled: true,
		Instance: &mockPluginService{
			handleEventFunc: func(ctx context.Context, _ *proto.Event) (*proto.EventResult, error) {
				<-ctx.Done()

				return nil, ctx.Err()
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
	assert.Contains(t, result.Errors[0].Error(), "context deadline exceeded")
	assert.False(t, plugin.IsEnabled(), "plugin must be disabled after a handler timeout")
}

func TestDispatcher_Dispatch_caller_deadline_does_not_disable_plugin(t *testing.T) {
	// The outer context expiring (e.g. the shared async delivery budget) is
	// not the plugin's fault — it must stay enabled.
	manager := newDispatcherTestManager()
	plugin := &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "innocent"},
		Enabled: true,
		Instance: &mockPluginService{
			handleEventFunc: func(ctx context.Context, _ *proto.Event) (*proto.EventResult, error) {
				<-ctx.Done()

				return nil, ctx.Err()
			},
		},
	}
	dispatcher := NewDispatcher(manager, discardLogger())
	dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] = []*LoadedPlugin{plugin}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	result := dispatcher.Dispatch(ctx, &proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_POST_START})

	require.Len(t, result.Errors, 1)
	assert.True(t, plugin.IsEnabled(), "plugin must stay enabled when the caller deadline expired")
}

func TestDispatcher_DispatchServerEventAsync_survives_caller_cancellation(t *testing.T) {
	manager := newDispatcherTestManager()
	received := make(chan *proto.Event, 1)
	plugin := &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "async"},
		Enabled: true,
		Instance: &mockPluginService{
			handleEventFunc: func(_ context.Context, event *proto.Event) (*proto.EventResult, error) {
				received <- event

				return &proto.EventResult{Handled: true}, nil
			},
		},
	}
	dispatcher := NewDispatcher(manager, discardLogger())
	dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_CREATED] = []*LoadedPlugin{plugin}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dispatcher.DispatchServerEventAsync(
		ctx, proto.EventType_EVENT_TYPE_SERVER_CREATED, &domain.Server{ID: 7, Name: "srv"}, nil,
	)

	select {
	case event := <-received:
		serverEvent := event.GetServerEvent()
		require.NotNil(t, serverEvent)
		require.NotNil(t, serverEvent.Server)
		assert.Equal(t, uint64(7), serverEvent.Server.Id)
	case <-time.After(2 * time.Second):
		t.Fatal("async event was not delivered")
	}
}

func TestDispatcher_DispatchServerEventsAsync_preserves_delivery_order(t *testing.T) {
	// Both events are delivered by one background goroutine, so a subscriber
	// to both must always observe them in the requested order.
	for i := range 10 {
		manager := newDispatcherTestManager()
		received := make(chan proto.EventType, 2)
		plugin := &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: "ordered"},
			Enabled: true,
			Instance: &mockPluginService{
				handleEventFunc: func(_ context.Context, event *proto.Event) (*proto.EventResult, error) {
					received <- event.Type

					return &proto.EventResult{Handled: true}, nil
				},
			},
		}
		dispatcher := NewDispatcher(manager, discardLogger())
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_DELETE] = []*LoadedPlugin{plugin}
		dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_DELETED] = []*LoadedPlugin{plugin}

		dispatcher.DispatchServerEventsAsync(
			context.Background(),
			[]proto.EventType{
				proto.EventType_EVENT_TYPE_SERVER_POST_DELETE,
				proto.EventType_EVENT_TYPE_SERVER_DELETED,
			},
			&domain.Server{ID: 1, Name: "srv"},
			nil,
		)

		var got []proto.EventType
		for len(got) < 2 {
			select {
			case eventType := <-received:
				got = append(got, eventType)
			case <-time.After(2 * time.Second):
				t.Fatalf("iteration %d: timed out waiting for events, got %v", i, got)
			}
		}

		require.Equal(t, []proto.EventType{
			proto.EventType_EVENT_TYPE_SERVER_POST_DELETE,
			proto.EventType_EVENT_TYPE_SERVER_DELETED,
		}, got, "iteration %d: post-delete must be delivered before deleted", i)
	}
}

func TestDispatcher_DispatchTaskEventAsync_delivers_payload(t *testing.T) {
	manager := newDispatcherTestManager()
	received := make(chan *proto.Event, 1)
	plugin := &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "taskwatcher"},
		Enabled: true,
		Instance: &mockPluginService{
			handleEventFunc: func(_ context.Context, event *proto.Event) (*proto.EventResult, error) {
				received <- event

				return &proto.EventResult{Handled: true}, nil
			},
		},
	}
	dispatcher := NewDispatcher(manager, discardLogger())
	dispatcher.subscriptions[proto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED] = []*LoadedPlugin{plugin}

	serverID := uint(5)
	dispatcher.DispatchTaskEventAsync(
		context.Background(), proto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED,
		11, 2, &serverID, "gsstart", "waiting", nil,
	)

	select {
	case event := <-received:
		taskEvent := event.GetTaskEvent()
		require.NotNil(t, taskEvent)
		assert.Equal(t, uint64(11), taskEvent.TaskId)
		assert.Equal(t, uint64(2), taskEvent.NodeId)
		require.NotNil(t, taskEvent.ServerId)
		assert.Equal(t, uint64(5), *taskEvent.ServerId)
		assert.Equal(t, "gsstart", taskEvent.TaskType)
		assert.Equal(t, "waiting", taskEvent.Status)
	case <-time.After(2 * time.Second):
		t.Fatal("async task event was not delivered")
	}
}
