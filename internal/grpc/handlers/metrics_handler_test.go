package handlers

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestMetricsHandler_HandleMetricsResponse(t *testing.T) {
	t.Run("poll_waiter_publishes_to_realtime_channel", func(t *testing.T) {
		ps := memory.New()
		t.Cleanup(func() { _ = ps.Close() })

		const nodeID uint64 = 42
		const requestID = "poll-req-1"

		var (
			received   *pubsub.Message
			receivedMu sync.Mutex
			done       = make(chan struct{})
		)

		err := ps.Subscribe(context.Background(), channels.RealtimeMetricsAll, func(_ context.Context, msg *pubsub.Message) error {
			receivedMu.Lock()
			received = msg
			receivedMu.Unlock()
			close(done)

			return nil
		})
		require.NoError(t, err)

		handler := NewMetricsHandler(ps, nil, slog.Default())
		handler.RegisterPollWaiter(requestID, nodeID)

		resp := &proto.MetricsResponse{
			Timestamp:    timestamppb.Now(),
			CommonLabels: map[string]string{"env": "prod"},
			Series: []*proto.MetricSeries{
				{
					Name: "cpu_usage_percent",
					Type: proto.MetricType_METRIC_TYPE_GAUGE,
					Unit: proto.MetricUnit_METRIC_UNIT_PERCENT,
					Points: []*proto.MetricPoint{
						{
							Timestamp: timestamppb.Now(),
							Value:     &proto.MetricPoint_DoubleValue{DoubleValue: 42.5},
						},
					},
				},
			},
		}

		err = handler.HandleMetricsResponse(context.Background(), nodeID, requestID, resp)
		require.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for pubsub delivery")
		}

		receivedMu.Lock()
		defer receivedMu.Unlock()

		require.NotNil(t, received)
		assert.Equal(t, channels.BuildRealtimeMetricsChannel(nodeID), received.Channel)
		assert.Equal(t, messages.TypeMetricsLive, received.Type)

		payload, err := messages.ParsePayload[messages.MetricsLivePayload](received)
		require.NoError(t, err)
		assert.Equal(t, nodeID, payload.NodeID)
		require.NotEmpty(t, payload.Data)

		var decoded proto.MetricsResponse
		require.NoError(t, decoded.UnmarshalVT(payload.Data))
		require.Len(t, decoded.Series, 1)
		assert.Equal(t, "cpu_usage_percent", decoded.Series[0].Name)
	})

	t.Run("remote_waiter_publishes_to_response_channel", func(t *testing.T) {
		ps := memory.New()
		t.Cleanup(func() { _ = ps.Close() })

		const nodeID uint64 = 7
		const requestID = "remote-req-1"
		const requesterInstanceID = "instance-b"

		var (
			received   *pubsub.Message
			receivedMu sync.Mutex
			done       = make(chan struct{})
		)

		err := ps.Subscribe(context.Background(), channels.BuildDaemonMetricsResponseChannel(requesterInstanceID),
			func(_ context.Context, msg *pubsub.Message) error {
				receivedMu.Lock()
				received = msg
				receivedMu.Unlock()
				close(done)

				return nil
			})
		require.NoError(t, err)

		handler := NewMetricsHandler(ps, nil, slog.Default())
		handler.RegisterRemoteWaiter(requestID, nodeID, requesterInstanceID)

		resp := &proto.MetricsResponse{
			Timestamp:           timestamppb.Now(),
			ActualWindowSeconds: 60,
			Series:              []*proto.MetricSeries{{Name: "memory_used_bytes"}},
		}

		err = handler.HandleMetricsResponse(context.Background(), nodeID, requestID, resp)
		require.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for pubsub delivery")
		}

		receivedMu.Lock()
		defer receivedMu.Unlock()

		require.NotNil(t, received)
		assert.Equal(t, channels.BuildDaemonMetricsResponseChannel(requesterInstanceID), received.Channel)
		assert.Equal(t, messages.TypeDaemonMetricsResponse, received.Type)

		payload, err := messages.ParsePayload[messages.DaemonMetricsResponsePayload](received)
		require.NoError(t, err)
		assert.Equal(t, requestID, payload.RequestID)
		assert.Equal(t, nodeID, payload.NodeID)
		require.NotEmpty(t, payload.Data)
	})

	t.Run("unknown_request_id_is_dropped", func(t *testing.T) {
		ps := memory.New()
		t.Cleanup(func() { _ = ps.Close() })

		published := make(chan struct{}, 1)
		err := ps.Subscribe(context.Background(), channels.RealtimeMetricsAll, func(_ context.Context, _ *pubsub.Message) error {
			published <- struct{}{}

			return nil
		})
		require.NoError(t, err)

		handler := NewMetricsHandler(ps, nil, slog.Default())

		err = handler.HandleMetricsResponse(context.Background(), 99, "unknown", &proto.MetricsResponse{})
		require.NoError(t, err)

		select {
		case <-published:
			t.Fatal("unexpected publish for unknown request_id")
		case <-time.After(50 * time.Millisecond):
		}
	})

	t.Run("cancel_waiter_drops_subsequent_response", func(t *testing.T) {
		ps := memory.New()
		t.Cleanup(func() { _ = ps.Close() })

		const requestID = "cancel-1"

		published := make(chan struct{}, 1)
		err := ps.Subscribe(context.Background(), channels.RealtimeMetricsAll, func(_ context.Context, _ *pubsub.Message) error {
			published <- struct{}{}

			return nil
		})
		require.NoError(t, err)

		handler := NewMetricsHandler(ps, nil, slog.Default())
		handler.RegisterPollWaiter(requestID, 1)
		handler.CancelWaiter(requestID)

		err = handler.HandleMetricsResponse(context.Background(), 1, requestID, &proto.MetricsResponse{})
		require.NoError(t, err)

		select {
		case <-published:
			t.Fatal("unexpected publish after waiter cancel")
		case <-time.After(50 * time.Millisecond):
		}
	})

	t.Run("nil_publisher_is_noop", func(t *testing.T) {
		handler := NewMetricsHandler(nil, nil, slog.Default())
		handler.RegisterPollWaiter("x", 1)

		err := handler.HandleMetricsResponse(context.Background(), 1, "x", &proto.MetricsResponse{})
		assert.NoError(t, err)
	})

	t.Run("drops_series_for_servers_not_on_this_node", func(t *testing.T) {
		ps := memory.New()
		t.Cleanup(func() { _ = ps.Close() })

		const nodeID uint64 = 5
		const requestID = "label-validation-1"

		serverRepo := inmemory.NewServerRepository()
		require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{ID: 10, DSID: uint(nodeID)}))
		require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{ID: 20, DSID: uint(nodeID)}))
		require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{ID: 99, DSID: 999}))

		var (
			received   *pubsub.Message
			receivedMu sync.Mutex
			done       = make(chan struct{})
		)
		err := ps.Subscribe(context.Background(), channels.RealtimeMetricsAll,
			func(_ context.Context, msg *pubsub.Message) error {
				receivedMu.Lock()
				received = msg
				receivedMu.Unlock()
				close(done)

				return nil
			})
		require.NoError(t, err)

		handler := NewMetricsHandler(ps, serverRepo, slog.Default())
		handler.RegisterPollWaiter(requestID, nodeID)

		resp := &proto.MetricsResponse{
			Timestamp: timestamppb.Now(),
			Series: []*proto.MetricSeries{
				{Name: "gameap_server_cpu", Labels: map[string]string{"server_id": "10"}},
				{Name: "gameap_server_cpu", Labels: map[string]string{"server_id": "20"}},
				{Name: "gameap_server_cpu", Labels: map[string]string{"server_id": "99"}},
				{Name: "gameap_node_cpu", Labels: map[string]string{"host": "n1"}},
			},
		}

		err = handler.HandleMetricsResponse(context.Background(), nodeID, requestID, resp)
		require.NoError(t, err)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for pubsub delivery")
		}

		receivedMu.Lock()
		defer receivedMu.Unlock()

		require.NotNil(t, received)
		payload, err := messages.ParsePayload[messages.MetricsLivePayload](received)
		require.NoError(t, err)

		var decoded proto.MetricsResponse
		require.NoError(t, decoded.UnmarshalVT(payload.Data))
		require.Len(t, decoded.Series, 3)

		gotIDs := make([]string, 0, len(decoded.Series))
		gotNoLabel := 0
		for _, s := range decoded.Series {
			raw, ok := s.GetLabels()["server_id"]
			if !ok {
				gotNoLabel++

				continue
			}
			gotIDs = append(gotIDs, raw)
		}
		assert.ElementsMatch(t, []string{"10", "20"}, gotIDs)
		assert.Equal(t, 1, gotNoLabel)
	})

	t.Run("drops_publish_when_all_series_invalid", func(t *testing.T) {
		ps := memory.New()
		t.Cleanup(func() { _ = ps.Close() })

		const nodeID uint64 = 5
		const requestID = "label-validation-2"

		serverRepo := inmemory.NewServerRepository()
		require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{ID: 10, DSID: 999}))

		published := make(chan struct{}, 1)
		err := ps.Subscribe(context.Background(), channels.RealtimeMetricsAll,
			func(_ context.Context, _ *pubsub.Message) error {
				published <- struct{}{}

				return nil
			})
		require.NoError(t, err)

		handler := NewMetricsHandler(ps, serverRepo, slog.Default())
		handler.RegisterPollWaiter(requestID, nodeID)

		resp := &proto.MetricsResponse{
			Timestamp: timestamppb.Now(),
			Series: []*proto.MetricSeries{
				{Name: "gameap_server_cpu", Labels: map[string]string{"server_id": "10"}},
				{Name: "gameap_server_cpu", Labels: map[string]string{"server_id": "11"}},
			},
		}

		err = handler.HandleMetricsResponse(context.Background(), nodeID, requestID, resp)
		require.NoError(t, err)

		select {
		case <-published:
			t.Fatal("expected no publish when all series dropped")
		case <-time.After(50 * time.Millisecond):
		}
	})
}

func TestMetricsHandler_pollWaiterExpires(t *testing.T) {
	handler := NewMetricsHandler(memory.New(), nil, slog.Default())
	handler.SetWaiterTTL(20 * time.Millisecond)

	handler.RegisterPollWaiter("req-1", 42)

	handler.waitersMu.Lock()
	require.Len(t, handler.waiters, 1, "waiter must be registered")
	handler.waitersMu.Unlock()

	require.Eventually(t, func() bool {
		handler.waitersMu.Lock()
		defer handler.waitersMu.Unlock()

		return len(handler.waiters) == 0
	}, time.Second, 5*time.Millisecond, "unanswered poll waiter must be reaped by its TTL")
}

func TestMetricsHandler_responseStopsReaper(t *testing.T) {
	handler := NewMetricsHandler(memory.New(), nil, slog.Default())
	handler.SetWaiterTTL(20 * time.Millisecond)

	handler.RegisterPollWaiter("req-1", 42)
	handler.CancelWaiter("req-1")

	handler.waitersMu.Lock()
	got := len(handler.waiters)
	handler.waitersMu.Unlock()
	assert.Equal(t, 0, got, "cancel must remove the waiter")
}
