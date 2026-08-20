package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
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
)

// errPublishBoom is the sentinel error returned by errPublisher.
var errPublishBoom = errors.New("boom")

// errPublisher is a stub Publisher that always returns an error from Publish.
// It is used to verify that handler methods log publish failures but never
// surface them to the caller.
type errPublisher struct{}

func (errPublisher) Publish(_ context.Context, _ string, _ *pubsub.Message) error {
	return errPublishBoom
}

func subscribeOnce(t *testing.T, ps pubsub.PubSub, channel string) (
	received func() *pubsub.Message,
	done <-chan struct{},
) {
	t.Helper()

	var (
		mu  sync.Mutex
		msg *pubsub.Message
	)
	doneCh := make(chan struct{})

	err := ps.Subscribe(context.Background(), channel, func(_ context.Context, m *pubsub.Message) error {
		mu.Lock()
		defer mu.Unlock()

		if msg != nil {
			return nil
		}
		msg = m
		close(doneCh)

		return nil
	})
	require.NoError(t, err)

	return func() *pubsub.Message {
		mu.Lock()
		defer mu.Unlock()

		return msg
	}, doneCh
}

func TestAttachHandler_HandleAttachStarted_publishes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	const (
		nodeID    uint64 = 7
		serverID  uint64 = 42
		sessionID        = "sess-started-1"
	)

	channel := channels.BuildRealtimeAttachStartedChannel(sessionID)
	getReceived, done := subscribeOnce(t, ps, channel)

	handler := NewAttachHandler(ps, slog.Default())

	// ACT
	err := handler.HandleAttachStarted(context.Background(), nodeID, &proto.AttachStarted{
		SessionId: sessionID,
		ServerId:  serverID,
	})

	// ASSERT
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pubsub delivery")
	}

	got := getReceived()
	require.NotNil(t, got)
	assert.Equal(t, channel, got.Channel)
	assert.Equal(t, messages.TypeAttachStarted, got.Type)

	payload, err := messages.ParsePayload[messages.AttachStartedPayload](got)
	require.NoError(t, err)
	assert.Equal(t, sessionID, payload.SessionID, "session id should round-trip")
	assert.Equal(t, serverID, payload.ServerID, "server id should round-trip")
}

func TestAttachHandler_HandleAttachStarted_nilPublisher_returnsNil(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewAttachHandler(nil, slog.Default())

	// ACT
	err := handler.HandleAttachStarted(context.Background(), 1, &proto.AttachStarted{
		SessionId: "sess-x",
		ServerId:  1,
	})

	// ASSERT
	require.NoError(t, err)
}

func TestAttachHandler_HandleAttachOutput_publishesData(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	const sessionID = "sess-output-1"
	payloadBytes := []byte("hello-world")
	channel := channels.BuildRealtimeAttachOutputChannel(sessionID)
	getReceived, done := subscribeOnce(t, ps, channel)

	handler := NewAttachHandler(ps, slog.Default())

	// ACT
	err := handler.HandleAttachOutput(context.Background(), 1, &proto.AttachOutput{
		SessionId: sessionID,
		Data:      payloadBytes,
	})

	// ASSERT
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pubsub delivery")
	}

	got := getReceived()
	require.NotNil(t, got)
	assert.Equal(t, channel, got.Channel)
	assert.Equal(t, messages.TypeAttachOutput, got.Type)

	payload, err := messages.ParsePayload[messages.AttachOutputPayload](got)
	require.NoError(t, err)
	assert.Equal(t, sessionID, payload.SessionID)
	assert.Equal(t, payloadBytes, payload.Data, "output bytes should be forwarded unchanged")
}

func TestAttachHandler_HandleAttachOutput_nilPublisher_returnsNil(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewAttachHandler(nil, slog.Default())

	// ACT
	err := handler.HandleAttachOutput(context.Background(), 1, &proto.AttachOutput{
		SessionId: "sess-x",
		Data:      []byte("x"),
	})

	// ASSERT
	require.NoError(t, err)
}

func TestAttachHandler_HandleAttachClosed_publishesAndUntracks(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	const (
		sessionID        = "sess-closed-1"
		serverID  uint64 = 42
		exitCode  int32  = 137
		reason           = "killed by signal"
	)

	channel := channels.BuildRealtimeAttachClosedChannel(sessionID)
	getReceived, done := subscribeOnce(t, ps, channel)

	handler := NewAttachHandler(ps, slog.Default())

	handler.TrackAttachSession(sessionID, serverID)
	got, ok := handler.SessionServerID(sessionID)
	require.True(t, ok, "session should be tracked before close")
	require.Equal(t, serverID, got)

	// ACT
	err := handler.HandleAttachClosed(context.Background(), 1, &proto.AttachClosed{
		SessionId: sessionID,
		Reason:    reason,
		ExitCode:  exitCode,
	})

	// ASSERT
	require.NoError(t, err)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pubsub delivery")
	}

	rawMsg := getReceived()
	require.NotNil(t, rawMsg)
	assert.Equal(t, channel, rawMsg.Channel)
	assert.Equal(t, messages.TypeAttachClosed, rawMsg.Type)

	payload, err := messages.ParsePayload[messages.AttachClosedPayload](rawMsg)
	require.NoError(t, err)
	assert.Equal(t, sessionID, payload.SessionID)
	assert.Equal(t, reason, payload.Reason)
	assert.Equal(t, exitCode, payload.ExitCode)

	gotID, found := handler.SessionServerID(sessionID)
	assert.False(t, found, "session must be untracked after close")
	assert.Equal(t, uint64(0), gotID, "untracked session lookup must return zero value")
}

func TestAttachHandler_HandleAttachClosed_nilPublisher_returnsNilAndUntracks(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewAttachHandler(nil, slog.Default())
	const sessionID = "sess-nil-pub"
	handler.TrackAttachSession(sessionID, 11)

	// ACT
	err := handler.HandleAttachClosed(context.Background(), 1, &proto.AttachClosed{
		SessionId: sessionID,
		Reason:    "done",
		ExitCode:  0,
	})

	// ASSERT
	require.NoError(t, err)

	_, ok := handler.SessionServerID(sessionID)
	assert.False(t, ok, "session must be untracked even when publisher is nil")
}

func TestAttachHandler_TrackUntrackSession(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(h *AttachHandler)
		lookup    string
		wantID    uint64
		wantFound bool
	}{
		{
			name: "track_then_lookup_hit",
			setup: func(h *AttachHandler) {
				h.TrackAttachSession("a", 100)
			},
			lookup:    "a",
			wantID:    100,
			wantFound: true,
		},
		{
			name: "untrack_removes",
			setup: func(h *AttachHandler) {
				h.TrackAttachSession("a", 100)
				h.UntrackAttachSession("a")
			},
			lookup:    "a",
			wantID:    0,
			wantFound: false,
		},
		{
			name: "lookup_unknown_returns_false",
			setup: func(_ *AttachHandler) {
				// no-op
			},
			lookup:    "missing",
			wantID:    0,
			wantFound: false,
		},
		{
			name: "track_overwrites_previous_value",
			setup: func(h *AttachHandler) {
				h.TrackAttachSession("a", 1)
				h.TrackAttachSession("a", 2)
			},
			lookup:    "a",
			wantID:    2,
			wantFound: true,
		},
		{
			name: "untrack_unknown_is_safe",
			setup: func(h *AttachHandler) {
				h.UntrackAttachSession("never-existed")
			},
			lookup:    "never-existed",
			wantID:    0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			handler := NewAttachHandler(nil, slog.Default())

			// ACT
			tt.setup(handler)
			gotID, gotFound := handler.SessionServerID(tt.lookup)

			// ASSERT
			assert.Equal(t, tt.wantFound, gotFound)
			assert.Equal(t, tt.wantID, gotID)
		})
	}
}

func TestAttachHandler_PublishError_isLoggedNotReturned(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewAttachHandler(errPublisher{}, slog.Default())
	ctx := context.Background()

	// ACT + ASSERT — every Handle* method should swallow publisher errors.
	err := handler.HandleAttachStarted(ctx, 1, &proto.AttachStarted{SessionId: "s", ServerId: 1})
	assert.NoError(t, err, "HandleAttachStarted must not surface publisher errors")

	err = handler.HandleAttachOutput(ctx, 1, &proto.AttachOutput{SessionId: "s", Data: []byte("x")})
	assert.NoError(t, err, "HandleAttachOutput must not surface publisher errors")

	err = handler.HandleAttachClosed(ctx, 1, &proto.AttachClosed{SessionId: "s", Reason: "r", ExitCode: 0})
	assert.NoError(t, err, "HandleAttachClosed must not surface publisher errors")
}

func TestAttachHandler_NewAttachHandler_nilLogger_usesDefault(t *testing.T) {
	t.Parallel()
	// ARRANGE + ACT
	handler := NewAttachHandler(nil, nil)

	// ASSERT — calling a method with a nil-logger constructor must not panic.
	require.NotNil(t, handler)
	err := handler.HandleAttachStarted(context.Background(), 1, &proto.AttachStarted{
		SessionId: "x", ServerId: 1,
	})
	assert.NoError(t, err)
}

func TestAttachHandler_TrackUntrack_concurrentAccess(t *testing.T) {
	t.Parallel()
	// ARRANGE
	handler := NewAttachHandler(nil, slog.Default())

	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// ACT
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()

			sessionID := fmt.Sprintf("sess-%d", id)
			handler.TrackAttachSession(sessionID, uint64(id))
			_, _ = handler.SessionServerID(sessionID)
			handler.UntrackAttachSession(sessionID)
		}(i)
	}

	wg.Wait()

	// ASSERT — every session must be removed after concurrent track/untrack.
	for i := range goroutines {
		sessionID := fmt.Sprintf("sess-%d", i)
		_, ok := handler.SessionServerID(sessionID)
		assert.False(t, ok, "session %s should be untracked after concurrent operations", sessionID)
	}
}
