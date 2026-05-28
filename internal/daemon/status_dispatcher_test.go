package daemon

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statusDispatcherTestSetup struct {
	dispatcher StatusDispatcher
	gateway    *fakeStatusGateway
	registry   *fakeConnectionChecker
	pubsub     pubsub.PubSub
	instanceID string
}

func setupStatusDispatcher(t *testing.T) *statusDispatcherTestSetup {
	t.Helper()

	gateway := &fakeStatusGateway{}
	registry := newFakeConnectionChecker()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	logger := slog.New(slog.DiscardHandler)
	instanceID := testInstanceID

	dispatcher := NewStatusDispatcher(ps, gateway, registry, instanceID, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, dispatcher.Start(ctx))

	return &statusDispatcherTestSetup{
		dispatcher: dispatcher,
		gateway:    gateway,
		registry:   registry,
		pubsub:     ps,
		instanceID: instanceID,
	}
}

func TestStatusDispatcher_DispatchStatus_Success(t *testing.T) {
	// ARRANGE
	s := setupStatusDispatcher(t)
	const nodeID uint64 = 7
	s.registry.setConnected(nodeID, true)

	var capturedNodeID uint64
	s.gateway.requestStatus = func(_ context.Context, n uint64) (*proto.StatusResponse, error) {
		capturedNodeID = n

		return &proto.StatusResponse{Success: true, Version: "1.2", UptimeSeconds: 60}, nil
	}

	// ACT
	resp, err := s.dispatcher.DispatchStatus(testContext(t), nodeID)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "1.2", resp.Version)
	assert.Equal(t, int64(60), resp.UptimeSeconds)
	assert.Equal(t, nodeID, capturedNodeID, "node id must reach gateway via pubsub payload")
}

func TestStatusDispatcher_DispatchStatus_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupStatusDispatcher(t)
	const nodeID uint64 = 8
	s.registry.setConnected(nodeID, true)

	s.gateway.requestStatus = func(_ context.Context, _ uint64) (*proto.StatusResponse, error) {
		return nil, errors.New("gateway boom")
	}

	// ACT
	resp, err := s.dispatcher.DispatchStatus(testContext(t), nodeID)

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "gateway boom", "gateway error must propagate via pubsub response")
}

func TestStatusDispatcher_NotConnected_TimesOut(t *testing.T) {
	// ARRANGE
	s := setupStatusDispatcher(t)
	const nodeID uint64 = 99
	s.registry.setConnected(nodeID, false)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// ACT
	resp, err := s.dispatcher.DispatchStatus(ctx, nodeID)

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "must surface context deadline when no daemon answers")
}

func TestStatusDispatcher_executeStatusRequest_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupStatusDispatcher(t)
	d := s.dispatcher.(*statusDispatcher)
	s.gateway.requestStatus = func(_ context.Context, _ uint64) (*proto.StatusResponse, error) {
		return nil, errors.New("gw err")
	}

	// ACT
	resp := d.executeStatusRequest(testContext(t), messages.DaemonStatusRequestPayload{
		NodeID:    1,
		RequestID: "req-test",
	})

	// ASSERT
	assert.Equal(t, "gw err", resp.Error, "gateway error must surface as payload error")
}

func TestNewStatusDispatcher_NilLoggerUsesDefault(t *testing.T) {
	// ARRANGE
	gateway := &fakeStatusGateway{}
	registry := newFakeConnectionChecker()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	// ACT
	dispatcher := NewStatusDispatcher(ps, gateway, registry, "id", nil)

	// ASSERT
	require.NotNil(t, dispatcher)
}
