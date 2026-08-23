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
	policy         *PathPolicy
}

// NodeCmdOption tunes a NodeCmdServiceImpl.
type NodeCmdOption func(*NodeCmdServiceImpl)

// WithNodeCmdPathPolicy confines the working directory a command may name;
// nil keeps the unrestricted policy. The policy says nothing about what the
// command itself does.
func WithNodeCmdPathPolicy(policy *PathPolicy) NodeCmdOption {
	return func(s *NodeCmdServiceImpl) {
		if policy != nil {
			s.policy = policy
		}
	}
}

func NewNodeCmdService(
	commandService NodeCommandService,
	nodeRepo repositories.NodeRepository,
	guard *PluginGuard,
	opts ...NodeCmdOption,
) *NodeCmdServiceImpl {
	service := &NodeCmdServiceImpl{
		commandService: commandService,
		nodeRepo:       nodeRepo,
		guard:          guard,
		policy:         DefaultPathPolicy(),
	}

	for _, opt := range opts {
		opt(service)
	}

	return service
}

// checkWorkDir refuses a working directory outside the node path policy; a
// command without one runs in the daemon's default directory and is not
// subject to the policy.
func (s *NodeCmdServiceImpl) checkWorkDir(ctx context.Context, node *domain.Node, workDir *string) string {
	if workDir == nil {
		return ""
	}

	scope, err := s.policy.ScopeFor(ctx, node)
	if err != nil {
		slog.ErrorContext(ctx, "failed to resolve the node path policy, refusing the command",
			slog.Uint64("node_id", uint64(node.ID)),
			slog.String("error", err.Error()))

		return "path policy: " + err.Error()
	}

	if denial := scope.CheckWorkDir(*workDir); denial != nil {
		s.guard.DenyPath(ctx, ModuleNodeCmd, "execute_command", uint64(node.ID), denial)

		return denial.Error()
	}

	return ""
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

	if msg := s.checkWorkDir(ctx, node, req.WorkDir); msg != "" {
		return &nodecmd.ExecuteCommandResponse{Error: new(msg)}, nil
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
	opts           []NodeCmdOption
}

func NewNodeCmdHostLibraryFactory(
	commandService NodeCommandService,
	nodeRepo repositories.NodeRepository,
	guard *Guard,
	opts ...NodeCmdOption,
) *NodeCmdHostLibraryFactory {
	return &NodeCmdHostLibraryFactory{
		commandService: commandService,
		nodeRepo:       nodeRepo,
		guard:          guard,
		opts:           opts,
	}
}

func (f *NodeCmdHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return &NodeCmdHostLibrary{
		impl: NewNodeCmdService(f.commandService, f.nodeRepo, f.guard.For(pluginID), f.opts...),
	}
}
