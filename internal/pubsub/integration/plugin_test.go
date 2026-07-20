package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errBoom       = errors.New("boom")
	errServerDown = errors.New("server-down")
	errTaskDown   = errors.New("task-down")
)

// recorder collects messages observed on a channel; used for state-based
// assertions on what reached subscribers via the in-memory pubsub.
type recorder struct {
	mu   sync.Mutex
	msgs []*pubsub.Message
}

func (r *recorder) record(_ context.Context, m *pubsub.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, m)

	return nil
}

func (r *recorder) snapshot() []*pubsub.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*pubsub.Message, len(r.msgs))
	copy(out, r.msgs)

	return out
}

// setupPubsub starts an in-memory pubsub bus and returns it along with a
// teardown that closes the bus and cancels the Start context.
func setupPubsub(t *testing.T) (*memory.Memory, context.Context) {
	t.Helper()

	bus := memory.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = bus.Start(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		<-done
		_ = bus.Close()
	})

	return bus, ctx
}

// subscribeRecorder subscribes a fresh recorder to the given channel pattern
// and returns it.
func subscribeRecorder(ctx context.Context, t *testing.T, bus *memory.Memory, pattern string) *recorder {
	t.Helper()

	rec := &recorder{}
	require.NoError(t, bus.Subscribe(ctx, pattern, rec.record))

	return rec
}

func TestPublishEvent_MapsServerEvent(t *testing.T) {
	// ARRANGE
	bus, ctx := setupPubsub(t)
	rec := subscribeRecorder(ctx, t, bus, channels.PluginServerEvents)

	publisher := NewPluginEventPublisher(bus)

	event := &pluginproto.Event{
		Type: pluginproto.EventType_EVENT_TYPE_SERVER_POST_START,
		Payload: &pluginproto.Event_ServerEvent{
			ServerEvent: &pluginproto.ServerEventPayload{
				Server:    &proto.Server{Id: 7},
				ExtraData: map[string]string{"k": "v"},
			},
		},
	}

	// ACT
	require.NoError(t, publisher.PublishEvent(ctx, event))

	// ASSERT
	got := rec.snapshot()
	require.Len(t, got, 1, "exactly one message must be delivered to PluginServerEvents")

	msg := got[0]
	assert.Equal(t, channels.PluginServerEvents, msg.Channel, "message channel must be PluginServerEvents")
	assert.Equal(t, messages.TypePluginEvent, msg.Type)

	payload, err := messages.ParsePayload[messages.PluginEventPayload](msg)
	require.NoError(t, err)
	assert.Equal(t, int32(pluginproto.EventType_EVENT_TYPE_SERVER_POST_START), payload.EventType)
	require.NotNil(t, payload.ServerID, "ServerID must be populated for server events")
	assert.Equal(t, uint(7), *payload.ServerID)
	assert.Nil(t, payload.TaskID, "TaskID must remain nil for non-task events")
	assert.Equal(t, "v", payload.ExtraData["k"], "ExtraData must be forwarded verbatim")
}

func TestPublishEvent_MapsTaskEvent_WithServerID(t *testing.T) {
	// ARRANGE
	bus, ctx := setupPubsub(t)
	rec := subscribeRecorder(ctx, t, bus, channels.PluginServerEvents)

	publisher := NewPluginEventPublisher(bus)

	serverID := uint64(33)
	event := &pluginproto.Event{
		Type: pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_COMPLETED,
		Payload: &pluginproto.Event_TaskEvent{
			TaskEvent: &pluginproto.TaskEventPayload{
				TaskId:    11,
				NodeId:    22,
				ServerId:  &serverID,
				ExtraData: map[string]string{"a": "b"},
			},
		},
	}

	// ACT
	require.NoError(t, publisher.PublishEvent(ctx, event))

	// ASSERT
	// When a TaskEvent carries a server_id, the publisher routes the message to
	// PluginServerEvents (server channel takes precedence over task channel).
	got := rec.snapshot()
	require.Len(t, got, 1, "task event with server_id must publish on PluginServerEvents")

	msg := got[0]
	assert.Equal(t, channels.PluginServerEvents, msg.Channel)

	payload, err := messages.ParsePayload[messages.PluginEventPayload](msg)
	require.NoError(t, err)
	require.NotNil(t, payload.TaskID)
	assert.Equal(t, uint(11), *payload.TaskID)
	require.NotNil(t, payload.NodeID)
	assert.Equal(t, uint(22), *payload.NodeID)
	require.NotNil(t, payload.ServerID, "ServerID must be carried over from TaskEvent.ServerId")
	assert.Equal(t, uint(33), *payload.ServerID)
	assert.Equal(t, "b", payload.ExtraData["a"])
}

func TestPublishEvent_MapsTaskEvent_WithoutServerID(t *testing.T) {
	// ARRANGE
	bus, ctx := setupPubsub(t)
	rec := subscribeRecorder(ctx, t, bus, channels.PluginTaskEvents)

	publisher := NewPluginEventPublisher(bus)

	event := &pluginproto.Event{
		Type: pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED,
		Payload: &pluginproto.Event_TaskEvent{
			TaskEvent: &pluginproto.TaskEventPayload{
				TaskId:    11,
				NodeId:    22,
				ServerId:  nil,
				ExtraData: map[string]string{"x": "y"},
			},
		},
	}

	// ACT
	require.NoError(t, publisher.PublishEvent(ctx, event))

	// ASSERT
	got := rec.snapshot()
	require.Len(t, got, 1, "task event without server_id must publish on PluginTaskEvents")

	msg := got[0]
	assert.Equal(t, channels.PluginTaskEvents, msg.Channel)

	payload, err := messages.ParsePayload[messages.PluginEventPayload](msg)
	require.NoError(t, err)
	assert.Nil(t, payload.ServerID, "ServerID must remain nil when TaskEvent.ServerId is nil")
	require.NotNil(t, payload.TaskID)
	assert.Equal(t, uint(11), *payload.TaskID)
	require.NotNil(t, payload.NodeID)
	assert.Equal(t, uint(22), *payload.NodeID)
}

func TestPublishEvent_GenericEvent(t *testing.T) {
	// ARRANGE
	bus, ctx := setupPubsub(t)
	rec := subscribeRecorder(ctx, t, bus, channels.PluginEvents)

	publisher := NewPluginEventPublisher(bus)

	// Event with neither Server nor Task body populated.
	event := &pluginproto.Event{
		Type: pluginproto.EventType_EVENT_TYPE_UNSPECIFIED,
	}

	// ACT
	require.NoError(t, publisher.PublishEvent(ctx, event))

	// ASSERT
	got := rec.snapshot()
	require.Len(t, got, 1, "generic event must reach the PluginEvents channel")

	msg := got[0]
	assert.Equal(t, channels.PluginEvents, msg.Channel)

	payload, err := messages.ParsePayload[messages.PluginEventPayload](msg)
	require.NoError(t, err)
	assert.Nil(t, payload.ServerID, "generic event must not populate ServerID")
	assert.Nil(t, payload.TaskID, "generic event must not populate TaskID")
}

func TestPublishEvent_ServerEvent_NilServer_FallsBackToGenericChannel(t *testing.T) {
	// ARRANGE
	// A ServerEvent with Server == nil currently does not set ServerID, so the
	// publisher falls back to the generic PluginEvents channel. Pinning this
	// behavior protects callers from a silent re-route should the guard change.
	bus, ctx := setupPubsub(t)
	rec := subscribeRecorder(ctx, t, bus, channels.PluginEvents)

	publisher := NewPluginEventPublisher(bus)

	event := &pluginproto.Event{
		Type: pluginproto.EventType_EVENT_TYPE_SERVER_PRE_START,
		Payload: &pluginproto.Event_ServerEvent{
			ServerEvent: &pluginproto.ServerEventPayload{
				Server: nil,
			},
		},
	}

	// ACT
	require.NoError(t, publisher.PublishEvent(ctx, event))

	// ASSERT
	got := rec.snapshot()
	require.Len(t, got, 1, "ServerEvent with nil Server must still publish on PluginEvents")
	assert.Equal(t, channels.PluginEvents, got[0].Channel)

	payload, err := messages.ParsePayload[messages.PluginEventPayload](got[0])
	require.NoError(t, err)
	assert.Nil(t, payload.ServerID)
}

func TestPublishServerEvent_BuildsCorrectChannel(t *testing.T) {
	// ARRANGE
	bus, ctx := setupPubsub(t)
	rec := subscribeRecorder(ctx, t, bus, channels.PluginServerEvents)

	publisher := NewPluginEventPublisher(bus)

	// ACT
	require.NoError(t, publisher.PublishServerEvent(
		ctx,
		pluginproto.EventType_EVENT_TYPE_SERVER_POST_STOP,
		7,
		map[string]string{"x": "y"},
	))

	// ASSERT
	got := rec.snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, channels.PluginServerEvents, got[0].Channel)
	assert.Equal(t, messages.TypePluginEvent, got[0].Type)

	payload, err := messages.ParsePayload[messages.PluginEventPayload](got[0])
	require.NoError(t, err)
	assert.Equal(t, int32(pluginproto.EventType_EVENT_TYPE_SERVER_POST_STOP), payload.EventType)
	require.NotNil(t, payload.ServerID)
	assert.Equal(t, uint(7), *payload.ServerID)
	assert.Equal(t, "y", payload.ExtraData["x"])
	assert.Nil(t, payload.TaskID, "PublishServerEvent must not populate TaskID")
}

func TestPublishTaskEvent_BuildsCorrectChannel(t *testing.T) {
	// ARRANGE
	bus, ctx := setupPubsub(t)
	rec := subscribeRecorder(ctx, t, bus, channels.PluginTaskEvents)

	publisher := NewPluginEventPublisher(bus)

	serverID := uint(99)

	// ACT
	require.NoError(t, publisher.PublishTaskEvent(
		ctx,
		pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_COMPLETED,
		11,
		22,
		&serverID,
		map[string]string{"a": "b"},
	))

	// ASSERT
	got := rec.snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, channels.PluginTaskEvents, got[0].Channel)
	assert.Equal(t, messages.TypePluginEvent, got[0].Type)

	payload, err := messages.ParsePayload[messages.PluginEventPayload](got[0])
	require.NoError(t, err)
	assert.Equal(t, int32(pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_COMPLETED), payload.EventType)
	require.NotNil(t, payload.TaskID)
	assert.Equal(t, uint(11), *payload.TaskID)
	require.NotNil(t, payload.NodeID)
	assert.Equal(t, uint(22), *payload.NodeID)
	require.NotNil(t, payload.ServerID)
	assert.Equal(t, uint(99), *payload.ServerID)
	assert.Equal(t, "b", payload.ExtraData["a"])
}

func TestPublishTaskEvent_NilServerID(t *testing.T) {
	// ARRANGE
	bus, ctx := setupPubsub(t)
	rec := subscribeRecorder(ctx, t, bus, channels.PluginTaskEvents)

	publisher := NewPluginEventPublisher(bus)

	// ACT
	require.NoError(t, publisher.PublishTaskEvent(
		ctx,
		pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED,
		11,
		22,
		nil,
		nil,
	))

	// ASSERT
	got := rec.snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, channels.PluginTaskEvents, got[0].Channel)

	payload, err := messages.ParsePayload[messages.PluginEventPayload](got[0])
	require.NoError(t, err)
	assert.Nil(t, payload.ServerID, "nil server_id arg must surface as nil ServerID")
}

// errorPublisher is a tiny fake satisfying pubsub.Publisher whose Publish
// always returns a sentinel error. Used to assert error propagation from
// PluginEventPublisher.
type errorPublisher struct {
	err error
}

func (e *errorPublisher) Publish(_ context.Context, _ string, _ *pubsub.Message) error {
	return e.err
}

func TestPublishEvent_PubsubError_Propagates(t *testing.T) {
	// ARRANGE
	publisher := NewPluginEventPublisher(&errorPublisher{err: errBoom})

	event := &pluginproto.Event{
		Type: pluginproto.EventType_EVENT_TYPE_SERVER_POST_START,
		Payload: &pluginproto.Event_ServerEvent{
			ServerEvent: &pluginproto.ServerEventPayload{
				Server: &proto.Server{Id: 1},
			},
		},
	}

	// ACT
	err := publisher.PublishEvent(context.Background(), event)

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom, "errors from the underlying Publisher must propagate unchanged")
}

func TestPublishServerEvent_PubsubError_Propagates(t *testing.T) {
	// ARRANGE
	publisher := NewPluginEventPublisher(&errorPublisher{err: errServerDown})

	// ACT
	err := publisher.PublishServerEvent(
		context.Background(),
		pluginproto.EventType_EVENT_TYPE_SERVER_POST_START,
		1,
		nil,
	)

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, errServerDown)
}

func TestPublishTaskEvent_PubsubError_Propagates(t *testing.T) {
	// ARRANGE
	publisher := NewPluginEventPublisher(&errorPublisher{err: errTaskDown})

	// ACT
	err := publisher.PublishTaskEvent(
		context.Background(),
		pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_CREATED,
		1,
		2,
		nil,
		nil,
	)

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, errTaskDown)
}

func TestNewPluginEventPublisher_AssignsLogger(t *testing.T) {
	// ARRANGE
	bus := memory.New()
	t.Cleanup(func() { _ = bus.Close() })

	// ACT
	publisher := NewPluginEventPublisher(bus)

	// ASSERT
	require.NotNil(t, publisher)
	assert.NotNil(t, publisher.logger, "constructor must wire a non-nil default logger")
	assert.Equal(t, pubsub.Publisher(bus), publisher.pubsub, "publisher must hold the supplied bus")
}
