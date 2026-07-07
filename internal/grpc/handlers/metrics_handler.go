package handlers

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/proto"
)

// defaultWaiterTTL bounds how long a metrics request waiter lives without a
// response. A connected-but-silent daemon (dropped reply, reconnect gap) would
// otherwise leave the waiter in the map forever while a fresh one is registered
// every poll interval — an unbounded leak.
const defaultWaiterTTL = 30 * time.Second

type MetricsHandler struct {
	publisher  pubsub.Publisher
	serverRepo repositories.ServerRepository
	logger     *slog.Logger
	waiterTTL  time.Duration

	waitersMu sync.Mutex
	waiters   map[string]metricsWaiter
}

type metricsWaiter struct {
	nodeID           uint64
	remoteInstanceID string
	timer            *time.Timer
}

func NewMetricsHandler(
	publisher pubsub.Publisher,
	serverRepo repositories.ServerRepository,
	logger *slog.Logger,
) *MetricsHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &MetricsHandler{
		publisher:  publisher,
		serverRepo: serverRepo,
		logger:     logger,
		waiterTTL:  defaultWaiterTTL,
		waiters:    make(map[string]metricsWaiter),
	}
}

// SetWaiterTTL overrides the waiter reaping timeout. Intended for tests that
// need the reaper to fire quickly.
func (h *MetricsHandler) SetWaiterTTL(ttl time.Duration) {
	if ttl > 0 {
		h.waiterTTL = ttl
	}
}

func (h *MetricsHandler) RegisterPollWaiter(requestID string, nodeID uint64) {
	h.registerWaiter(requestID, metricsWaiter{nodeID: nodeID})
}

func (h *MetricsHandler) RegisterRemoteWaiter(requestID string, nodeID uint64, requesterInstanceID string) {
	h.registerWaiter(requestID, metricsWaiter{
		nodeID:           nodeID,
		remoteInstanceID: requesterInstanceID,
	})
}

func (h *MetricsHandler) registerWaiter(requestID string, waiter metricsWaiter) {
	h.waitersMu.Lock()
	defer h.waitersMu.Unlock()

	if existing, ok := h.waiters[requestID]; ok && existing.timer != nil {
		existing.timer.Stop()
	}

	waiter.timer = time.AfterFunc(h.waiterTTL, func() { h.expireWaiter(requestID) })
	h.waiters[requestID] = waiter
}

func (h *MetricsHandler) expireWaiter(requestID string) {
	h.waitersMu.Lock()
	defer h.waitersMu.Unlock()

	if _, ok := h.waiters[requestID]; ok {
		delete(h.waiters, requestID)
		h.logger.Debug("metrics waiter expired without response", "request_id", requestID)
	}
}

// removeWaiterLocked deletes a waiter and stops its reaper timer. Caller holds
// waitersMu.
func (h *MetricsHandler) removeWaiterLocked(requestID string) (metricsWaiter, bool) {
	waiter, ok := h.waiters[requestID]
	if ok {
		if waiter.timer != nil {
			waiter.timer.Stop()
		}
		delete(h.waiters, requestID)
	}

	return waiter, ok
}

func (h *MetricsHandler) CancelWaiter(requestID string) {
	h.waitersMu.Lock()
	defer h.waitersMu.Unlock()

	h.removeWaiterLocked(requestID)
}

func (h *MetricsHandler) HandleMetricsResponse(
	ctx context.Context, nodeID uint64, requestID string, resp *proto.MetricsResponse,
) error {
	h.waitersMu.Lock()
	waiter, ok := h.removeWaiterLocked(requestID)
	h.waitersMu.Unlock()

	if !ok {
		h.logger.Warn("metrics response with unknown request_id",
			"node_id", nodeID,
			"request_id", requestID,
		)

		return nil
	}

	if h.publisher == nil {
		return nil
	}

	if !h.filterUntrustedSeries(ctx, nodeID, resp) {
		return nil
	}

	if len(resp.Series) == 0 {
		return nil
	}

	data, err := resp.MarshalVT()
	if err != nil {
		h.logger.Warn("failed to marshal metrics response",
			"node_id", nodeID,
			"error", err,
		)

		return nil
	}

	if waiter.remoteInstanceID == "" {
		return h.publishLiveFanout(ctx, nodeID, data)
	}

	return h.publishRemoteReply(ctx, waiter.remoteInstanceID, requestID, nodeID, data)
}

// filterUntrustedSeries drops MetricSeries whose server_id label points to a
// server that does not belong to the daemon's own node — a compromised daemon
// must not be able to fabricate metrics for other tenants' servers. Series
// without a server_id label (machine-level metrics) are kept untouched.
//
// Returns false if the lookup itself failed, in which case nothing should be
// published — fail closed rather than letting potentially-bad data through.
func (h *MetricsHandler) filterUntrustedSeries(
	ctx context.Context, nodeID uint64, resp *proto.MetricsResponse,
) bool {
	if h.serverRepo == nil || resp == nil || len(resp.Series) == 0 {
		return true
	}

	hasServerLabel := false
	for _, s := range resp.Series {
		if _, ok := s.GetLabels()["server_id"]; ok {
			hasServerLabel = true

			break
		}
	}
	if !hasServerLabel {
		return true
	}

	servers, err := h.serverRepo.Find(ctx, filters.FindServerByNodeIDs(uint(nodeID)), nil, nil)
	if err != nil {
		h.logger.Warn("failed to lookup servers for metric label validation",
			"node_id", nodeID,
			"error", err,
		)

		return false
	}

	allowed := make(map[string]struct{}, len(servers))
	for i := range servers {
		allowed[strconv.FormatUint(uint64(servers[i].ID), 10)] = struct{}{}
	}

	kept := resp.Series[:0]
	for _, s := range resp.Series {
		raw, hasLabel := s.GetLabels()["server_id"]
		if hasLabel {
			if _, ok := allowed[raw]; !ok {
				h.logger.Warn("dropping metric series for server not on this node",
					"node_id", nodeID,
					"claimed_server_id", raw,
					"metric", s.GetName(),
				)

				continue
			}
		}
		kept = append(kept, s)
	}
	resp.Series = kept

	return true
}

func (h *MetricsHandler) publishLiveFanout(ctx context.Context, nodeID uint64, data []byte) error {
	channel := channels.BuildRealtimeMetricsChannel(nodeID)

	msg, err := messages.NewMessage(channel, messages.TypeMetricsLive, messages.MetricsLivePayload{
		NodeID: nodeID,
		Data:   data,
	})
	if err != nil {
		h.logger.Warn("failed to create metrics live message",
			"node_id", nodeID,
			"error", err,
		)

		return nil
	}

	if err := h.publisher.Publish(ctx, channel, msg); err != nil {
		h.logger.Warn("failed to publish metrics live",
			"node_id", nodeID,
			"error", err,
		)
	}

	return nil
}

func (h *MetricsHandler) publishRemoteReply(
	ctx context.Context, requesterInstanceID, requestID string, nodeID uint64, data []byte,
) error {
	channel := channels.BuildDaemonMetricsResponseChannel(requesterInstanceID)

	msg, err := messages.NewMessage(channel, messages.TypeDaemonMetricsResponse, messages.DaemonMetricsResponsePayload{
		RequestID: requestID,
		NodeID:    nodeID,
		Data:      data,
	})
	if err != nil {
		h.logger.Warn("failed to create metrics reply message",
			"node_id", nodeID,
			"error", err,
		)

		return nil
	}

	if err := h.publisher.Publish(ctx, channel, msg); err != nil {
		h.logger.Warn("failed to publish metrics reply",
			"node_id", nodeID,
			"request_id", requestID,
			"error", err,
		)
	}

	return nil
}
