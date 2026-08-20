package console

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/grpc/handlers"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingStream tracks the *proto.GatewayMessage values pushed via Send.
// It satisfies session.Stream so a real session.Registry can dispatch to it.
type recordingStream struct {
	*fakeRegistryStream

	sendErr error
	sent    []*proto.GatewayMessage
}

func newRecordingStream() *recordingStream {
	return &recordingStream{fakeRegistryStream: newFakeRegistryStream()}
}

func (r *recordingStream) Send(msg *proto.GatewayMessage) error {
	if r.sendErr != nil {
		return r.sendErr
	}
	r.sent = append(r.sent, msg)

	return nil
}

// ---------- newGRPCMessageHandler ----------

func TestHandler_NewGRPCMessageHandler(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string

		canSend         bool
		rbac            base.RBAC
		msgType         string
		payload         any
		streamSendErr   error
		registerSession bool // true = register a session for nodeID 7 so SendCommand goes via stream

		wantWireFrameType  string
		wantWireFrameError string
		wantStreamSends    int
		wantTracked        bool // commandID was added to tracked list (via cleanup count)
	}{
		{
			name:    "non_command_message_type_is_ignored",
			canSend: true,
			rbac:    allowAllRBAC{},
			msgType: typeConsoleHistory,
			payload: consoleCommandPayload{Command: "ignored"},
		},
		{
			name:               "user_without_send_permission_gets_error_frame",
			canSend:            false,
			rbac:               allowAllRBAC{},
			msgType:            typeConsoleCommand,
			payload:            consoleCommandPayload{Command: "say denied"},
			wantWireFrameType:  ws.TypeError,
			wantWireFrameError: "permission denied: cannot send commands",
		},
		{
			name:    "malformed_payload_is_silently_dropped",
			canSend: true,
			rbac:    allowAllRBAC{},
			msgType: typeConsoleCommand,
			payload: 12345,
		},
		{
			name:    "empty_command_string_is_silently_dropped",
			canSend: true,
			rbac:    allowAllRBAC{},
			msgType: typeConsoleCommand,
			payload: consoleCommandPayload{Command: ""},
		},
		{
			name:               "rbac_recheck_failure_returns_error_frame",
			canSend:            true,
			rbac:               denyAllRBAC{},
			msgType:            typeConsoleCommand,
			payload:            consoleCommandPayload{Command: "say re-denied"},
			wantWireFrameType:  ws.TypeError,
			wantWireFrameError: "permission denied: cannot send commands",
		},
		{
			name:              "valid_command_with_no_session_falls_back_to_pubsub_dispatch_and_no_error",
			canSend:           true,
			rbac:              allowAllRBAC{},
			msgType:           typeConsoleCommand,
			payload:           consoleCommandPayload{Command: "say via-pubsub"},
			registerSession:   false, // dispatch goes via pubsub publish; no error from registry
			wantWireFrameType: "",    // no frame
			wantStreamSends:   0,
			wantTracked:       true,
		},
		{
			name:            "valid_command_with_local_session_sends_via_stream",
			canSend:         true,
			rbac:            allowAllRBAC{},
			msgType:         typeConsoleCommand,
			payload:         consoleCommandPayload{Command: "say via-stream"},
			registerSession: true,
			wantStreamSends: 1,
			wantTracked:     true,
		},
		{
			name:               "stream_send_error_emits_error_frame",
			canSend:            true,
			rbac:               allowAllRBAC{},
			msgType:            typeConsoleCommand,
			payload:            consoleCommandPayload{Command: "say boom"},
			registerSession:    true,
			streamSendErr:      errors.New("stream broken"),
			wantWireFrameType:  ws.TypeError,
			wantWireFrameError: "failed to send command",
			wantStreamSends:    0, // recordingStream.Send returns the error and does not append
			wantTracked:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			d := dialConsoleClient(t)

			mem := memory.New()
			t.Cleanup(func() { _ = mem.Close() })

			registry := session.NewRegistry(mem, "test-instance", silentLogger())

			var stream *recordingStream
			if tt.registerSession {
				stream = newRecordingStream()
				stream.sendErr = tt.streamSendErr

				sess := session.NewSession(7, stream, "1.0", nil, func() {})
				require.NoError(t, registry.Register(context.Background(), sess))
			}

			cmdHandler := handlers.NewCommandHandler(mem, silentLogger())

			h := &Handler{
				abilityChecker: newAbilityCheckerWithRBAC(tt.rbac),
				registry:       registry,
				commandHandler: cmdHandler,
				logger:         silentLogger(),
			}

			server := newTestServer()
			node := newTestNode(nil)
			user := &domain.User{ID: 1}

			handler, cleanup := h.newGRPCMessageHandler(
				context.Background(), d.srvClient, server, node, user, tt.canSend,
			)
			defer cleanup()

			// ACT
			callMessageHandler(t, handler, tt.msgType, tt.payload)

			// ASSERT — wire side first.
			if tt.wantWireFrameType != "" {
				frame, ok := readConsoleFrame(t, d.cliConn, time.Second)
				require.True(t, ok, "expected a frame")
				assert.Equal(t, tt.wantWireFrameType, frame.Type)

				if tt.wantWireFrameError != "" {
					var payload struct {
						Message string `json:"message"`
					}
					require.NoError(t, json.Unmarshal(frame.Payload, &payload))
					assert.Equal(t, tt.wantWireFrameError, payload.Message)
				}
			} else {
				expectNoConsoleFrame(t, d.cliConn, 100*time.Millisecond)
			}

			if stream != nil {
				assert.Len(t, stream.sent, tt.wantStreamSends,
					"recorded gateway messages count")

				if tt.wantStreamSends > 0 {
					last := stream.sent[len(stream.sent)-1]
					require.NotNil(t, last.GetCommand(),
						"sent message must carry a CommandRequest payload")
					cmd := tt.payload.(consoleCommandPayload).Command
					assert.Equal(t, cmd, last.GetCommand().Command)
					assert.Equal(t, uint64(server.ID), last.GetCommand().ServerId)
					assert.True(t, last.GetCommand().StreamOutput,
						"console commands must request streamed output")
					assert.Greater(t, last.GetCommand().Timeout.AsDuration(), time.Duration(0))
				}
			}
		})
	}
}

func TestHandler_NewGRPCMessageHandler_cleanupUntracksAllRegisteredCommands(t *testing.T) {
	t.Parallel()
	// ARRANGE
	d := dialConsoleClient(t)

	mem := memory.New()
	t.Cleanup(func() { _ = mem.Close() })

	registry := session.NewRegistry(mem, "test-instance", silentLogger())
	stream := newRecordingStream()
	sess := session.NewSession(7, stream, "1.0", nil, func() {})
	require.NoError(t, registry.Register(context.Background(), sess))

	cmdHandler := handlers.NewCommandHandler(mem, silentLogger())

	h := &Handler{
		abilityChecker: newAbilityCheckerWithRBAC(allowAllRBAC{}),
		registry:       registry,
		commandHandler: cmdHandler,
		logger:         silentLogger(),
	}

	server := newTestServer()
	node := newTestNode(nil)
	user := &domain.User{ID: 1}

	handler, cleanup := h.newGRPCMessageHandler(
		context.Background(), d.srvClient, server, node, user, true,
	)

	// Issue three valid commands to populate the tracked list.
	const numCommands = 3
	for i := range numCommands {
		callMessageHandler(t, handler, typeConsoleCommand, consoleCommandPayload{
			Command: "say " + string(rune('a'+i)),
		})
	}

	require.Len(t, stream.sent, numCommands, "all valid commands should hit the stream")

	// Each TrackCommandServer was called exactly once per Send. Verify the
	// command_id → server_id mapping is now non-empty by issuing a
	// CommandResult that should publish a console.result frame to the
	// realtime channel.
	firstID := stream.sent[0].GetCommand().GetCommandId()

	// ACT — cleanup unregisters all tracked commands.
	cleanup()

	// ASSERT — after cleanup, HandleCommandResult for that ID must not find
	// a tracked server, so no realtime publish happens. Subscribe to the
	// channel; we should NOT see a message.
	type collected struct {
		got int
	}
	c := &collected{}
	require.NoError(t, mem.Subscribe(
		context.Background(), "gameap:realtime:server:0:console.result",
		func(_ context.Context, _ *pubsub.Message) error {
			c.got++

			return nil
		},
	))

	require.NoError(t, cmdHandler.HandleCommandResult(context.Background(), 7, &proto.CommandResult{
		CommandId: firstID,
		ExitCode:  0,
	}))

	// Allow async publish a brief window.
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, c.got,
		"after cleanup the command_id should be untracked and no console.result frame should be published")
}
