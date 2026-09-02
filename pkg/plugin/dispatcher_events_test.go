package plugin

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingPlugin subscribes to the given events and keeps every event it
// received.
type recordingPlugin struct {
	mu       sync.Mutex
	received []*proto.Event
	result   *proto.EventResult
}

func (r *recordingPlugin) events() []*proto.Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]*proto.Event(nil), r.received...)
}

func newRecordingPlugin(id string, dbID uint64, result *proto.EventResult, subscribed ...proto.EventType) (*LoadedPlugin, *recordingPlugin) {
	recorder := &recordingPlugin{result: result}

	return &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: id},
		Enabled: true,
		DBID:    dbID,
		Instance: &mockPluginService{
			getSubscribedEventsFunc: func(context.Context, *proto.GetSubscribedEventsRequest) (*proto.GetSubscribedEventsResponse, error) {
				return &proto.GetSubscribedEventsResponse{Events: subscribed}, nil
			},
			handleEventFunc: func(_ context.Context, event *proto.Event) (*proto.EventResult, error) {
				recorder.mu.Lock()
				defer recorder.mu.Unlock()
				recorder.received = append(recorder.received, event)

				if recorder.result != nil {
					return recorder.result, nil
				}

				return &proto.EventResult{Handled: true}, nil
			},
		},
	}, recorder
}

func waitForEvents(t *testing.T, recorder *recordingPlugin, want int) []*proto.Event {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if events := recorder.events(); len(events) >= want {
			return events
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("expected %d events, got %d", want, len(recorder.events()))

	return nil
}

func TestDispatcher_sets_plugin_id_per_subscriber(t *testing.T) {
	t.Parallel()

	manager := newDispatcherTestManager()
	first, firstRecorder := newRecordingPlugin("first", 1, nil, proto.EventType_EVENT_TYPE_USER_CREATED)
	second, secondRecorder := newRecordingPlugin("second", 2, nil, proto.EventType_EVENT_TYPE_USER_CREATED)
	manager.plugins["first"] = first
	manager.plugins["second"] = second

	dispatcher := NewDispatcher(manager, discardLogger())
	require.NoError(t, dispatcher.RefreshSubscriptions(context.Background()))

	user := &domain.User{ID: 5, Login: "alice", Email: "alice@example.com", Name: new("Alice")}
	result := dispatcher.DispatchUserEvent(context.Background(), proto.EventType_EVENT_TYPE_USER_CREATED, user,
		map[string]string{"source": "test"})
	require.False(t, result.Cancelled)
	assert.ElementsMatch(t, []string{"first", "second"}, result.HandledBy)

	firstEvents := firstRecorder.events()
	secondEvents := secondRecorder.events()
	require.Len(t, firstEvents, 1)
	require.Len(t, secondEvents, 1)

	assert.Equal(t, "first", firstEvents[0].Context.PluginId)
	assert.Equal(t, "second", secondEvents[0].Context.PluginId)
	assert.Equal(t, firstEvents[0].Context.RequestId, secondEvents[0].Context.RequestId, "one delivery, one request id")
	assert.Empty(t, firstEvents[0].Context.Permissions, "reserved: grants are read through gameap-host")

	payload := firstEvents[0].GetUserEvent()
	require.NotNil(t, payload)
	assert.Equal(t, uint64(5), payload.User.Id)
	assert.Equal(t, "alice", payload.User.Login)
	assert.Equal(t, "Alice", payload.User.GetName())
	assert.Equal(t, map[string]string{"source": "test"}, payload.ExtraData)
	assert.Same(t, payload, secondEvents[0].GetUserEvent(), "the payload is shared between subscribers")
}

func TestDispatcher_user_pre_delete_can_be_cancelled(t *testing.T) {
	t.Parallel()

	manager := newDispatcherTestManager()
	blocker, _ := newRecordingPlugin("blocker", 1, &proto.EventResult{
		Handled: true, ShouldCancel: true, Message: new("user is billed"),
	}, proto.EventType_EVENT_TYPE_USER_PRE_DELETE)
	manager.plugins["blocker"] = blocker

	dispatcher := NewDispatcher(manager, discardLogger())
	require.NoError(t, dispatcher.RefreshSubscriptions(context.Background()))

	result := dispatcher.DispatchUserEvent(context.Background(), proto.EventType_EVENT_TYPE_USER_PRE_DELETE,
		&domain.User{ID: 5}, nil)
	assert.True(t, result.Cancelled)
	assert.Equal(t, "blocker", result.CancelledBy)
	assert.Equal(t, "user is billed", result.CancelMessage)

	result = dispatcher.DispatchNodeEvent(context.Background(), proto.EventType_EVENT_TYPE_NODE_PRE_DELETE,
		&domain.Node{ID: 3}, nil)
	assert.False(t, result.Cancelled, "no subscriber for node events")
}

func TestDispatcher_node_and_settings_events_deliver_payloads(t *testing.T) {
	t.Parallel()

	manager := newDispatcherTestManager()
	plugin, recorder := newRecordingPlugin("watcher", 1, nil,
		proto.EventType_EVENT_TYPE_NODE_ONLINE, proto.EventType_EVENT_TYPE_SERVER_SETTINGS_CHANGED)
	manager.plugins["watcher"] = plugin

	dispatcher := NewDispatcher(manager, discardLogger())
	require.NoError(t, dispatcher.RefreshSubscriptions(context.Background()))

	dispatcher.DispatchNodeEventAsync(context.Background(), proto.EventType_EVENT_TYPE_NODE_ONLINE,
		&domain.Node{ID: 3, Name: "node-3", OS: domain.NodeOSLinux, WorkPath: "/srv", GdaemonAPIKey: "top-secret"},
		map[string]string{"daemon_version": "4.4.0"})

	events := waitForEvents(t, recorder, 1)
	node := events[0].GetNodeEvent()
	require.NotNil(t, node)
	assert.Equal(t, uint64(3), node.Node.Id)
	assert.Equal(t, "node-3", node.Node.Name)
	assert.Equal(t, "4.4.0", node.ExtraData["daemon_version"])
	encoded, err := node.MarshalVT()
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "top-secret", "daemon credentials never reach plugins")

	dispatcher.DispatchServerSettingsEventAsync(context.Background(), 9,
		[]domain.ServerSetting{{ID: 1, ServerID: 9, Name: "autostart", Value: domain.NewServerSettingValue("on")}},
		map[string]string{"changed_fields": "autostart"})

	events = waitForEvents(t, recorder, 2)
	settings := events[1].GetServerSettingsEvent()
	require.NotNil(t, settings)
	assert.Equal(t, uint64(9), settings.ServerId)
	require.Len(t, settings.Settings, 1)
	assert.Equal(t, "autostart", settings.Settings[0].Name)
	assert.Equal(t, "on", settings.Settings[0].Value)
}

func TestDispatcher_plugin_events_skip_their_subject(t *testing.T) {
	t.Parallel()

	manager := newDispatcherTestManager()
	subject, subjectRecorder := newRecordingPlugin("subject", 42, nil, proto.EventType_EVENT_TYPE_PLUGIN_LOADED)
	observer, observerRecorder := newRecordingPlugin("observer", 43, nil, proto.EventType_EVENT_TYPE_PLUGIN_LOADED)
	manager.plugins["subject"] = subject
	manager.plugins["observer"] = observer

	dispatcher := NewDispatcher(manager, discardLogger())
	require.NoError(t, dispatcher.RefreshSubscriptions(context.Background()))

	dispatcher.DispatchPluginEventAsync(context.Background(), proto.EventType_EVENT_TYPE_PLUGIN_LOADED,
		EventInfo{DBID: 42, Name: "Subject", Version: "1.0.0", Status: "active"},
		map[string]string{"trigger": "reload"})

	events := waitForEvents(t, observerRecorder, 1)
	payload := events[0].GetPluginEvent()
	require.NotNil(t, payload)
	assert.Equal(t, CompactPluginID(42), payload.PluginId)
	assert.Equal(t, "Subject", payload.Name)
	assert.Equal(t, "active", payload.Status)
	assert.Equal(t, "reload", payload.ExtraData["trigger"])

	time.Sleep(20 * time.Millisecond)
	assert.Empty(t, subjectRecorder.events(), "a plugin never hears about its own lifecycle")
}

func TestIsPluginEventSubject_matches_declared_id(t *testing.T) {
	t.Parallel()

	event := buildPluginEvent(context.Background(), proto.EventType_EVENT_TYPE_PLUGIN_ERROR,
		EventInfo{DBID: 42}, nil)

	assert.True(t, isPluginEventSubject(&LoadedPlugin{Info: &proto.PluginInfo{Id: CompactPluginID(42)}}, event))
	assert.True(t, isPluginEventSubject(&LoadedPlugin{DBID: 42}, event))
	assert.False(t, isPluginEventSubject(&LoadedPlugin{DBID: 7, Info: &proto.PluginInfo{Id: "other"}}, event))
	assert.False(t, isPluginEventSubject(&LoadedPlugin{DBID: 42}, buildUserEvent(context.Background(),
		proto.EventType_EVENT_TYPE_USER_CREATED, &domain.User{}, nil)))
}

func TestWithSubscriberContext_tolerates_missing_context(t *testing.T) {
	t.Parallel()

	event := &proto.Event{Type: proto.EventType_EVENT_TYPE_SERVER_POST_START}
	copied := withSubscriberContext(event, &LoadedPlugin{Info: &proto.PluginInfo{Id: "p"}})

	require.NotNil(t, copied.Context)
	assert.Equal(t, "p", copied.Context.PluginId)
	assert.Nil(t, event.Context, "the original is untouched")

	assert.Same(t, event, withSubscriberContext(event, &LoadedPlugin{}), "a plugin without info gets the event as is")
}

func TestIsCancellableEvent_new_pre_events(t *testing.T) {
	t.Parallel()

	assert.True(t, isCancellableEvent(proto.EventType_EVENT_TYPE_USER_PRE_DELETE))
	assert.True(t, isCancellableEvent(proto.EventType_EVENT_TYPE_NODE_PRE_DELETE))
	assert.False(t, isCancellableEvent(proto.EventType_EVENT_TYPE_USER_DELETED))
	assert.False(t, isCancellableEvent(proto.EventType_EVENT_TYPE_NODE_OFFLINE))
	assert.False(t, isCancellableEvent(proto.EventType_EVENT_TYPE_PLUGIN_ERROR))
}

func TestDomainUserToProto_carries_identity_only(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	user := DomainUserToProto(&domain.User{
		ID: 5, Login: "alice", Email: "a@example.com", Name: new("Alice"), Password: "hash", CreatedAt: &now,
	})
	assert.Equal(t, uint64(5), user.Id)
	assert.Equal(t, int64(1_700_000_000), user.GetCreatedAt())

	encoded, err := user.MarshalVT()
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "hash")

	assert.Nil(t, DomainUserToProto(nil))
	assert.Nil(t, DomainNodeToProto(nil))
}

func TestDispatcher_DispatchUserEventAsync_delivers_the_user_payload(t *testing.T) {
	t.Parallel()

	// ARRANGE
	manager := newDispatcherTestManager()
	plugin, recorder := newRecordingPlugin("watcher", 1, nil, proto.EventType_EVENT_TYPE_USER_UPDATED)
	manager.plugins["watcher"] = plugin

	dispatcher := NewDispatcher(manager, discardLogger())
	require.NoError(t, dispatcher.RefreshSubscriptions(context.Background()))

	// ACT
	dispatcher.DispatchUserEventAsync(context.Background(), proto.EventType_EVENT_TYPE_USER_UPDATED,
		&domain.User{
			ID:                     5,
			Login:                  "alice",
			Email:                  "alice@example.com",
			Name:                   new("Alice"),
			Password:               "$2y$10$hashed-secret",
			TwoFactorSecret:        new("totp-ciphertext"),
			TwoFactorRecoveryCodes: new("recovery-codes"),
		},
		map[string]string{"source": "profile", "changed_fields": "email"})

	// ASSERT
	events := waitForEvents(t, recorder, 1)
	payload := events[0].GetUserEvent()
	require.NotNil(t, payload)
	require.NotNil(t, payload.User)
	assert.Equal(t, uint64(5), payload.User.Id)
	assert.Equal(t, "alice", payload.User.Login)
	assert.Equal(t, "alice@example.com", payload.User.Email)
	assert.Equal(t, "Alice", payload.User.GetName())
	assert.Equal(t, "profile", payload.ExtraData["source"])
	assert.Equal(t, "email", payload.ExtraData["changed_fields"])
	assert.Equal(t, "watcher", events[0].Context.PluginId, "each subscriber sees its own id")

	encoded, err := payload.MarshalVT()
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "hashed-secret", "credentials never reach plugins")
	assert.NotContains(t, string(encoded), "totp-ciphertext", "2FA material never reaches plugins")
	assert.NotContains(t, string(encoded), "recovery-codes", "recovery codes never reach plugins")
}

func TestDispatcher_DispatchUserEventAsync_is_a_noop_on_a_nil_dispatcher(t *testing.T) {
	t.Parallel()

	// ARRANGE
	var dispatcher *Dispatcher

	// ACT + ASSERT
	assert.NotPanics(t, func() {
		dispatcher.DispatchUserEventAsync(context.Background(), proto.EventType_EVENT_TYPE_USER_CREATED,
			&domain.User{ID: 5, Login: "alice"}, nil)
	}, "plugins disabled means the event goes nowhere, not a crash")
}

func TestDispatcher_AsyncBacklog_tracks_in_flight_deliveries(t *testing.T) {
	t.Parallel()

	// ARRANGE
	entered := make(chan struct{})
	release := make(chan struct{})

	manager := newDispatcherTestManager()
	plugin := &LoadedPlugin{
		Info:    &proto.PluginInfo{Id: "slow"},
		Enabled: true,
		Instance: &mockPluginService{
			handleEventFunc: func(context.Context, *proto.Event) (*proto.EventResult, error) {
				close(entered)
				<-release

				return &proto.EventResult{Handled: true}, nil
			},
		},
	}

	dispatcher := NewDispatcher(manager, discardLogger())
	dispatcher.subscriptions[proto.EventType_EVENT_TYPE_USER_CREATED] = []*LoadedPlugin{plugin}
	require.Equal(t, 0, dispatcher.AsyncBacklog(), "an idle dispatcher holds no slots")

	// ACT
	dispatcher.DispatchUserEventAsync(context.Background(), proto.EventType_EVENT_TYPE_USER_CREATED,
		&domain.User{ID: 5, Login: "alice"}, nil)

	// ASSERT
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("async delivery never started")
	}

	assert.Equal(t, 1, dispatcher.AsyncBacklog(), "a delivery in flight occupies one of the bounded slots")

	close(release)

	assert.Eventually(t, func() bool { return dispatcher.AsyncBacklog() == 0 },
		5*time.Second, 5*time.Millisecond, "the slot is handed back once the delivery finishes")
}
