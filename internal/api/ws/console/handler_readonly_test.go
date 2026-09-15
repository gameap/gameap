package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authInjector installs an authenticated Session in the request context so the
// handler sees a logged-in user, then forwards to next.
func authInjector(user *domain.User, next http.Handler) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		s := &auth.Session{User: user}
		next.ServeHTTP(rw, r.WithContext(auth.ContextWithSession(r.Context(), s)))
	}
}

// TestHandler_ServeHTTP_consoleCommand_isNotDispatched is the regression guard
// for the removed host-command sink. The console WebSocket is read-only, so a
// console.command frame must never reach the daemon. The test stands up the
// real endpoint over a live socket with a registered daemon session and asserts
// that sending console.command dispatches no command to that session.
func TestHandler_ServeHTTP_consoleCommand_isNotDispatched(t *testing.T) {
	t.Parallel()

	const (
		userID   uint = 1
		serverID uint = 42
		nodeID   uint = 7
	)

	serverRepo := inmemory.NewServerRepository()
	require.NoError(t, serverRepo.Save(t.Context(), &domain.Server{
		ID: serverID, DSID: nodeID, Dir: "/srv/gs/test",
	}))
	serverRepo.AddUserServer(userID, serverID)

	nodeRepo := inmemory.NewNodeRepository()
	require.NoError(t, nodeRepo.Save(t.Context(), &domain.Node{
		ID: nodeID, Enabled: true, Name: "n", WorkPath: "/srv/gameap",
	}))

	mem := memory.New()
	t.Cleanup(func() { _ = mem.Close() })

	registry := session.NewRegistry(mem, "test-instance", silentLogger())

	stream := newRecordingRegistryStream()
	sess := session.NewSession(uint64(nodeID), stream, "1.0.0", nil, func() {})
	require.NoError(t, registry.Register(t.Context(), sess))

	hub := ws.NewHub(silentLogger())

	// A non-empty console log makes the endpoint emit a history frame on connect,
	// which the test waits for as proof the read-only stream is live before it
	// probes the (now absent) command path.
	logSvc := &fakeConsoleLogService{result: "boot log\n"}

	h := NewHandler(
		serverRepo, nodeRepo, allowAllRBAC{}, hub, nil, registry,
		&fakeDaemonCommands{}, logSvc, &fakeResponder{},
	)
	h.logger = silentLogger()

	mr := mux.NewRouter()
	mr.Handle("/api/ws/servers/{server}/console", authInjector(&domain.User{ID: userID}, h)).
		Methods(http.MethodGet)
	httpSrv := httptest.NewServer(mr)
	t.Cleanup(httpSrv.Close)

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/api/ws/servers/42/console"

	dialCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })

	frame, ok := readConsoleFrame(t, conn, 2*time.Second)
	require.True(t, ok, "read-only console must emit its connect-time history frame")
	require.Equal(t, typeConsoleHistory, frame.Type)

	cmd, err := json.Marshal(map[string]any{
		"type":    "console.command",
		"payload": map[string]string{"command": "id"},
	})
	require.NoError(t, err)
	require.NoError(t, conn.Write(dialCtx, websocket.MessageText, cmd))

	expectNoConsoleFrame(t, conn, 200*time.Millisecond)
	assert.Zero(t, stream.commandCount(),
		"console.command must not be forwarded to the daemon on the read-only endpoint")
}
