package metricsbase

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/metrics"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// nodeSeries returns a minimal MetricSeries that matches NodePrefixFilter.
func nodeSeries(name string) *proto.MetricSeries {
	return &proto.MetricSeries{
		Name:   name,
		Type:   proto.MetricType_METRIC_TYPE_GAUGE,
		Unit:   proto.MetricUnit_METRIC_UNIT_PERCENT,
		Points: []*proto.MetricPoint{},
	}
}

// nodeResponse wraps the given series in a MetricsResponse with a fixed timestamp.
func nodeResponse(series ...*proto.MetricSeries) *proto.MetricsResponse {
	return &proto.MetricsResponse{
		Timestamp: timestamppb.New(time.Unix(1700000000, 0)),
		Series:    series,
	}
}

func TestEncodeReplay(t *testing.T) {
	nodeFilter := NodePrefixFilter()
	passAllFilter := func(*proto.MetricSeries) bool { return true }
	dropAllFilter := func(*proto.MetricSeries) bool { return false }

	tests := []struct {
		name    string
		entries []*proto.MetricsResponse
		filter  metrics.SeriesFilter
		wantLen int
	}{
		{
			name:    "empty_input_returns_empty_slice",
			entries: nil,
			filter:  passAllFilter,
			wantLen: 0,
		},
		{
			name: "all_pass_node_filter",
			entries: []*proto.MetricsResponse{
				nodeResponse(nodeSeries("gameap_node_cpu")),
				nodeResponse(nodeSeries("gameap_node_mem")),
				nodeResponse(nodeSeries("gameap_node_disk")),
			},
			filter:  nodeFilter,
			wantLen: 3,
		},
		{
			name: "all_drop_node_filter_returns_empty",
			entries: []*proto.MetricsResponse{
				nodeResponse(nodeSeries("gameap_server_a")),
				nodeResponse(nodeSeries("gameap_server_b")),
			},
			filter:  nodeFilter,
			wantLen: 0,
		},
		{
			name: "partial_match_node_filter",
			entries: []*proto.MetricsResponse{
				nodeResponse(nodeSeries("gameap_node_cpu")),
				nodeResponse(nodeSeries("gameap_server_x")),
				nodeResponse(nodeSeries("gameap_node_disk")),
			},
			filter:  nodeFilter,
			wantLen: 2,
		},
		{
			name: "drop_all_filter_yields_zero",
			entries: []*proto.MetricsResponse{
				nodeResponse(nodeSeries("gameap_node_cpu")),
			},
			filter:  dropAllFilter,
			wantLen: 0,
		},
		{
			name: "pass_all_filter_keeps_every_entry",
			entries: []*proto.MetricsResponse{
				nodeResponse(nodeSeries("anything_here")),
				nodeResponse(nodeSeries("does_not_matter")),
			},
			filter:  passAllFilter,
			wantLen: 2,
		},
		{
			name: "nil_response_is_skipped",
			entries: []*proto.MetricsResponse{
				nil,
				nodeResponse(nodeSeries("gameap_node_cpu")),
				nil,
			},
			filter:  nodeFilter,
			wantLen: 1,
		},
		{
			name: "response_with_empty_series_is_skipped",
			entries: []*proto.MetricsResponse{
				nodeResponse(),
				nodeResponse(nodeSeries("gameap_node_cpu")),
			},
			filter:  nodeFilter,
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE — table case binds entries and filter

			// ACT
			out := encodeReplay(tt.entries, tt.filter)

			// ASSERT
			require.Len(t, out, tt.wantLen)
			assert.NotNil(t, out, "encodeReplay always returns non-nil slice")
			for _, w := range out {
				require.NotNil(t, w, "no nil entries should leak into the slice")
				assert.NotEmpty(t, w.Series, "every wire response must have at least one series")
			}
		})
	}
}

func TestEncodeReplay_PreservesOrder(t *testing.T) {
	// ARRANGE
	entries := []*proto.MetricsResponse{
		nodeResponse(nodeSeries("gameap_node_cpu_first")),
		nodeResponse(nodeSeries("gameap_node_cpu_second")),
		nodeResponse(nodeSeries("gameap_node_cpu_third")),
	}

	// ACT
	out := encodeReplay(entries, NodePrefixFilter())

	// ASSERT
	require.Len(t, out, 3)
	assert.Equal(t, "gameap_node_cpu_first", out[0].Series[0].Name)
	assert.Equal(t, "gameap_node_cpu_second", out[1].Series[0].Name)
	assert.Equal(t, "gameap_node_cpu_third", out[2].Series[0].Name)
}

func TestEncodeReplay_NilFilter_KeepsAllSeries(t *testing.T) {
	// ARRANGE
	entries := []*proto.MetricsResponse{
		nodeResponse(nodeSeries("any_name_at_all")),
	}

	// ACT
	out := encodeReplay(entries, nil)

	// ASSERT
	require.Len(t, out, 1)
	require.Len(t, out[0].Series, 1)
	assert.Equal(t, "any_name_at_all", out[0].Series[0].Name)
}

// ----- Pump tests: real WebSocket plumbing + fake metrics.Hub -----

// silentLogger returns a logger that discards everything so test output stays clean.
func silentLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// fakeHub implements metrics.Hub. Subscribe returns the configured subscription
// and replay slice, or subErr if non-nil.
type fakeHub struct {
	sub        metrics.Subscription
	replay     []*proto.MetricsResponse
	subErr     error
	subscribed atomic.Bool
}

func (f *fakeHub) Start(_ context.Context) error { return nil }

func (f *fakeHub) Subscribe(
	_ context.Context, _ uint64, _ time.Duration,
) (metrics.Subscription, []*proto.MetricsResponse, error) {
	f.subscribed.Store(true)
	if f.subErr != nil {
		return nil, nil, f.subErr
	}

	return f.sub, f.replay, nil
}

func (f *fakeHub) GetHistory(
	_ context.Context, _ uint64, _ time.Duration,
) (*proto.MetricsResponse, error) {
	return nil, nil
}

func (f *fakeHub) Stop() {}

// fakeSub is a metrics.Subscription backed by a buffered channel. Close
// closes the channel exactly once and is safe for concurrent use.
type fakeSub struct {
	ch     chan *proto.MetricsResponse
	closed atomic.Bool
}

func newFakeSub() *fakeSub {
	return &fakeSub{ch: make(chan *proto.MetricsResponse, 8)}
}

func (f *fakeSub) Samples() <-chan *proto.MetricsResponse { return f.ch }

func (f *fakeSub) Close() {
	if f.closed.CompareAndSwap(false, true) {
		close(f.ch)
	}
}

// dialedClient is a real ws.Client whose underlying socket is the server side
// of a WebSocket connection that the test holds the client side of.
type dialedClient struct {
	srvClient *ws.Client
	cliConn   *websocket.Conn
	httpSrv   *httptest.Server
	cleanup   func()
}

// dialClient stands up an httptest server that accepts a WebSocket, wraps the
// server side in a ws.Client, dials it from the client side, and runs the
// Client read/write pumps in the background. The returned cleanup tears
// everything down in the right order.
func dialClient(t *testing.T) *dialedClient {
	t.Helper()

	hub := ws.NewHub(silentLogger())

	type accepted struct {
		client *ws.Client
		err    error
	}
	acceptedCh := make(chan accepted, 1)

	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Accept(w, r, nil)
		if err != nil {
			acceptedCh <- accepted{nil, err}

			return
		}
		client := ws.NewClient(r.Context(), conn, hub, nil, silentLogger())
		hub.Register(client)
		acceptedCh <- accepted{client, nil}
		client.Run()
	}))

	dialCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http")
	cliConn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	got := <-acceptedCh
	require.NoError(t, got.err, "server-side accept must succeed")
	require.NotNil(t, got.client)

	d := &dialedClient{
		srvClient: got.client,
		cliConn:   cliConn,
		httpSrv:   httpSrv,
	}
	d.cleanup = func() {
		_ = cliConn.Close(websocket.StatusNormalClosure, "")
		got.client.Close()
		httpSrv.Close()
	}
	t.Cleanup(d.cleanup)

	return d
}

// readFrame reads the next text frame and decodes it into a wsFrame. Returns
// ok=false on read error or timeout.
type wsFrame struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	Error   string          `json:"error"`
	Ts      int64           `json:"ts"`
}

func readFrame(t *testing.T, c *websocket.Conn, timeout time.Duration) (wsFrame, bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, data, err := c.Read(ctx)
	if err != nil {
		return wsFrame{}, false
	}

	var f wsFrame
	require.NoError(t, json.Unmarshal(data, &f))

	return f, true
}

func TestPump_HubSubscribeError_SendsErrorMessage(t *testing.T) {
	// ARRANGE
	d := dialClient(t)
	hub := &fakeHub{subErr: errors.New("subscribe nope")}

	// ACT
	Pump(t.Context(), hub, 7, NodePrefixFilter(), d.srvClient, time.Minute, silentLogger())

	// ASSERT
	frame, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok, "client must receive an error frame")
	assert.Equal(t, TypeError, frame.Type)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(frame.Payload, &payload))
	assert.Equal(t, "subscribe nope", payload["error"], "payload.error must echo the hub's error message")

	if _, ok := readFrame(t, d.cliConn, 100*time.Millisecond); ok {
		t.Fatal("no further frames expected after subscribe error")
	}
	assert.True(t, hub.subscribed.Load(), "hub.Subscribe must have been called")
}

func TestPump_NoReplay_SendsReplayDoneOnly(t *testing.T) {
	// ARRANGE
	d := dialClient(t)
	sub := newFakeSub()
	hub := &fakeHub{sub: sub, replay: nil}

	// ACT
	Pump(t.Context(), hub, 1, NodePrefixFilter(), d.srvClient, time.Minute, silentLogger())

	// ASSERT
	frame, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok)
	assert.Equal(t, TypeReplayDone, frame.Type, "first frame must be replay-done when replay is empty")

	if extra, ok := readFrame(t, d.cliConn, 100*time.Millisecond); ok {
		t.Fatalf("no live frames expected, got type=%q", extra.Type)
	}

	sub.Close()
}

func TestPump_WithReplay_SendsReplayThenDone(t *testing.T) {
	// ARRANGE
	d := dialClient(t)
	sub := newFakeSub()
	hub := &fakeHub{
		sub: sub,
		replay: []*proto.MetricsResponse{
			nodeResponse(nodeSeries("gameap_node_cpu")),
		},
	}

	// ACT
	Pump(t.Context(), hub, 9, NodePrefixFilter(), d.srvClient, time.Minute, silentLogger())

	// ASSERT
	first, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok)
	assert.Equal(t, TypeReplay, first.Type, "first frame must be the replay payload")

	var replayPayload []*metrics.WireResponse
	require.NoError(t, json.Unmarshal(first.Payload, &replayPayload))
	require.Len(t, replayPayload, 1)
	require.Len(t, replayPayload[0].Series, 1)
	assert.Equal(t, "gameap_node_cpu", replayPayload[0].Series[0].Name)

	second, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok)
	assert.Equal(t, TypeReplayDone, second.Type, "second frame must signal replay completion")

	sub.Close()
}

func TestPump_LiveSamples_ForwardedThroughFilter(t *testing.T) {
	// ARRANGE
	d := dialClient(t)
	sub := newFakeSub()
	hub := &fakeHub{sub: sub}

	// ACT
	Pump(t.Context(), hub, 9, NodePrefixFilter(), d.srvClient, time.Minute, silentLogger())

	done, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok)
	require.Equal(t, TypeReplayDone, done.Type)

	sub.ch <- nodeResponse(nodeSeries("gameap_node_cpu"))

	// ASSERT — node sample is delivered as live metrics
	live, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok)
	assert.Equal(t, TypeMetrics, live.Type)

	var liveWire metrics.WireResponse
	require.NoError(t, json.Unmarshal(live.Payload, &liveWire))
	require.Len(t, liveWire.Series, 1)
	assert.Equal(t, "gameap_node_cpu", liveWire.Series[0].Name)

	// ASSERT — non-node sample is dropped by the filter (wire is nil) and no frame is emitted
	sub.ch <- nodeResponse(nodeSeries("gameap_server_x"))

	if extra, ok := readFrame(t, d.cliConn, 150*time.Millisecond); ok {
		t.Fatalf("filter must drop non-node series; unexpected frame type=%q", extra.Type)
	}

	sub.Close()
}

func TestPump_ContextCancelled_StopsForwarding(t *testing.T) {
	// ARRANGE
	d := dialClient(t)
	sub := newFakeSub()
	hub := &fakeHub{sub: sub}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// ACT
	Pump(ctx, hub, 1, NodePrefixFilter(), d.srvClient, time.Minute, silentLogger())

	done, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok)
	require.Equal(t, TypeReplayDone, done.Type)

	cancel()

	// ASSERT — once the parent ctx is cancelled, the goroutine drains and closes the
	// subscription via its deferred Close. No live frames are emitted.
	if extra, ok := readFrame(t, d.cliConn, 250*time.Millisecond); ok {
		t.Fatalf("after ctx cancel forwarding must stop; got frame type=%q", extra.Type)
	}

	require.Eventually(t, func() bool {
		return sub.closed.Load()
	}, time.Second, 10*time.Millisecond, "Pump goroutine must close the subscription on ctx cancel")
}

func TestPump_ClientClosed_StopsForwarding(t *testing.T) {
	// ARRANGE
	d := dialClient(t)
	sub := newFakeSub()
	hub := &fakeHub{sub: sub}

	// ACT
	Pump(t.Context(), hub, 1, NodePrefixFilter(), d.srvClient, time.Minute, silentLogger())

	done, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok)
	require.Equal(t, TypeReplayDone, done.Type)

	d.srvClient.Close()

	// ASSERT — closing the client tears down the connection so further reads error out
	if extra, ok := readFrame(t, d.cliConn, 500*time.Millisecond); ok {
		t.Fatalf("client.Close must end forwarding; got frame type=%q", extra.Type)
	}

	sub.Close()
}

func TestPump_SubscriptionChannelClosed_Exits(t *testing.T) {
	// ARRANGE
	d := dialClient(t)
	sub := newFakeSub()
	hub := &fakeHub{sub: sub}

	// ACT
	Pump(t.Context(), hub, 1, NodePrefixFilter(), d.srvClient, time.Minute, silentLogger())

	done, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok)
	require.Equal(t, TypeReplayDone, done.Type)

	sub.Close()

	// ASSERT — once the subscription channel is closed, no further metric frames arrive
	if extra, ok := readFrame(t, d.cliConn, 200*time.Millisecond); ok {
		t.Fatalf("closed subscription must stop forwarding; got frame type=%q", extra.Type)
	}
}

func TestPump_LiveSamplesUseCustomFilter(t *testing.T) {
	// ARRANGE
	d := dialClient(t)
	sub := newFakeSub()
	hub := &fakeHub{sub: sub}

	// Filter that accepts only series with a specific Name.
	filter := func(s *proto.MetricSeries) bool {
		return s.GetName() == "specific_keep_me"
	}

	// ACT
	Pump(t.Context(), hub, 1, filter, d.srvClient, time.Minute, silentLogger())

	done, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok)
	require.Equal(t, TypeReplayDone, done.Type)

	sub.ch <- nodeResponse(nodeSeries("specific_keep_me"))
	sub.ch <- nodeResponse(nodeSeries("drop_this_one"))

	// ASSERT — only the kept series is forwarded
	live, ok := readFrame(t, d.cliConn, 2*time.Second)
	require.True(t, ok)
	assert.Equal(t, TypeMetrics, live.Type)

	var wire metrics.WireResponse
	require.NoError(t, json.Unmarshal(live.Payload, &wire))
	require.Len(t, wire.Series, 1)
	assert.Equal(t, "specific_keep_me", wire.Series[0].Name)

	if extra, ok := readFrame(t, d.cliConn, 150*time.Millisecond); ok {
		t.Fatalf("custom filter must drop the second sample; got frame type=%q", extra.Type)
	}

	sub.Close()
}

// Compile-time check: fakeHub satisfies metrics.Hub.
var _ metrics.Hub = (*fakeHub)(nil)

// Compile-time check: fakeSub satisfies metrics.Subscription.
var _ metrics.Subscription = (*fakeSub)(nil)
