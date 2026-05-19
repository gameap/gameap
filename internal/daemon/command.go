package daemon

import (
	"context"
	"log/slog"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/idgen"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/durationpb"
)

const defaultCommandTimeout = 300 * time.Second

type CommandService struct {
	gateway    CommandGateway
	registry   ConnectionChecker
	dispatcher CommandDispatcher
	logger     *slog.Logger
}

func NewCommandService(
	gateway CommandGateway,
	registry ConnectionChecker,
	dispatcher CommandDispatcher,
	logger *slog.Logger,
) *CommandService {
	if logger == nil {
		logger = slog.Default()
	}

	return &CommandService{
		gateway:    gateway,
		registry:   registry,
		dispatcher: dispatcher,
		logger:     logger,
	}
}

func (s *CommandService) ExecuteCommand(
	ctx context.Context,
	node *domain.Node,
	command string,
	opts ...CommandServiceOption,
) (*CommandResult, error) {
	nodeID := uint64(node.ID)
	o := applyCommandOptions(opts)

	if s.registry.IsConnected(nodeID) {
		return s.executeViaGateway(ctx, nodeID, command, o)
	}

	if s.registry.IsConnectedAnywhere(nodeID) {
		return s.executeViaDispatcher(ctx, nodeID, command, o)
	}

	return nil, ErrDaemonNotConnected
}

func (s *CommandService) executeViaGateway(
	ctx context.Context,
	nodeID uint64,
	command string,
	o CommandOption,
) (*CommandResult, error) {
	req := s.buildCommandRequest(command, o)

	result, err := s.gateway.RequestCommand(ctx, nodeID, req)
	if err != nil {
		return nil, errors.WithMessage(err, "gateway command request")
	}

	return protoCommandResultToResult(result), nil
}

func (s *CommandService) executeViaDispatcher(
	ctx context.Context,
	nodeID uint64,
	command string,
	o CommandOption,
) (*CommandResult, error) {
	req := s.buildCommandRequest(command, o)

	result, err := s.dispatcher.DispatchCommand(ctx, nodeID, req)
	if err != nil {
		return nil, errors.WithMessage(err, "dispatched command request")
	}

	return protoCommandResultToResult(result), nil
}

func (s *CommandService) buildCommandRequest(command string, o CommandOption) *proto.CommandRequest {
	return &proto.CommandRequest{
		CommandId: idgen.New(),
		Command:   command,
		WorkDir:   o.WorkDir,
		Timeout:   durationpb.New(defaultCommandTimeout),
	}
}

func protoCommandResultToResult(r *proto.CommandResult) *CommandResult {
	return &CommandResult{
		Output:   string(r.Output),
		ExitCode: int(r.ExitCode),
	}
}
