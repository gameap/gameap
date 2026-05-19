package daemon

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCommandGateway is a configurable in-test fake for CommandGateway.
type fakeCommandGateway struct {
	mu sync.Mutex

	requestCommand func(ctx context.Context, nodeID uint64, req *proto.CommandRequest) (*proto.CommandResult, error)

	requestCalls atomic.Int32

	lastNodeID  uint64
	lastRequest *proto.CommandRequest
}

func (f *fakeCommandGateway) RequestCommand(
	ctx context.Context, nodeID uint64, req *proto.CommandRequest,
) (*proto.CommandResult, error) {
	f.requestCalls.Add(1)
	f.mu.Lock()
	f.lastNodeID = nodeID
	f.lastRequest = req
	fn := f.requestCommand
	f.mu.Unlock()

	if fn == nil {
		return &proto.CommandResult{}, nil
	}

	return fn(ctx, nodeID, req)
}

// fakeCommandDispatcher is a configurable in-test fake for CommandDispatcher.
type fakeCommandDispatcher struct {
	mu sync.Mutex

	dispatchCommand func(ctx context.Context, nodeID uint64, req *proto.CommandRequest) (*proto.CommandResult, error)

	dispatchCalls atomic.Int32

	lastNodeID  uint64
	lastRequest *proto.CommandRequest
}

func (f *fakeCommandDispatcher) Start(_ context.Context) error {
	return nil
}

func (f *fakeCommandDispatcher) DispatchCommand(
	ctx context.Context, nodeID uint64, req *proto.CommandRequest,
) (*proto.CommandResult, error) {
	f.dispatchCalls.Add(1)
	f.mu.Lock()
	f.lastNodeID = nodeID
	f.lastRequest = req
	fn := f.dispatchCommand
	f.mu.Unlock()

	if fn == nil {
		return &proto.CommandResult{}, nil
	}

	return fn(ctx, nodeID, req)
}

func TestCommandService_ExecuteCommand(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		gatewayResult       *proto.CommandResult
		gatewayErr          error
		dispatcherResult    *proto.CommandResult
		dispatcherErr       error
	}

	tests := []struct {
		name              string
		setup             setup
		wantGatewayCalls  int32
		wantDispatchCalls int32
		wantResult        *CommandResult
		wantError         string
	}{
		{
			name: "routes_to_gateway_when_IsConnected_true",
			setup: setup{
				isConnected:   true,
				gatewayResult: &proto.CommandResult{Output: []byte("hello"), ExitCode: 0},
			},
			wantGatewayCalls:  1,
			wantDispatchCalls: 0,
			wantResult:        &CommandResult{Output: "hello", ExitCode: 0},
		},
		{
			name: "routes_to_dispatcher_when_only_IsConnectedAnywhere_true",
			setup: setup{
				isConnected:         false,
				isConnectedAnywhere: true,
				dispatcherResult:    &proto.CommandResult{Output: []byte("world"), ExitCode: 1},
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 1,
			wantResult:        &CommandResult{Output: "world", ExitCode: 1},
		},
		{
			name: "returns_ErrDaemonNotConnected_when_neither_connected",
			setup: setup{
				isConnected:         false,
				isConnectedAnywhere: false,
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 0,
			wantError:         "daemon not connected",
		},
		{
			name: "gateway_error_wrapped_with_message",
			setup: setup{
				isConnected: true,
				gatewayErr:  errors.New("transport boom"),
			},
			wantGatewayCalls:  1,
			wantDispatchCalls: 0,
			wantError:         "gateway command request: transport boom",
		},
		{
			name: "dispatcher_error_wrapped_with_message",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherErr:       errors.New("dispatch boom"),
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 1,
			wantError:         "dispatched command request: dispatch boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			node := &domain.Node{ID: 42}

			gateway := &fakeCommandGateway{
				requestCommand: func(_ context.Context, _ uint64, _ *proto.CommandRequest) (*proto.CommandResult, error) {
					if tt.setup.gatewayErr != nil {
						return nil, tt.setup.gatewayErr
					}

					return tt.setup.gatewayResult, nil
				},
			}
			dispatcher := &fakeCommandDispatcher{
				dispatchCommand: func(_ context.Context, _ uint64, _ *proto.CommandRequest) (*proto.CommandResult, error) {
					if tt.setup.dispatcherErr != nil {
						return nil, tt.setup.dispatcherErr
					}

					return tt.setup.dispatcherResult, nil
				},
			}
			registry := newFakeConnectionChecker()
			registry.setConnected(42, tt.setup.isConnected)
			registry.connectedAnywhere[42] = tt.setup.isConnectedAnywhere

			service := NewCommandService(gateway, registry, dispatcher, nil)

			// ACT
			got, err := service.ExecuteCommand(ctx, node, "ls")

			// ASSERT
			assert.Equal(t, tt.wantGatewayCalls, gateway.requestCalls.Load(), "gateway calls count mismatch")
			assert.Equal(t, tt.wantDispatchCalls, dispatcher.dispatchCalls.Load(), "dispatcher calls count mismatch")

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, got, "result must be nil on error")

				if tt.name == "returns_ErrDaemonNotConnected_when_neither_connected" {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantResult.Output, got.Output, "output must match")
			assert.Equal(t, tt.wantResult.ExitCode, got.ExitCode, "exit code must match")
		})
	}
}

func TestCommandService_ExecuteCommand_appliesWorkDirOption(t *testing.T) {
	t.Parallel()

	t.Run("uses_provided_workdir_when_option_set", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		node := &domain.Node{ID: 7}
		gateway := &fakeCommandGateway{}
		dispatcher := &fakeCommandDispatcher{}
		registry := newFakeConnectionChecker()
		registry.setConnected(7, true)

		service := NewCommandService(gateway, registry, dispatcher, nil)

		// ACT
		_, err := service.ExecuteCommand(ctx, node, "ls", CommandServiceOptionWithWorkDir("/srv/games/test"))

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, gateway.lastRequest, "gateway must have been invoked")
		assert.Equal(t, "/srv/games/test", gateway.lastRequest.WorkDir, "work_dir must be propagated to proto request")
	})

	t.Run("uses_default_workdir_when_option_absent", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		node := &domain.Node{ID: 7}
		gateway := &fakeCommandGateway{}
		dispatcher := &fakeCommandDispatcher{}
		registry := newFakeConnectionChecker()
		registry.setConnected(7, true)

		service := NewCommandService(gateway, registry, dispatcher, nil)

		// ACT
		_, err := service.ExecuteCommand(ctx, node, "ls")

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, gateway.lastRequest, "gateway must have been invoked")
		assert.Equal(t, "/", gateway.lastRequest.WorkDir, "default work_dir must be '/'")
	})
}

func TestCommandService_ExecuteCommand_setsCommandIDAndTimeout(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	node := &domain.Node{ID: 11}
	gateway := &fakeCommandGateway{}
	dispatcher := &fakeCommandDispatcher{}
	registry := newFakeConnectionChecker()
	registry.setConnected(11, true)

	service := NewCommandService(gateway, registry, dispatcher, nil)

	// ACT
	_, err := service.ExecuteCommand(ctx, node, "ls -la")

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, gateway.lastRequest, "gateway must have been invoked")
	assert.Equal(t, "ls -la", gateway.lastRequest.Command, "command string must be propagated")
	assert.NotEmpty(t, gateway.lastRequest.CommandId, "command id must be auto-generated and non-empty")
	require.NotNil(t, gateway.lastRequest.Timeout, "timeout must be populated")
	assert.Equal(t, 300*time.Second, gateway.lastRequest.Timeout.AsDuration(), "default timeout must be 300s")
	assert.Equal(t, uint64(11), gateway.lastNodeID, "node id must be propagated")
}

func TestNewCommandService_nilLogger_usesDefault(t *testing.T) {
	t.Parallel()

	// ARRANGE
	gateway := &fakeCommandGateway{}
	dispatcher := &fakeCommandDispatcher{}
	registry := newFakeConnectionChecker()

	// ACT
	service := NewCommandService(gateway, registry, dispatcher, nil)

	// ASSERT
	require.NotNil(t, service, "service must be constructed")
	assert.NotNil(t, service.logger, "logger must default to a non-nil instance when nil is passed")
}
