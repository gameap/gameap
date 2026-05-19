package attach

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/grpc/handlers"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gateEnv collects the minimum collaborators needed to exercise the ServeHTTP
// validation gates without ever upgrading the response to a WebSocket. Unlike
// attachEnv the registry has no local session registered, so IsConnectedAnywhere
// returns false for every node and the 503 gate fires.
type gateEnv struct {
	serverRepo *inmemory.ServerRepository
	nodeRepo   *inmemory.NodeRepository
	rbac       *fakeRBAC
	hub        *ws.Hub
	registry   *session.Registry
	attachH    *handlers.AttachHandler
	responder  *fakeResponder
	user       *domain.User
	server     *domain.Server
	node       *domain.Node
}

// newGateEnv assembles the gate fixture. Each helper uses isAdmin=true so that
// FindUserServer and the AbilityChecker.Can(admin) check succeed and any
// downstream gate the test wants to exercise can be reached without filling in
// per-entity allow sets. Tests that intentionally want a denial pass a
// pre-configured *fakeRBAC into newGateHandler.
func newGateEnv(t *testing.T) *gateEnv {
	t.Helper()

	const (
		userID   uint = 11
		serverID uint = 33
		nodeID   uint = 7
	)

	user := &domain.User{ID: userID}

	server := &domain.Server{
		ID:         serverID,
		Enabled:    true,
		Name:       "gate-server",
		GameID:     "cs",
		DSID:       nodeID,
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
		Dir:        "/srv/games/gate",
		UID:        uuid.New(),
	}
	server.Hydrate()

	node := &domain.Node{
		ID:       nodeID,
		Enabled:  true,
		Name:     "gate-node",
		WorkPath: "/srv/games",
	}

	serverRepo := inmemory.NewServerRepository()
	require.NoError(t, serverRepo.Save(t.Context(), server))
	serverRepo.AddUserServer(userID, server.ID)

	nodeRepo := inmemory.NewNodeRepository()
	require.NoError(t, nodeRepo.Save(t.Context(), node))

	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	registry := session.NewRegistry(ps, "gate-instance", silentLogger())
	attachH := handlers.NewAttachHandler(ps, silentLogger())

	return &gateEnv{
		serverRepo: serverRepo,
		nodeRepo:   nodeRepo,
		rbac:       &fakeRBAC{isAdmin: true},
		hub:        ws.NewHub(silentLogger()),
		registry:   registry,
		attachH:    attachH,
		responder:  &fakeResponder{},
		user:       user,
		server:     server,
		node:       node,
	}
}

// newGateHandler wires a Handler from the env using the env's pre-configured
// rbac. Callers can mutate env.rbac before this call to flip admin/allow sets.
func newGateHandler(env *gateEnv) *Handler {
	h := NewHandler(
		env.serverRepo,
		env.nodeRepo,
		env.rbac,
		env.hub,
		nil,
		env.registry,
		env.attachH,
		env.responder,
	)
	h.logger = silentLogger()

	return h
}

// gateRequest builds an authenticated GET request for the attach endpoint with
// the given server URL parameter. When auth is false no Session is attached so
// auth.SessionFromContext returns an unauthenticated zero session.
func gateRequest(t *testing.T, serverParam string, auth bool, user *domain.User) *http.Request {
	t.Helper()

	url := "/api/ws/servers/" + serverParam + "/attach?server=" + serverParam
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = mux.SetURLVars(req, map[string]string{"server": serverParam})

	if auth {
		req = withSession(req, user)
	}

	return req
}

// withSession returns a request with an authenticated Session for user attached
// to its context.
func withSession(req *http.Request, user *domain.User) *http.Request {
	ctx := authSession(req.Context(), user)

	return req.WithContext(ctx)
}

// authSession installs an authenticated Session on ctx.
func authSession(parent context.Context, user *domain.User) context.Context {
	return auth.ContextWithSession(parent, &auth.Session{User: user})
}

// firstError returns the first WriteError argument or nil if no error has been
// recorded yet. Tests use it instead of indexing into errorList() so a missing
// entry yields a clear failure rather than a panic.
func firstError(r *fakeResponder) error {
	errs := r.errorList()
	if len(errs) == 0 {
		return nil
	}

	return errs[0]
}

// TestHandler_ServeHTTP_unauthenticated_returns401 verifies that ServeHTTP
// refuses requests without a Session in the context with a 401 before reading
// any path or query parameters.
func TestHandler_ServeHTTP_unauthenticated_returns401(t *testing.T) {
	// ARRANGE
	env := newGateEnv(t)
	h := newGateHandler(env)

	rec := httptest.NewRecorder()
	req := gateRequest(t, "33", false, nil)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "missing session must yield 401")
	require.Len(t, env.responder.errorList(), 1, "WriteError must be called exactly once")

	err := firstError(env.responder)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user not authenticated",
		"401 error must explain the failure reason")
}

// TestHandler_ServeHTTP_invalidServerID_returns400 verifies that a non-numeric
// server path parameter is rejected at the input-reader gate with a 400.
func TestHandler_ServeHTTP_invalidServerID_returns400(t *testing.T) {
	// ARRANGE
	env := newGateEnv(t)
	h := newGateHandler(env)

	rec := httptest.NewRecorder()
	req := gateRequest(t, "abc", true, env.user)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusBadRequest, rec.Code, "non-numeric server id must yield 400")
	require.Len(t, env.responder.errorList(), 1)
	assert.Contains(t, firstError(env.responder).Error(), "invalid server id")
}

// TestHandler_ServeHTTP_unknownServer_returns404 verifies that referencing a
// server that does not belong to the user surfaces a 404 from ServerFinder.
func TestHandler_ServeHTTP_unknownServer_returns404(t *testing.T) {
	// ARRANGE — pristine repo, no servers saved.
	env := newGateEnv(t)
	env.serverRepo = inmemory.NewServerRepository()
	h := newGateHandler(env)

	rec := httptest.NewRecorder()
	req := gateRequest(t, "9999", true, env.user)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusNotFound, rec.Code, "unknown server must yield 404")
	require.Len(t, env.responder.errorList(), 1)
	assert.Contains(t, firstError(env.responder).Error(), "server not found")
}

// TestHandler_ServeHTTP_lacksConsoleViewPermission_returns403 verifies that
// once the server is reachable but the user lacks GameServerConsoleView the
// AbilityChecker maps the denial to a 403.
func TestHandler_ServeHTTP_lacksConsoleViewPermission_returns403(t *testing.T) {
	// ARRANGE — strip the admin shortcut and intentionally leave the entity
	// allow set empty: every CanForEntity call will return false.
	env := newGateEnv(t)
	env.rbac = &fakeRBAC{isAdmin: false}
	env.rbac.allowAbility(domain.AbilityNameGameServerStart)
	env.rbac.denyAbility(domain.AbilityNameGameServerStart)
	h := newGateHandler(env)

	rec := httptest.NewRecorder()
	req := gateRequest(t, "33", true, env.user)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT — denial must produce a non-2xx response and exactly one error,
	// because if the gate let the request through we would attempt the WS
	// upgrade and write zero or multiple errors.
	assert.NotEqual(t, http.StatusOK, rec.Code,
		"console-view denial must not yield a 2xx response")
	require.Len(t, env.responder.errorList(), 1,
		"exactly one error frame must be written for a denied request")
	assert.Equal(t, http.StatusForbidden, rec.Code,
		"console-view denial must yield 403")
	assert.Contains(t, firstError(env.responder).Error(), "user does not have required permissions",
		"403 message must describe the permission failure")
}

// TestHandler_ServeHTTP_unknownNode_returns404 verifies that a server pointing
// to a non-existent DSID surfaces a 404 from findNode.
func TestHandler_ServeHTTP_unknownNode_returns404(t *testing.T) {
	// ARRANGE — server exists but the node repo is empty so findNode fails.
	env := newGateEnv(t)
	env.nodeRepo = inmemory.NewNodeRepository()
	h := newGateHandler(env)

	rec := httptest.NewRecorder()
	req := gateRequest(t, "33", true, env.user)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusNotFound, rec.Code, "missing node must yield 404")
	require.Len(t, env.responder.errorList(), 1)
	assert.Contains(t, firstError(env.responder).Error(), "node not found")
}

// TestHandler_ServeHTTP_daemonNotConnected_returns503 is the critical
// regression guard: when all prior gates pass but the registry has no local
// session and no global record for the node, the handler must refuse with a
// 503 before attempting the WebSocket upgrade.
func TestHandler_ServeHTTP_daemonNotConnected_returns503(t *testing.T) {
	// ARRANGE — server exists, user authenticated, RBAC admin (so view passes),
	// node exists, but no session is registered for the node anywhere.
	env := newGateEnv(t)
	h := newGateHandler(env)

	require.False(t, env.registry.IsConnectedAnywhere(uint64(env.node.ID)),
		"precondition: registry must report node as not connected")

	rec := httptest.NewRecorder()
	req := gateRequest(t, "33", true, env.user)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code,
		"unreachable daemon must yield 503 instead of attempting a WS upgrade")
	require.Len(t, env.responder.errorList(), 1,
		"exactly one error frame must be written when the daemon is not connected")
	assert.Contains(t, firstError(env.responder).Error(), "daemon is not connected via grpc",
		"503 message must explicitly name the grpc-connection failure")
}
