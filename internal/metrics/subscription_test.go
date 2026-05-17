package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSubscription_Samples_ReturnsBufferedChannel(t *testing.T) {
	// ARRANGE
	sub := newSubscription(nil, 7, 4)

	// ACT
	ch := sub.Samples()

	// ASSERT
	require.NotNil(t, ch)
	assert.Equal(t, 4, cap(ch), "channel capacity must equal the requested buffer size")
}

func TestSubscription_Deliver_PutsEntryOnChannel(t *testing.T) {
	// ARRANGE
	sub := newSubscription(nil, 7, 4)
	entry := &proto.MetricsResponse{
		Timestamp: timestamppb.New(time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)),
	}

	// ACT
	sub.deliver(entry)

	// ASSERT
	select {
	case got := <-sub.Samples():
		assert.Same(t, entry, got, "delivered entry must be the same pointer as enqueued")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivered entry")
	}
}

func TestSubscription_Deliver_WhenBufferFull_DropsSilently(t *testing.T) {
	// ARRANGE
	sub := newSubscription(nil, 7, 1)
	first := &proto.MetricsResponse{Timestamp: timestamppb.Now()}
	second := &proto.MetricsResponse{Timestamp: timestamppb.Now()}

	// ACT
	sub.deliver(first)
	sub.deliver(second)

	// ASSERT
	select {
	case got := <-sub.Samples():
		assert.Same(t, first, got, "first entry occupies the buffer")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first entry")
	}

	// Buffer-full second deliver must drop without queueing — no further entry should be available.
	select {
	case got := <-sub.Samples():
		t.Fatalf("expected no more entries, got %v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSubscription_Deliver_AfterCloseChannel_DoesNotPanic(t *testing.T) {
	// ARRANGE
	sub := newSubscription(nil, 7, 4)
	sub.closeChannel()

	// ACT / ASSERT — closed flag must short-circuit the send before it touches the closed channel.
	assert.NotPanics(t, func() {
		sub.deliver(&proto.MetricsResponse{Timestamp: timestamppb.Now()})
	})
}

func TestSubscription_Close_idempotent(t *testing.T) {
	// ARRANGE
	// A bare hub has no registered state for this nodeID, so unsubscribe
	// takes its state==nil branch and closes the subscription channel. This
	// is the real production path for a sub whose node was never registered.
	h := &hub{}
	sub := newSubscription(h, 7, 4)

	// ACT
	sub.Close()

	// ASSERT — first Close must close the samples channel exactly once.
	select {
	case got, open := <-sub.Samples():
		assert.False(t, open, "samples channel must be closed after first Close")
		assert.Nil(t, got, "closed channel must yield the zero value")
	case <-time.After(time.Second):
		t.Fatal("timed out: samples channel was not closed by Close")
	}

	// ACT + ASSERT — a second Close is short-circuited by the unsubscribed
	// guard: it must not re-run unsubscribe / re-close the channel (which
	// would panic), and channel state must stay consistent.
	assert.NotPanics(t, sub.Close, "second Close must be a safe no-op")

	_, open := <-sub.Samples()
	assert.False(t, open, "samples channel must remain closed after the second Close")

	assert.NotPanics(t, func() {
		sub.Close()
		sub.Close()
	}, "further Close calls must remain no-ops")
}

func TestSubscription_ConcurrentCloseAndDeliver_NoPanic(t *testing.T) {
	// ARRANGE
	const goroutines = 40

	h := &hub{}
	sub := newSubscription(h, 7, 8)

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	// ACT — race deliver(), closeChannel() and Close() concurrently. All
	// three are serialized by closeMu and must never produce a
	// "send on closed channel" panic.
	for range goroutines {
		go func() {
			defer wg.Done()

			assert.NotPanics(t, func() {
				sub.deliver(&proto.MetricsResponse{Timestamp: timestamppb.Now()})
			}, "deliver must not panic when racing close")
		}()

		go func() {
			defer wg.Done()

			assert.NotPanics(t, sub.closeChannel, "closeChannel must be safe to race")
		}()

		go func() {
			defer wg.Done()

			assert.NotPanics(t, sub.Close, "Close must be safe to race")
		}()
	}

	// ASSERT — the test completing without panic (and clean under -race)
	// is the assertion; drain whatever remains so no goroutine leaks.
	wg.Wait()

	for range sub.Samples() { //nolint:revive
	}
}

func TestSubscription_CloseChannel_FirstCallClosesUnderlyingChannel(t *testing.T) {
	// ARRANGE
	// closeChannel is intentionally a single-call contract: a second call would panic by
	// re-closing the channel. We only verify the documented first-call behaviour here.
	sub := newSubscription(nil, 7, 4)
	first := &proto.MetricsResponse{Timestamp: timestamppb.Now()}
	second := &proto.MetricsResponse{Timestamp: timestamppb.Now()}
	sub.deliver(first)
	sub.deliver(second)

	// ACT
	sub.closeChannel()

	// ASSERT — ranging over a closed channel must drain remaining buffered entries and exit.
	drained := make([]*proto.MetricsResponse, 0, 2)
	for entry := range sub.Samples() {
		drained = append(drained, entry)
	}
	require.Len(t, drained, 2)
	assert.Same(t, first, drained[0])
	assert.Same(t, second, drained[1])
}
