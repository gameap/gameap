package session

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

const testInstanceID = "instance-a"

var (
	errTestTransportClosed = errors.New("transport closed")
	errTestSendBroken      = errors.New("send broken")
	errTestStreamBroken    = errors.New("stream broken")
)

func quietLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// setupRegistry returns a Registry wired to a fresh in-memory pubsub bus.
// If start is true, Start(ctx) is called so the registry handles incoming
// pubsub messages.
func setupRegistry(t *testing.T, start bool) (*Registry, *memory.Memory, context.Context) {
	t.Helper()

	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	r := NewRegistry(ps, testInstanceID, quietLogger())
	if start {
		require.NoError(t, r.Start(ctx))
	}

	return r, ps, ctx
}

// waitFor polls cond every 5ms, failing the test after one second.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", msg)
}

// publishSessionEvent publishes a remote session connected/closed event so the
// registry can populate or remove its globalNodes map.
func publishSessionEvent(
	ctx context.Context,
	t *testing.T,
	ps pubsub.Publisher,
	channel, msgType string,
	nodeID uint64,
	instanceID string,
) {
	t.Helper()

	msg, err := messages.NewMessage(channel, msgType, messages.DaemonSessionPayload{
		NodeID:      nodeID,
		InstanceID:  instanceID,
		Version:     "remote",
		ConnectedAt: time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, ps.Publish(ctx, channel, msg))
}

// newTestSession creates a Session attached to a stub stream with an
// optional cancel-recording function.
func newTestSession(nodeID uint64, cancel context.CancelFunc) (*Session, *stubStream) {
	stream := newStubStream(context.Background())
	s := NewSession(nodeID, stream, "v1.0", []string{"core"}, cancel)

	return s, stream
}

func TestNewRegistry_nilLoggerFallsBackToDefault(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	// ACT
	r := NewRegistry(ps, testInstanceID, nil)

	// ASSERT
	require.NotNil(t, r)
	assert.NotNil(t, r.logger, "logger must be initialised when nil is passed")
	assert.Equal(t, testInstanceID, r.InstanceID())
}

func TestRegistry_Register_addsLocalSession(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, _ := newTestSession(7, nil)

	// ACT
	require.NoError(t, r.Register(ctx, s))

	// ASSERT
	got, ok := r.GetSession(7)
	require.True(t, ok, "GetSession must return registered session")
	assert.Same(t, s, got, "returned session must be the same instance")
	assert.True(t, r.IsConnected(7), "IsConnected must be true for local node")
	assert.True(t, r.IsConnectedAnywhere(7), "IsConnectedAnywhere must be true for local node")
	assert.Equal(t, 1, r.SessionCount(), "session count must be 1 after one Register")
}

func TestRegistry_Register_replacesExistingSessionForSameNode(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)

	var aCancels atomic.Int32
	sessionA, _ := newTestSession(11, func() { aCancels.Add(1) })
	require.NoError(t, r.Register(ctx, sessionA))

	sessionB, _ := newTestSession(11, nil)

	// ACT
	require.NoError(t, r.Register(ctx, sessionB))

	// ASSERT
	assert.Equal(t, int32(1), aCancels.Load(),
		"old session's cancel must be invoked exactly once")

	got, ok := r.GetSession(11)
	require.True(t, ok, "node 11 must still resolve after replacement")
	assert.Same(t, sessionB, got, "stored session must be the new one")
	assert.Equal(t, 1, r.SessionCount(), "session count must remain 1 after replacement")
}

func TestRegistry_Register_publishesConnectedEvent(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, false)

	var (
		received   *pubsub.Message
		receivedMu sync.Mutex
		done       = make(chan struct{})
	)

	require.NoError(t, ps.Subscribe(ctx, channels.DaemonSessionConnected,
		func(_ context.Context, msg *pubsub.Message) error {
			receivedMu.Lock()
			received = msg
			receivedMu.Unlock()
			close(done)

			return nil
		}))

	s, _ := newTestSession(99, nil)

	// ACT
	require.NoError(t, r.Register(ctx, s))

	// ASSERT
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session connected event")
	}

	receivedMu.Lock()
	defer receivedMu.Unlock()

	require.NotNil(t, received)
	assert.Equal(t, channels.DaemonSessionConnected, received.Channel)
	assert.Equal(t, messages.TypeDaemonConnected, received.Type)

	payload, err := messages.ParsePayload[messages.DaemonSessionPayload](received)
	require.NoError(t, err)
	assert.Equal(t, uint64(99), payload.NodeID)
	assert.Equal(t, testInstanceID, payload.InstanceID)
	assert.Equal(t, "v1.0", payload.Version)
}

func TestRegistry_Unregister_removesAndPublishesDisconnectedEvent(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, false)
	s, _ := newTestSession(50, nil)
	require.NoError(t, r.Register(ctx, s))

	var (
		received   *pubsub.Message
		receivedMu sync.Mutex
		done       = make(chan struct{})
	)
	require.NoError(t, ps.Subscribe(ctx, channels.DaemonSessionClosed,
		func(_ context.Context, msg *pubsub.Message) error {
			receivedMu.Lock()
			received = msg
			receivedMu.Unlock()
			close(done)

			return nil
		}))

	// ACT
	require.NoError(t, r.Unregister(ctx, 50))

	// ASSERT
	_, ok := r.GetSession(50)
	assert.False(t, ok, "session must be removed after Unregister")
	assert.Equal(t, 0, r.SessionCount())

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session closed event")
	}

	receivedMu.Lock()
	defer receivedMu.Unlock()

	require.NotNil(t, received)
	assert.Equal(t, messages.TypeDaemonClosed, received.Type)
	payload, err := messages.ParsePayload[messages.DaemonSessionPayload](received)
	require.NoError(t, err)
	assert.Equal(t, uint64(50), payload.NodeID)
	assert.Equal(t, testInstanceID, payload.InstanceID)
}

func TestRegistry_UnregisterSession_staleSession_doesNotEvictReconnectedSession(t *testing.T) {
	t.Parallel()
	// ARRANGE: a daemon connects (s1), then reconnects (s2) under the same
	// node ID. Register replaces s1 with s2. The old stream's deferred cleanup
	// then runs UnregisterSession(s1); it must NOT evict the live s2.
	r, ps, ctx := setupRegistry(t, false)

	s1, _ := newTestSession(7, nil)
	require.NoError(t, r.Register(ctx, s1))

	s2, _ := newTestSession(7, nil)
	require.NoError(t, r.Register(ctx, s2))

	closedPublished := make(chan struct{}, 1)
	require.NoError(t, ps.Subscribe(ctx, channels.DaemonSessionClosed,
		func(_ context.Context, _ *pubsub.Message) error {
			closedPublished <- struct{}{}

			return nil
		}))

	// ACT: stale cleanup for the superseded session.
	require.NoError(t, r.UnregisterSession(ctx, s1))

	// ASSERT: the live session survives and no closed event is published.
	got, ok := r.GetSession(7)
	require.True(t, ok, "reconnected session must remain registered")
	assert.Same(t, s2, got, "registry must still hold the new session, not the stale one")
	assert.Equal(t, 1, r.SessionCount())

	select {
	case <-closedPublished:
		t.Fatal("stale UnregisterSession must not publish a session closed event")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRegistry_UnregisterSession_ownSession_removesAndPublishes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, false)
	s, _ := newTestSession(8, nil)
	require.NoError(t, r.Register(ctx, s))

	done := make(chan struct{})
	require.NoError(t, ps.Subscribe(ctx, channels.DaemonSessionClosed,
		func(_ context.Context, _ *pubsub.Message) error {
			close(done)

			return nil
		}))

	// ACT
	require.NoError(t, r.UnregisterSession(ctx, s))

	// ASSERT
	_, ok := r.GetSession(8)
	assert.False(t, ok, "own session must be removed")
	assert.Equal(t, 0, r.SessionCount())

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session closed event")
	}
}

func TestRegistry_UnregisterSession_neverRegistered_isNoop(t *testing.T) {
	t.Parallel()
	// ARRANGE: a session whose node was never registered exercises the
	// UnregisterSession `!ok` branch — it must neither panic nor publish a
	// spurious closed event, and must leave registry state untouched.
	r, ps, ctx := setupRegistry(t, false)

	published := make(chan struct{}, 1)
	require.NoError(t, ps.Subscribe(ctx, channels.DaemonSessionClosed,
		func(_ context.Context, _ *pubsub.Message) error {
			published <- struct{}{}

			return nil
		}))

	orphan, _ := newTestSession(12345, nil)

	// ACT
	err := r.UnregisterSession(ctx, orphan)

	// ASSERT
	require.NoError(t, err, "unregistering a never-registered session must be a no-op")
	assert.Equal(t, 0, r.SessionCount(), "session count must stay zero")
	assert.False(t, r.IsConnected(12345), "the orphan node must not become connected")

	select {
	case <-published:
		t.Fatal("must not publish a closed event for a session that was never registered")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRegistry_Unregister_unknownNode_isNoop(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, false)

	published := make(chan struct{}, 1)
	require.NoError(t, ps.Subscribe(ctx, channels.DaemonSessionClosed,
		func(_ context.Context, _ *pubsub.Message) error {
			published <- struct{}{}

			return nil
		}))

	// ACT
	err := r.Unregister(ctx, 999)

	// ASSERT
	require.NoError(t, err, "unregister of unknown node must not fail")
	select {
	case <-published:
		t.Fatal("must not publish disconnect event for unknown node")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRegistry_GetSession_unknown_returnsNil(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, _ := setupRegistry(t, false)

	// ACT
	got, ok := r.GetSession(999)

	// ASSERT
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestRegistry_GetSession_globalRouteOnly_returnsNil(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	// ACT
	publishSessionEvent(ctx, t, ps,
		channels.DaemonSessionConnected, messages.TypeDaemonConnected,
		77, "instance-other")

	waitFor(t, func() bool {
		return r.IsConnectedAnywhere(77)
	}, "remote session to be tracked globally")

	// ASSERT
	got, ok := r.GetSession(77)
	assert.False(t, ok, "remote-only session must not be reported as local")
	assert.Nil(t, got)
	assert.False(t, r.IsConnected(77), "IsConnected must be false for remote-only node")
	assert.True(t, r.IsConnectedAnywhere(77), "IsConnectedAnywhere must be true for remote-only node")
}

func TestRegistry_IsConnected_table(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		setup            func(t *testing.T, r *Registry, ps pubsub.Publisher, ctx context.Context)
		nodeID           uint64
		wantLocal        bool
		wantConnectedAny bool
	}{
		{
			name:             "no_session_returns_false",
			setup:            func(_ *testing.T, _ *Registry, _ pubsub.Publisher, _ context.Context) {},
			nodeID:           1,
			wantLocal:        false,
			wantConnectedAny: false,
		},
		{
			name: "local_session_returns_true_for_both",
			setup: func(t *testing.T, r *Registry, _ pubsub.Publisher, ctx context.Context) {
				t.Helper()
				s, _ := newTestSession(1, nil)
				require.NoError(t, r.Register(ctx, s))
			},
			nodeID:           1,
			wantLocal:        true,
			wantConnectedAny: true,
		},
		{
			name: "remote_session_returns_true_only_for_anywhere",
			setup: func(t *testing.T, r *Registry, ps pubsub.Publisher, ctx context.Context) {
				t.Helper()
				publishSessionEvent(ctx, t, ps,
					channels.DaemonSessionConnected, messages.TypeDaemonConnected,
					1, "instance-other")
				waitFor(t, func() bool {
					return r.IsConnectedAnywhere(1)
				}, "remote session tracked")
			},
			nodeID:           1,
			wantLocal:        false,
			wantConnectedAny: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			r, ps, ctx := setupRegistry(t, true)
			tt.setup(t, r, ps, ctx)

			// ACT + ASSERT
			assert.Equal(t, tt.wantLocal, r.IsConnected(tt.nodeID), "IsConnected mismatch")
			assert.Equal(t, tt.wantConnectedAny, r.IsConnectedAnywhere(tt.nodeID),
				"IsConnectedAnywhere mismatch")
		})
	}
}

func TestRegistry_HasCapability(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, _ := newTestSession(5, nil)
	require.NoError(t, r.Register(ctx, s))

	// ACT + ASSERT
	assert.True(t, r.HasCapability(5, "core"), "registered cap must be reported")
	assert.False(t, r.HasCapability(5, "missing"), "unknown cap must report false")
	assert.False(t, r.HasCapability(999, "core"), "unknown node must report false")
}

func TestRegistry_SendTask_localDirect_callsStream(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, stream := newTestSession(20, nil)
	require.NoError(t, r.Register(ctx, s))

	task := &proto.GatewayMessage{
		RequestId: "task-req-1",
		Payload: &proto.GatewayMessage_Task{
			Task: &proto.DaemonTask{Id: 42},
		},
	}

	// ACT
	require.NoError(t, r.SendTask(ctx, 20, task))

	// ASSERT
	sent := stream.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "task-req-1", sent[0].RequestId)
	require.NotNil(t, sent[0].GetTask())
	assert.Equal(t, uint64(42), sent[0].GetTask().Id)
}

func TestRegistry_SendTask_localStreamErrorIsWrapped(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, stream := newTestSession(20, nil)
	stream.sendErr = errTestTransportClosed
	require.NoError(t, r.Register(ctx, s))

	// ACT
	err := r.SendTask(ctx, 20, &proto.GatewayMessage{})

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send task to local session", "error must be wrapped with context")
	assert.Contains(t, err.Error(), "transport closed", "underlying cause must be preserved")
}

func TestRegistry_SendTask_remoteViaPubSub_publishes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	publishSessionEvent(ctx, t, ps,
		channels.DaemonSessionConnected, messages.TypeDaemonConnected,
		33, "instance-other")
	waitFor(t, func() bool {
		return r.IsConnectedAnywhere(33)
	}, "remote session tracked")

	channel := channels.BuildDaemonTaskDispatchChannel(33)

	var (
		received   *pubsub.Message
		receivedMu sync.Mutex
		done       = make(chan struct{})
	)
	require.NoError(t, ps.Subscribe(ctx, channel, func(_ context.Context, msg *pubsub.Message) error {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		// Skip the registry's own subscriber on DaemonTaskDispatchAll —
		// we want to capture only this listener's view.
		if received == nil {
			received = msg
			close(done)
		}

		return nil
	}))

	gw := &proto.GatewayMessage{
		RequestId: "req-r-1",
		Payload:   &proto.GatewayMessage_Task{Task: &proto.DaemonTask{Id: 7}},
	}

	// ACT
	require.NoError(t, r.SendTask(ctx, 33, gw))

	// ASSERT
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for task dispatch publish")
	}

	receivedMu.Lock()
	defer receivedMu.Unlock()
	require.NotNil(t, received)
	assert.Equal(t, channel, received.Channel)
	assert.Equal(t, messages.TypeDaemonTask, received.Type)

	payload, err := messages.ParsePayload[messages.DaemonTaskDispatchPayload](received)
	require.NoError(t, err)
	assert.Equal(t, uint64(33), payload.NodeID)
	assert.Equal(t, "req-r-1", payload.RequestID)
	assert.Equal(t, uint64(7), payload.TaskID, "task id must be extracted from gateway message")
	assert.NotEmpty(t, payload.TaskData, "marshaled task data must be present")

	var decoded proto.GatewayMessage
	require.NoError(t, decoded.UnmarshalVT(payload.TaskData))
	require.NotNil(t, decoded.GetTask())
	assert.Equal(t, uint64(7), decoded.GetTask().Id)
}

func TestRegistry_SendTask_unknownNode_publishesViaPubSub(t *testing.T) {
	t.Parallel()
	// Sending to an unknown node still publishes — the registry does not
	// gate dispatch on prior knowledge of remote nodes, since global state
	// can lag. This locks in the current contract.

	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	channel := channels.BuildDaemonTaskDispatchChannel(404)

	var hits atomic.Int32
	require.NoError(t, ps.Subscribe(ctx, channel, func(_ context.Context, _ *pubsub.Message) error {
		hits.Add(1)

		return nil
	}))

	// ACT
	err := r.SendTask(ctx, 404, &proto.GatewayMessage{
		Payload: &proto.GatewayMessage_Task{Task: &proto.DaemonTask{Id: 1}},
	})

	// ASSERT
	require.NoError(t, err)
	waitFor(t, func() bool { return hits.Load() >= 1 },
		"published task dispatch event")
}

func TestRegistry_SendCommand_localDirect_callsStream(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, stream := newTestSession(80, nil)
	require.NoError(t, r.Register(ctx, s))

	cmd := &proto.CommandRequest{
		CommandId: "cmd-1",
		ServerId:  10,
		Command:   "status",
		Timeout:   durationpb.New(5 * time.Second),
	}

	// ACT
	require.NoError(t, r.SendCommand(ctx, 80, cmd))

	// ASSERT
	sent := stream.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "cmd-1", sent[0].RequestId, "RequestId must be set from CommandId")
	got := sent[0].GetCommand()
	require.NotNil(t, got)
	assert.Equal(t, "cmd-1", got.CommandId)
	assert.Equal(t, "status", got.Command)
}

func TestRegistry_SendCommand_remoteViaPubSub_publishes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)
	channel := channels.BuildDaemonCommandDispatchChannel(81)

	var (
		received   *pubsub.Message
		receivedMu sync.Mutex
		done       = make(chan struct{})
	)
	require.NoError(t, ps.Subscribe(ctx, channel, func(_ context.Context, msg *pubsub.Message) error {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		if received == nil {
			received = msg
			close(done)
		}

		return nil
	}))

	cmd := &proto.CommandRequest{
		CommandId: "cmd-2",
		ServerId:  20,
		Command:   "ls",
		Timeout:   durationpb.New(7 * time.Second),
	}

	// ACT
	require.NoError(t, r.SendCommand(ctx, 81, cmd))

	// ASSERT
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for command dispatch publish")
	}

	receivedMu.Lock()
	defer receivedMu.Unlock()
	require.NotNil(t, received)
	assert.Equal(t, messages.TypeDaemonCommand, received.Type)
	payload, err := messages.ParsePayload[messages.DaemonCommandDispatchPayload](received)
	require.NoError(t, err)
	assert.Equal(t, uint64(81), payload.NodeID)
	assert.Equal(t, "cmd-2", payload.CommandID)
	assert.Equal(t, "ls", payload.Command)
	assert.Equal(t, int32(7), payload.Timeout)
}

func TestRegistry_SendAttachRequest_localDirect_callsStream(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, stream := newTestSession(60, nil)
	require.NoError(t, r.Register(ctx, s))

	req := &proto.AttachRequest{SessionId: "att-1", ServerId: 10}

	// ACT
	require.NoError(t, r.SendAttachRequest(ctx, 60, req))

	// ASSERT
	sent := stream.Sent()
	require.Len(t, sent, 1)
	got := sent[0].GetAttachRequest()
	require.NotNil(t, got)
	assert.Equal(t, "att-1", got.SessionId)
	assert.Equal(t, uint64(10), got.ServerId)
}

func TestRegistry_SendAttachInput_localDirect_callsStream(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, stream := newTestSession(60, nil)
	require.NoError(t, r.Register(ctx, s))

	in := &proto.AttachInput{SessionId: "att-1", Data: []byte("hello\n")}

	// ACT
	require.NoError(t, r.SendAttachInput(ctx, 60, in))

	// ASSERT
	sent := stream.Sent()
	require.Len(t, sent, 1)
	got := sent[0].Payload.(*proto.GatewayMessage_AttachInput).AttachInput
	require.NotNil(t, got)
	assert.Equal(t, "att-1", got.SessionId)
	assert.Equal(t, []byte("hello\n"), got.Data)
}

func TestRegistry_SendAttachDetach_localDirect_callsStream(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, stream := newTestSession(60, nil)
	require.NoError(t, r.Register(ctx, s))

	det := &proto.AttachDetach{SessionId: "att-1", Reason: "done"}

	// ACT
	require.NoError(t, r.SendAttachDetach(ctx, 60, det))

	// ASSERT
	sent := stream.Sent()
	require.Len(t, sent, 1)
	got := sent[0].Payload.(*proto.GatewayMessage_AttachDetach).AttachDetach
	require.NotNil(t, got)
	assert.Equal(t, "done", got.Reason)
}

func TestRegistry_SendAttachRequest_remoteViaPubSub_publishes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	channel := channels.BuildDaemonAttachDispatchChannel(70)

	var (
		received   *pubsub.Message
		receivedMu sync.Mutex
		done       = make(chan struct{})
	)
	require.NoError(t, ps.Subscribe(ctx, channel, func(_ context.Context, msg *pubsub.Message) error {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		if received == nil {
			received = msg
			close(done)
		}

		return nil
	}))

	// ACT
	require.NoError(t, r.SendAttachRequest(ctx, 70,
		&proto.AttachRequest{SessionId: "att-2", ServerId: 5}))

	// ASSERT
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for attach dispatch publish")
	}

	receivedMu.Lock()
	defer receivedMu.Unlock()
	require.NotNil(t, received)
	assert.Equal(t, messages.TypeDaemonAttach, received.Type)
	payload, err := messages.ParsePayload[messages.DaemonAttachDispatchPayload](received)
	require.NoError(t, err)
	assert.Equal(t, uint64(70), payload.NodeID)
	require.NotEmpty(t, payload.Data, "attach payload data must be marshaled")

	var decoded proto.GatewayMessage
	require.NoError(t, decoded.UnmarshalVT(payload.Data))
	require.NotNil(t, decoded.GetAttachRequest())
	assert.Equal(t, "att-2", decoded.GetAttachRequest().SessionId)
}

func TestRegistry_SendMetricsRequest_localDirect_doesNotRegisterWaiter(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	waiters := &fakeMetricsWaiterRegistrar{}
	r.SetMetricsWaiterRegistrar(waiters)

	s, stream := newTestSession(90, nil)
	require.NoError(t, r.Register(ctx, s))

	req := &proto.MetricsRequest{Kind: &proto.MetricsRequest_Current{Current: &proto.CurrentMetricsRequest{}}}

	// ACT
	require.NoError(t, r.SendMetricsRequest(ctx, 90, "metrics-req-1", req))

	// ASSERT
	sent := stream.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "metrics-req-1", sent[0].RequestId)
	assert.NotNil(t, sent[0].GetMetricsRequest())

	assert.Empty(t, waiters.Registered(),
		"local-direct path must not register a remote waiter; the requester is on this instance")
	assert.Empty(t, waiters.Canceled())
}

func TestRegistry_SendMetricsRequest_localStreamErrorIsWrapped(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, stream := newTestSession(90, nil)
	stream.sendErr = errTestSendBroken
	require.NoError(t, r.Register(ctx, s))

	// ACT
	err := r.SendMetricsRequest(ctx, 90, "req", &proto.MetricsRequest{})

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send metrics request to local session")
	assert.Contains(t, err.Error(), "send broken")
}

func TestRegistry_SendMetricsRequest_remote_publishes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)
	channel := channels.BuildDaemonMetricsRequestChannel(91)

	var (
		received   *pubsub.Message
		receivedMu sync.Mutex
		done       = make(chan struct{})
	)
	require.NoError(t, ps.Subscribe(ctx, channel, func(_ context.Context, msg *pubsub.Message) error {
		receivedMu.Lock()
		defer receivedMu.Unlock()
		if received == nil {
			received = msg
			close(done)
		}

		return nil
	}))

	// ACT
	require.NoError(t, r.SendMetricsRequest(ctx, 91, "metrics-r-1",
		&proto.MetricsRequest{Kind: &proto.MetricsRequest_Current{Current: &proto.CurrentMetricsRequest{}}}))

	// ASSERT
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for metrics request publish")
	}

	receivedMu.Lock()
	defer receivedMu.Unlock()
	require.NotNil(t, received)
	assert.Equal(t, messages.TypeDaemonMetricsRequest, received.Type)
	payload, err := messages.ParsePayload[messages.DaemonMetricsRequestPayload](received)
	require.NoError(t, err)
	assert.Equal(t, uint64(91), payload.NodeID)
	assert.Equal(t, "metrics-r-1", payload.RequestID)
	assert.Equal(t, testInstanceID, payload.InstanceID,
		"requester instance id must be the local registry's id so the holder can route the response back")
	assert.NotEmpty(t, payload.Data)
}

func TestRegistry_BroadcastToAll_sendsToEveryLocalSession(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)

	streams := make([]*stubStream, 0, 3)
	for _, nodeID := range []uint64{1, 2, 3} {
		s, stream := newTestSession(nodeID, nil)
		require.NoError(t, r.Register(ctx, s))
		streams = append(streams, stream)
	}

	msg := &proto.GatewayMessage{RequestId: "broadcast"}

	// ACT
	r.BroadcastToAll(ctx, msg)

	// ASSERT
	for i, stream := range streams {
		sent := stream.Sent()
		require.Lenf(t, sent, 1, "stream %d must have received broadcast", i)
		assert.Equal(t, "broadcast", sent[0].RequestId)
	}
}

func TestRegistry_BroadcastToAll_continuesOnSendError(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)

	failingSession, failingStream := newTestSession(1, nil)
	failingStream.sendErr = errTestStreamBroken
	require.NoError(t, r.Register(ctx, failingSession))

	goodSession, goodStream := newTestSession(2, nil)
	require.NoError(t, r.Register(ctx, goodSession))

	// ACT
	r.BroadcastToAll(ctx, &proto.GatewayMessage{RequestId: "x"})

	// ASSERT
	assert.Empty(t, failingStream.Sent(), "failing stream must not record sends")
	assert.Len(t, goodStream.Sent(), 1, "second session must still receive broadcast")
}

func TestRegistry_BroadcastShutdown_sendsShutdownMessage(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, stream := newTestSession(5, nil)
	require.NoError(t, r.Register(ctx, s))

	// ACT
	r.BroadcastShutdown(ctx, "going down", 30*time.Second)

	// ASSERT
	sent := stream.Sent()
	require.Len(t, sent, 1)
	sd := sent[0].GetShutdown()
	require.NotNil(t, sd, "shutdown payload must be present on broadcast")
	assert.Equal(t, "going down", sd.Reason)
	assert.Equal(t, 30*time.Second, sd.ReconnectDelay.AsDuration())
}

func TestRegistry_CloseAllSessions_invokesEverySessionCancel(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)

	var (
		cancelMu sync.Mutex
		canceled []uint64
	)
	for _, nodeID := range []uint64{1, 2, 3} {
		s, _ := newTestSession(nodeID, func() {
			cancelMu.Lock()
			canceled = append(canceled, nodeID)
			cancelMu.Unlock()
		})
		require.NoError(t, r.Register(ctx, s))
	}

	// ACT
	r.CloseAllSessions()

	// ASSERT
	cancelMu.Lock()
	defer cancelMu.Unlock()
	assert.ElementsMatch(t, []uint64{1, 2, 3}, canceled,
		"all session cancel funcs must fire on CloseAllSessions")
}

func TestRegistry_ConnectedNodeIDs_returnsLocalNodes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	for _, nodeID := range []uint64{1, 2, 3} {
		s, _ := newTestSession(nodeID, nil)
		require.NoError(t, r.Register(ctx, s))
	}

	// ACT
	got := r.ConnectedNodeIDs()

	// ASSERT
	assert.ElementsMatch(t, []uint64{1, 2, 3}, got)
}

func TestRegistry_SessionCount_returnsLocalCount(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	assert.Equal(t, 0, r.SessionCount(), "fresh registry has no sessions")

	for _, nodeID := range []uint64{1, 2} {
		s, _ := newTestSession(nodeID, nil)
		require.NoError(t, r.Register(ctx, s))
	}

	// ACT + ASSERT
	assert.Equal(t, 2, r.SessionCount(), "after two Register calls count must be 2")

	require.NoError(t, r.Unregister(ctx, 1))
	assert.Equal(t, 1, r.SessionCount(), "after Unregister count must drop")
}

func TestRegistry_WaitSessionsClosed_returnsImmediatelyWhenEmpty(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, _ := setupRegistry(t, false)

	// ACT
	start := time.Now()
	got := r.WaitSessionsClosed(time.Second)
	elapsed := time.Since(start)

	// ASSERT
	assert.True(t, got, "must return true immediately when there are no sessions")
	assert.Less(t, elapsed, 100*time.Millisecond, "must not block when registry is empty")
}

func TestRegistry_WaitSessionsClosed_returnsWhenAllUnregistered(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	for _, nodeID := range []uint64{1, 2} {
		s, _ := newTestSession(nodeID, nil)
		require.NoError(t, r.Register(ctx, s))
	}

	result := make(chan bool, 1)

	// ACT
	go func() {
		result <- r.WaitSessionsClosed(2 * time.Second)
	}()

	time.Sleep(10 * time.Millisecond)
	require.NoError(t, r.Unregister(ctx, 1))
	require.NoError(t, r.Unregister(ctx, 2))

	// ASSERT
	select {
	case got := <-result:
		assert.True(t, got, "must return true once all sessions are unregistered")
	case <-time.After(time.Second):
		t.Fatal("WaitSessionsClosed did not return after all sessions unregistered")
	}
}

func TestRegistry_WaitSessionsClosed_returnsFalseOnTimeout(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, _, ctx := setupRegistry(t, false)
	s, _ := newTestSession(1, nil)
	require.NoError(t, r.Register(ctx, s))

	// ACT
	got := r.WaitSessionsClosed(50 * time.Millisecond)

	// ASSERT
	assert.False(t, got, "must report false when sessions are still active after timeout")
}

func TestRegistry_handleSessionEvent_addsGlobalNodeForRemoteInstance(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	// ACT
	publishSessionEvent(ctx, t, ps,
		channels.DaemonSessionConnected, messages.TypeDaemonConnected,
		200, "instance-other")

	// ASSERT
	waitFor(t, func() bool {
		return r.IsConnectedAnywhere(200)
	}, "remote node tracked in globalNodes")

	assert.False(t, r.IsConnected(200), "remote node must not be reported as local")
}

func TestRegistry_handleSessionEvent_removesGlobalNodeOnDisconnect(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	publishSessionEvent(ctx, t, ps,
		channels.DaemonSessionConnected, messages.TypeDaemonConnected,
		201, "instance-other")
	waitFor(t, func() bool {
		return r.IsConnectedAnywhere(201)
	}, "remote node added")

	// ACT
	publishSessionEvent(ctx, t, ps,
		channels.DaemonSessionClosed, messages.TypeDaemonClosed,
		201, "instance-other")

	// ASSERT
	waitFor(t, func() bool {
		return !r.IsConnectedAnywhere(201)
	}, "remote node removed")
}

func TestRegistry_handleSessionEvent_ignoresOwnInstance(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	// ACT
	publishSessionEvent(ctx, t, ps,
		channels.DaemonSessionConnected, messages.TypeDaemonConnected,
		202, testInstanceID)

	// ASSERT — give the subscriber a moment, then confirm nothing landed in globalNodes.
	time.Sleep(50 * time.Millisecond)

	r.globalMu.RLock()
	_, ok := r.globalNodes[202]
	r.globalMu.RUnlock()
	assert.False(t, ok, "registry must ignore session events from its own instance")
	assert.False(t, r.IsConnectedAnywhere(202),
		"node must not be considered connected from a self-instance event")
}

func TestRegistry_handleTaskDispatch_routesToLocalSession(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	s, stream := newTestSession(300, nil)
	require.NoError(t, r.Register(ctx, s))

	gw := &proto.GatewayMessage{
		RequestId: "ext-1",
		Payload:   &proto.GatewayMessage_Task{Task: &proto.DaemonTask{Id: 11}},
	}
	data, err := gw.MarshalVT()
	require.NoError(t, err)

	channel := channels.BuildDaemonTaskDispatchChannel(300)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonTask, messages.DaemonTaskDispatchPayload{
		NodeID:    300,
		RequestID: "ext-1",
		TaskID:    11,
		TaskData:  data,
	})
	require.NoError(t, err)

	// ACT
	require.NoError(t, ps.Publish(ctx, channel, msg))

	// ASSERT
	waitFor(t, func() bool {
		return len(stream.Sent()) >= 1
	}, "stream received task forwarded from pubsub")

	sent := stream.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "ext-1", sent[0].RequestId)
	require.NotNil(t, sent[0].GetTask())
	assert.Equal(t, uint64(11), sent[0].GetTask().Id)
}

func TestRegistry_handleTaskDispatch_skipsForeignNode(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	// Register a different node locally
	s, stream := newTestSession(301, nil)
	require.NoError(t, r.Register(ctx, s))

	gw := &proto.GatewayMessage{Payload: &proto.GatewayMessage_Task{Task: &proto.DaemonTask{Id: 1}}}
	data, err := gw.MarshalVT()
	require.NoError(t, err)

	channel := channels.BuildDaemonTaskDispatchChannel(999)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonTask, messages.DaemonTaskDispatchPayload{
		NodeID:   999,
		TaskID:   1,
		TaskData: data,
	})
	require.NoError(t, err)

	// ACT
	require.NoError(t, ps.Publish(ctx, channel, msg))

	// ASSERT
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, stream.Sent(),
		"local session for node 301 must not receive a task targeted at node 999")
}

func TestRegistry_handleTaskDispatch_enqueueError_dropsWithoutPanic(t *testing.T) {
	t.Parallel()
	// ARRANGE: register a session then close it so its outbound queue rejects
	// new work (Enqueue → ErrSessionClosed). handleTaskDispatch must hit its
	// enqueue-error branch, drop the task and neither panic nor tear down the
	// pubsub subscription.
	r, ps, ctx := setupRegistry(t, true)

	s, stream := newTestSession(302, nil)
	require.NoError(t, r.Register(ctx, s))
	s.Close()

	gw := &proto.GatewayMessage{
		RequestId: "task-closed",
		Payload:   &proto.GatewayMessage_Task{Task: &proto.DaemonTask{Id: 5}},
	}
	data, err := gw.MarshalVT()
	require.NoError(t, err)

	channel := channels.BuildDaemonTaskDispatchChannel(302)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonTask, messages.DaemonTaskDispatchPayload{
		NodeID:    302,
		RequestID: "task-closed",
		TaskID:    5,
		TaskData:  data,
	})
	require.NoError(t, err)

	// ACT
	require.NoError(t, ps.Publish(ctx, channel, msg))

	// ASSERT
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, stream.Sent(), "a closed session must not deliver the dispatched task")
	assert.Equal(t, 1, r.SessionCount(),
		"the closed session stays registered until its own stream cleanup unregisters it")
}

func TestRegistry_handleAttachDispatch_routesToLocalSession(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	s, stream := newTestSession(310, nil)
	require.NoError(t, r.Register(ctx, s))

	gw := &proto.GatewayMessage{
		Payload: &proto.GatewayMessage_AttachRequest{
			AttachRequest: &proto.AttachRequest{SessionId: "sess-x", ServerId: 7},
		},
	}
	data, err := gw.MarshalVT()
	require.NoError(t, err)

	channel := channels.BuildDaemonAttachDispatchChannel(310)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonAttach,
		messages.DaemonAttachDispatchPayload{NodeID: 310, Data: data})
	require.NoError(t, err)

	// ACT
	require.NoError(t, ps.Publish(ctx, channel, msg))

	// ASSERT
	waitFor(t, func() bool {
		return len(stream.Sent()) >= 1
	}, "attach request was forwarded to local stream")

	sent := stream.Sent()
	require.Len(t, sent, 1)
	got := sent[0].GetAttachRequest()
	require.NotNil(t, got)
	assert.Equal(t, "sess-x", got.SessionId)
}

func TestRegistry_handleAttachDispatch_enqueueError_dropsWithoutPanic(t *testing.T) {
	t.Parallel()
	// ARRANGE: closed session → Enqueue rejects, exercising handleAttachDispatch's
	// enqueue-error branch. The message must be dropped without a panic.
	r, ps, ctx := setupRegistry(t, true)

	s, stream := newTestSession(311, nil)
	require.NoError(t, r.Register(ctx, s))
	s.Close()

	gw := &proto.GatewayMessage{
		Payload: &proto.GatewayMessage_AttachRequest{
			AttachRequest: &proto.AttachRequest{SessionId: "sess-closed", ServerId: 9},
		},
	}
	data, err := gw.MarshalVT()
	require.NoError(t, err)

	channel := channels.BuildDaemonAttachDispatchChannel(311)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonAttach,
		messages.DaemonAttachDispatchPayload{NodeID: 311, Data: data})
	require.NoError(t, err)

	// ACT
	require.NoError(t, ps.Publish(ctx, channel, msg))

	// ASSERT
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, stream.Sent(), "a closed session must not deliver the dispatched attach message")
}

func TestRegistry_handleMetricsRequest_registersRemoteWaiterAndForwards(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	waiters := &fakeMetricsWaiterRegistrar{}
	r.SetMetricsWaiterRegistrar(waiters)

	s, stream := newTestSession(320, nil)
	require.NoError(t, r.Register(ctx, s))

	gw := &proto.GatewayMessage{
		RequestId: "mreq-1",
		Payload: &proto.GatewayMessage_MetricsRequest{
			MetricsRequest: &proto.MetricsRequest{
				Kind: &proto.MetricsRequest_Current{Current: &proto.CurrentMetricsRequest{}},
			},
		},
	}
	data, err := gw.MarshalVT()
	require.NoError(t, err)

	channel := channels.BuildDaemonMetricsRequestChannel(320)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonMetricsRequest,
		messages.DaemonMetricsRequestPayload{
			NodeID:     320,
			RequestID:  "mreq-1",
			InstanceID: "instance-other",
			Data:       data,
		})
	require.NoError(t, err)

	// ACT
	require.NoError(t, ps.Publish(ctx, channel, msg))

	// ASSERT
	waitFor(t, func() bool {
		return len(stream.Sent()) >= 1 && len(waiters.Registered()) >= 1
	}, "remote waiter registered and request forwarded")

	registered := waiters.Registered()
	require.Len(t, registered, 1)
	assert.Equal(t, "mreq-1", registered[0].requestID)
	assert.Equal(t, uint64(320), registered[0].nodeID)
	assert.Equal(t, "instance-other", registered[0].instanceID,
		"requester instance id must be propagated so response can be routed back")

	assert.Empty(t, waiters.Canceled(), "no cancel expected on successful forward")

	sent := stream.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "mreq-1", sent[0].RequestId)
	assert.NotNil(t, sent[0].GetMetricsRequest())
}

func TestRegistry_handleMetricsRequest_skipsRegisterForOwnInstance(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	waiters := &fakeMetricsWaiterRegistrar{}
	r.SetMetricsWaiterRegistrar(waiters)

	s, stream := newTestSession(321, nil)
	require.NoError(t, r.Register(ctx, s))

	gw := &proto.GatewayMessage{RequestId: "mreq-2"}
	data, err := gw.MarshalVT()
	require.NoError(t, err)

	channel := channels.BuildDaemonMetricsRequestChannel(321)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonMetricsRequest,
		messages.DaemonMetricsRequestPayload{
			NodeID:     321,
			RequestID:  "mreq-2",
			InstanceID: testInstanceID,
			Data:       data,
		})
	require.NoError(t, err)

	// ACT
	require.NoError(t, ps.Publish(ctx, channel, msg))

	// ASSERT
	waitFor(t, func() bool {
		return len(stream.Sent()) >= 1
	}, "request forwarded")

	assert.Empty(t, waiters.Registered(),
		"must not register a remote waiter for messages originating from this instance")
}

func TestRegistry_handleMetricsRequest_unknownNode_isNoop(t *testing.T) {
	t.Parallel()
	// ARRANGE
	r, ps, ctx := setupRegistry(t, true)

	waiters := &fakeMetricsWaiterRegistrar{}
	r.SetMetricsWaiterRegistrar(waiters)

	channel := channels.BuildDaemonMetricsRequestChannel(404)
	gw := &proto.GatewayMessage{}
	data, err := gw.MarshalVT()
	require.NoError(t, err)

	msg, err := messages.NewMessage(channel, messages.TypeDaemonMetricsRequest,
		messages.DaemonMetricsRequestPayload{
			NodeID:     404,
			RequestID:  "mreq-3",
			InstanceID: "instance-other",
			Data:       data,
		})
	require.NoError(t, err)

	// ACT
	require.NoError(t, ps.Publish(ctx, channel, msg))

	// ASSERT
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, waiters.Registered(),
		"no waiter must be registered when no local session exists for the target node")
}

func TestRegistry_handleMetricsRequest_enqueueError_cancelsRemoteWaiter(t *testing.T) {
	t.Parallel()
	// ARRANGE: a remote metrics request registers a cross-instance waiter before
	// forwarding to the local session. If that forward fails (here: the session
	// is closed so Enqueue → ErrSessionClosed), the waiter would otherwise leak
	// until its TTL — handleMetricsRequest must cancel it immediately.
	r, ps, ctx := setupRegistry(t, true)

	waiters := &fakeMetricsWaiterRegistrar{}
	r.SetMetricsWaiterRegistrar(waiters)

	s, stream := newTestSession(330, nil)
	require.NoError(t, r.Register(ctx, s))
	s.Close() // Enqueue now returns ErrSessionClosed

	gw := &proto.GatewayMessage{RequestId: "mreq-closed"}
	data, err := gw.MarshalVT()
	require.NoError(t, err)

	channel := channels.BuildDaemonMetricsRequestChannel(330)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonMetricsRequest,
		messages.DaemonMetricsRequestPayload{
			NodeID:     330,
			RequestID:  "mreq-closed",
			InstanceID: "instance-other",
			Data:       data,
		})
	require.NoError(t, err)

	// ACT
	require.NoError(t, ps.Publish(ctx, channel, msg))

	// ASSERT: the waiter is registered then cancelled because the forward failed.
	waitFor(t, func() bool {
		return len(waiters.Canceled()) >= 1
	}, "waiter cancelled after enqueue failure")

	assert.Equal(t, []string{"mreq-closed"}, waiters.Canceled(),
		"the failed request's waiter must be cancelled so it does not leak")

	registered := waiters.Registered()
	require.Len(t, registered, 1)
	assert.Equal(t, "mreq-closed", registered[0].requestID,
		"the same request that was registered must be the one cancelled")

	assert.Empty(t, stream.Sent(), "nothing may be delivered to the closed session")
}

// Sanity guard against accidental future changes to the Stream interface
// shape: the stub stream must remain a Stream.
var _ Stream = (*stubStream)(nil)

// Compile-time guard that fakeMetricsWaiterRegistrar satisfies the
// MetricsWaiterRegistrar interface.
var _ MetricsWaiterRegistrar = (*fakeMetricsWaiterRegistrar)(nil)

// Ensure io.EOF is referenced so unused-import linters don't complain in
// case stubStream's Recv path is exercised by future tests.
var _ = io.EOF

type sessionObserverRecorder struct {
	mu     sync.Mutex
	events []string
}

func (o *sessionObserverRecorder) SessionRegistered(_ context.Context, nodeID uint64, version string, reconnect bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, fmt.Sprintf("registered:%d:%s:%t", nodeID, version, reconnect))
}

func (o *sessionObserverRecorder) SessionUnregistered(_ context.Context, nodeID uint64, version string, _ time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, fmt.Sprintf("unregistered:%d:%s", nodeID, version))
}

func (o *sessionObserverRecorder) all() []string {
	o.mu.Lock()
	defer o.mu.Unlock()

	return append([]string(nil), o.events...)
}

func TestRegistry_Observer_sees_only_local_transitions(t *testing.T) {
	t.Parallel()

	r, ps, ctx := setupRegistry(t, true)
	observer := &sessionObserverRecorder{}
	r.SetSessionObserver(observer)

	s1, _ := newTestSession(7, nil)
	s1.Version = "4.4.0"
	require.NoError(t, r.Register(ctx, s1))

	s2, _ := newTestSession(7, nil)
	s2.Version = "4.4.1"
	require.NoError(t, r.Register(ctx, s2))

	require.NoError(t, r.UnregisterSession(ctx, s1), "stale cleanup of the replaced session")
	require.NoError(t, r.UnregisterSession(ctx, s2))
	require.NoError(t, r.Unregister(ctx, 7), "nothing registered any more")

	// A session owned by another instance only reaches this registry through
	// pub/sub and is not reported.
	publishSessionEvent(ctx, t, ps, channels.DaemonSessionConnected, messages.TypeDaemonConnected, 9, "instance-other")
	waitFor(t, func() bool { return r.IsConnectedAnywhere(9) }, "remote session to be tracked globally")

	assert.Equal(t, []string{
		"registered:7:4.4.0:false",
		"registered:7:4.4.1:true",
		"unregistered:7:4.4.1",
	}, observer.all())
}
