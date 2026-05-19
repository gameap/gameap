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

// fakeStatusGateway is a configurable in-test fake for StatusGateway.
type fakeStatusGateway struct {
	mu sync.Mutex

	requestStatus func(ctx context.Context, nodeID uint64) (*proto.StatusResponse, error)

	requestCalls atomic.Int32

	lastNodeID uint64
}

func (f *fakeStatusGateway) RequestStatus(
	ctx context.Context, nodeID uint64,
) (*proto.StatusResponse, error) {
	f.requestCalls.Add(1)
	f.mu.Lock()
	f.lastNodeID = nodeID
	fn := f.requestStatus
	f.mu.Unlock()

	if fn == nil {
		return &proto.StatusResponse{Success: true}, nil
	}

	return fn(ctx, nodeID)
}

// fakeStatusDispatcher is a configurable in-test fake for StatusDispatcher.
type fakeStatusDispatcher struct {
	mu sync.Mutex

	dispatchStatus func(ctx context.Context, nodeID uint64) (*proto.StatusResponse, error)

	dispatchCalls atomic.Int32

	lastNodeID uint64
}

func (f *fakeStatusDispatcher) Start(_ context.Context) error {
	return nil
}

func (f *fakeStatusDispatcher) DispatchStatus(
	ctx context.Context, nodeID uint64,
) (*proto.StatusResponse, error) {
	f.dispatchCalls.Add(1)
	f.mu.Lock()
	f.lastNodeID = nodeID
	fn := f.dispatchStatus
	f.mu.Unlock()

	if fn == nil {
		return &proto.StatusResponse{Success: true}, nil
	}

	return fn(ctx, nodeID)
}

func TestStatusService_Status(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		gatewayResp         *proto.StatusResponse
		gatewayErr          error
		dispatcherResp      *proto.StatusResponse
		dispatcherErr       error
	}

	tests := []struct {
		name              string
		setup             setup
		wantGatewayCalls  int32
		wantDispatchCalls int32
		wantStatus        *NodeStatus
		wantError         string
		wantSentinel      bool
	}{
		{
			name: "routes_to_gateway_when_IsConnected_true",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.StatusResponse{
					Success:       true,
					Version:       "1.0",
					BuildDate:     "2026-04-01",
					UptimeSeconds: 60,
					WorkingTasks:  2,
					WaitingTasks:  3,
					OnlineServers: 5,
				},
			},
			wantGatewayCalls:  1,
			wantDispatchCalls: 0,
			wantStatus: &NodeStatus{
				Uptime:        60 * time.Second,
				Version:       "1.0",
				BuildDate:     "2026-04-01",
				WorkingTasks:  2,
				WaitingTasks:  3,
				OnlineServers: 5,
			},
		},
		{
			name: "routes_to_dispatcher_when_only_IsConnectedAnywhere_true",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherResp: &proto.StatusResponse{
					Success:       true,
					Version:       "1.1",
					BuildDate:     "2026-04-02",
					UptimeSeconds: 120,
					OnlineServers: 7,
				},
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 1,
			wantStatus: &NodeStatus{
				Uptime:        120 * time.Second,
				Version:       "1.1",
				BuildDate:     "2026-04-02",
				OnlineServers: 7,
			},
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
			name: "gateway_returns_error_response_when_Success_false",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.StatusResponse{Success: false, Error: "daemon overloaded"},
			},
			wantGatewayCalls:  1,
			wantDispatchCalls: 0,
			wantError:         "status request: daemon overloaded",
		},
		{
			name: "dispatcher_returns_error_response_when_Success_false",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherResp:      &proto.StatusResponse{Success: false, Error: "node busy"},
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 1,
			wantError:         "status request: node busy",
		},
		{
			name: "gateway_transport_error_wrapped",
			setup: setup{
				isConnected: true,
				gatewayErr:  errors.New("boom"),
			},
			wantGatewayCalls:  1,
			wantDispatchCalls: 0,
			wantError:         "gateway status request: boom",
		},
		{
			name: "dispatcher_transport_error_wrapped",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherErr:       errors.New("boom"),
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 1,
			wantError:         "dispatched status request: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			node := &domain.Node{ID: 99}

			gateway := &fakeStatusGateway{
				requestStatus: func(_ context.Context, _ uint64) (*proto.StatusResponse, error) {
					if tt.setup.gatewayErr != nil {
						return nil, tt.setup.gatewayErr
					}

					return tt.setup.gatewayResp, nil
				},
			}
			dispatcher := &fakeStatusDispatcher{
				dispatchStatus: func(_ context.Context, _ uint64) (*proto.StatusResponse, error) {
					if tt.setup.dispatcherErr != nil {
						return nil, tt.setup.dispatcherErr
					}

					return tt.setup.dispatcherResp, nil
				},
			}
			registry := newFakeConnectionChecker()
			registry.setConnected(99, tt.setup.isConnected)
			registry.connectedAnywhere[99] = tt.setup.isConnectedAnywhere

			service := NewStatusService(gateway, registry, dispatcher, nil)

			// ACT
			got, err := service.Status(ctx, node)

			// ASSERT
			assert.Equal(t, tt.wantGatewayCalls, gateway.requestCalls.Load(), "gateway calls count mismatch")
			assert.Equal(t, tt.wantDispatchCalls, dispatcher.dispatchCalls.Load(), "dispatcher calls count mismatch")

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, got, "status must be nil on error")

				if tt.wantSentinel {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantStatus.Uptime, got.Uptime, "uptime must match")
			assert.Equal(t, tt.wantStatus.Version, got.Version, "version must match")
			assert.Equal(t, tt.wantStatus.BuildDate, got.BuildDate, "build date must match")
			assert.Equal(t, tt.wantStatus.WorkingTasks, got.WorkingTasks, "working tasks must match")
			assert.Equal(t, tt.wantStatus.WaitingTasks, got.WaitingTasks, "waiting tasks must match")
			assert.Equal(t, tt.wantStatus.OnlineServers, got.OnlineServers, "online servers must match")
		})
	}
}

func TestStatusService_Version(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		gatewayResp         *proto.StatusResponse
		gatewayErr          error
		dispatcherResp      *proto.StatusResponse
		dispatcherErr       error
	}

	tests := []struct {
		name              string
		setup             setup
		wantGatewayCalls  int32
		wantDispatchCalls int32
		wantVersion       *NodeVersion
		wantError         string
		wantSentinel      bool
	}{
		{
			name: "gateway_returns_version_on_success",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.StatusResponse{Version: "2.5.1", BuildDate: "2026-01-15"},
			},
			wantGatewayCalls:  1,
			wantDispatchCalls: 0,
			wantVersion:       &NodeVersion{Version: "2.5.1", BuildDate: "2026-01-15"},
		},
		{
			name: "dispatcher_returns_version_on_anywhere",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherResp:      &proto.StatusResponse{Version: "3.0.0", BuildDate: "2026-02-10"},
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 1,
			wantVersion:       &NodeVersion{Version: "3.0.0", BuildDate: "2026-02-10"},
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
			name: "gateway_transport_error_wrapped",
			setup: setup{
				isConnected: true,
				gatewayErr:  errors.New("boom"),
			},
			wantGatewayCalls:  1,
			wantDispatchCalls: 0,
			wantError:         "gateway status request: boom",
		},
		{
			name: "dispatcher_transport_error_wrapped",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherErr:       errors.New("boom"),
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 1,
			wantError:         "dispatched status request: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			node := &domain.Node{ID: 123}

			gateway := &fakeStatusGateway{
				requestStatus: func(_ context.Context, _ uint64) (*proto.StatusResponse, error) {
					if tt.setup.gatewayErr != nil {
						return nil, tt.setup.gatewayErr
					}

					return tt.setup.gatewayResp, nil
				},
			}
			dispatcher := &fakeStatusDispatcher{
				dispatchStatus: func(_ context.Context, _ uint64) (*proto.StatusResponse, error) {
					if tt.setup.dispatcherErr != nil {
						return nil, tt.setup.dispatcherErr
					}

					return tt.setup.dispatcherResp, nil
				},
			}
			registry := newFakeConnectionChecker()
			registry.setConnected(123, tt.setup.isConnected)
			registry.connectedAnywhere[123] = tt.setup.isConnectedAnywhere

			service := NewStatusService(gateway, registry, dispatcher, nil)

			// ACT
			got, err := service.Version(ctx, node)

			// ASSERT
			assert.Equal(t, tt.wantGatewayCalls, gateway.requestCalls.Load(), "gateway calls count mismatch")
			assert.Equal(t, tt.wantDispatchCalls, dispatcher.dispatchCalls.Load(), "dispatcher calls count mismatch")

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, got, "version must be nil on error")

				if tt.wantSentinel {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantVersion.Version, got.Version, "version must match")
			assert.Equal(t, tt.wantVersion.BuildDate, got.BuildDate, "build date must match")
		})
	}
}

func TestStatusService_ConnectionType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		isConnected         bool
		isConnectedAnywhere bool
		want                string
	}{
		{
			name:        "returns_grpc_when_IsConnected_true",
			isConnected: true,
			want:        ConnectionTypeGRPC,
		},
		{
			name:                "returns_grpc_when_only_IsConnectedAnywhere_true",
			isConnectedAnywhere: true,
			want:                ConnectionTypeGRPC,
		},
		{
			name: "returns_none_when_neither_connected",
			want: ConnectionTypeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			const nodeID uint64 = 55
			gateway := &fakeStatusGateway{}
			dispatcher := &fakeStatusDispatcher{}
			registry := newFakeConnectionChecker()
			registry.setConnected(nodeID, tt.isConnected)
			registry.connectedAnywhere[nodeID] = tt.isConnectedAnywhere

			service := NewStatusService(gateway, registry, dispatcher, nil)

			// ACT
			got := service.ConnectionType(nodeID)

			// ASSERT
			assert.Equal(t, tt.want, got, "connection type mismatch")
		})
	}
}

func TestNewStatusService_nilLogger_usesDefault(t *testing.T) {
	t.Parallel()

	// ARRANGE
	gateway := &fakeStatusGateway{}
	dispatcher := &fakeStatusDispatcher{}
	registry := newFakeConnectionChecker()

	// ACT
	service := NewStatusService(gateway, registry, dispatcher, nil)

	// ASSERT
	require.NotNil(t, service, "service must be constructed")
	assert.NotNil(t, service.logger, "logger must default to a non-nil instance when nil is passed")
}
