package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
)

type StatusService struct {
	gateway    StatusGateway
	registry   ConnectionChecker
	dispatcher StatusDispatcher
	logger     *slog.Logger
}

func NewStatusService(
	gateway StatusGateway,
	registry ConnectionChecker,
	dispatcher StatusDispatcher,
	logger *slog.Logger,
) *StatusService {
	if logger == nil {
		logger = slog.Default()
	}

	return &StatusService{
		gateway:    gateway,
		registry:   registry,
		dispatcher: dispatcher,
		logger:     logger,
	}
}

func (s *StatusService) Status(ctx context.Context, node *domain.Node) (*NodeStatus, error) {
	nodeID := uint64(node.ID)

	if s.registry.IsConnected(nodeID) {
		return s.statusViaGateway(ctx, nodeID)
	}

	if s.registry.IsConnectedAnywhere(nodeID) {
		return s.statusViaDispatcher(ctx, nodeID)
	}

	return nil, ErrDaemonNotConnected
}

// ConnectionType reports which communication channel the API will use to talk
// to the daemon for the given node:
//   - "grpc"   the daemon has a registered gRPC bidi session (local or any cluster instance);
//   - "none"   the daemon is not reachable.
func (s *StatusService) ConnectionType(nodeID uint64) string {
	if s.registry.IsConnected(nodeID) || s.registry.IsConnectedAnywhere(nodeID) {
		return ConnectionTypeGRPC
	}

	return ConnectionTypeNone
}

const (
	ConnectionTypeGRPC = "grpc"
	ConnectionTypeNone = "none"
)

func (s *StatusService) Version(ctx context.Context, node *domain.Node) (*NodeVersion, error) {
	nodeID := uint64(node.ID)

	if s.registry.IsConnected(nodeID) {
		resp, err := s.gateway.RequestStatus(ctx, nodeID)
		if err != nil {
			return nil, errors.WithMessage(err, "gateway status request")
		}

		return protoStatusResponseToVersion(resp), nil
	}

	if s.registry.IsConnectedAnywhere(nodeID) {
		resp, err := s.dispatcher.DispatchStatus(ctx, nodeID)
		if err != nil {
			return nil, errors.WithMessage(err, "dispatched status request")
		}

		return protoStatusResponseToVersion(resp), nil
	}

	return nil, ErrDaemonNotConnected
}

func (s *StatusService) statusViaGateway(ctx context.Context, nodeID uint64) (*NodeStatus, error) {
	resp, err := s.gateway.RequestStatus(ctx, nodeID)
	if err != nil {
		return nil, errors.WithMessage(err, "gateway status request")
	}

	if !resp.Success {
		return nil, errors.Errorf("status request: %s", resp.Error)
	}

	return protoStatusResponseToNodeStatus(resp), nil
}

func (s *StatusService) statusViaDispatcher(ctx context.Context, nodeID uint64) (*NodeStatus, error) {
	resp, err := s.dispatcher.DispatchStatus(ctx, nodeID)
	if err != nil {
		return nil, errors.WithMessage(err, "dispatched status request")
	}

	if !resp.Success {
		return nil, errors.Errorf("status request: %s", resp.Error)
	}

	return protoStatusResponseToNodeStatus(resp), nil
}

func protoStatusResponseToNodeStatus(r *proto.StatusResponse) *NodeStatus {
	return &NodeStatus{
		Uptime:        time.Duration(r.UptimeSeconds) * time.Second,
		Version:       r.Version,
		BuildDate:     r.BuildDate,
		WorkingTasks:  int(r.WorkingTasks),
		WaitingTasks:  int(r.WaitingTasks),
		OnlineServers: int(r.OnlineServers),
	}
}

func protoStatusResponseToVersion(r *proto.StatusResponse) *NodeVersion {
	return &NodeVersion{
		Version:   r.Version,
		BuildDate: r.BuildDate,
	}
}
