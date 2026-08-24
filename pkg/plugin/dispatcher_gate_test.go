package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func subscribingPlugin(id string, dbID uint64) *LoadedPlugin {
	return &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: id},
		Enabled: true,
		DBID:    dbID,
		Instance: &mockPluginService{
			getSubscribedEventsFunc: func(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
				return &proto.GetSubscribedEventsResponse{
					Events: []proto.EventType{proto.EventType_EVENT_TYPE_SERVER_POST_START},
				}, nil
			},
		},
	}
}

func TestDispatcher_SubscriptionGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		gate            SubscriptionGate
		wantSubscribers []string
	}{
		{
			name:            "no_gate_admits_everyone",
			gate:            nil,
			wantSubscribers: []string{"granted", "ungranted"},
		},
		{
			name: "gate_keeps_only_granted_plugins",
			gate: func(_ context.Context, plugin *LoadedPlugin) bool {
				return plugin.DBID == 1
			},
			wantSubscribers: []string{"granted"},
		},
		{
			name:            "gate_refusing_everyone_leaves_no_subscriptions",
			gate:            func(context.Context, *LoadedPlugin) bool { return false },
			wantSubscribers: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			manager := newDispatcherTestManager()
			manager.plugins["granted"] = subscribingPlugin("granted", 1)
			manager.plugins["ungranted"] = subscribingPlugin("ungranted", 2)

			dispatcher := NewDispatcher(manager, discardLogger(), WithSubscriptionGate(tt.gate))

			require.NoError(t, dispatcher.RefreshSubscriptions(context.Background()))
			assert.True(t, dispatcher.subscriptionsOK)

			subscribers := make([]string, 0)
			for _, plugin := range dispatcher.subscriptions[proto.EventType_EVENT_TYPE_SERVER_POST_START] {
				subscribers = append(subscribers, plugin.Info.Id)
			}

			assert.ElementsMatch(t, tt.wantSubscribers, subscribers)
		})
	}
}

func TestDispatcher_SubscriptionGate_is_not_consulted_for_plugins_without_subscriptions(t *testing.T) {
	t.Parallel()

	manager := newDispatcherTestManager()
	manager.plugins["silent"] = &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "silent"},
		Enabled: true,
		Instance: &mockPluginService{
			getSubscribedEventsFunc: func(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
				return &proto.GetSubscribedEventsResponse{}, nil
			},
		},
	}

	consulted := false
	dispatcher := NewDispatcher(manager, discardLogger(), WithSubscriptionGate(func(context.Context, *LoadedPlugin) bool {
		consulted = true

		return false
	}))

	require.NoError(t, dispatcher.RefreshSubscriptions(context.Background()))
	assert.False(t, consulted, "a plugin that subscribes to nothing needs no grant lookup")
}

func TestDispatcher_observes_event_outcomes(t *testing.T) {
	t.Parallel()

	manager := newDispatcherTestManager()
	manager.plugins["handler"] = &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "handler"},
		Enabled: true,
		Instance: &mockPluginService{
			getSubscribedEventsFunc: func(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
				return &proto.GetSubscribedEventsResponse{
					Events: []proto.EventType{proto.EventType_EVENT_TYPE_SERVER_POST_START, proto.EventType_EVENT_TYPE_SERVER_PRE_STOP},
				}, nil
			},
			handleEventFunc: func(_ context.Context, event *proto.Event) (*proto.EventResult, error) {
				if event.Type == proto.EventType_EVENT_TYPE_SERVER_PRE_STOP {
					return &proto.EventResult{Handled: true, ShouldCancel: true}, nil
				}

				return &proto.EventResult{Handled: true}, nil
			},
		},
	}

	observer := &observerRecorder{}
	dispatcher := NewDispatcher(manager, discardLogger(), WithDispatcherObserver(observer))
	require.NoError(t, dispatcher.RefreshSubscriptions(context.Background()))

	server := &domain.Server{ID: 1, Name: "cs"}
	dispatcher.DispatchServerEvent(context.Background(), proto.EventType_EVENT_TYPE_SERVER_POST_START, server, nil)
	result := dispatcher.DispatchServerEvent(context.Background(), proto.EventType_EVENT_TYPE_SERVER_PRE_STOP, server, nil)
	require.True(t, result.Cancelled)

	_, _, events := observer.snapshot()
	assert.Equal(t, []string{
		"EVENT_TYPE_SERVER_POST_START:handled",
		"EVENT_TYPE_SERVER_PRE_STOP:cancelled",
	}, events)
}

func TestBuildEventContext_carries_the_initiating_user(t *testing.T) {
	t.Parallel()

	server := &domain.Server{ID: 5, Name: "cs"}

	anonymous := buildServerEvent(context.Background(), proto.EventType_EVENT_TYPE_SERVER_POST_START, server, nil)
	require.NotNil(t, anonymous.Context)
	assert.NotEmpty(t, anonymous.Context.RequestId)
	assert.Nil(t, anonymous.Context.UserId, "no session, no user")

	ctx := auth.ContextWithSession(context.Background(), &auth.Session{User: &domain.User{ID: 42, Login: "admin"}})

	byUser := buildServerEvent(ctx, proto.EventType_EVENT_TYPE_SERVER_POST_START, server, nil)
	require.NotNil(t, byUser.Context.UserId)
	assert.Equal(t, uint64(42), *byUser.Context.UserId)

	task := buildTaskEvent(ctx, proto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED, 1, 2, nil, "gsstart", "waiting", nil)
	require.NotNil(t, task.Context.UserId)
	assert.Equal(t, uint64(42), *task.Context.UserId)
}

func TestHTTPHandler_buildProtoRequest_carries_the_user(t *testing.T) {
	t.Parallel()

	handler := NewHTTPHandler(NewManager(ManagerConfig{}), &mockMiddleware{}, &mockMiddleware{})

	anonymous := httptest.NewRequest(http.MethodGet, "/api/plugins/p/status", nil)
	req, err := handler.buildProtoRequest(anonymous, "p", "/status", nil)
	require.NoError(t, err)
	assert.Nil(t, req.Context.UserId)
	assert.Nil(t, req.Session)

	ctx := auth.ContextWithSession(context.Background(), &auth.Session{User: &domain.User{ID: 9, Login: "user"}})
	authenticated := httptest.NewRequest(http.MethodGet, "/api/plugins/p/status", nil).WithContext(ctx)
	req, err = handler.buildProtoRequest(authenticated, "p", "/status", nil)
	require.NoError(t, err)
	require.NotNil(t, req.Context.UserId)
	assert.Equal(t, uint64(9), *req.Context.UserId)
	require.NotNil(t, req.Session)
	assert.Equal(t, uint64(9), req.Session.User.Id)
}

func TestDispatcher_Dispatch_revalidates_the_gate_per_delivery(t *testing.T) {
	t.Parallel()

	delivered := make([]string, 0)
	var mu sync.Mutex

	newPlugin := func(id string, dbID uint64) *LoadedPlugin {
		return &LoadedPlugin{
			Info:    &proto.PluginInfo{Id: id},
			Enabled: true,
			DBID:    dbID,
			Instance: &mockPluginService{
				getSubscribedEventsFunc: func(_ context.Context, _ *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
					return &proto.GetSubscribedEventsResponse{
						Events: []proto.EventType{proto.EventType_EVENT_TYPE_SERVER_POST_START},
					}, nil
				},
				handleEventFunc: func(_ context.Context, _ *proto.Event) (*proto.EventResult, error) {
					mu.Lock()
					defer mu.Unlock()
					delivered = append(delivered, id)

					return &proto.EventResult{Handled: true}, nil
				},
			},
		}
	}

	manager := newDispatcherTestManager()
	manager.plugins["keeps"] = newPlugin("keeps", 1)
	manager.plugins["revoked"] = newPlugin("revoked", 2)

	// The grant of plugin 2 is withdrawn after the subscription map was
	// built, as a revocation on another panel instance does.
	revoked := false
	observer := &observerRecorder{}
	dispatcher := NewDispatcher(manager, discardLogger(),
		WithDispatcherObserver(observer),
		WithSubscriptionGate(func(_ context.Context, plugin *LoadedPlugin) bool {
			return !revoked || plugin.DBID != 2
		}))

	require.NoError(t, dispatcher.RefreshSubscriptions(context.Background()))

	server := &domain.Server{ID: 1, Name: "cs"}
	dispatcher.DispatchServerEvent(context.Background(), proto.EventType_EVENT_TYPE_SERVER_POST_START, server, nil)

	mu.Lock()
	assert.ElementsMatch(t, []string{"keeps", "revoked"}, delivered, "both hold the grant before the revocation")
	delivered = delivered[:0]
	mu.Unlock()

	revoked = true
	dispatcher.DispatchServerEvent(context.Background(), proto.EventType_EVENT_TYPE_SERVER_POST_START, server, nil)

	mu.Lock()
	assert.Equal(t, []string{"keeps"}, delivered,
		"the revoked plugin is skipped without waiting for a subscription refresh")
	mu.Unlock()

	_, _, events := observer.snapshot()
	assert.Contains(t, events, "EVENT_TYPE_SERVER_POST_START:denied")
}
