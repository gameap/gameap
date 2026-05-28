package daemon

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeConsoleLogGateway is a configurable in-test fake for ConsoleLogGateway.
type fakeConsoleLogGateway struct {
	mu sync.Mutex

	requestConsoleLog func(ctx context.Context, nodeID uint64, serverID uint64, maxBytes int64) (*proto.ConsoleLogResponse, error)

	requestCalls atomic.Int32

	lastNodeID   uint64
	lastServerID uint64
	lastMaxBytes int64
}

func (f *fakeConsoleLogGateway) RequestConsoleLog(
	ctx context.Context, nodeID uint64, serverID uint64, maxBytes int64,
) (*proto.ConsoleLogResponse, error) {
	f.requestCalls.Add(1)
	f.mu.Lock()
	f.lastNodeID = nodeID
	f.lastServerID = serverID
	f.lastMaxBytes = maxBytes
	fn := f.requestConsoleLog
	f.mu.Unlock()

	if fn == nil {
		return &proto.ConsoleLogResponse{Success: true}, nil
	}

	return fn(ctx, nodeID, serverID, maxBytes)
}

// fakeConsoleLogDispatcher is a configurable in-test fake for ConsoleLogDispatcher.
type fakeConsoleLogDispatcher struct {
	mu sync.Mutex

	dispatchConsoleLog func(ctx context.Context, nodeID uint64, serverID uint64, maxBytes int64) (*proto.ConsoleLogResponse, error)

	dispatchCalls atomic.Int32

	lastNodeID   uint64
	lastServerID uint64
	lastMaxBytes int64
}

func (f *fakeConsoleLogDispatcher) Start(_ context.Context) error {
	return nil
}

func (f *fakeConsoleLogDispatcher) DispatchConsoleLog(
	ctx context.Context, nodeID uint64, serverID uint64, maxBytes int64,
) (*proto.ConsoleLogResponse, error) {
	f.dispatchCalls.Add(1)
	f.mu.Lock()
	f.lastNodeID = nodeID
	f.lastServerID = serverID
	f.lastMaxBytes = maxBytes
	fn := f.dispatchConsoleLog
	f.mu.Unlock()

	if fn == nil {
		return &proto.ConsoleLogResponse{Success: true}, nil
	}

	return fn(ctx, nodeID, serverID, maxBytes)
}

func TestConsoleLogService_GetConsoleLog(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		gatewayResp         *proto.ConsoleLogResponse
		gatewayErr          error
		dispatcherResp      *proto.ConsoleLogResponse
		dispatcherErr       error
	}

	tests := []struct {
		name              string
		setup             setup
		wantGatewayCalls  int32
		wantDispatchCalls int32
		wantOutput        string
		wantError         string
		wantSentinel      bool
	}{
		{
			name: "routes_to_gateway_when_IsConnected_true",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.ConsoleLogResponse{Success: true, Data: []byte("gateway log")},
			},
			wantGatewayCalls:  1,
			wantDispatchCalls: 0,
			wantOutput:        "gateway log",
		},
		{
			name: "routes_to_dispatcher_when_only_IsConnectedAnywhere_true",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherResp:      &proto.ConsoleLogResponse{Success: true, Data: []byte("dispatched log")},
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 1,
			wantOutput:        "dispatched log",
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
			wantSentinel:      true,
		},
		{
			name: "gateway_error_wrapped_with_message",
			setup: setup{
				isConnected: true,
				gatewayErr:  errors.New("transport boom"),
			},
			wantGatewayCalls: 1,
			wantError:        "gateway console log request: transport boom",
		},
		{
			name: "dispatcher_error_wrapped_with_message",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherErr:       errors.New("dispatch boom"),
			},
			wantDispatchCalls: 1,
			wantError:         "dispatched console log request: dispatch boom",
		},
		{
			name: "gateway_unsuccessful_response_surfaces_error",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.ConsoleLogResponse{Success: false, Error: "server not found"},
			},
			wantGatewayCalls: 1,
			wantError:        "server not found",
		},
		{
			name: "dispatcher_unsuccessful_response_surfaces_error",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherResp:      &proto.ConsoleLogResponse{Success: false, Error: "node busy"},
			},
			wantDispatchCalls: 1,
			wantError:         "node busy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			const nodeID uint64 = 42

			gateway := &fakeConsoleLogGateway{
				requestConsoleLog: func(_ context.Context, _ uint64, _ uint64, _ int64) (*proto.ConsoleLogResponse, error) {
					if tt.setup.gatewayErr != nil {
						return nil, tt.setup.gatewayErr
					}

					return tt.setup.gatewayResp, nil
				},
			}
			dispatcher := &fakeConsoleLogDispatcher{
				dispatchConsoleLog: func(_ context.Context, _ uint64, _ uint64, _ int64) (*proto.ConsoleLogResponse, error) {
					if tt.setup.dispatcherErr != nil {
						return nil, tt.setup.dispatcherErr
					}

					return tt.setup.dispatcherResp, nil
				},
			}
			registry := newFakeConnectionChecker()
			registry.setConnected(nodeID, tt.setup.isConnected)
			registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere

			service := NewConsoleLogService(gateway, registry, dispatcher, nil)

			// ACT
			got, err := service.GetConsoleLog(ctx, nodeID, 7, 1024)

			// ASSERT
			assert.Equal(t, tt.wantGatewayCalls, gateway.requestCalls.Load(), "gateway calls count mismatch")
			assert.Equal(t, tt.wantDispatchCalls, dispatcher.dispatchCalls.Load(), "dispatcher calls count mismatch")

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Empty(t, got, "output must be empty on error")

				if tt.wantSentinel {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOutput, got, "console log output mismatch")
		})
	}
}

func TestConsoleLogService_GetConsoleLog_propagatesServerIDAndMaxBytes(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	const nodeID uint64 = 5
	gateway := &fakeConsoleLogGateway{}
	dispatcher := &fakeConsoleLogDispatcher{}
	registry := newFakeConnectionChecker()
	registry.setConnected(nodeID, true)

	service := NewConsoleLogService(gateway, registry, dispatcher, nil)

	// ACT
	_, err := service.GetConsoleLog(ctx, nodeID, 99, 2048)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, nodeID, gateway.lastNodeID, "node id must be propagated to gateway")
	assert.Equal(t, uint64(99), gateway.lastServerID, "server id must be propagated to gateway")
	assert.Equal(t, int64(2048), gateway.lastMaxBytes, "positive maxBytes must be propagated unchanged")
}

func TestConsoleLogService_GetConsoleLog_defaultsNonPositiveMaxBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		maxBytes int64
	}{
		{name: "zero_maxBytes_uses_default", maxBytes: 0},
		{name: "negative_maxBytes_uses_default", maxBytes: -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			const nodeID uint64 = 6
			gateway := &fakeConsoleLogGateway{}
			dispatcher := &fakeConsoleLogDispatcher{}
			registry := newFakeConnectionChecker()
			registry.setConnected(nodeID, true)

			service := NewConsoleLogService(gateway, registry, dispatcher, nil)

			// ACT
			_, err := service.GetConsoleLog(ctx, nodeID, 1, tt.maxBytes)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, defaultConsoleLogMaxBytes, gateway.lastMaxBytes,
				"non-positive maxBytes must default to defaultConsoleLogMaxBytes")
		})
	}
}

func TestNewConsoleLogService_nilLogger_usesDefault(t *testing.T) {
	t.Parallel()

	// ARRANGE
	gateway := &fakeConsoleLogGateway{}
	dispatcher := &fakeConsoleLogDispatcher{}
	registry := newFakeConnectionChecker()

	// ACT
	service := NewConsoleLogService(gateway, registry, dispatcher, nil)

	// ASSERT
	require.NotNil(t, service, "service must be constructed")
	assert.NotNil(t, service.logger, "logger must default to a non-nil instance when nil is passed")
}
