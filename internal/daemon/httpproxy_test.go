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

// fakeHTTPProxyGateway is a configurable in-test fake for HTTPProxyGateway.
type fakeHTTPProxyGateway struct {
	mu sync.Mutex

	requestHTTPProxy func(ctx context.Context, nodeID uint64, req *proto.HTTPProxyRequest) (*proto.HTTPProxyResponse, error)

	requestCalls atomic.Int32

	lastNodeID  uint64
	lastRequest *proto.HTTPProxyRequest
}

func (f *fakeHTTPProxyGateway) RequestHTTPProxy(
	ctx context.Context, nodeID uint64, req *proto.HTTPProxyRequest,
) (*proto.HTTPProxyResponse, error) {
	f.requestCalls.Add(1)
	f.mu.Lock()
	f.lastNodeID = nodeID
	f.lastRequest = req
	fn := f.requestHTTPProxy
	f.mu.Unlock()

	if fn == nil {
		return &proto.HTTPProxyResponse{Success: true}, nil
	}

	return fn(ctx, nodeID, req)
}

// fakeHTTPProxyDispatcher is a configurable in-test fake for HTTPProxyDispatcher.
type fakeHTTPProxyDispatcher struct {
	mu sync.Mutex

	dispatchHTTPProxy func(ctx context.Context, nodeID uint64, req *proto.HTTPProxyRequest) (*proto.HTTPProxyResponse, error)

	dispatchCalls atomic.Int32

	lastNodeID  uint64
	lastRequest *proto.HTTPProxyRequest
}

func (f *fakeHTTPProxyDispatcher) Start(_ context.Context) error {
	return nil
}

func (f *fakeHTTPProxyDispatcher) DispatchHTTPProxy(
	ctx context.Context, nodeID uint64, req *proto.HTTPProxyRequest,
) (*proto.HTTPProxyResponse, error) {
	f.dispatchCalls.Add(1)
	f.mu.Lock()
	f.lastNodeID = nodeID
	f.lastRequest = req
	fn := f.dispatchHTTPProxy
	f.mu.Unlock()

	if fn == nil {
		return &proto.HTTPProxyResponse{Success: true}, nil
	}

	return fn(ctx, nodeID, req)
}

func TestHTTPProxyService_ProxyHTTP(t *testing.T) {
	t.Parallel()

	type setup struct {
		hasCapability       bool
		isConnected         bool
		isConnectedAnywhere bool
		gatewayResp         *proto.HTTPProxyResponse
		gatewayErr          error
		dispatcherResp      *proto.HTTPProxyResponse
		dispatcherErr       error
	}

	tests := []struct {
		name              string
		setup             setup
		wantGatewayCalls  int32
		wantDispatchCalls int32
		wantStatusCode    int32
		wantError         string
		wantSentinel      bool
	}{
		{
			name: "no_capability_returns_error_without_calling_transport",
			setup: setup{
				hasCapability: false,
				isConnected:   true,
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 0,
			wantError:         "does not support http_proxy",
		},
		{
			name: "routes_to_gateway_when_capable_and_IsConnected_true",
			setup: setup{
				hasCapability: true,
				isConnected:   true,
				gatewayResp:   &proto.HTTPProxyResponse{Success: true, StatusCode: 200, Body: []byte("ok")},
			},
			wantGatewayCalls:  1,
			wantDispatchCalls: 0,
			wantStatusCode:    200,
		},
		{
			name: "routes_to_dispatcher_when_capable_and_only_IsConnectedAnywhere_true",
			setup: setup{
				hasCapability:       true,
				isConnectedAnywhere: true,
				dispatcherResp:      &proto.HTTPProxyResponse{Success: true, StatusCode: 204},
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 1,
			wantStatusCode:    204,
		},
		{
			name: "returns_ErrDaemonNotConnected_when_capable_but_neither_connected",
			setup: setup{
				hasCapability:       true,
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
				hasCapability: true,
				isConnected:   true,
				gatewayErr:    errors.New("transport boom"),
			},
			wantGatewayCalls: 1,
			wantError:        "gateway http proxy request: transport boom",
		},
		{
			name: "dispatcher_error_wrapped_with_message",
			setup: setup{
				hasCapability:       true,
				isConnectedAnywhere: true,
				dispatcherErr:       errors.New("dispatch boom"),
			},
			wantDispatchCalls: 1,
			wantError:         "dispatched http proxy request: dispatch boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			const nodeID uint64 = 42

			gateway := &fakeHTTPProxyGateway{
				requestHTTPProxy: func(_ context.Context, _ uint64, _ *proto.HTTPProxyRequest) (*proto.HTTPProxyResponse, error) {
					if tt.setup.gatewayErr != nil {
						return nil, tt.setup.gatewayErr
					}

					return tt.setup.gatewayResp, nil
				},
			}
			dispatcher := &fakeHTTPProxyDispatcher{
				dispatchHTTPProxy: func(_ context.Context, _ uint64, _ *proto.HTTPProxyRequest) (*proto.HTTPProxyResponse, error) {
					if tt.setup.dispatcherErr != nil {
						return nil, tt.setup.dispatcherErr
					}

					return tt.setup.dispatcherResp, nil
				},
			}
			registry := newFakeConnectionChecker()
			registry.setConnected(nodeID, tt.setup.isConnected)
			registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere
			if tt.setup.hasCapability {
				registry.capabilities[nodeID] = map[string]bool{capabilityHTTPProxy: true}
			}

			service := NewHTTPProxyService(gateway, registry, dispatcher, nil)
			req := &proto.HTTPProxyRequest{Method: "GET", Url: "http://localhost/health"}

			// ACT
			got, err := service.ProxyHTTP(ctx, nodeID, req)

			// ASSERT
			assert.Equal(t, tt.wantGatewayCalls, gateway.requestCalls.Load(), "gateway calls count mismatch")
			assert.Equal(t, tt.wantDispatchCalls, dispatcher.dispatchCalls.Load(), "dispatcher calls count mismatch")

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, got, "response must be nil on error")

				if tt.wantSentinel {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantStatusCode, got.StatusCode, "status code must surface from proxied response")
		})
	}
}

func TestHTTPProxyService_ProxyHTTP_propagatesRequest(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	const nodeID uint64 = 11
	gateway := &fakeHTTPProxyGateway{}
	dispatcher := &fakeHTTPProxyDispatcher{}
	registry := newFakeConnectionChecker()
	registry.setConnected(nodeID, true)
	registry.capabilities[nodeID] = map[string]bool{capabilityHTTPProxy: true}

	service := NewHTTPProxyService(gateway, registry, dispatcher, nil)
	req := &proto.HTTPProxyRequest{Method: "POST", Url: "http://localhost/api"}

	// ACT
	_, err := service.ProxyHTTP(ctx, nodeID, req)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, gateway.lastRequest, "gateway must receive the request")
	assert.Equal(t, req, gateway.lastRequest, "request must be propagated to gateway unchanged")
	assert.Equal(t, nodeID, gateway.lastNodeID, "node id must be propagated to gateway")
}

func TestNewHTTPProxyService_nilLogger_usesDefault(t *testing.T) {
	t.Parallel()

	// ARRANGE
	gateway := &fakeHTTPProxyGateway{}
	dispatcher := &fakeHTTPProxyDispatcher{}
	registry := newFakeConnectionChecker()

	// ACT
	service := NewHTTPProxyService(gateway, registry, dispatcher, nil)

	// ASSERT
	require.NotNil(t, service, "service must be constructed")
	assert.NotNil(t, service.logger, "logger must default to a non-nil instance when nil is passed")
}
