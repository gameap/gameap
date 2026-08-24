package hostlibrary

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodecmd"
	"github.com/tetratelabs/wazero"
)

// NodeCmdServiceImpl is per plugin: command execution is gated on the
// plugin's node_commands grant, rate limited and audited. The command text
// itself is not written to the audit stream (it may carry credentials); the
// node, working directory and exit code are.
type NodeCmdServiceImpl struct {
	commandService NodeCommandService
	nodeRepo       repositories.NodeRepository
	guard          *PluginGuard
}

func NewNodeCmdService(
	commandService NodeCommandService,
	nodeRepo repositories.NodeRepository,
	guard *PluginGuard,
) *NodeCmdServiceImpl {
	return &NodeCmdServiceImpl{
		commandService: commandService,
		nodeRepo:       nodeRepo,
		guard:          guard,
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
	if msg := s.guard.Check(ctx, ModuleNodeCmd, "execute_command"); msg != "" {
		return &nodecmd.ExecuteCommandResponse{Error: new(msg)}, nil
	}

	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodecmd.ExecuteCommandResponse{Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodecmd.ExecuteCommandResponse{Error: new("node not found")}, nil
	}

	var opts []daemon.CommandServiceOption

	attrs := []slog.Attr{}
	if req.WorkDir != nil {
		opts = append(opts, daemon.CommandServiceOptionWithWorkDir(*req.WorkDir))
		attrs = append(attrs, slog.String("work_dir", *req.WorkDir))
	}

	result, err := s.commandService.ExecuteCommand(ctx, node, req.Command, opts...)
	if err != nil {
		s.guard.Audit(ctx, audit.EventPluginNodeCommand, "execute", "node", nodeResourceID(req.NodeId), err, attrs...)

		return &nodecmd.ExecuteCommandResponse{Error: new(err.Error())}, nil
	}

	attrs = append(attrs, slog.Int("exit_code", result.ExitCode))
	s.guard.Audit(ctx, audit.EventPluginNodeCommand, "execute", "node", nodeResourceID(req.NodeId), nil, attrs...)

	return &nodecmd.ExecuteCommandResponse{
		Output:   result.Output,
		ExitCode: int32(result.ExitCode), //nolint:gosec
	}, nil
}

type NodeCmdHostLibrary struct {
	impl *NodeCmdServiceImpl
}

func (l *NodeCmdHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return nodecmd.Instantiate(ctx, r, l.impl)
}

// NodeCmdHostLibraryFactory builds a per-plugin nodecmd module bound to the
// plugin's guard.
type NodeCmdHostLibraryFactory struct {
	commandService NodeCommandService
	nodeRepo       repositories.NodeRepository
	guard          *Guard
}

func NewNodeCmdHostLibraryFactory(
	commandService NodeCommandService,
	nodeRepo repositories.NodeRepository,
	guard *Guard,
) *NodeCmdHostLibraryFactory {
	return &NodeCmdHostLibraryFactory{
		commandService: commandService,
		nodeRepo:       nodeRepo,
		guard:          guard,
	}
}

func (f *NodeCmdHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &NodeCmdHostLibrary{
		impl: NewNodeCmdService(f.commandService, f.nodeRepo, f.guard.For(pluginID)),
	}
}
