package hostlibrary

import (
	"context"
	"errors"
	"testing"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodecmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errCommandTransport = errors.New("daemon unreachable")

var _ NodeCommandService = (*mockCommandService)(nil)

type mockCommandService struct {
	executeFunc func(
		ctx context.Context,
		node *domain.Node,
		command string,
		opts ...daemon.CommandServiceOption,
	) (*daemon.CommandResult, error)
}

func (m *mockCommandService) ExecuteCommand(
	ctx context.Context,
	node *domain.Node,
	command string,
	opts ...daemon.CommandServiceOption,
) (*daemon.CommandResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, node, command, opts...)
	}

	return &daemon.CommandResult{}, nil
}

// newNodeCmdService builds the real NodeCmdServiceImpl under test. repoFails
// swaps in a repository whose node lookup errors.
func newNodeCmdService(
	cmd NodeCommandService, repo *inmemory.NodeRepository, repoFails bool,
) *NodeCmdServiceImpl {
	if repoFails {
		return NewNodeCmdService(cmd, &errNodeRepository{NodeRepository: repo})
	}

	return NewNodeCmdService(cmd, repo)
}

func TestNodeCmdService_ExecuteCommand(t *testing.T) {
	tests := []struct {
		name         string
		setupRepo    func(*inmemory.NodeRepository)
		repoFails    bool
		setupCmd     func() *mockCommandService
		request      *nodecmd.ExecuteCommandRequest
		wantError    string
		wantOutput   string
		wantExitCode int32
	}{
		{
			name:      "node_not_found_returns_error",
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupCmd: func() *mockCommandService {
				return &mockCommandService{}
			},
			request: &nodecmd.ExecuteCommandRequest{
				NodeId:  999,
				Command: "echo hello",
			},
			wantError: "node not found",
		},
		{
			name:      "node_lookup_error_is_reported",
			setupRepo: seedTestNode,
			repoFails: true,
			setupCmd: func() *mockCommandService {
				return &mockCommandService{}
			},
			request: &nodecmd.ExecuteCommandRequest{
				NodeId:  1,
				Command: "echo hello",
			},
			wantError: "node lookup unavailable",
		},
		{
			name:      "command_executed_successfully",
			setupRepo: seedTestNode,
			setupCmd: func() *mockCommandService {
				return &mockCommandService{
					executeFunc: func(
						_ context.Context, _ *domain.Node, _ string, _ ...daemon.CommandServiceOption,
					) (*daemon.CommandResult, error) {
						return &daemon.CommandResult{
							Output:   "hello\n",
							ExitCode: 0,
						}, nil
					},
				}
			},
			request: &nodecmd.ExecuteCommandRequest{
				NodeId:  1,
				Command: "echo hello",
			},
			wantOutput:   "hello\n",
			wantExitCode: 0,
		},
		{
			name:      "command_with_workdir",
			setupRepo: seedTestNode,
			setupCmd: func() *mockCommandService {
				return &mockCommandService{
					executeFunc: func(
						_ context.Context, _ *domain.Node, _ string, opts ...daemon.CommandServiceOption,
					) (*daemon.CommandResult, error) {
						if len(opts) != 1 {
							return nil, errCommandTransport
						}

						return &daemon.CommandResult{
							Output:   "/home/user\n",
							ExitCode: 0,
						}, nil
					},
				}
			},
			request: &nodecmd.ExecuteCommandRequest{
				NodeId:  1,
				Command: "pwd",
				WorkDir: new("/home/user"),
			},
			wantOutput:   "/home/user\n",
			wantExitCode: 0,
		},
		{
			name:      "command_returns_nonzero_exit_code",
			setupRepo: seedTestNode,
			setupCmd: func() *mockCommandService {
				return &mockCommandService{
					executeFunc: func(
						_ context.Context, _ *domain.Node, _ string, _ ...daemon.CommandServiceOption,
					) (*daemon.CommandResult, error) {
						return &daemon.CommandResult{
							Output:   "command not found",
							ExitCode: 127,
						}, nil
					},
				}
			},
			request: &nodecmd.ExecuteCommandRequest{
				NodeId:  1,
				Command: "nonexistent_command",
			},
			wantOutput:   "command not found",
			wantExitCode: 127,
		},
		{
			name:      "execution_error_is_reported",
			setupRepo: seedTestNode,
			setupCmd: func() *mockCommandService {
				return &mockCommandService{
					executeFunc: func(
						_ context.Context, _ *domain.Node, _ string, _ ...daemon.CommandServiceOption,
					) (*daemon.CommandResult, error) {
						return nil, errCommandTransport
					},
				}
			},
			request: &nodecmd.ExecuteCommandRequest{
				NodeId:  1,
				Command: "echo hello",
			},
			wantError: "daemon unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := setupNodeFSRepo(tt.setupRepo)
			svc := newNodeCmdService(tt.setupCmd(), repo, tt.repoFails)

			// ACT
			resp, err := svc.ExecuteCommand(context.Background(), tt.request)

			// ASSERT
			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")

				return
			}

			assert.Nil(t, resp.Error)
			assert.Equal(t, tt.wantOutput, resp.Output)
			assert.Equal(t, tt.wantExitCode, resp.ExitCode)
		})
	}
}

func TestNodeCmdService_ExecuteCommand_ForwardsCommandAndWorkDir(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewNodeRepository()
	seedTestNode(repo)

	var gotCommand string
	var gotOptCount int
	cmdSvc := &mockCommandService{
		executeFunc: func(
			_ context.Context, _ *domain.Node, command string, opts ...daemon.CommandServiceOption,
		) (*daemon.CommandResult, error) {
			gotCommand, gotOptCount = command, len(opts)

			return &daemon.CommandResult{Output: "ok", ExitCode: 0}, nil
		},
	}
	svc := NewNodeCmdService(cmdSvc, repo)

	// ACT
	resp, err := svc.ExecuteCommand(context.Background(), &nodecmd.ExecuteCommandRequest{
		NodeId:  1,
		Command: "ls -la",
		WorkDir: new("/srv"),
	})

	// ASSERT
	require.NoError(t, err)
	assert.Nil(t, resp.Error)
	assert.Equal(t, "ls -la", gotCommand, "command must be forwarded unchanged")
	assert.Equal(t, 1, gotOptCount, "work dir must be translated into a single command option")
}

func TestNodeCmdService_ExecuteCommand_OmitsWorkDirOptionWhenUnset(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewNodeRepository()
	seedTestNode(repo)

	var gotOptCount int
	cmdSvc := &mockCommandService{
		executeFunc: func(
			_ context.Context, _ *domain.Node, _ string, opts ...daemon.CommandServiceOption,
		) (*daemon.CommandResult, error) {
			gotOptCount = len(opts)

			return &daemon.CommandResult{Output: "ok", ExitCode: 0}, nil
		},
	}
	svc := NewNodeCmdService(cmdSvc, repo)

	// ACT
	_, err := svc.ExecuteCommand(context.Background(), &nodecmd.ExecuteCommandRequest{
		NodeId:  1,
		Command: "uptime",
	})

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, 0, gotOptCount, "no options must be passed when work dir is unset")
}

func TestNewNodeCmdHostLibrary(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	lib := NewNodeCmdHostLibrary(nil, repo)

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
