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

type commandDispatcherTestSetup struct {
	dispatcher CommandDispatcher
	gateway    *fakeCommandGateway
	registry   *fakeConnectionChecker
	pubsub     pubsub.PubSub
	instanceID string
}

func setupCommandDispatcher(t *testing.T) *commandDispatcherTestSetup {
	t.Helper()

	gateway := &fakeCommandGateway{}
	registry := newFakeConnectionChecker()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	logger := slog.New(slog.DiscardHandler)
	instanceID := testInstanceID

	dispatcher := NewCommandDispatcher(ps, gateway, registry, instanceID, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, dispatcher.Start(ctx))

	return &commandDispatcherTestSetup{
		dispatcher: dispatcher,
		gateway:    gateway,
		registry:   registry,
		pubsub:     ps,
		instanceID: instanceID,
	}
}

func TestCommandDispatcher_DispatchCommand_Success(t *testing.T) {
	// ARRANGE
	s := setupCommandDispatcher(t)
	const nodeID uint64 = 7
	s.registry.setConnected(nodeID, true)

	var capturedCommand string
	s.gateway.requestCommand = func(_ context.Context, _ uint64, req *proto.CommandRequest) (*proto.CommandResult, error) {
		capturedCommand = req.Command

		return &proto.CommandResult{Output: []byte("ok"), ExitCode: 3}, nil
	}

	// ACT
	result, err := s.dispatcher.DispatchCommand(testContext(t), nodeID, &proto.CommandRequest{Command: "ls -la"})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ok", string(result.Output))
	assert.Equal(t, int32(3), result.ExitCode)
	assert.Equal(t, "ls -la", capturedCommand, "command request must reach gateway via pubsub payload")
}

func TestCommandDispatcher_DispatchCommand_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupCommandDispatcher(t)
	const nodeID uint64 = 8
	s.registry.setConnected(nodeID, true)

	s.gateway.requestCommand = func(_ context.Context, _ uint64, _ *proto.CommandRequest) (*proto.CommandResult, error) {
		return nil, errors.New("gateway boom")
	}

	// ACT
	result, err := s.dispatcher.DispatchCommand(testContext(t), nodeID, &proto.CommandRequest{Command: "ls"})

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "gateway boom", "gateway error must propagate via pubsub response")
}

func TestCommandDispatcher_NotConnected_TimesOut(t *testing.T) {
	// ARRANGE
	s := setupCommandDispatcher(t)
	const nodeID uint64 = 99
	s.registry.setConnected(nodeID, false)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// ACT
	result, err := s.dispatcher.DispatchCommand(ctx, nodeID, &proto.CommandRequest{Command: "ls"})

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "must surface context deadline when no daemon answers")
}

func TestCommandDispatcher_executeCommandRequest_UnmarshalError(t *testing.T) {
	// ARRANGE
	s := setupCommandDispatcher(t)
	d := s.dispatcher.(*commandDispatcher)

	// ACT
	resp := d.executeCommandRequest(testContext(t), messages.DaemonCommandRequestPayload{
		NodeID:    1,
		RequestID: "req-test",
		Data:      []byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9},
	})

	// ASSERT
	assert.NotEmpty(t, resp.Error, "malformed request payload must yield error response")
}

func TestNewCommandDispatcher_NilLoggerUsesDefault(t *testing.T) {
	// ARRANGE
	gateway := &fakeCommandGateway{}
	registry := newFakeConnectionChecker()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	// ACT
	dispatcher := NewCommandDispatcher(ps, gateway, registry, "id", nil)

	// ASSERT
	require.NotNil(t, dispatcher)
}
