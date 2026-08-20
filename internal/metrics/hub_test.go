package metrics

import (
	"context"
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
	"google.golang.org/protobuf/types/known/timestamppb"
)

var errFakeNetworkDown = errors.New("network down")

type fakeRegistry struct {
	instanceID    string
	connectedHere bool
	connectedAny  bool

	mu          sync.Mutex
	sentRequest atomic.Pointer[sentRequest]
	sendErr     error
}

type sentRequest struct {
	nodeID    uint64
	requestID string
	req       *proto.MetricsRequest
}

func (f *fakeRegistry) InstanceID() string              { return f.instanceID }
func (f *fakeRegistry) IsConnected(uint64) bool         { return f.connectedHere }
func (f *fakeRegistry) IsConnectedAnywhere(uint64) bool { return f.connectedAny }
func (f *fakeRegistry) ConnectedNodeIDs() []uint64      { return nil }

func (f *fakeRegistry) SendMetricsRequest(
	_ context.Context, nodeID uint64, requestID string, req *proto.MetricsRequest,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.sendErr != nil {
		return f.sendErr
	}

	f.sentRequest.Store(&sentRequest{nodeID: nodeID, requestID: requestID, req: req})

	return nil
}

type fakeWaiters struct {
	mu            sync.Mutex
	pollWaiters   []string
	remoteWaiters []remoteWaiterEntry
	cancelledIDs  []string
}

type remoteWaiterEntry struct {
	requestID  string
	nodeID     uint64
	instanceID string
}

func (f *fakeWaiters) RegisterPollWaiter(requestID string, _ uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.pollWaiters = append(f.pollWaiters, requestID)
}

func (f *fakeWaiters) RegisterRemoteWaiter(requestID string, nodeID uint64, instanceID string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.remoteWaiters = append(f.remoteWaiters, remoteWaiterEntry{requestID, nodeID, instanceID})
}

func (f *fakeWaiters) CancelWaiter(requestID string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.cancelledIDs = append(f.cancelledIDs, requestID)
}

func (f *fakeWaiters) pollWaiterCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.pollWaiters)
}

func newTestHub(
	t *testing.T, ps pubsub.PubSub, registry Registry, waiters HandlerWaiters, opts Options,
) *hub {
	t.Helper()

	if opts.PollInterval == 0 {
		opts.PollInterval = 30 * time.Millisecond
	}
	if opts.HeartbeatInterval == 0 {
		opts.HeartbeatInterval = 30 * time.Millisecond
	}
	if opts.HeartbeatTTL == 0 {
		opts.HeartbeatTTL = 200 * time.Millisecond
	}
	if opts.StopDebounce == 0 {
		opts.StopDebounce = 30 * time.Millisecond
	}
	if opts.HistoryTimeout == 0 {
		opts.HistoryTimeout = 200 * time.Millisecond
	}

	h := NewHub(ps, registry, waiters, registry.InstanceID(), slog.Default(), opts).(*hub)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		h.Stop()
	})

	require.NoError(t, h.Start(ctx))

	return h
}

func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition never satisfied within %s", time.Second)
}

func TestHub_Subscribe_StartsPollOnHolder(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: true, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{})

	sub, _, err := h.Subscribe(context.Background(), 7, 0)
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	eventually(t, func() bool {
		return waiters.pollWaiterCount() >= 1
	})

	got := registry.sentRequest.Load()
	require.NotNil(t, got)
	assert.Equal(t, uint64(7), got.nodeID)
	assert.NotNil(t, got.req.GetCurrent())
}

func TestHub_Unsubscribe_DebouncedStopOfPoll(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: true, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{
		StopDebounce: 50 * time.Millisecond,
	})

	sub, _, err := h.Subscribe(context.Background(), 11, 0)
	require.NoError(t, err)

	eventually(t, func() bool {
		return waiters.pollWaiterCount() >= 1
	})

	beforeUnsub := waiters.pollWaiterCount()
	sub.Close()

	time.Sleep(150 * time.Millisecond)
	afterDebounce := waiters.pollWaiterCount()

	assert.LessOrEqual(t, afterDebounce-beforeUnsub, 3, "poll should stop within a few ticks of debounce window")
}

func TestHub_LiveSample_FanoutAndRing(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "b", connectedHere: false, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{})

	const nodeID uint64 = 5
	sub, _, err := h.Subscribe(context.Background(), nodeID, 0)
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	resp := &proto.MetricsResponse{
		Timestamp: timestamppb.Now(),
		Series:    []*proto.MetricSeries{{Name: "cpu_usage_percent"}},
	}
	data, err := resp.MarshalVT()
	require.NoError(t, err)

	channel := channels.BuildRealtimeMetricsChannel(nodeID)
	msg, err := messages.NewMessage(channel, messages.TypeMetricsLive, messages.MetricsLivePayload{
		NodeID: nodeID,
		Data:   data,
	})
	require.NoError(t, err)

	require.NoError(t, ps.Publish(context.Background(), channel, msg))

	select {
	case got := <-sub.Samples():
		require.NotNil(t, got)
		require.Len(t, got.Series, 1)
		assert.Equal(t, "cpu_usage_percent", got.Series[0].Name)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live sample")
	}

	state := h.lookupState(nodeID)
	require.NotNil(t, state)
	assert.GreaterOrEqual(t, state.ring.Len(), 1)
}

func TestHub_GetHistory_TimesOut(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: true, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{
		HistoryTimeout: 80 * time.Millisecond,
	})

	_, err := h.GetHistory(context.Background(), 1, time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestHub_GetHistory_NotConnected(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: false, connectedAny: false}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{})

	_, err := h.GetHistory(context.Background(), 1, time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestHub_GetHistory_ResolvesViaResponseChannel(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: true, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{
		HistoryTimeout: time.Second,
	})

	type res struct {
		resp *proto.MetricsResponse
		err  error
	}
	resultCh := make(chan res, 1)

	go func() {
		resp, err := h.GetHistory(context.Background(), 9, time.Minute)
		resultCh <- res{resp, err}
	}()

	eventually(t, func() bool {
		return registry.sentRequest.Load() != nil
	})

	sent := registry.sentRequest.Load()
	require.NotNil(t, sent)
	assert.NotNil(t, sent.req.GetHistory())

	expected := &proto.MetricsResponse{
		Timestamp:           timestamppb.Now(),
		ActualWindowSeconds: 60,
		Series:              []*proto.MetricSeries{{Name: "memory_used_bytes"}},
	}
	data, err := expected.MarshalVT()
	require.NoError(t, err)

	respChannel := channels.BuildDaemonMetricsResponseChannel("a")
	msg, err := messages.NewMessage(respChannel, messages.TypeDaemonMetricsResponse, messages.DaemonMetricsResponsePayload{
		RequestID: sent.requestID,
		NodeID:    9,
		Data:      data,
	})
	require.NoError(t, err)

	require.NoError(t, ps.Publish(context.Background(), respChannel, msg))

	select {
	case r := <-resultCh:
		require.NoError(t, r.err)
		require.NotNil(t, r.resp)
		require.Len(t, r.resp.Series, 1)
		assert.Equal(t, "memory_used_bytes", r.resp.Series[0].Name)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for history result")
	}
}

func TestHub_GetHistory_PropagatesErrorPayload(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: true, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{
		HistoryTimeout: time.Second,
	})

	resultCh := make(chan error, 1)
	go func() {
		_, err := h.GetHistory(context.Background(), 1, time.Minute)
		resultCh <- err
	}()

	eventually(t, func() bool {
		return registry.sentRequest.Load() != nil
	})

	sent := registry.sentRequest.Load()
	respChannel := channels.BuildDaemonMetricsResponseChannel("a")
	msg, err := messages.NewMessage(respChannel, messages.TypeDaemonMetricsResponse, messages.DaemonMetricsResponsePayload{
		RequestID: sent.requestID,
		Error:     "daemon is on fire",
	})
	require.NoError(t, err)

	require.NoError(t, ps.Publish(context.Background(), respChannel, msg))

	select {
	case err := <-resultCh:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "daemon is on fire")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for error result")
	}
}

func TestHub_GetHistory_SendError(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{
		instanceID:    "a",
		connectedHere: true,
		connectedAny:  true,
		sendErr:       errFakeNetworkDown,
	}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{})

	_, err := h.GetHistory(context.Background(), 1, time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network down")
}

func TestHub_RemoteHeartbeat_AggregatesRefcount(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: true, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{})

	const nodeID uint64 = 2

	channel := channels.BuildMetricsSubscribersChannel(nodeID)
	msg, err := messages.NewMessage(channel, messages.TypeMetricsSubscribers, messages.MetricsSubscribersPayload{
		InstanceID: "b",
		NodeID:     nodeID,
		Count:      3,
		Timestamp:  time.Now(),
	})
	require.NoError(t, err)

	require.NoError(t, ps.Publish(context.Background(), channel, msg))

	eventually(t, func() bool {
		return waiters.pollWaiterCount() >= 1
	})

	state := h.lookupState(nodeID)
	require.NotNil(t, state)

	state.mu.Lock()
	defer state.mu.Unlock()
	assert.Equal(t, 3, state.aggregatedCount)
}

func TestHub_LiveSample_DropsPayloadOnNodeIDMismatch(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: false, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{})

	const channelNodeID uint64 = 5
	const spoofedNodeID uint64 = 99

	sub, _, err := h.Subscribe(context.Background(), channelNodeID, 0)
	require.NoError(t, err)
	t.Cleanup(sub.Close)

	resp := &proto.MetricsResponse{
		Timestamp: timestamppb.Now(),
		Series:    []*proto.MetricSeries{{Name: "spoofed_series"}},
	}
	data, err := resp.MarshalVT()
	require.NoError(t, err)

	channel := channels.BuildRealtimeMetricsChannel(channelNodeID)
	msg, err := messages.NewMessage(channel, messages.TypeMetricsLive, messages.MetricsLivePayload{
		NodeID: spoofedNodeID,
		Data:   data,
	})
	require.NoError(t, err)

	require.NoError(t, ps.Publish(context.Background(), channel, msg))

	select {
	case got := <-sub.Samples():
		t.Fatalf("expected no delivery on nodeID mismatch, got %v", got)
	case <-time.After(100 * time.Millisecond):
	}

	if state := h.lookupState(spoofedNodeID); state != nil {
		assert.Equal(t, 0, state.ring.Len(), "spoofed nodeID ring must stay empty")
	}
}

func TestHub_GetHistory_ClampsWindowToMax(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: true, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{
		HistoryTimeout: 100 * time.Millisecond,
	})

	go func() {
		_, _ = h.GetHistory(context.Background(), 1, 1000*time.Hour)
	}()

	eventually(t, func() bool {
		return registry.sentRequest.Load() != nil
	})

	sent := registry.sentRequest.Load()
	require.NotNil(t, sent)
	require.NotNil(t, sent.req.GetHistory())
	assert.Equal(t, uint32(maxHistoryWindow.Seconds()), sent.req.GetHistory().GetSeconds())
}

func TestHub_Stop_ClosesSubscriberChannels(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: true, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{})

	// Two subscribers on distinct nodes: Stop must release every node's state.
	subA, _, err := h.Subscribe(context.Background(), 3, 0)
	require.NoError(t, err)
	subB, _, err := h.Subscribe(context.Background(), 4, 0)
	require.NoError(t, err)

	// ACT — do NOT call sub.Close() here; Stop owns the channel lifecycle so
	// consumers ranging over Samples() unblock on graceful shutdown.
	h.Stop()

	// ASSERT — every subscriber's sample channel is closed.
	for name, sub := range map[string]Subscription{"node-3": subA, "node-4": subB} {
		require.Eventuallyf(t, func() bool {
			select {
			case _, open := <-sub.Samples():
				return !open
			default:
				return false
			}
		}, time.Second, 10*time.Millisecond, "Stop must close the %s subscriber sample channel", name)
	}
}

func TestHub_Stop_StopsPolling(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: true, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{
		PollInterval: 20 * time.Millisecond,
	})

	// A subscriber on a locally-connected node starts the per-node poll loop.
	_, _, err := h.Subscribe(context.Background(), 6, 0)
	require.NoError(t, err)

	eventually(t, func() bool {
		return waiters.pollWaiterCount() >= 1
	})

	// ACT
	h.Stop()

	// Let any poll already in flight at Stop finish registering its waiter.
	time.Sleep(40 * time.Millisecond)
	stable := waiters.pollWaiterCount()

	// ASSERT — with the poll loop cancelled, no further poll waiters appear over
	// several poll intervals.
	require.Never(t, func() bool {
		return waiters.pollWaiterCount() > stable
	}, 120*time.Millisecond, 15*time.Millisecond,
		"Stop must cancel the per-node poll loop so no new poll requests are issued")
}

func TestHub_Stop_IsIdempotent(t *testing.T) {
	t.Parallel()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := &fakeRegistry{instanceID: "a", connectedHere: true, connectedAny: true}
	waiters := &fakeWaiters{}

	h := newTestHub(t, ps, registry, waiters, Options{})

	_, _, err := h.Subscribe(context.Background(), 8, 0)
	require.NoError(t, err)

	// ACT + ASSERT — repeated Stop calls (the explicit one plus newTestHub's
	// cleanup) must not panic by re-closing an already-closed subscriber channel.
	assert.NotPanics(t, func() {
		h.Stop()
		h.Stop()
	}, "Stop must be safe to call more than once")
}
