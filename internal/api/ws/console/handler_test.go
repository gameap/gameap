package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/secretmask"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResponder records the number and types of WriteError/Write calls and
// echoes the error's HTTPStatus into the recorder so the test can assert on
// the response code.
type fakeResponder struct {
	mu          sync.Mutex
	errors      int
	lastErr     error
	successes   int
	lastSuccess any
}

func (r *fakeResponder) WriteError(_ context.Context, rw http.ResponseWriter, err error) {
	r.mu.Lock()
	r.errors++
	r.lastErr = err
	r.mu.Unlock()

	type httpError interface{ HTTPStatus() int }
	status := http.StatusInternalServerError

	var he httpError
	if errors.As(err, &he) {
		status = he.HTTPStatus()
	}
	rw.WriteHeader(status)
	_, _ = rw.Write([]byte(err.Error()))
}

func (r *fakeResponder) Write(_ context.Context, rw http.ResponseWriter, result any) {
	r.mu.Lock()
	r.successes++
	r.lastSuccess = result
	r.mu.Unlock()
	rw.WriteHeader(http.StatusOK)
}

func (r *fakeResponder) errorCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.errors
}

func (r *fakeResponder) lastError() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastErr
}

// testUserID is the user ID embedded in every authenticated test session.
// Kept as a constant so test cases can check error messages without coupling
// to a specific numeric value.
const testUserID uint = 1

// newAuthedRequest builds an HTTP request with the given URL and an
// authenticated session attached. The same testUserID is used in every
// request, mirroring the single-tenant nature of these handler tests.
func newAuthedRequest(t *testing.T, serverParam string) *http.Request {
	t.Helper()

	url := "/api/ws/servers/" + serverParam + "/console?server=" + serverParam
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = mux.SetURLVars(req, map[string]string{"server": serverParam})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: testUserID},
	}))

	return req
}

// newServeHTTPHandler builds a Handler wired with inmemory repos, a real
// session.Registry against memory pubsub, and the supplied RBAC backend.
func newServeHTTPHandler(
	t *testing.T,
	rbac base.RBAC,
) (*Handler, *inmemory.ServerRepository, *inmemory.NodeRepository, *fakeResponder) {
	t.Helper()

	mem := memory.New()
	t.Cleanup(func() { _ = mem.Close() })

	registry := session.NewRegistry(mem, "test-instance", silentLogger())

	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	hub := ws.NewHub(silentLogger())
	responder := &fakeResponder{}

	h := NewHandler(
		serverRepo,
		nodeRepo,
		rbac,
		hub,
		nil, // originPatterns: not exercised by validation tests
		registry,
		nil, // commandHandler
		nil, // daemonCommands
		nil, // consoleLogService
		responder,
	)

	return h, serverRepo, nodeRepo, responder
}

// ---------- ServeHTTP gates ----------

func TestHandler_ServeHTTP_unauthenticated_returns401(t *testing.T) {
	// ARRANGE
	h, _, _, responder := newServeHTTPHandler(t, allowAllRBAC{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/1/console?server=1", nil)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
	require.Error(t, responder.lastError())
	assert.Contains(t, responder.lastError().Error(), "user not authenticated")
}

func TestHandler_ServeHTTP_invalidServerID_returns400(t *testing.T) {
	// ARRANGE
	h, _, _, responder := newServeHTTPHandler(t, allowAllRBAC{})
	rec := httptest.NewRecorder()
	// Use a non-numeric server param that cannot parse as uint.
	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/abc/console?server=abc", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "abc"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: testUserID},
	}))

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
	assert.Contains(t, responder.lastError().Error(), "invalid server id")
}

func TestHandler_ServeHTTP_unknownServer_returns404(t *testing.T) {
	// ARRANGE
	h, _, _, responder := newServeHTTPHandler(t, allowAllRBAC{})
	rec := httptest.NewRecorder()
	req := newAuthedRequest(t, "999")

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
	assert.Contains(t, responder.lastError().Error(), "server not found")
}

func TestHandler_ServeHTTP_serverFinderError_propagates(t *testing.T) {
	// ARRANGE
	h, _, _, responder := newServeHTTPHandler(t, errorRBAC{err: errors.New("rbac broken")})
	rec := httptest.NewRecorder()
	req := newAuthedRequest(t, "1")

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT — RBAC failure surfaces as a 500 from the AbilityChecker.
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_ServeHTTP_lacksConsoleViewPermission_returns403(t *testing.T) {
	// ARRANGE — server exists but RBAC denies all abilities.
	h, serverRepo, _, responder := newServeHTTPHandler(t, denyAllRBAC{})

	require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{
		ID: 1, DSID: 1, Dir: "/srv/gs/test",
	}))
	serverRepo.AddUserServer(1, 1)

	rec := httptest.NewRecorder()
	req := newAuthedRequest(t, "1")

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT — denial happens first at FindUserServer (server is not visible
	// to a non-admin without that user-server link being authorized), then
	// at the ability check. Either way, the handler must return a non-2xx
	// code and write exactly one error.
	assert.NotEqual(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_ServeHTTP_unknownNode_returns404(t *testing.T) {
	// ARRANGE — admin can find any server; but the node referenced by DSID
	// does not exist so findNode must 404.
	h, serverRepo, _, responder := newServeHTTPHandler(t, allowAllRBAC{})

	require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{
		ID: 1, DSID: 999, Dir: "/srv/gs/test",
	}))

	rec := httptest.NewRecorder()
	req := newAuthedRequest(t, "1")

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
	assert.Contains(t, responder.lastError().Error(), "node not found")
}

func TestHandler_ServeHTTP_validRequest_doesNotWriteError(t *testing.T) {
	// ARRANGE — admin RBAC, valid server, valid node, but no gRPC session
	// registered for the node. After legacy removal the handler must refuse
	// with 503 before attempting the WebSocket upgrade.
	h, serverRepo, nodeRepo, responder := newServeHTTPHandler(t, allowAllRBAC{})

	require.NoError(t, nodeRepo.Save(context.Background(), &domain.Node{
		ID: 1, Enabled: true, Name: "n", WorkPath: "/srv/gameap",
	}))
	require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{
		ID: 1, DSID: 1, Dir: "/srv/gs/test",
	}))

	rec := httptest.NewRecorder()
	req := newAuthedRequest(t, "1")

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code,
		"node without gRPC session must yield 503")
	require.Equal(t, 1, responder.errorCalls())
	assert.Contains(t, responder.lastError().Error(), "daemon is not connected via grpc")
}

// ---------- findNode ----------

func TestHandler_findNode(t *testing.T) {
	tests := []struct {
		name      string
		nodes     []domain.Node
		queryID   uint
		want      uint
		wantError string
	}{
		{
			name: "returns_node_when_found",
			nodes: []domain.Node{
				{ID: 7, Name: "found"},
			},
			queryID: 7,
			want:    7,
		},
		{
			name:      "returns_not_found_error_when_missing",
			nodes:     nil,
			queryID:   42,
			wantError: "node not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewNodeRepository()
			for i := range tt.nodes {
				require.NoError(t, repo.Save(context.Background(), &tt.nodes[i]))
			}

			h := &Handler{
				nodeRepo: repo,
				logger:   silentLogger(),
			}

			// ACT
			got, err := h.findNode(context.Background(), tt.queryID)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.want, got.ID)
			}
		})
	}
}

// ---------- NewHandler ----------

func TestNewHandler_assemblesAllDependencies(t *testing.T) {
	// ARRANGE
	mem := memory.New()
	t.Cleanup(func() { _ = mem.Close() })
	registry := session.NewRegistry(mem, "i", silentLogger())

	serverRepo := inmemory.NewServerRepository()
	nodeRepo := inmemory.NewNodeRepository()
	hub := ws.NewHub(silentLogger())
	responder := &fakeResponder{}

	dc := &fakeDaemonCommands{}
	cls := &fakeConsoleLogService{}

	// ACT
	h := NewHandler(
		serverRepo,
		nodeRepo,
		allowAllRBAC{},
		hub,
		[]string{"https://example.org"},
		registry,
		nil,
		dc,
		cls,
		responder,
	)

	// ASSERT
	require.NotNil(t, h)
	assert.NotNil(t, h.serverFinder, "serverFinder must be wired")
	assert.NotNil(t, h.abilityChecker, "abilityChecker must be wired")
	assert.Same(t, nodeRepo, h.nodeRepo)
	assert.Same(t, hub, h.hub)
	assert.Equal(t, []string{"https://example.org"}, h.originPatterns)
	assert.Same(t, registry, h.registry)
	assert.Equal(t, dc, h.daemonCommands)
	assert.Equal(t, cls, h.consoleLogService)
	assert.Same(t, responder, h.responder)
	assert.NotNil(t, h.logger, "logger must default to slog.Default")
}

// ---------- shared RBAC stubs ----------

// errorRBAC always returns an error from Can/CanForEntity. Used to drive the
// 500 path of the AbilityChecker.
type errorRBAC struct {
	err error
}

func (e errorRBAC) Can(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return false, e.err
}

func (e errorRBAC) CanOneOf(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return false, e.err
}

func (e errorRBAC) CanForEntity(
	_ context.Context, _ uint, _ domain.EntityType, _ uint, _ []domain.AbilityName,
) (bool, error) {
	return false, e.err
}

func (e errorRBAC) GetRoles(_ context.Context, _ uint) ([]string, error) { return nil, nil }

func (e errorRBAC) SetRolesToUser(_ context.Context, _ uint, _ []string) error { return nil }

func (e errorRBAC) AdministrativeRoles(_ context.Context) ([]string, error) {
	return nil, nil
}

func (e errorRBAC) AllowUserAbilitiesForEntity(
	_ context.Context, _ uint, _ uint, _ domain.EntityType, _ []domain.AbilityName,
) error {
	return nil
}

func (e errorRBAC) RevokeOrForbidUserAbilitiesForEntity(
	_ context.Context, _ uint, _ uint, _ domain.EntityType, _ []domain.AbilityName,
) error {
	return nil
}

// ---------- sendConsoleHistory ----------

// newConsoleLogTestHandler builds a minimal *Handler that wires only the
// fields sendConsoleHistory / getConsoleLog read. The handler is otherwise
// unable to serve HTTP; tests must call the private methods directly.
func newConsoleLogTestHandler(logSvc consoleLogService, dc daemonCommands) *Handler {
	return &Handler{
		consoleLogService: logSvc,
		daemonCommands:    dc,
		logger:            silentLogger(),
	}
}

func TestHandler_sendConsoleHistory(t *testing.T) {
	tests := []struct {
		name string

		logSvc       *fakeConsoleLogService
		dc           *fakeDaemonCommands
		node         *domain.Node
		rconPassword string

		wantFrame      bool
		wantOutput     string
		wantDCCalls    int32
		wantLogSvcHits int32
	}{
		{
			name:           "emits_history_frame_with_output_when_log_service_returns_data",
			logSvc:         &fakeConsoleLogService{result: "server output line\n"},
			dc:             &fakeDaemonCommands{},
			node:           newTestNode(nil),
			wantFrame:      true,
			wantOutput:     "server output line\n",
			wantDCCalls:    0,
			wantLogSvcHits: 1,
		},
		{
			name:           "skips_when_output_empty",
			logSvc:         &fakeConsoleLogService{result: ""},
			dc:             &fakeDaemonCommands{},
			node:           newTestNode(nil),
			wantFrame:      false,
			wantDCCalls:    0,
			wantLogSvcHits: 1,
		},
		{
			name:           "falls_back_to_script_when_log_service_errors_and_ScriptGetConsole_set",
			logSvc:         &fakeConsoleLogService{err: errors.New("boom")},
			dc:             &fakeDaemonCommands{result: &daemon.CommandResult{Output: "from script"}},
			node:           newTestNode(new("get_console.sh {id}")),
			wantFrame:      true,
			wantOutput:     "from script",
			wantDCCalls:    1,
			wantLogSvcHits: 1,
		},
		{
			name:           "does_not_emit_when_log_service_errors_and_script_empty",
			logSvc:         &fakeConsoleLogService{err: errors.New("boom")},
			dc:             &fakeDaemonCommands{},
			node:           newTestNode(nil),
			wantFrame:      false,
			wantDCCalls:    0,
			wantLogSvcHits: 1,
		},
		{
			name:           "masks_rcon_password_in_history",
			logSvc:         &fakeConsoleLogService{result: "./hlds_run +rcon_password s3cr3tRc0n\n"},
			dc:             &fakeDaemonCommands{},
			node:           newTestNode(nil),
			rconPassword:   "s3cr3tRc0n",
			wantFrame:      true,
			wantOutput:     "./hlds_run +rcon_password ******\n",
			wantDCCalls:    0,
			wantLogSvcHits: 1,
		},
		{
			name:           "leaves_history_untouched_without_rcon_password",
			logSvc:         &fakeConsoleLogService{result: "./hlds_run +rcon_password s3cr3tRc0n\n"},
			dc:             &fakeDaemonCommands{},
			node:           newTestNode(nil),
			wantFrame:      true,
			wantOutput:     "./hlds_run +rcon_password s3cr3tRc0n\n",
			wantDCCalls:    0,
			wantLogSvcHits: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			d := dialConsoleClient(t)
			h := newConsoleLogTestHandler(tt.logSvc, tt.dc)
			server := newTestServer()
			if tt.rconPassword != "" {
				server.Rcon = new(tt.rconPassword)
			}

			// ACT
			h.sendConsoleHistory(
				context.Background(), d.srvClient, server, tt.node, secretmask.New(server.RconPassword()),
			)

			// ASSERT
			assert.Equal(t, tt.wantLogSvcHits, tt.logSvc.calls.Load(),
				"console log service should be hit exactly the expected number of times")
			assert.Equal(t, tt.wantDCCalls, tt.dc.calls.Load(),
				"daemon-commands fallback should be hit only when scripted")

			if tt.wantFrame {
				frame, ok := readConsoleFrame(t, d.cliConn, time.Second)
				require.True(t, ok, "client must receive a history frame")
				assert.Equal(t, typeConsoleHistory, frame.Type,
					"frame type must mark this as a console history payload")

				var payload consoleHistoryPayload
				require.NoError(t, json.Unmarshal(frame.Payload, &payload))
				assert.Equal(t, tt.wantOutput, payload.Output,
					"history payload must echo the resolved output unchanged")
			} else {
				expectNoConsoleFrame(t, d.cliConn, 100*time.Millisecond)
			}
		})
	}
}

// ---------- getConsoleLog ----------

func TestHandler_getConsoleLog(t *testing.T) {
	tests := []struct {
		name string

		logSvc *fakeConsoleLogService
		dc     *fakeDaemonCommands
		node   *domain.Node

		wantResult     string
		wantError      string
		wantDCCalls    int32
		wantLogSvcHits int32
	}{
		{
			name:           "returns_log_service_output_when_available",
			logSvc:         &fakeConsoleLogService{result: "hello"},
			dc:             &fakeDaemonCommands{},
			node:           newTestNode(nil),
			wantResult:     "hello",
			wantDCCalls:    0,
			wantLogSvcHits: 1,
		},
		{
			name:           "falls_back_to_script_when_log_service_errors",
			logSvc:         &fakeConsoleLogService{err: errors.New("svc down")},
			dc:             &fakeDaemonCommands{result: &daemon.CommandResult{Output: "script output"}},
			node:           newTestNode(new("script.sh")),
			wantResult:     "script output",
			wantDCCalls:    1,
			wantLogSvcHits: 1,
		},
		{
			name:           "returns_empty_when_no_log_service_and_no_script",
			logSvc:         nil,
			dc:             &fakeDaemonCommands{},
			node:           newTestNode(nil),
			wantResult:     "",
			wantDCCalls:    0,
			wantLogSvcHits: 0,
		},
		{
			name:           "wraps_command_error",
			logSvc:         &fakeConsoleLogService{err: errors.New("svc down")},
			dc:             &fakeDaemonCommands{err: errors.New("cmd failed")},
			node:           newTestNode(new("script.sh")),
			wantError:      "failed to execute get console script",
			wantDCCalls:    1,
			wantLogSvcHits: 1,
		},
		{
			name:           "replaces_server_shortcodes_in_script",
			logSvc:         nil,
			dc:             &fakeDaemonCommands{result: &daemon.CommandResult{Output: "ok"}},
			node:           newTestNode(new("echo {host}:{port}")),
			wantResult:     "ok",
			wantDCCalls:    1,
			wantLogSvcHits: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			var logSvcArg consoleLogService
			if tt.logSvc != nil {
				logSvcArg = tt.logSvc
			}
			h := newConsoleLogTestHandler(logSvcArg, tt.dc)
			server := newTestServer()

			// ACT
			got, err := h.getConsoleLog(context.Background(), server, tt.node)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError,
					"command error must be wrapped with the documented message")
				assert.Contains(t, err.Error(), "cmd failed",
					"the original cause must remain accessible via the message chain")
				assert.Empty(t, got, "result must be empty when an error is returned")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantResult, got,
					"result must match the expected log/script output")
			}

			assert.Equal(t, tt.wantDCCalls, tt.dc.calls.Load(),
				"daemon-commands fallback call count must match expectation")
			if tt.logSvc != nil {
				assert.Equal(t, tt.wantLogSvcHits, tt.logSvc.calls.Load(),
					"console log service call count must match expectation")
			}

			if tt.name == "replaces_server_shortcodes_in_script" {
				assert.Equal(t, "echo 127.0.0.1:27015", tt.dc.lastCmd,
					"shortcodes must be replaced with the server's host and port before dispatch")
			}
		})
	}
}

// ---------- canSendCommands ----------

func TestHandler_canSendCommands(t *testing.T) {
	tests := []struct {
		name string
		rbac base.RBAC
		want bool
	}{
		{
			name: "returns_true_when_RBAC_grants_send",
			rbac: allowAllRBAC{},
			want: true,
		},
		{
			name: "returns_false_when_RBAC_denies_send",
			rbac: denyAllRBAC{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			h := &Handler{
				abilityChecker: newAbilityCheckerWithRBAC(tt.rbac),
				logger:         silentLogger(),
			}
			user := &domain.User{ID: 1}
			server := newTestServer()

			// ACT
			got := h.canSendCommands(context.Background(), user, server)

			// ASSERT
			assert.Equal(t, tt.want, got,
				"canSendCommands must echo the RBAC decision for the send ability")
		})
	}
}
