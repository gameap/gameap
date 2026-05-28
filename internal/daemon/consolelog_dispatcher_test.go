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

type consoleLogDispatcherTestSetup struct {
	dispatcher ConsoleLogDispatcher
	gateway    *fakeConsoleLogGateway
	registry   *fakeConnectionChecker
	pubsub     pubsub.PubSub
	instanceID string
}

func setupConsoleLogDispatcher(t *testing.T) *consoleLogDispatcherTestSetup {
	t.Helper()

	gateway := &fakeConsoleLogGateway{}
	registry := newFakeConnectionChecker()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	logger := slog.New(slog.DiscardHandler)
	instanceID := testInstanceID

	dispatcher := NewConsoleLogDispatcher(ps, gateway, registry, instanceID, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, dispatcher.Start(ctx))

	return &consoleLogDispatcherTestSetup{
		dispatcher: dispatcher,
		gateway:    gateway,
		registry:   registry,
		pubsub:     ps,
		instanceID: instanceID,
	}
}

func TestConsoleLogDispatcher_DispatchConsoleLog_Success(t *testing.T) {
	// ARRANGE
	s := setupConsoleLogDispatcher(t)
	const nodeID uint64 = 7
	s.registry.setConnected(nodeID, true)

	var capturedServerID uint64
	var capturedMaxBytes int64
	s.gateway.requestConsoleLog = func(_ context.Context, _ uint64, serverID uint64, maxBytes int64) (*proto.ConsoleLogResponse, error) {
		capturedServerID = serverID
		capturedMaxBytes = maxBytes

		return &proto.ConsoleLogResponse{Success: true, Data: []byte("console output")}, nil
	}

	// ACT
	resp, err := s.dispatcher.DispatchConsoleLog(testContext(t), nodeID, 55, 4096)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "console output", string(resp.Data))
	assert.Equal(t, uint64(55), capturedServerID, "server id must reach gateway via pubsub payload")
	assert.Equal(t, int64(4096), capturedMaxBytes, "max bytes must reach gateway via pubsub payload")
}

func TestConsoleLogDispatcher_DispatchConsoleLog_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupConsoleLogDispatcher(t)
	const nodeID uint64 = 8
	s.registry.setConnected(nodeID, true)

	s.gateway.requestConsoleLog = func(_ context.Context, _ uint64, _ uint64, _ int64) (*proto.ConsoleLogResponse, error) {
		return nil, errors.New("gateway boom")
	}

	// ACT
	resp, err := s.dispatcher.DispatchConsoleLog(testContext(t), nodeID, 1, 1024)

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "gateway boom", "gateway error must propagate via pubsub response")
}

func TestConsoleLogDispatcher_NotConnected_TimesOut(t *testing.T) {
	// ARRANGE
	s := setupConsoleLogDispatcher(t)
	const nodeID uint64 = 99
	s.registry.setConnected(nodeID, false)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// ACT
	resp, err := s.dispatcher.DispatchConsoleLog(ctx, nodeID, 1, 1024)

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "must surface context deadline when no daemon answers")
}

func TestConsoleLogDispatcher_executeRequest_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupConsoleLogDispatcher(t)
	d := s.dispatcher.(*consoleLogDispatcher)
	s.gateway.requestConsoleLog = func(_ context.Context, _ uint64, _ uint64, _ int64) (*proto.ConsoleLogResponse, error) {
		return nil, errors.New("gw err")
	}

	// ACT
	resp := d.executeRequest(testContext(t), messages.DaemonConsoleLogRequestPayload{
		NodeID:    1,
		RequestID: "req-test",
		ServerID:  2,
		MaxBytes:  1024,
	})

	// ASSERT
	assert.Equal(t, "gw err", resp.Error, "gateway error must surface as payload error")
}

func TestNewConsoleLogDispatcher_NilLoggerUsesDefault(t *testing.T) {
	// ARRANGE
	gateway := &fakeConsoleLogGateway{}
	registry := newFakeConnectionChecker()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	// ACT
	dispatcher := NewConsoleLogDispatcher(ps, gateway, registry, "id", nil)

	// ASSERT
	require.NotNil(t, dispatcher)
}
