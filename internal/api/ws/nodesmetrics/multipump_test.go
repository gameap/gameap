package nodesmetrics

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/ws/metricsbase"
	"github.com/gameap/gameap/internal/metrics"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var errSubscribe = errors.New("subscribe failed")

func TestTagNodeID_AddsLabelWithoutSharingMap(t *testing.T) {
	t.Parallel()
	original := &proto.MetricsResponse{
		Timestamp:    timestamppb.New(time.Unix(1700000000, 0)),
		CommonLabels: map[string]string{"foo": "bar"},
	}

	tagged := tagNodeID(original, 42)

	require.NotNil(t, tagged)
	assert.Equal(t, "42", tagged.GetCommonLabels()["node_id"])
	assert.Equal(t, "bar", tagged.GetCommonLabels()["foo"])

	_, hadID := original.GetCommonLabels()["node_id"]
	assert.False(t, hadID, "must not mutate the source CommonLabels")
}

func TestTagNodeID_NilResponse(t *testing.T) {
	t.Parallel()
	assert.Nil(t, tagNodeID(nil, 1))
}

func TestPumpAll_NoNodes_SendsReplayDoneOnly(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	hub := newFakeHub()
	sink := newFakeSink()

	pumpAll(ctx, hub, nil, sink, silentLogger())

	msgs := sink.collect(50 * time.Millisecond)
	require.Len(t, msgs, 1)
	assert.Equal(t, metricsbase.TypeReplayDone, msgs[0].Type)
	assert.Equal(t, 0, hub.subscribeCalls())
}

func TestPumpAll_TagsEachEnvelopeWithNodeID(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	hub := newFakeHub()
	sink := newFakeSink()

	pumpAll(ctx, hub, []uint64{10, 20}, sink, silentLogger())

	hub.publish(10, simpleResponse())
	hub.publish(20, simpleResponse())

	msgs := sink.collect(200 * time.Millisecond)

	var seenNodeIDs []string
	for _, m := range msgs {
		if m.Type != metricsbase.TypeMetrics {
			continue
		}
		wire, ok := m.Payload.(*metrics.WireResponse)
		require.True(t, ok, "expected payload to be *metrics.WireResponse")
		seenNodeIDs = append(seenNodeIDs, wire.CommonLabels["node_id"])
	}

	assert.ElementsMatch(t, []string{"10", "20"}, seenNodeIDs)
}

func TestPumpAll_ClosesSubscriptionsOnClientDone(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	hub := newFakeHub()
	sink := newFakeSink()

	pumpAll(ctx, hub, []uint64{1, 2, 3}, sink, silentLogger())

	assert.Equal(t, 3, hub.subscribeCalls())

	sink.signalDone()

	require.Eventually(t, func() bool {
		return hub.closedSubs() == 3
	}, time.Second, 10*time.Millisecond)
}

func TestPumpAll_DropsNonNodeSeries(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	hub := newFakeHub()
	sink := newFakeSink()

	pumpAll(ctx, hub, []uint64{10}, sink, silentLogger())

	hub.publish(10, mixedResponse())

	msgs := sink.collect(200 * time.Millisecond)

	var liveWires []*metrics.WireResponse
	for _, m := range msgs {
		if m.Type != metricsbase.TypeMetrics {
			continue
		}
		wire, ok := m.Payload.(*metrics.WireResponse)
		require.True(t, ok, "expected payload to be *metrics.WireResponse")
		liveWires = append(liveWires, wire)
	}

	require.Len(t, liveWires, 1)
	require.Len(t, liveWires[0].Series, 1)
	assert.Equal(t, "gameap_node_cpu_usage_percent", liveWires[0].Series[0].Name)
}

func TestPumpAll_AllSeriesFiltered_NoEnvelopeSent(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	hub := newFakeHub()
	sink := newFakeSink()

	pumpAll(ctx, hub, []uint64{10}, sink, silentLogger())

	hub.publish(10, serverOnlyResponse())

	msgs := sink.collect(200 * time.Millisecond)

	var liveCount int
	for _, m := range msgs {
		if m.Type == metricsbase.TypeMetrics {
			liveCount++
		}
	}
	assert.Equal(t, 0, liveCount, "envelopes with no surviving series must be dropped")
}

func TestPumpAll_HubSubscribeError_SkipsFailingNode(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	hub := newFakeHub()
	hub.subscribeErrFor = map[uint64]error{2: errSubscribe}
	sink := newFakeSink()

	pumpAll(ctx, hub, []uint64{1, 2, 3}, sink, silentLogger())

	hub.publish(1, simpleResponse())
	hub.publish(3, simpleResponse())

	msgs := sink.collect(200 * time.Millisecond)

	var liveCount int
	for _, m := range msgs {
		if m.Type == metricsbase.TypeMetrics {
			liveCount++
		}
	}
	assert.Equal(t, 2, liveCount, "only successful subscriptions forward samples")
}

// ----- helpers -----

func cpuPoint(v float64) *proto.MetricPoint {
	return &proto.MetricPoint{
		Timestamp: timestamppb.New(time.Unix(1700000000, 0)),
		Value:     &proto.MetricPoint_DoubleValue{DoubleValue: v},
	}
}

func simpleResponse() *proto.MetricsResponse {
	return &proto.MetricsResponse{
		Timestamp: timestamppb.New(time.Unix(1700000000, 0)),
		Series: []*proto.MetricSeries{{
			Name:   "gameap_node_cpu_usage_percent",
			Type:   proto.MetricType_METRIC_TYPE_GAUGE,
			Unit:   proto.MetricUnit_METRIC_UNIT_PERCENT,
			Points: []*proto.MetricPoint{cpuPoint(0.5)},
		}},
	}
}

func mixedResponse() *proto.MetricsResponse {
	return &proto.MetricsResponse{
		Timestamp: timestamppb.New(time.Unix(1700000000, 0)),
		Series: []*proto.MetricSeries{
			{
				Name:   "gameap_node_cpu_usage_percent",
				Type:   proto.MetricType_METRIC_TYPE_GAUGE,
				Unit:   proto.MetricUnit_METRIC_UNIT_PERCENT,
				Points: []*proto.MetricPoint{cpuPoint(0.5)},
			},
			{
				Name:   "gameap_server_memory_limit_bytes",
				Type:   proto.MetricType_METRIC_TYPE_GAUGE,
				Unit:   proto.MetricUnit_METRIC_UNIT_BYTES,
				Labels: map[string]string{"server_id": "27"},
				Points: []*proto.MetricPoint{cpuPoint(2069725184)},
			},
			{
				Name:   "gameap_server_process_pids",
				Type:   proto.MetricType_METRIC_TYPE_GAUGE,
				Unit:   proto.MetricUnit_METRIC_UNIT_COUNT,
				Labels: map[string]string{"server_id": "27"},
				Points: []*proto.MetricPoint{cpuPoint(3)},
			},
		},
	}
}

func serverOnlyResponse() *proto.MetricsResponse {
	return &proto.MetricsResponse{
		Timestamp: timestamppb.New(time.Unix(1700000000, 0)),
		Series: []*proto.MetricSeries{{
			Name:   "gameap_server_memory_limit_bytes",
			Type:   proto.MetricType_METRIC_TYPE_GAUGE,
			Unit:   proto.MetricUnit_METRIC_UNIT_BYTES,
			Labels: map[string]string{"server_id": "27"},
			Points: []*proto.MetricPoint{cpuPoint(2069725184)},
		}},
	}
}

func silentLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

type fakeSink struct {
	mu       sync.Mutex
	messages []*ws.OutboundMessage
	done     chan struct{}
}

func newFakeSink() *fakeSink {
	return &fakeSink{done: make(chan struct{})}
}

func (s *fakeSink) SendMessage(msg *ws.OutboundMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
}

func (s *fakeSink) Done() <-chan struct{} {
	return s.done
}

func (s *fakeSink) signalDone() {
	close(s.done)
}

func (s *fakeSink) collect(wait time.Duration) []*ws.OutboundMessage {
	time.Sleep(wait)

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*ws.OutboundMessage, len(s.messages))
	copy(out, s.messages)

	return out
}

type fakeHub struct {
	mu              sync.Mutex
	subscribes      int
	subscribeErrFor map[uint64]error
	subs            map[uint64]*fakeSubscription
	closedCount     int
}

func newFakeHub() *fakeHub {
	return &fakeHub{subs: make(map[uint64]*fakeSubscription)}
}

func (h *fakeHub) Start(_ context.Context) error { return nil }

func (h *fakeHub) Subscribe(
	_ context.Context, nodeID uint64, _ time.Duration,
) (metrics.Subscription, []*proto.MetricsResponse, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.subscribes++
	if err, ok := h.subscribeErrFor[nodeID]; ok {
		return nil, nil, err
	}

	sub := &fakeSubscription{
		samples: make(chan *proto.MetricsResponse, 8),
		onClose: h.recordClose,
	}
	h.subs[nodeID] = sub

	return sub, nil, nil
}

func (h *fakeHub) GetHistory(
	_ context.Context, _ uint64, _ time.Duration,
) (*proto.MetricsResponse, error) {
	return nil, nil
}

func (h *fakeHub) Stop() {}

func (h *fakeHub) publish(nodeID uint64, resp *proto.MetricsResponse) {
	h.mu.Lock()
	sub, ok := h.subs[nodeID]
	h.mu.Unlock()

	if !ok {
		return
	}
	sub.samples <- resp
}

func (h *fakeHub) subscribeCalls() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.subscribes
}

func (h *fakeHub) closedSubs() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.closedCount
}

func (h *fakeHub) recordClose() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.closedCount++
}

type fakeSubscription struct {
	samples chan *proto.MetricsResponse
	onClose func()
	closed  bool
	mu      sync.Mutex
}

func (s *fakeSubscription) Samples() <-chan *proto.MetricsResponse {
	return s.samples
}

func (s *fakeSubscription) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true
	if s.onClose != nil {
		s.onClose()
	}
	close(s.samples)
}
