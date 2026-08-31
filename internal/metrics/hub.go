package metrics

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/pkg/idgen"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
)

// maxHistoryWindow caps the history-replay window the API will request from
// daemons. Defense-in-depth: the daemon's ring buffer already bounds what it
// can return, but we also reject absurd uint32 values up front to avoid DoS
// from a misbehaving client or future routes.
const maxHistoryWindow = 24 * time.Hour

// parseRealtimeMetricsNodeID extracts the nodeID encoded in the channel name
// gameap:realtime:metrics:<nodeID>. We use it to cross-check pubsub payloads
// against the channel they arrived on — the channel is the trust anchor.
func parseRealtimeMetricsNodeID(channel string) (uint64, error) {
	suffix, ok := strings.CutPrefix(channel, channels.RealtimeMetrics)
	if !ok {
		return 0, errors.New("channel is not a realtime metrics channel")
	}

	id, err := strconv.ParseUint(suffix, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "parse channel node id")
	}

	return id, nil
}

// Options configures Hub timings. Zero values are replaced with
// sensible defaults.
type Options struct {
	PollInterval      time.Duration
	ReplayWindow      time.Duration
	RingMaxAge        time.Duration
	HeartbeatInterval time.Duration
	HeartbeatTTL      time.Duration
	StopDebounce      time.Duration
	HistoryTimeout    time.Duration
	SamplesBuffer     int
}

func (o *Options) applyDefaults() {
	if o.PollInterval <= 0 {
		o.PollInterval = 5 * time.Second
	}
	if o.ReplayWindow <= 0 {
		o.ReplayWindow = 30 * time.Minute
	}
	if o.RingMaxAge <= 0 {
		o.RingMaxAge = 30 * time.Minute
	}
	if o.HeartbeatInterval <= 0 {
		o.HeartbeatInterval = 5 * time.Second
	}
	if o.HeartbeatTTL <= 0 {
		o.HeartbeatTTL = 15 * time.Second
	}
	if o.StopDebounce <= 0 {
		o.StopDebounce = 5 * time.Second
	}
	if o.HistoryTimeout <= 0 {
		o.HistoryTimeout = 10 * time.Second
	}
	if o.SamplesBuffer <= 0 {
		o.SamplesBuffer = 64
	}
}

func (o *Options) ringCapacity() int {
	if o.PollInterval <= 0 {
		return 1
	}

	return max(int(o.RingMaxAge/o.PollInterval), 1)
}

type hub struct {
	pubsub       pubsub.PubSub
	registry     Registry
	waiters      HandlerWaiters
	instanceID   string
	logger       *slog.Logger
	opts         Options
	ringCapacity int

	startedFlag atomic.Bool

	statesMu sync.Mutex
	states   map[uint64]*nodeState

	historyMu      sync.Mutex
	historyWaiters map[string]chan *historyResult

	rootCtx    context.Context
	rootCancel context.CancelFunc
}

type nodeState struct {
	nodeID uint64

	mu sync.Mutex

	localCount  int
	subscribers map[*subscription]struct{}
	ring        *ring

	remoteCounts    map[string]remoteCountEntry
	aggregatedCount int

	pollCancel context.CancelFunc
	stopTimer  *time.Timer
}

type remoteCountEntry struct {
	count   int
	expires time.Time
}

type historyResult struct {
	resp *proto.MetricsResponse
	err  error
}

func NewHub(
	ps pubsub.PubSub,
	registry Registry,
	waiters HandlerWaiters,
	instanceID string,
	logger *slog.Logger,
	opts Options,
) Hub {
	if logger == nil {
		logger = slog.Default()
	}
	opts.applyDefaults()

	return &hub{
		pubsub:         ps,
		registry:       registry,
		waiters:        waiters,
		instanceID:     instanceID,
		logger:         logger,
		opts:           opts,
		ringCapacity:   opts.ringCapacity(),
		states:         make(map[uint64]*nodeState),
		historyWaiters: make(map[string]chan *historyResult),
	}
}

func (h *hub) Start(ctx context.Context) error {
	if !h.startedFlag.CompareAndSwap(false, true) {
		return nil
	}

	h.rootCtx, h.rootCancel = context.WithCancel(ctx)

	if err := h.pubsub.Subscribe(h.rootCtx, channels.RealtimeMetricsAll, h.handleLiveSample); err != nil {
		return errors.Wrap(err, "subscribe to realtime metrics")
	}

	if err := h.pubsub.Subscribe(h.rootCtx, channels.MetricsSubscribersAll, h.handleSubscribersHeartbeat); err != nil {
		return errors.Wrap(err, "subscribe to metrics subscribers")
	}

	responseChannel := channels.BuildDaemonMetricsResponseChannel(h.instanceID)
	if err := h.pubsub.Subscribe(h.rootCtx, responseChannel, h.handleHistoryReply); err != nil {
		return errors.Wrap(err, "subscribe to metrics response")
	}

	go h.heartbeatLoop()

	h.logger.Info("metrics hub started",
		"instance_id", h.instanceID,
		"poll_interval", h.opts.PollInterval,
		"ring_capacity", h.ringCapacity,
	)

	return nil
}

func (h *hub) Subscribe(
	ctx context.Context, nodeID uint64, replayWindow time.Duration,
) (Subscription, []*proto.MetricsResponse, error) {
	if !h.startedFlag.Load() {
		return nil, nil, errors.New("metrics hub is not started")
	}
	if replayWindow <= 0 {
		replayWindow = h.opts.ReplayWindow
	}
	if replayWindow > maxHistoryWindow {
		replayWindow = maxHistoryWindow
	}

	state := h.getOrCreateState(nodeID)

	sub := newSubscription(h, nodeID, h.opts.SamplesBuffer)

	state.mu.Lock()
	state.subscribers[sub] = struct{}{}
	state.localCount++
	wasZero := state.localCount == 1
	state.mu.Unlock()

	if wasZero {
		h.publishHeartbeat(state)
	}
	h.maybeStartOrStopPoll(state)

	replay := h.gatherReplay(ctx, state, replayWindow)

	return sub, replay, nil
}

func (h *hub) unsubscribe(sub *subscription) {
	state := h.lookupState(sub.nodeID)
	if state == nil {
		sub.closeChannel()

		return
	}

	state.mu.Lock()
	if _, ok := state.subscribers[sub]; ok {
		delete(state.subscribers, sub)
		if state.localCount > 0 {
			state.localCount--
		}
	}
	wasLast := state.localCount == 0
	state.mu.Unlock()

	sub.closeChannel()

	h.publishHeartbeat(state)

	if wasLast {
		h.maybeStartOrStopPoll(state)
	}
}

func (h *hub) GetHistory(
	ctx context.Context, nodeID uint64, window time.Duration,
) (*proto.MetricsResponse, error) {
	if !h.startedFlag.Load() {
		return nil, errors.New("metrics hub is not started")
	}
	if window <= 0 {
		window = h.opts.ReplayWindow
	}
	if window > maxHistoryWindow {
		window = maxHistoryWindow
	}
	if !h.registry.IsConnectedAnywhere(nodeID) {
		return nil, errors.New("daemon is not connected")
	}

	requestID := idgen.New()
	resultCh := make(chan *historyResult, 1)

	h.historyMu.Lock()
	h.historyWaiters[requestID] = resultCh
	h.historyMu.Unlock()

	defer func() {
		h.historyMu.Lock()
		delete(h.historyWaiters, requestID)
		h.historyMu.Unlock()

		h.waiters.CancelWaiter(requestID)
	}()

	h.waiters.RegisterRemoteWaiter(requestID, nodeID, h.instanceID)

	req := &proto.MetricsRequest{
		Kind: &proto.MetricsRequest_History{
			History: &proto.MetricsHistoryRequest{Seconds: uint32(window.Seconds())},
		},
	}

	if err := h.registry.SendMetricsRequest(ctx, nodeID, requestID, req); err != nil {
		return nil, errors.Wrap(err, "send metrics history request")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, h.opts.HistoryTimeout)
	defer cancel()

	select {
	case <-timeoutCtx.Done():
		if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("metrics history request timed out")
		}

		return nil, timeoutCtx.Err()

	case res := <-resultCh:
		if res == nil {
			return nil, errors.New("metrics history request cancelled")
		}
		if res.err != nil {
			return nil, res.err
		}

		return res.resp, nil
	}
}

func (h *hub) Stop() {
	if h.rootCancel != nil {
		h.rootCancel()
	}

	h.statesMu.Lock()
	defer h.statesMu.Unlock()

	for _, state := range h.states {
		state.mu.Lock()
		if state.pollCancel != nil {
			state.pollCancel()
			state.pollCancel = nil
		}
		if state.stopTimer != nil {
			state.stopTimer.Stop()
			state.stopTimer = nil
		}
		for sub := range state.subscribers {
			sub.closeChannel()
		}
		state.subscribers = nil
		state.mu.Unlock()
	}
}

func (h *hub) getOrCreateState(nodeID uint64) *nodeState {
	h.statesMu.Lock()
	defer h.statesMu.Unlock()

	if state, ok := h.states[nodeID]; ok {
		return state
	}

	state := &nodeState{
		nodeID:       nodeID,
		subscribers:  make(map[*subscription]struct{}),
		ring:         newRing(h.ringCapacity),
		remoteCounts: make(map[string]remoteCountEntry),
	}
	h.states[nodeID] = state

	return state
}

func (h *hub) lookupState(nodeID uint64) *nodeState {
	h.statesMu.Lock()
	defer h.statesMu.Unlock()

	return h.states[nodeID]
}

func (h *hub) gatherReplay(
	ctx context.Context, state *nodeState, window time.Duration,
) []*proto.MetricsResponse {
	cutoff := time.Now().Add(-window)
	entries := state.ring.Snapshot(cutoff)

	if h.replayCoversWindow(entries, window) {
		return mergedReplay(entries)
	}

	if !h.registry.IsConnectedAnywhere(state.nodeID) {
		return mergedReplay(entries)
	}

	resp, err := h.GetHistory(ctx, state.nodeID, window)
	if err != nil {
		h.logger.Warn("failed to fetch metrics history for replay",
			"node_id", state.nodeID,
			"error", err,
		)

		return mergedReplay(entries)
	}

	if resp == nil {
		return mergedReplay(entries)
	}

	state.ring.Append(resp)

	return []*proto.MetricsResponse{resp}
}

// mergedReplay collapses per-tick ring entries into a single series-grouped
// response. Returns nil for the empty case so callers can skip sending an
// empty replay envelope.
func mergedReplay(entries []*proto.MetricsResponse) []*proto.MetricsResponse {
	merged := mergeResponses(entries)
	if merged == nil {
		return nil
	}

	return []*proto.MetricsResponse{merged}
}

func (h *hub) replayCoversWindow(entries []*proto.MetricsResponse, window time.Duration) bool {
	if len(entries) == 0 {
		return false
	}

	oldest := entries[0].GetTimestamp().AsTime()
	cutoff := time.Now().Add(-window).Add(window / 2) // tolerate half window for "fresh enough"

	return !oldest.After(cutoff)
}

func (h *hub) handleLiveSample(_ context.Context, msg *pubsub.Message) error {
	payload, err := messages.ParsePayload[messages.MetricsLivePayload](msg)
	if err != nil {
		h.logger.Warn("failed to parse metrics live payload", "error", err)

		return nil
	}

	channelNodeID, err := parseRealtimeMetricsNodeID(msg.Channel)
	if err != nil {
		h.logger.Warn("metrics live message on malformed channel",
			"channel", msg.Channel,
			"error", err,
		)

		return nil
	}
	if channelNodeID != payload.NodeID {
		h.logger.Warn("metrics live payload nodeID mismatch with channel",
			"channel", msg.Channel,
			"channel_node_id", channelNodeID,
			"payload_node_id", payload.NodeID,
		)

		return nil
	}

	var resp proto.MetricsResponse
	if err := resp.UnmarshalVT(payload.Data); err != nil {
		h.logger.Warn("failed to unmarshal metrics live response",
			"node_id", payload.NodeID,
			"error", err,
		)

		return nil
	}

	state := h.getOrCreateState(payload.NodeID)
	state.ring.Append(&resp)

	state.mu.Lock()
	subs := make([]*subscription, 0, len(state.subscribers))
	for sub := range state.subscribers {
		subs = append(subs, sub)
	}
	state.mu.Unlock()

	for _, sub := range subs {
		sub.deliver(&resp)
	}

	return nil
}

func (h *hub) handleSubscribersHeartbeat(_ context.Context, msg *pubsub.Message) error {
	payload, err := messages.ParsePayload[messages.MetricsSubscribersPayload](msg)
	if err != nil {
		h.logger.Warn("failed to parse metrics subscribers payload", "error", err)

		return nil
	}

	if payload.InstanceID == h.instanceID {
		return nil
	}

	state := h.getOrCreateState(payload.NodeID)

	state.mu.Lock()
	if payload.Count <= 0 {
		delete(state.remoteCounts, payload.InstanceID)
	} else {
		state.remoteCounts[payload.InstanceID] = remoteCountEntry{
			count:   payload.Count,
			expires: time.Now().Add(h.opts.HeartbeatTTL),
		}
	}
	state.mu.Unlock()

	h.maybeStartOrStopPoll(state)

	return nil
}

func (h *hub) handleHistoryReply(_ context.Context, msg *pubsub.Message) error {
	payload, err := messages.ParsePayload[messages.DaemonMetricsResponsePayload](msg)
	if err != nil {
		h.logger.Warn("failed to parse metrics history reply payload", "error", err)

		return nil
	}

	h.historyMu.Lock()
	resultCh, ok := h.historyWaiters[payload.RequestID]
	h.historyMu.Unlock()

	if !ok {
		return nil
	}

	res := &historyResult{}
	if payload.Error != "" {
		res.err = errors.New(payload.Error)
	} else if len(payload.Data) > 0 {
		var resp proto.MetricsResponse
		if err := resp.UnmarshalVT(payload.Data); err != nil {
			res.err = errors.Wrap(err, "unmarshal metrics history response")
		} else {
			res.resp = &resp
		}
	}

	select {
	case resultCh <- res:
	default:
	}

	return nil
}

func (h *hub) heartbeatLoop() {
	ticker := time.NewTicker(h.opts.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.rootCtx.Done():
			return
		case <-ticker.C:
			h.broadcastHeartbeats()
		}
	}
}

func (h *hub) broadcastHeartbeats() {
	h.statesMu.Lock()
	states := make([]*nodeState, 0, len(h.states))
	for _, s := range h.states {
		states = append(states, s)
	}
	h.statesMu.Unlock()

	for _, state := range states {
		state.mu.Lock()
		count := state.localCount
		state.mu.Unlock()

		if count == 0 {
			continue
		}
		h.publishHeartbeat(state)
	}
}

func (h *hub) publishHeartbeat(state *nodeState) {
	state.mu.Lock()
	count := state.localCount
	state.mu.Unlock()

	channel := channels.BuildMetricsSubscribersChannel(state.nodeID)
	msg, err := messages.NewMessage(channel, messages.TypeMetricsSubscribers, messages.MetricsSubscribersPayload{
		InstanceID: h.instanceID,
		NodeID:     state.nodeID,
		Count:      count,
		Timestamp:  time.Now(),
	})
	if err != nil {
		h.logger.Warn("failed to create metrics subscribers message",
			"node_id", state.nodeID,
			"error", err,
		)

		return
	}

	if err := h.pubsub.Publish(h.rootCtx, channel, msg); err != nil {
		h.logger.Warn("failed to publish metrics subscribers heartbeat",
			"node_id", state.nodeID,
			"error", err,
		)
	}
}

func (h *hub) maybeStartOrStopPoll(state *nodeState) {
	if !h.registry.IsConnected(state.nodeID) {
		return
	}

	state.mu.Lock()

	now := time.Now()
	for instanceID, entry := range state.remoteCounts {
		if entry.expires.Before(now) {
			delete(state.remoteCounts, instanceID)
		}
	}

	aggregated := state.localCount
	for _, entry := range state.remoteCounts {
		aggregated += entry.count
	}
	state.aggregatedCount = aggregated

	wantPoll := aggregated > 0
	hasPoll := state.pollCancel != nil

	switch {
	case wantPoll && !hasPoll:
		if state.stopTimer != nil {
			state.stopTimer.Stop()
			state.stopTimer = nil
		}
		// cancel is stored in state.pollCancel; invoked by stopPollIfStillIdle / Stop.
		pollCtx, cancel := context.WithCancel(h.rootCtx)
		state.pollCancel = cancel
		state.mu.Unlock()
		go h.pollLoop(pollCtx, state.nodeID)

		return

	case !wantPoll && hasPoll:
		if state.stopTimer == nil {
			nodeID := state.nodeID
			state.stopTimer = time.AfterFunc(h.opts.StopDebounce, func() {
				h.stopPollIfStillIdle(nodeID)
			})
		}
	}

	state.mu.Unlock()
}

func (h *hub) stopPollIfStillIdle(nodeID uint64) {
	state := h.lookupState(nodeID)
	if state == nil {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.stopTimer = nil

	now := time.Now()
	for instanceID, entry := range state.remoteCounts {
		if entry.expires.Before(now) {
			delete(state.remoteCounts, instanceID)
		}
	}

	aggregated := state.localCount
	for _, entry := range state.remoteCounts {
		aggregated += entry.count
	}

	if aggregated > 0 {
		return
	}

	if state.pollCancel != nil {
		state.pollCancel()
		state.pollCancel = nil
	}
}

func (h *hub) pollLoop(ctx context.Context, nodeID uint64) {
	h.runPollOnce(ctx, nodeID)

	ticker := time.NewTicker(h.opts.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.runPollOnce(ctx, nodeID)
		}
	}
}

func (h *hub) runPollOnce(ctx context.Context, nodeID uint64) {
	if !h.registry.IsConnected(nodeID) {
		return
	}

	requestID := idgen.New()
	h.waiters.RegisterPollWaiter(requestID, nodeID)

	req := &proto.MetricsRequest{
		Kind: &proto.MetricsRequest_Current{Current: &proto.CurrentMetricsRequest{}},
	}

	if err := h.registry.SendMetricsRequest(ctx, nodeID, requestID, req); err != nil {
		h.waiters.CancelWaiter(requestID)
		h.logger.Warn("failed to send metrics poll request",
			"node_id", nodeID,
			"error", err,
		)
	}
}
