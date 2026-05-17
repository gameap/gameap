package session

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/durationpb"
)

type Registry struct {
	pubsub         pubsub.PubSub
	instanceID     string
	metricsWaiters MetricsWaiterRegistrar
	logger         *slog.Logger

	mu            sync.RWMutex
	localSessions map[uint64]*Session

	globalMu    sync.RWMutex
	globalNodes map[uint64]string
}

// MetricsWaiterRegistrar lets the registry register cross-instance
// metrics request waiters before forwarding the request to the local
// daemon session.
type MetricsWaiterRegistrar interface {
	RegisterRemoteWaiter(requestID string, nodeID uint64, requesterInstanceID string)
	CancelWaiter(requestID string)
}

func NewRegistry(ps pubsub.PubSub, instanceID string, logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}

	return &Registry{
		pubsub:        ps,
		instanceID:    instanceID,
		logger:        logger,
		localSessions: make(map[uint64]*Session),
		globalNodes:   make(map[uint64]string),
	}
}

// SetMetricsWaiterRegistrar wires the metrics handler used to track
// cross-instance request waiters. Called once during application
// bootstrap.
func (r *Registry) SetMetricsWaiterRegistrar(reg MetricsWaiterRegistrar) {
	r.metricsWaiters = reg
}

// InstanceID returns this registry's instance identifier.
func (r *Registry) InstanceID() string {
	return r.instanceID
}

func (r *Registry) Start(ctx context.Context) error {
	err := r.pubsub.Subscribe(ctx, channels.DaemonTaskDispatchAll, r.handleTaskDispatch)
	if err != nil {
		return errors.Wrap(err, "subscribe to task dispatch")
	}

	err = r.pubsub.Subscribe(ctx, channels.DaemonSessionAll, r.handleSessionEvent)
	if err != nil {
		return errors.Wrap(err, "subscribe to session events")
	}

	err = r.pubsub.Subscribe(ctx, channels.DaemonAttachDispatchAll, r.handleAttachDispatch)
	if err != nil {
		return errors.Wrap(err, "subscribe to attach dispatch")
	}

	err = r.pubsub.Subscribe(ctx, channels.DaemonMetricsRequestAll, r.handleMetricsRequest)
	if err != nil {
		return errors.Wrap(err, "subscribe to metrics request")
	}

	r.logger.Info("session registry started", "instance_id", r.instanceID)

	return nil
}

func (r *Registry) Register(ctx context.Context, session *Session) error {
	r.mu.Lock()
	if old, ok := r.localSessions[session.NodeID]; ok {
		old.Cancel()
		r.logger.Info("closed existing session for reconnecting daemon",
			"node_id", session.NodeID,
		)
	}
	r.localSessions[session.NodeID] = session
	r.mu.Unlock()

	msg, err := messages.NewMessage(
		channels.DaemonSessionConnected,
		messages.TypeDaemonConnected,
		messages.DaemonSessionPayload{
			NodeID:      session.NodeID,
			InstanceID:  r.instanceID,
			Version:     session.Version,
			ConnectedAt: time.Now(),
		},
	)
	if err != nil {
		return errors.Wrap(err, "create session connected message")
	}

	if err := r.pubsub.Publish(ctx, channels.DaemonSessionConnected, msg); err != nil {
		r.logger.Warn("failed to publish session connected event",
			"node_id", session.NodeID,
			"error", err,
		)
	}

	r.logger.Info("daemon session registered",
		"node_id", session.NodeID,
		"version", session.Version,
		"capabilities", session.Capabilities,
	)

	return nil
}

func (r *Registry) Unregister(ctx context.Context, nodeID uint64) error {
	r.mu.Lock()
	session, ok := r.localSessions[nodeID]
	if ok {
		delete(r.localSessions, nodeID)
	}
	r.mu.Unlock()

	if !ok {
		return nil
	}

	msg, err := messages.NewMessage(
		channels.DaemonSessionClosed,
		messages.TypeDaemonClosed,
		messages.DaemonSessionPayload{
			NodeID:      nodeID,
			InstanceID:  r.instanceID,
			Version:     session.Version,
			ConnectedAt: session.ConnectedAt,
		},
	)
	if err != nil {
		return errors.Wrap(err, "create session closed message")
	}

	if err := r.pubsub.Publish(ctx, channels.DaemonSessionClosed, msg); err != nil {
		r.logger.Warn("failed to publish session closed event",
			"node_id", nodeID,
			"error", err,
		)
	}

	r.logger.Info("daemon session unregistered", "node_id", nodeID)

	return nil
}

func (r *Registry) GetSession(nodeID uint64) (*Session, bool) {
	r.mu.RLock()
	session, ok := r.localSessions[nodeID]
	r.mu.RUnlock()

	return session, ok
}

func (r *Registry) IsConnected(nodeID uint64) bool {
	_, ok := r.GetSession(nodeID)

	return ok
}

func (r *Registry) IsConnectedAnywhere(nodeID uint64) bool {
	if r.IsConnected(nodeID) {
		return true
	}

	r.globalMu.RLock()
	_, ok := r.globalNodes[nodeID]
	r.globalMu.RUnlock()

	return ok
}

func (r *Registry) HasCapability(nodeID uint64, capability string) bool {
	r.mu.RLock()
	session, ok := r.localSessions[nodeID]
	r.mu.RUnlock()

	if !ok {
		return false
	}

	return session.HasCapability(capability)
}

func (r *Registry) handleSessionEvent(_ context.Context, msg *pubsub.Message) error {
	switch msg.Type {
	case messages.TypeDaemonConnected:
		payload, err := messages.ParsePayload[messages.DaemonSessionPayload](msg)
		if err != nil {
			r.logger.Warn("failed to parse session connected payload", "error", err)

			return nil
		}

		if payload.InstanceID == r.instanceID {
			return nil
		}

		r.globalMu.Lock()
		r.globalNodes[payload.NodeID] = payload.InstanceID
		r.globalMu.Unlock()

		r.logger.Debug("tracked remote daemon session",
			"node_id", payload.NodeID,
			"instance_id", payload.InstanceID,
		)

	case messages.TypeDaemonClosed:
		payload, err := messages.ParsePayload[messages.DaemonSessionPayload](msg)
		if err != nil {
			r.logger.Warn("failed to parse session closed payload", "error", err)

			return nil
		}

		r.globalMu.Lock()
		delete(r.globalNodes, payload.NodeID)
		r.globalMu.Unlock()

		r.logger.Debug("removed remote daemon session",
			"node_id", payload.NodeID,
		)
	}

	return nil
}

func (r *Registry) SendTask(ctx context.Context, nodeID uint64, task *proto.GatewayMessage) error {
	r.mu.RLock()
	session, isLocal := r.localSessions[nodeID]
	r.mu.RUnlock()

	if isLocal {
		if err := session.Send(task); err != nil {
			return errors.Wrap(err, "send task to local session")
		}

		return nil
	}

	return r.dispatchViaPubSub(ctx, nodeID, task)
}

func (r *Registry) dispatchViaPubSub(ctx context.Context, nodeID uint64, gatewayMsg *proto.GatewayMessage) error {
	taskData, err := gatewayMsg.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal gateway message")
	}

	var taskID uint64
	if t := gatewayMsg.GetTask(); t != nil {
		taskID = t.Id
	}

	channel := channels.BuildDaemonTaskDispatchChannel(nodeID)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonTask, messages.DaemonTaskDispatchPayload{
		NodeID:    nodeID,
		RequestID: gatewayMsg.RequestId,
		TaskID:    taskID,
		TaskData:  taskData,
	})
	if err != nil {
		return errors.Wrap(err, "create task dispatch message")
	}

	return r.pubsub.Publish(ctx, channel, msg)
}

func (r *Registry) handleTaskDispatch(_ context.Context, msg *pubsub.Message) error {
	nodeID, err := extractNodeIDFromChannel(msg.Channel)
	if err != nil {
		r.logger.Warn("failed to extract node ID from channel",
			"channel", msg.Channel,
			"error", err,
		)

		return nil
	}

	r.mu.RLock()
	session, ok := r.localSessions[nodeID]
	r.mu.RUnlock()

	if !ok {
		return nil
	}

	payload, err := messages.ParsePayload[messages.DaemonTaskDispatchPayload](msg)
	if err != nil {
		r.logger.Warn("failed to parse task dispatch payload",
			"channel", msg.Channel,
			"error", err,
		)

		return nil
	}

	var gatewayMsg proto.GatewayMessage
	if err := gatewayMsg.UnmarshalVT(payload.TaskData); err != nil {
		r.logger.Warn("failed to unmarshal gateway message",
			"node_id", nodeID,
			"error", err,
		)

		return nil
	}

	if err := session.Send(&gatewayMsg); err != nil {
		r.logger.Warn("failed to send task to session",
			"node_id", nodeID,
			"task_id", payload.TaskID,
			"error", err,
		)

		return errors.Wrap(err, "send task to session")
	}

	r.logger.Debug("task dispatched via pub-sub",
		"node_id", nodeID,
		"task_id", payload.TaskID,
	)

	return nil
}

func (r *Registry) SendCommand(ctx context.Context, nodeID uint64, cmd *proto.CommandRequest) error {
	msg := &proto.GatewayMessage{
		RequestId: cmd.CommandId,
		Payload: &proto.GatewayMessage_Command{
			Command: cmd,
		},
	}

	r.mu.RLock()
	session, isLocal := r.localSessions[nodeID]
	r.mu.RUnlock()

	if isLocal {
		return session.Send(msg)
	}

	return r.dispatchCommandViaPubSub(ctx, nodeID, cmd)
}

func (r *Registry) dispatchCommandViaPubSub(ctx context.Context, nodeID uint64, cmd *proto.CommandRequest) error {
	channel := channels.BuildDaemonCommandDispatchChannel(nodeID)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonCommand, messages.DaemonCommandDispatchPayload{
		NodeID:    nodeID,
		RequestID: cmd.CommandId,
		CommandID: cmd.CommandId,
		ServerID:  cmd.ServerId,
		Command:   cmd.Command,
		Timeout:   int32(cmd.Timeout.AsDuration().Seconds()),
	})
	if err != nil {
		return errors.Wrap(err, "create command dispatch message")
	}

	return r.pubsub.Publish(ctx, channel, msg)
}

func (r *Registry) BroadcastToAll(_ context.Context, msg *proto.GatewayMessage) {
	r.mu.RLock()
	sessions := make([]*Session, 0, len(r.localSessions))
	for _, s := range r.localSessions {
		sessions = append(sessions, s)
	}
	r.mu.RUnlock()

	for _, session := range sessions {
		if err := session.Send(msg); err != nil {
			r.logger.Warn("failed to broadcast message",
				"node_id", session.NodeID,
				"error", err,
			)
		}
	}
}

func (r *Registry) BroadcastShutdown(ctx context.Context, reason string, reconnectDelay time.Duration) {
	msg := &proto.GatewayMessage{
		Payload: &proto.GatewayMessage_Shutdown{
			Shutdown: &proto.ShutdownNotification{
				Reason:         reason,
				ReconnectDelay: durationpb.New(reconnectDelay),
			},
		},
	}
	r.BroadcastToAll(ctx, msg)
}

func (r *Registry) SendAttachRequest(ctx context.Context, nodeID uint64, req *proto.AttachRequest) error {
	msg := &proto.GatewayMessage{
		Payload: &proto.GatewayMessage_AttachRequest{
			AttachRequest: req,
		},
	}

	return r.sendOrDispatchAttach(ctx, nodeID, msg)
}

func (r *Registry) SendAttachInput(ctx context.Context, nodeID uint64, input *proto.AttachInput) error {
	msg := &proto.GatewayMessage{
		Payload: &proto.GatewayMessage_AttachInput{
			AttachInput: input,
		},
	}

	return r.sendOrDispatchAttach(ctx, nodeID, msg)
}

func (r *Registry) SendAttachDetach(ctx context.Context, nodeID uint64, detach *proto.AttachDetach) error {
	msg := &proto.GatewayMessage{
		Payload: &proto.GatewayMessage_AttachDetach{
			AttachDetach: detach,
		},
	}

	return r.sendOrDispatchAttach(ctx, nodeID, msg)
}

func (r *Registry) sendOrDispatchAttach(
	ctx context.Context, nodeID uint64, msg *proto.GatewayMessage,
) error {
	r.mu.RLock()
	session, isLocal := r.localSessions[nodeID]
	r.mu.RUnlock()

	if isLocal {
		return session.Send(msg)
	}

	return r.dispatchAttachViaPubSub(ctx, nodeID, msg)
}

func (r *Registry) dispatchAttachViaPubSub(
	ctx context.Context, nodeID uint64, gatewayMsg *proto.GatewayMessage,
) error {
	data, err := gatewayMsg.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal attach gateway message")
	}

	channel := channels.BuildDaemonAttachDispatchChannel(nodeID)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonAttach, messages.DaemonAttachDispatchPayload{
		NodeID: nodeID,
		Data:   data,
	})
	if err != nil {
		return errors.Wrap(err, "create attach dispatch message")
	}

	return r.pubsub.Publish(ctx, channel, msg)
}

func (r *Registry) handleAttachDispatch(_ context.Context, msg *pubsub.Message) error {
	payload, err := messages.ParsePayload[messages.DaemonAttachDispatchPayload](msg)
	if err != nil {
		r.logger.Warn("failed to parse attach dispatch payload", "error", err)

		return nil
	}

	r.mu.RLock()
	session, ok := r.localSessions[payload.NodeID]
	r.mu.RUnlock()

	if !ok {
		return nil
	}

	var gatewayMsg proto.GatewayMessage
	if err := gatewayMsg.UnmarshalVT(payload.Data); err != nil {
		r.logger.Warn("failed to unmarshal attach gateway message",
			"node_id", payload.NodeID,
			"error", err,
		)

		return nil
	}

	if err := session.Send(&gatewayMsg); err != nil {
		r.logger.Warn("failed to send attach message to session",
			"node_id", payload.NodeID,
			"error", err,
		)

		return errors.Wrap(err, "send attach message to session")
	}

	return nil
}

// SendMetricsRequest delivers a MetricsRequest to the daemon session
// for nodeID. If the session is local, it is sent directly via the
// bidi stream; otherwise the request is dispatched to the holder
// instance via pubsub. The request_id is used by the holder to register
// a remote waiter so the response is routed back to this instance.
func (r *Registry) SendMetricsRequest(
	ctx context.Context, nodeID uint64, requestID string, req *proto.MetricsRequest,
) error {
	msg := &proto.GatewayMessage{
		RequestId: requestID,
		Payload: &proto.GatewayMessage_MetricsRequest{
			MetricsRequest: req,
		},
	}

	r.mu.RLock()
	session, isLocal := r.localSessions[nodeID]
	r.mu.RUnlock()

	if isLocal {
		if err := session.Send(msg); err != nil {
			return errors.Wrap(err, "send metrics request to local session")
		}

		return nil
	}

	return r.dispatchMetricsRequestViaPubSub(ctx, nodeID, requestID, msg)
}

func (r *Registry) dispatchMetricsRequestViaPubSub(
	ctx context.Context, nodeID uint64, requestID string, gatewayMsg *proto.GatewayMessage,
) error {
	data, err := gatewayMsg.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal metrics gateway message")
	}

	channel := channels.BuildDaemonMetricsRequestChannel(nodeID)
	msg, err := messages.NewMessage(channel, messages.TypeDaemonMetricsRequest, messages.DaemonMetricsRequestPayload{
		NodeID:     nodeID,
		RequestID:  requestID,
		InstanceID: r.instanceID,
		Data:       data,
	})
	if err != nil {
		return errors.Wrap(err, "create metrics request message")
	}

	return r.pubsub.Publish(ctx, channel, msg)
}

func (r *Registry) handleMetricsRequest(_ context.Context, msg *pubsub.Message) error {
	payload, err := messages.ParsePayload[messages.DaemonMetricsRequestPayload](msg)
	if err != nil {
		r.logger.Warn("failed to parse metrics request payload", "error", err)

		return nil
	}

	r.mu.RLock()
	session, ok := r.localSessions[payload.NodeID]
	r.mu.RUnlock()

	if !ok {
		return nil
	}

	var gatewayMsg proto.GatewayMessage
	if err := gatewayMsg.UnmarshalVT(payload.Data); err != nil {
		r.logger.Warn("failed to unmarshal metrics gateway message",
			"node_id", payload.NodeID,
			"error", err,
		)

		return nil
	}

	if r.metricsWaiters != nil && payload.InstanceID != "" && payload.InstanceID != r.instanceID {
		r.metricsWaiters.RegisterRemoteWaiter(payload.RequestID, payload.NodeID, payload.InstanceID)
	}

	if err := session.Send(&gatewayMsg); err != nil {
		if r.metricsWaiters != nil {
			r.metricsWaiters.CancelWaiter(payload.RequestID)
		}

		r.logger.Warn("failed to send metrics request to session",
			"node_id", payload.NodeID,
			"error", err,
		)

		return errors.Wrap(err, "send metrics request to session")
	}

	return nil
}

func (r *Registry) ConnectedNodeIDs() []uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]uint64, 0, len(r.localSessions))
	for id := range r.localSessions {
		ids = append(ids, id)
	}

	return ids
}

func (r *Registry) SessionCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.localSessions)
}

// CloseAllSessions cancels every local session's context. Used during
// graceful shutdown to unblock gateway.Connect handlers that are stuck
// waiting on Recv after BroadcastShutdown.
func (r *Registry) CloseAllSessions() {
	r.mu.RLock()
	sessions := make([]*Session, 0, len(r.localSessions))
	for _, s := range r.localSessions {
		sessions = append(sessions, s)
	}
	r.mu.RUnlock()

	for _, s := range sessions {
		s.Cancel()
	}
}

// WaitSessionsClosed blocks until every local session has been
// unregistered, or until the timeout elapses. Returns true if all
// sessions closed cleanly, false on deadline.
func (r *Registry) WaitSessionsClosed(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if r.SessionCount() == 0 {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}

	return r.SessionCount() == 0
}

func extractNodeIDFromChannel(channel string) (uint64, error) {
	prefix := channels.DaemonTaskDispatch
	if !strings.HasPrefix(channel, prefix) {
		return 0, errors.New("invalid channel format")
	}

	nodeIDStr := strings.TrimPrefix(channel, prefix)
	nodeID, err := strconv.ParseUint(nodeIDStr, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "parse node ID")
	}

	return nodeID, nil
}
