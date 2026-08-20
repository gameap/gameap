package attach

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunAttachSession_grpcMode_sendsAttachRequestToRegistry verifies that on
// session start the handler issues an AttachRequest with the right server id
// to the daemon session registered in the registry.
func TestRunAttachSession_grpcMode_sendsAttachRequestToRegistry(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)

	// ACT
	conn := env.dial(t)
	require.NotNil(t, conn)

	// Wait for the registry to receive the attach request via the local stream.
	require.Eventually(t, func() bool {
		for _, m := range env.stream.sentMessages() {
			if m.GetAttachRequest() != nil {
				return true
			}
		}

		return false
	}, 2*time.Second, 10*time.Millisecond, "registry must forward the attach request to the local session")

	// ASSERT — the attach request carries the server id and a non-empty session id
	var attachReq *proto.AttachRequest
	for _, m := range env.stream.sentMessages() {
		if r := m.GetAttachRequest(); r != nil {
			attachReq = r

			break
		}
	}
	require.NotNil(t, attachReq)
	assert.Equal(t, uint64(env.server.ID), attachReq.ServerId, "server id must be propagated")
	assert.NotEmpty(t, attachReq.SessionId, "session id must be generated")
}

// TestRunAttachSession_grpcMode_forwardsInputToRegistry verifies that an
// attach.input frame received over the WebSocket is translated into an
// AttachInput gateway message containing the same payload bytes.
func TestRunAttachSession_grpcMode_forwardsInputToRegistry(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)
	conn := env.dial(t)

	// Wait until the attach request is on the wire so the session id is known.
	require.Eventually(t, func() bool {
		for _, m := range env.stream.sentMessages() {
			if m.GetAttachRequest() != nil {
				return true
			}
		}

		return false
	}, 2*time.Second, 10*time.Millisecond)

	// ACT — send an attach.input frame from the client
	writeJSONFrame(t, conn, &ws.InboundMessage{
		Type:    typeAttachInput,
		Payload: rawPayload(t, attachInputPayload{Data: []byte("status\n")}),
	})

	// ASSERT — the registry must forward an AttachInput with the same data
	require.Eventually(t, func() bool {
		for _, m := range env.stream.sentMessages() {
			if in := m.GetAttachInput(); in != nil && string(in.Data) == "status\n" {
				return true
			}
		}

		return false
	}, 2*time.Second, 10*time.Millisecond, "AttachInput must be sent to the daemon session")
}

// TestRunAttachSession_grpcMode_relaysOutputToWS verifies that an
// AttachOutputPayload published on the realtime channel is delivered to the
// connected WebSocket client through the bridge → hub → client pipeline.
func TestRunAttachSession_grpcMode_relaysOutputToWS(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)
	conn := env.dial(t)

	// Find the session id from the attach request the handler just sent.
	var sessionID string
	require.Eventually(t, func() bool {
		for _, m := range env.stream.sentMessages() {
			if r := m.GetAttachRequest(); r != nil {
				sessionID = r.SessionId

				return true
			}
		}

		return false
	}, 2*time.Second, 10*time.Millisecond)
	require.NotEmpty(t, sessionID, "session id must be discovered before publishing output")

	// ACT — publish an output payload on the realtime channel
	channel := channels.BuildRealtimeAttachOutputChannel(sessionID)
	msg, err := messages.NewMessage(channel, messages.TypeAttachOutput, messages.AttachOutputPayload{
		SessionID: sessionID,
		Data:      []byte("hello-from-daemon"),
	})
	require.NoError(t, err)
	env.publishPubsub(t, channel, msg)

	// ASSERT — client must receive an attach.output frame with the same data
	frame, ok := readFrameOfType(t, conn, messages.TypeAttachOutput, 2*time.Second)
	require.True(t, ok, "client must receive an attach.output frame")

	var payload messages.AttachOutputPayload
	require.NoError(t, json.Unmarshal(frame.Payload, &payload))
	assert.Equal(t, sessionID, payload.SessionID, "session id must be carried through")
	assert.Equal(t, []byte("hello-from-daemon"), payload.Data, "output bytes must be relayed unchanged")
}

// TestRunAttachSession_grpcMode_masksRconPasswordInOutput verifies that the server's RCON
// password — which the game server prints as part of its launch command line — never reaches
// the WebSocket client, whichever instance published the output.
func TestRunAttachSession_grpcMode_masksRconPasswordInOutput(t *testing.T) {
	t.Parallel()
	const rconPassword = "s3cr3tRc0n"

	tests := []struct {
		name         string
		rconPassword string
		data         string
		wantData     string
	}{
		{
			name:         "launch_line_is_masked",
			rconPassword: rconPassword,
			data:         "./hlds_run -game cs +rcon_password " + rconPassword + "\r\n",
			wantData:     "./hlds_run -game cs +rcon_password ******\r\n",
		},
		{
			name:         "every_occurrence_is_masked",
			rconPassword: rconPassword,
			data:         rconPassword + " and " + rconPassword,
			wantData:     "****** and ******",
		},
		{
			name:         "output_without_password_is_relayed_unchanged",
			rconPassword: rconPassword,
			data:         "L 01/02/2026 - 12:00:00: Started map \"de_dust2\"\r\n",
			wantData:     "L 01/02/2026 - 12:00:00: Started map \"de_dust2\"\r\n",
		},
		{
			name:     "server_without_rcon_password_is_relayed_unchanged",
			data:     "./hlds_run -game cs +rcon_password " + rconPassword + "\r\n",
			wantData: "./hlds_run -game cs +rcon_password " + rconPassword + "\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			var opts []attachEnvOption
			if tt.rconPassword != "" {
				opts = append(opts, withRconPassword(tt.rconPassword))
			}

			env := newAttachEnv(t, opts...)
			conn := env.dial(t)

			var sessionID string
			require.Eventually(t, func() bool {
				for _, m := range env.stream.sentMessages() {
					if r := m.GetAttachRequest(); r != nil {
						sessionID = r.SessionId

						return true
					}
				}

				return false
			}, 2*time.Second, 10*time.Millisecond)
			require.NotEmpty(t, sessionID, "session id must be discovered before publishing output")

			// ACT
			channel := channels.BuildRealtimeAttachOutputChannel(sessionID)
			msg, err := messages.NewMessage(channel, messages.TypeAttachOutput, messages.AttachOutputPayload{
				SessionID: sessionID,
				Data:      []byte(tt.data),
			})
			require.NoError(t, err)
			env.publishPubsub(t, channel, msg)

			// ASSERT
			frame, ok := readFrameOfType(t, conn, messages.TypeAttachOutput, 2*time.Second)
			require.True(t, ok, "client must receive an attach.output frame")

			var payload messages.AttachOutputPayload
			require.NoError(t, json.Unmarshal(frame.Payload, &payload))
			assert.Equal(t, sessionID, payload.SessionID, "session id must be carried through")
			assert.Equal(t, tt.wantData, string(payload.Data))

			if tt.rconPassword != "" {
				assert.NotContains(t, string(payload.Data), tt.rconPassword)
			}
		})
	}
}

// TestRunAttachSession_grpcMode_clientDetachClosesConnection verifies that an
// attach.detach frame from the client triggers a detach gateway message and
// closes the WebSocket connection.
func TestRunAttachSession_grpcMode_clientDetachClosesConnection(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)
	conn := env.dial(t)

	require.Eventually(t, func() bool {
		for _, m := range env.stream.sentMessages() {
			if m.GetAttachRequest() != nil {
				return true
			}
		}

		return false
	}, 2*time.Second, 10*time.Millisecond)

	// ACT — send a detach frame
	writeJSONFrame(t, conn, &ws.InboundMessage{
		Type:    typeAttachDetach,
		Payload: nil,
	})

	// ASSERT — the registry must observe at least one AttachDetach message
	require.Eventually(t, func() bool {
		for _, m := range env.stream.sentMessages() {
			if d := m.GetAttachDetach(); d != nil {
				return true
			}
		}

		return false
	}, 2*time.Second, 10*time.Millisecond, "AttachDetach must be sent")

	// ASSERT — connection is closed shortly after detach
	require.Eventually(t, func() bool {
		_, _, err := conn.Read(t.Context())

		return err != nil
	}, 2*time.Second, 50*time.Millisecond, "client connection must be closed after detach")
}

// TestRunAttachSession_grpcMode_clientCloseTriggersDetach verifies that
// closing the client side of the WebSocket causes the handler to send a
// detach gateway message on its way out.
func TestRunAttachSession_grpcMode_clientCloseTriggersDetach(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)
	conn := env.dial(t)

	require.Eventually(t, func() bool {
		for _, m := range env.stream.sentMessages() {
			if m.GetAttachRequest() != nil {
				return true
			}
		}

		return false
	}, 2*time.Second, 10*time.Millisecond)

	// ACT — close the client connection
	_ = conn.Close(websocket.StatusNormalClosure, "bye")

	// ASSERT — handler must observe and send a detach
	require.Eventually(t, func() bool {
		for _, m := range env.stream.sentMessages() {
			if m.GetAttachDetach() != nil {
				return true
			}
		}

		return false
	}, 2*time.Second, 10*time.Millisecond, "AttachDetach must be sent when the client disconnects")
}

// TestRunAttachSession_grpcMode_inputDeniedWhenCanSendFalse verifies that
// when the user lacks the GameServerConsoleSend ability, attach.input frames
// are dropped (no AttachInput message reaches the registry) and the client
// receives an error frame.
func TestRunAttachSession_grpcMode_inputDeniedWhenCanSendFalse(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)
	// Drop admin so the per-entity allow set is consulted, then grant only
	// view (so the connection is accepted) and not send.
	env.rbac.isAdmin = false
	env.rbac.allowAbility(domain.AbilityNameGameServerConsoleView)

	conn := env.dial(t)

	// Wait for the attach request to be sent first.
	require.Eventually(t, func() bool {
		for _, m := range env.stream.sentMessages() {
			if m.GetAttachRequest() != nil {
				return true
			}
		}

		return false
	}, 2*time.Second, 10*time.Millisecond)

	beforeInputs := countInputs(env)

	// ACT — try to send input
	writeJSONFrame(t, conn, &ws.InboundMessage{
		Type:    typeAttachInput,
		Payload: rawPayload(t, attachInputPayload{Data: []byte("rm -rf /\n")}),
	})

	// ASSERT — client must receive an error frame
	frame, ok := readFrameOfType(t, conn, ws.TypeError, 2*time.Second)
	require.True(t, ok, "client must receive an error frame for forbidden input")

	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(frame.Payload, &ep))
	assert.Contains(t, ep.Message, "permission denied",
		"error message must mention permission denial")

	// Give the handler a moment in case it (incorrectly) forwarded the input.
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, beforeInputs, countInputs(env),
		"no AttachInput must be forwarded to the registry")
}

// TestServeHTTP_unauthenticated_returns401 verifies that a request without an
// authenticated session is rejected via the responder before any WebSocket
// upgrade happens.
func TestServeHTTP_unauthenticated_returns401(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/33/attach", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "33"})

	h := NewHandler(
		nil, nil, env.rbac, env.hub, nil, env.registry, env.attachH, env.responder,
	)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "missing session must yield 401")
	require.Len(t, env.responder.errorList(), 1, "WriteError must be called exactly once")
	assert.Contains(t, env.responder.errorList()[0].Error(), "not authenticated",
		"error message must explain the failure")
}

// TestServeHTTP_invalidServerID_returns400 verifies path parameter parsing.
func TestServeHTTP_invalidServerID_returns400(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/abc/attach", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "abc"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{User: env.user}))

	h := NewHandler(
		nil, nil, env.rbac, env.hub, nil, env.registry, env.attachH, env.responder,
	)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	require.Len(t, env.responder.errorList(), 1)
	assert.Contains(t, env.responder.errorList()[0].Error(), "invalid server id")
}

// TestServeHTTP_unknownServer_returns404 verifies that requesting a server
// the user has no access to (here: nonexistent) yields a 404 from the
// ServerFinder.
func TestServeHTTP_unknownServer_returns404(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/9999/attach", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "9999"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{User: env.user}))

	// Build a fresh handler with the same env's repos.
	serverRepoEmpty := emptyServerRepoForServerLookup(t)
	h := NewHandler(
		serverRepoEmpty, nil, env.rbac, env.hub, nil, env.registry, env.attachH, env.responder,
	)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.Len(t, env.responder.errorList(), 1)
	assert.Contains(t, env.responder.errorList()[0].Error(), "server not found")
}

// TestServeHTTP_consoleViewDenied_returns403 verifies the abilityChecker
// gate produces a Forbidden when the user is neither admin nor explicitly
// granted GameServerConsoleView for the server.
func TestServeHTTP_consoleViewDenied_returns403(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)
	env.rbac.isAdmin = false
	// Initialise the allow set without granting view → CanForEntity returns false.
	env.rbac.allowAbility(domain.AbilityNameGameServerConsoleSend)
	env.rbac.denyAbility(domain.AbilityNameGameServerConsoleSend)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/33/attach", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "33"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{User: env.user}))

	// Build a server repo that contains the server and is also reachable by the
	// user (admin=false branch returns nothing without UserIDs match), so add
	// a user-server mapping.
	serverRepo := serverRepoWith(t, env.server, env.user.ID)
	h := NewHandler(
		serverRepo, nil, env.rbac, env.hub, nil, env.registry, env.attachH, env.responder,
	)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT — view ability denial is mapped to 403 by abilityChecker.CheckOrError
	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.Len(t, env.responder.errorList(), 1)
	assert.Contains(t, env.responder.errorList()[0].Error(), "user does not have required permissions")
}

// TestServeHTTP_unknownNode_returns404 verifies that a server pointing to a
// non-existent DSID surfaces a NotFound from the node lookup.
func TestServeHTTP_unknownNode_returns404(t *testing.T) {
	t.Parallel()
	// ARRANGE
	env := newAttachEnv(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/33/attach", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "33"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{User: env.user}))

	// Server exists, but node repo is empty.
	serverRepo := serverRepoWith(t, env.server, env.user.ID)
	emptyNodeRepo := emptyNodeRepoForLookup(t)

	h := NewHandler(
		serverRepo, emptyNodeRepo, env.rbac, env.hub, nil, env.registry, env.attachH, env.responder,
	)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.Len(t, env.responder.errorList(), 1)
	assert.Contains(t, env.responder.errorList()[0].Error(), "node not found")
}

// ----- helpers used only by the gRPC suite -----

// countInputs counts the number of AttachInput messages on the registered
// stream so that "no input was forwarded" can be asserted by comparing
// before/after counts.
func countInputs(env *attachEnv) int {
	count := 0
	for _, m := range env.stream.sentMessages() {
		if m.GetAttachInput() != nil {
			count++
		}
	}

	return count
}

// emptyServerRepoForServerLookup returns a fresh empty repo used in tests
// that need the FindUserServer call to surface a NotFound.
func emptyServerRepoForServerLookup(t *testing.T) *inmemory.ServerRepository {
	t.Helper()

	return inmemory.NewServerRepository()
}

// emptyNodeRepoForLookup returns a node repo containing no nodes.
func emptyNodeRepoForLookup(t *testing.T) *inmemory.NodeRepository {
	t.Helper()

	return inmemory.NewNodeRepository()
}

// serverRepoWith returns a server repo containing the given server and a
// user-server mapping for userID.
func serverRepoWith(t *testing.T, server *domain.Server, userID uint) *inmemory.ServerRepository {
	t.Helper()

	repo := inmemory.NewServerRepository()
	require.NoError(t, repo.Save(t.Context(), server))
	repo.AddUserServer(userID, server.ID)

	return repo
}
