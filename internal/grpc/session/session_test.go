package session

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errTestStream = errors.New("test stream error")

func TestNewSession_initialState(t *testing.T) {
	t.Parallel()
	// ARRANGE
	stream := newStubStream(context.Background())
	caps := []string{"cap-a", "cap-b"}
	before := time.Now()

	// ACT
	s := NewSession(42, stream, "1.2.3", caps, func() {})

	// ASSERT
	require.NotNil(t, s)
	assert.Equal(t, uint64(42), s.NodeID, "NodeID must be set")
	assert.Equal(t, "1.2.3", s.Version, "Version must be set")
	assert.Equal(t, caps, s.Capabilities, "Capabilities must be set")
	assert.Same(t, stream, s.Stream, "Stream must be the same instance")

	assert.False(t, s.ConnectedAt.Before(before), "ConnectedAt must be at or after start time")
	assert.True(t, s.ConnectedAt.Before(time.Now().Add(time.Second)),
		"ConnectedAt must be within last second")

	lastPing := s.LastPing()
	assert.False(t, lastPing.Before(before), "lastPing must be at or after start time")
	assert.True(t, lastPing.Sub(s.ConnectedAt) < 100*time.Millisecond,
		"initial lastPing must be near ConnectedAt")

	require.NotNil(t, s.pendingReqs, "pendingReqs map must be initialised")
	assert.Empty(t, s.pendingReqs, "pendingReqs map must start empty")
}

func TestSession_SetLogger(t *testing.T) {
	t.Parallel()
	t.Run("overrides_the_default_logger", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)
		custom := slog.New(slog.DiscardHandler)

		// ACT
		s.SetLogger(custom)

		// ASSERT
		assert.Same(t, custom, s.logger, "SetLogger must install the provided logger")
	})

	t.Run("nil_logger_is_ignored", func(t *testing.T) {
		t.Parallel()
		// ARRANGE: the nil-guard must protect the working logger so a stray
		// SetLogger(nil) cannot leave the session with a nil logger that later
		// panics inside dispatchLoop / Enqueue.
		s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)
		original := s.logger
		require.NotNil(t, original, "session must start with a non-nil default logger")

		// ACT
		s.SetLogger(nil)

		// ASSERT
		assert.Same(t, original, s.logger, "SetLogger(nil) must keep the existing logger")
	})
}

func TestSession_Send_proxiesToStream(t *testing.T) {
	t.Parallel()
	t.Run("happy_path_forwards_to_stream", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		stream := newStubStream(context.Background())
		s := NewSession(1, stream, "v", nil, nil)
		msg := &proto.GatewayMessage{RequestId: "r-1"}

		// ACT
		err := s.Send(msg)

		// ASSERT
		require.NoError(t, err)
		sent := stream.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, "r-1", sent[0].RequestId)
	})

	t.Run("propagates_stream_error", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		stream := newStubStream(context.Background())
		stream.sendErr = errTestStream
		s := NewSession(1, stream, "v", nil, nil)

		// ACT
		err := s.Send(&proto.GatewayMessage{})

		// ASSERT
		require.Error(t, err)
		assert.ErrorIs(t, err, errTestStream, "stream error must be returned unchanged")
	})
}

func TestSession_Send_concurrent(t *testing.T) {
	t.Parallel()
	// ARRANGE
	const goroutines = 50

	stream := newStubStream(context.Background())
	s := NewSession(1, stream, "v", nil, nil)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// ACT
	for i := range goroutines {
		go func() {
			defer wg.Done()

			assert.NotPanics(t, func() {
				err := s.Send(&proto.GatewayMessage{RequestId: strconv.Itoa(i)})
				assert.NoError(t, err)
			}, "concurrent Send must not panic")
		}()
	}

	wg.Wait()

	// ASSERT
	sent := stream.Sent()
	require.Len(t, sent, goroutines,
		"every concurrent Send must be serialized through sendMu and recorded exactly once")

	seen := make(map[string]struct{}, goroutines)
	for _, m := range sent {
		seen[m.RequestId] = struct{}{}
	}
	assert.Len(t, seen, goroutines, "each goroutine's message must be delivered without loss")
}

func TestSession_Enqueue_deliversToStreamAsync(t *testing.T) {
	t.Parallel()
	// ARRANGE
	stream := newStubStream(context.Background())
	s := NewSession(1, stream, "v", nil, nil)
	t.Cleanup(s.Close)

	// ACT
	require.NoError(t, s.Enqueue(&proto.GatewayMessage{RequestId: "q-1"}))

	// ASSERT: the worker delivers it asynchronously.
	require.Eventually(t, func() bool {
		return len(stream.Sent()) == 1
	}, time.Second, 5*time.Millisecond, "queued message must reach the stream")
	assert.Equal(t, "q-1", stream.Sent()[0].RequestId)
}

func TestSession_Enqueue_afterClose_returnsClosed(t *testing.T) {
	t.Parallel()
	// ARRANGE
	s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)
	s.Close()

	// ACT
	err := s.Enqueue(&proto.GatewayMessage{})

	// ASSERT
	require.ErrorIs(t, err, ErrSessionClosed)
}

// blockingStream blocks every Send until release is closed, simulating a
// backpressured / stalled daemon stream.
type blockingStream struct {
	*stubStream

	release chan struct{}
}

func (b *blockingStream) Send(m *proto.GatewayMessage) error {
	<-b.release

	return b.stubStream.Send(m)
}

func TestSession_Enqueue_stalledStream_doesNotBlockCaller(t *testing.T) {
	t.Parallel()
	// ARRANGE: a stream whose Send blocks indefinitely. Enqueue must never
	// block the caller (the shared pubsub goroutine) on it — this is the whole
	// point of the outbound queue.
	stream := &blockingStream{stubStream: newStubStream(context.Background()), release: make(chan struct{})}
	s := NewSession(1, stream, "v", nil, nil)
	t.Cleanup(func() {
		close(stream.release)
		s.Close()
	})

	// ACT: enqueue well past the worker's in-flight message + buffer capacity.
	done := make(chan struct{})
	var fullErrs int
	go func() {
		defer close(done)
		for range outboundBufferSize + 8 {
			if errors.Is(s.Enqueue(&proto.GatewayMessage{}), ErrOutboundFull) {
				fullErrs++
			}
		}
	}()

	// ASSERT: all enqueues return promptly despite the permanently stalled Send.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Enqueue blocked on a stalled stream — head-of-line blocking not fixed")
	}
	assert.Positive(t, fullErrs, "a saturated queue must surface ErrOutboundFull, not block")
}

// flakyStream returns an error for its first failUntil Send calls, then
// delegates to the embedded stubStream. It exercises the dispatchLoop
// swallow-and-continue path without mutating a shared field concurrently with
// the worker (the counter is atomic; failUntil is fixed at construction).
type flakyStream struct {
	*stubStream

	failUntil int
	sendCalls atomic.Int32
}

func (f *flakyStream) Send(m *proto.GatewayMessage) error {
	if int(f.sendCalls.Add(1)) <= f.failUntil {
		return errTestStream
	}

	return f.stubStream.Send(m)
}

func TestSession_dispatchLoop_survivesSendError(t *testing.T) {
	t.Parallel()
	// ARRANGE: the first queued Send fails. dispatchLoop must log-and-continue
	// rather than let the worker die, so a subsequent message still reaches the
	// stream.
	stream := &flakyStream{stubStream: newStubStream(context.Background()), failUntil: 1}
	s := NewSession(1, stream, "v", nil, nil)
	t.Cleanup(s.Close)

	// ACT: the first Enqueue's Send errors and is swallowed; the second
	// Enqueue's Send succeeds on the recovered stream.
	require.NoError(t, s.Enqueue(&proto.GatewayMessage{RequestId: "q-err"}))
	require.NoError(t, s.Enqueue(&proto.GatewayMessage{RequestId: "q-ok"}))

	// ASSERT: exactly the recovered message is delivered — proving the worker
	// kept draining the outbound queue after the swallowed error.
	require.Eventually(t, func() bool {
		return len(stream.Sent()) == 1
	}, time.Second, 5*time.Millisecond,
		"worker must keep draining the queue after a Send error")

	sent := stream.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, "q-ok", sent[0].RequestId,
		"only the message whose Send succeeded is recorded; the worker did not die on the error")

	assert.NotPanics(t, s.Close, "Close must remain safe after a swallowed Send error")
}

func TestSession_UpdateLastPing_advancesTime(t *testing.T) {
	t.Parallel()
	// ARRANGE
	s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)
	original := s.LastPing()
	time.Sleep(2 * time.Millisecond)

	// ACT
	s.UpdateLastPing()

	// ASSERT
	updated := s.LastPing()
	assert.True(t, updated.After(original), "lastPing must advance after UpdateLastPing")
}

func TestSession_RegisterPendingRequest_returnsBufferedChan(t *testing.T) {
	t.Parallel()
	// ARRANGE
	s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)

	// ACT
	ch := s.RegisterPendingRequest("req-1")

	// ASSERT
	require.NotNil(t, ch)
	assert.Equal(t, 1, cap(ch), "pending request channel must have capacity 1")

	select {
	case <-ch:
		t.Fatal("freshly registered channel must not be readable")
	default:
	}

	s.mu.RLock()
	_, ok := s.pendingReqs["req-1"]
	s.mu.RUnlock()
	assert.True(t, ok, "request must be tracked in pendingReqs map")
}

func TestSession_ResolvePendingRequest_deliversAndCloses(t *testing.T) {
	t.Parallel()
	// ARRANGE
	s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)
	ch := s.RegisterPendingRequest("req-1")
	msg := &proto.DaemonMessage{}

	// ACT
	ok := s.ResolvePendingRequest("req-1", msg)

	// ASSERT
	require.True(t, ok, "first resolve must report success")

	select {
	case got, open := <-ch:
		assert.True(t, open, "channel must deliver value before closing")
		assert.Same(t, msg, got, "delivered message must be the one resolved with")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivered message")
	}

	_, open := <-ch
	assert.False(t, open, "channel must be closed after Resolve")

	ok2 := s.ResolvePendingRequest("req-1", msg)
	assert.False(t, ok2, "second resolve for same id must report not found")
}

func TestSession_ResolvePendingRequest_unknownID_returnsFalse(t *testing.T) {
	t.Parallel()
	// ARRANGE
	s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)

	// ACT
	ok := s.ResolvePendingRequest("never-registered", &proto.DaemonMessage{})

	// ASSERT
	assert.False(t, ok, "resolving unknown request id must return false")
}

func TestSession_CancelPendingRequest_closesWithoutDelivery(t *testing.T) {
	t.Parallel()
	// ARRANGE
	s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)
	ch := s.RegisterPendingRequest("req-1")

	// ACT
	s.CancelPendingRequest("req-1")

	// ASSERT
	select {
	case msg, open := <-ch:
		assert.False(t, open, "channel must be closed without delivering a value")
		assert.Nil(t, msg, "no message should be delivered on cancel")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for closed channel")
	}

	ok := s.ResolvePendingRequest("req-1", &proto.DaemonMessage{})
	assert.False(t, ok, "resolve after cancel must report not found")

	assert.NotPanics(t, func() {
		s.CancelPendingRequest("req-1")
	}, "cancelling an already-cancelled request must be a no-op")
}

func TestSession_HasCapability(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		caps []string
		want map[string]bool
	}{
		{
			name: "empty_capabilities_returns_false_for_anything",
			caps: nil,
			want: map[string]bool{"a": false, "b": false, "": false},
		},
		{
			name: "matches_known_returns_true_unknown_returns_false",
			caps: []string{"a", "b"},
			want: map[string]bool{"a": true, "b": true, "z": false},
		},
		{
			name: "case_sensitive",
			caps: []string{"Cap"},
			want: map[string]bool{"Cap": true, "cap": false, "CAP": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			s := NewSession(1, newStubStream(context.Background()), "v", tt.caps, nil)

			// ACT + ASSERT
			for cap, want := range tt.want {
				got := s.HasCapability(cap)
				assert.Equal(t, want, got, "HasCapability(%q) mismatch", cap)
			}
		})
	}
}

func TestSession_Cancel_invokesCancelFunc(t *testing.T) {
	t.Parallel()
	// ARRANGE
	var calls atomic.Int32
	s := NewSession(1, newStubStream(context.Background()), "v", nil, func() {
		calls.Add(1)
	})

	// ACT
	s.Cancel()
	s.Cancel()

	// ASSERT
	assert.Equal(t, int32(2), calls.Load(),
		"Cancel must invoke the cancel func every call (no internal idempotency in Session)")
}

func TestSession_Cancel_nilCancel_noPanic(t *testing.T) {
	t.Parallel()
	// ARRANGE
	s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)

	// ACT + ASSERT
	assert.NotPanics(t, s.Cancel, "Cancel with nil cancel func must not panic")
}

func TestSession_Close_failsPendingRequests(t *testing.T) {
	t.Parallel()
	s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)
	ch := s.RegisterPendingRequest("req-1")

	// ACT: closing the session (daemon disconnected) must unblock the waiter.
	s.Close()

	select {
	case msg, open := <-ch:
		assert.False(t, open, "pending request channel must be closed on session Close")
		assert.Nil(t, msg, "no response is delivered when the session dies")
	case <-time.After(time.Second):
		t.Fatal("Close must unblock in-flight request waiters, not leave them hanging")
	}

	assert.False(t, s.ResolvePendingRequest("req-1", &proto.DaemonMessage{}),
		"the request must have been removed from the pending map")
}

func TestSession_Close_isIdempotent(t *testing.T) {
	t.Parallel()
	s := NewSession(1, newStubStream(context.Background()), "v", nil, nil)
	s.RegisterPendingRequest("req-1")

	assert.NotPanics(t, func() {
		s.Close()
		s.Close()
	}, "Close must be safe to call more than once")
}
