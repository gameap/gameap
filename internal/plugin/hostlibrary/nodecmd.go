package hostlibrary

import (
	"context"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodecmd"
	"github.com/samber/lo"
	"github.com/tetratelabs/wazero"
)

type NodeCmdServiceImpl struct {
	commandService *daemon.CommandService
	nodeRepo       repositories.NodeRepository
}

func NewNodeCmdService(
	commandService *daemon.CommandService,
	nodeRepo repositories.NodeRepository,
) *NodeCmdServiceImpl {
	return &NodeCmdServiceImpl{
		commandService: commandService,
		nodeRepo:       nodeRepo,
	}
}

func (s *NodeCmdServiceImpl) getNode(ctx context.Context, nodeID uint64) (*domain.Node, error) {
	nodes, err := s.nodeRepo.Find(ctx, filters.FindNodeByIDs(uint(nodeID)), nil, nil)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return nil, nil
	}

	return &nodes[0], nil
}

func (s *NodeCmdServiceImpl) ExecuteCommand(
	ctx context.Context,
	req *nodecmd.ExecuteCommandRequest,
) (*nodecmd.ExecuteCommandResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodecmd.ExecuteCommandResponse{Error: lo.ToPtr(err.Error())}, nil
	}

	if node == nil {
		return &nodecmd.ExecuteCommandResponse{Error: lo.ToPtr("node not found")}, nil
	}

	var opts []daemon.CommandServiceOption
	if req.WorkDir != nil {
		opts = append(opts, daemon.CommandServiceOptionWithWorkDir(*req.WorkDir))
	}

	result, err := s.commandService.ExecuteCommand(ctx, node, req.Command, opts...)
	if err != nil {
		return &nodecmd.ExecuteCommandResponse{Error: lo.ToPtr(err.Error())}, nil
	}

	return &nodecmd.ExecuteCommandResponse{
		Output:   result.Output,
		ExitCode: int32(result.ExitCode), //nolint:gosec
	}, nil
}

type NodeCmdHostLibrary struct {
	impl *NodeCmdServiceImpl
}

func NewNodeCmdHostLibrary(
	commandService *daemon.CommandService,
	nodeRepo repositories.NodeRepository,
) *NodeCmdHostLibrary {
	return &NodeCmdHostLibrary{
		impl: NewNodeCmdService(commandService, nodeRepo),
	}
}

func (l *NodeCmdHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return nodecmd.Instantiate(ctx, r, l.impl)
}
