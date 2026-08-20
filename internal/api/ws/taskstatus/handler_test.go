package taskstatus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errRBACBoom = errors.New("rbac failure")
	errRepoDead = errors.New("repository unavailable")
)

// makeRequest builds an authenticated request scoped to the given task id (mux URL var).
func makeRequest(t *testing.T, taskIDVar string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/ws/tasks/"+taskIDVar+"/status", nil)
	req = mux.SetURLVars(req, map[string]string{"id": taskIDVar})

	return req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))
}

func TestHandler_RejectsUnauthenticated(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/tasks/1/status", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})

	responder := &fakeResponder{}
	h := NewHandler(&fakeDaemonTaskRepo{}, &fakeServerRepo{}, &fakeRBAC{}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_MissingID_Returns400(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/tasks/status", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	responder := &fakeResponder{}
	h := NewHandler(&fakeDaemonTaskRepo{}, &fakeServerRepo{}, &fakeRBAC{}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_InvalidID_Returns400(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "abc")

	responder := &fakeResponder{}
	h := NewHandler(&fakeDaemonTaskRepo{}, &fakeServerRepo{}, &fakeRBAC{}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_RepoError_Returns500(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "1")

	responder := &fakeResponder{}
	h := NewHandler(
		&fakeDaemonTaskRepo{err: errRepoDead},
		&fakeServerRepo{},
		&fakeRBAC{can: true},
		ws.NewHub(nil),
		nil,
		responder,
	)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_TaskNotFound_Returns404(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "999")

	responder := &fakeResponder{}
	h := NewHandler(
		&fakeDaemonTaskRepo{}, // empty results
		&fakeServerRepo{},
		&fakeRBAC{can: true},
		ws.NewHub(nil),
		nil,
		responder,
	)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, 1, responder.errorCalls())
	assert.Contains(t, responder.lastErr.Error(), "daemon task not found")
}

func TestHandler_RBACAdminCheckError_Returns500(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "1")

	serverID := uint(2)
	taskRepo := &fakeDaemonTaskRepo{
		tasks: []domain.DaemonTask{{
			ID:       1,
			ServerID: &serverID,
			Task:     domain.DaemonTaskTypeServerStart,
			Status:   domain.DaemonTaskStatusSuccess,
		}},
	}

	responder := &fakeResponder{}
	h := NewHandler(taskRepo, &fakeServerRepo{}, &fakeRBAC{err: errRBACBoom}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_RegularUser_TaskWithoutServer_Returns403(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "1")

	taskRepo := &fakeDaemonTaskRepo{
		tasks: []domain.DaemonTask{{
			ID:       1,
			ServerID: nil,
			Task:     domain.DaemonTaskTypeServerStart,
			Status:   domain.DaemonTaskStatusWorking,
		}},
	}

	responder := &fakeResponder{}
	h := NewHandler(taskRepo, &fakeServerRepo{}, &fakeRBAC{can: false}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, 1, responder.errorCalls())
	assert.Contains(t, responder.lastErr.Error(), "task has no associated server")
}

func TestHandler_RegularUser_UnknownTaskType_Returns403(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "1")

	serverID := uint(7)
	taskRepo := &fakeDaemonTaskRepo{
		tasks: []domain.DaemonTask{{
			ID:       1,
			ServerID: &serverID,
			// CmdExec is not in DaemonTaskTypeAbilities map.
			Task:   domain.DaemonTaskTypeCmdExec,
			Status: domain.DaemonTaskStatusWorking,
		}},
	}

	responder := &fakeResponder{}
	h := NewHandler(taskRepo, &fakeServerRepo{}, &fakeRBAC{can: false}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, 1, responder.errorCalls())
	assert.Contains(t, responder.lastErr.Error(), "task type not allowed for regular users")
}

func TestHandler_RegularUser_NoAccessToServer_Returns403(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "1")

	serverID := uint(7)
	taskRepo := &fakeDaemonTaskRepo{
		tasks: []domain.DaemonTask{{
			ID:       1,
			ServerID: &serverID,
			Task:     domain.DaemonTaskTypeServerStart,
			Status:   domain.DaemonTaskStatusWorking,
		}},
	}
	// Server repo returns empty for the user — server finder converts to NotFound.
	serverRepo := &fakeServerRepo{}

	responder := &fakeResponder{}
	h := NewHandler(taskRepo, serverRepo, &fakeRBAC{can: false}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, 1, responder.errorCalls())
	assert.Contains(t, responder.lastErr.Error(), "no access to the server")
}

func TestHandler_RegularUser_ServerRepoError_Returns500(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "1")

	serverID := uint(7)
	taskRepo := &fakeDaemonTaskRepo{
		tasks: []domain.DaemonTask{{
			ID:       1,
			ServerID: &serverID,
			Task:     domain.DaemonTaskTypeServerStart,
			Status:   domain.DaemonTaskStatusWorking,
		}},
	}
	serverRepo := &fakeServerRepo{err: errRepoDead}

	responder := &fakeResponder{}
	h := NewHandler(taskRepo, serverRepo, &fakeRBAC{can: false}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_RegularUser_LacksAbility_Returns403(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "1")

	serverID := uint(7)
	taskRepo := &fakeDaemonTaskRepo{
		tasks: []domain.DaemonTask{{
			ID:       1,
			ServerID: &serverID,
			Task:     domain.DaemonTaskTypeServerStart,
			Status:   domain.DaemonTaskStatusWorking,
		}},
	}
	// Server is found for the user but RBAC denies the per-server ability.
	serverRepo := &fakeServerRepo{
		servers: []domain.Server{{ID: serverID}},
	}

	responder := &fakeResponder{}
	h := NewHandler(taskRepo, serverRepo, &fakeRBAC{can: false}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, 1, responder.errorCalls())
	assert.Contains(t, responder.lastErr.Error(), "user does not have required permissions")
}

func TestHandler_AdminPassesAuthz_NoErrorResponse(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "1")

	serverID := uint(7)
	taskRepo := &fakeDaemonTaskRepo{
		tasks: []domain.DaemonTask{{
			ID:       1,
			ServerID: &serverID,
			Task:     domain.DaemonTaskTypeCmdExec, // admin can access any task type
			Status:   domain.DaemonTaskStatusSuccess,
		}},
	}

	responder := &fakeResponder{}
	h := NewHandler(taskRepo, &fakeServerRepo{}, &fakeRBAC{can: true}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	// ws.Accept fails on httptest.NewRecorder, so the handler returns silently
	// after authorization passes. No error response should be written.
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "should not be 401")
	assert.NotEqual(t, http.StatusForbidden, rec.Code, "should not be 403")
	assert.NotEqual(t, http.StatusNotFound, rec.Code, "should not be 404")
	assert.NotEqual(t, http.StatusInternalServerError, rec.Code, "should not be 500")
	assert.Equal(t, 0, responder.errorCalls(), "WriteError must not be invoked when authorization passes")
}

func TestHandler_FindWithOutputFilterShape(t *testing.T) {
	t.Parallel()
	// ARRANGE
	rec := httptest.NewRecorder()
	req := makeRequest(t, "5")

	serverID := uint(7)
	taskRepo := &fakeDaemonTaskRepo{
		tasks: []domain.DaemonTask{{
			ID:       5,
			ServerID: &serverID,
			Task:     domain.DaemonTaskTypeServerStart,
			Status:   domain.DaemonTaskStatusSuccess,
		}},
	}

	responder := &fakeResponder{}
	h := NewHandler(taskRepo, &fakeServerRepo{}, &fakeRBAC{can: true}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	calls := taskRepo.findWithOutputCalls()
	require.Len(t, calls, 1, "task lookup must run exactly once")
	assert.Equal(t, []uint{5}, calls[0].filter.IDs, "filter must scope by the requested task id")
	require.NotNil(t, calls[0].pagination, "pagination must be provided")
	assert.Equal(t, uint64(1), calls[0].pagination.Limit, "lookup must request a single row")
}

func TestIsTerminalStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status domain.DaemonTaskStatus
		want   bool
	}{
		{name: "success_is_terminal", status: domain.DaemonTaskStatusSuccess, want: true},
		{name: "error_is_terminal", status: domain.DaemonTaskStatusError, want: true},
		{name: "canceled_is_terminal", status: domain.DaemonTaskStatusCanceled, want: true},
		{name: "waiting_is_not_terminal", status: domain.DaemonTaskStatusWaiting, want: false},
		{name: "working_is_not_terminal", status: domain.DaemonTaskStatusWorking, want: false},
		{name: "unknown_is_not_terminal", status: domain.DaemonTaskStatus("unknown"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ACT
			got := isTerminalStatus(tt.status)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

// dialAndCollect spins up the handler under an httptest server, completes a real
// WebSocket handshake against it, and reads the initial-state messages the
// handler emits before client.Run blocks the goroutine. Returns the parsed
// outbound payloads keyed by Type.
func dialAndCollect(
	t *testing.T,
	taskID uint,
	task domain.DaemonTask,
	rbacCan bool,
) []map[string]any {
	t.Helper()

	taskRepo := &fakeDaemonTaskRepo{tasks: []domain.DaemonTask{task}}
	serverRepo := &fakeServerRepo{}
	responder := &fakeResponder{}

	hub := ws.NewHub(nil)
	h := NewHandler(taskRepo, serverRepo, &fakeRBAC{can: rbacCan}, hub, nil, responder)

	router := mux.NewRouter()
	router.HandleFunc("/api/ws/tasks/{id}/status", h.ServeHTTP)

	// Wrap the router with a session-injecting middleware so the dial passes the auth gate.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.ContextWithSession(r.Context(), &auth.Session{User: &domain.User{ID: 42}})
		router.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/ws/tasks/" +
		uintToStr(taskID) + "/status"

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dialCancel()

	conn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	var collected []map[string]any
	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel()

	for range 3 { // sendInitialState may emit up to 3 messages
		readOnce, readOnceCancel := context.WithTimeout(readCtx, 500*time.Millisecond)
		_, data, readErr := conn.Read(readOnce)
		readOnceCancel()
		if readErr != nil {
			break
		}

		var msg map[string]any
		require.NoError(t, json.Unmarshal(data, &msg))
		collected = append(collected, msg)
	}

	return collected
}

func TestHandler_SendInitialState_WithOutputAndTerminal(t *testing.T) {
	t.Parallel()
	// ARRANGE
	serverID := uint(7)
	output := "task complete"
	task := domain.DaemonTask{
		ID:       1,
		ServerID: &serverID,
		Task:     domain.DaemonTaskTypeServerStart,
		Status:   domain.DaemonTaskStatusSuccess,
		Output:   &output,
	}

	// ACT
	msgs := dialAndCollect(t, 1, task, true)

	// ASSERT
	require.Len(t, msgs, 3, "must emit task.status, task.output and task.complete for a successful task with output")

	types := []string{
		msgs[0]["type"].(string),
		msgs[1]["type"].(string),
		msgs[2]["type"].(string),
	}
	assert.Contains(t, types, typeTaskStatus)
	assert.Contains(t, types, typeTaskOutput)
	assert.Contains(t, types, typeTaskComplete)
}

func TestHandler_SendInitialState_RunningTaskWithoutOutput(t *testing.T) {
	t.Parallel()
	// ARRANGE
	serverID := uint(7)
	task := domain.DaemonTask{
		ID:       1,
		ServerID: &serverID,
		Task:     domain.DaemonTaskTypeServerStart,
		Status:   domain.DaemonTaskStatusWorking,
	}

	// ACT
	msgs := dialAndCollect(t, 1, task, true)

	// ASSERT
	require.Len(t, msgs, 1, "running task without output must emit only task.status")
	assert.Equal(t, typeTaskStatus, msgs[0]["type"])
}

func TestHandler_SendInitialState_TerminalEmptyOutput(t *testing.T) {
	t.Parallel()
	// ARRANGE
	serverID := uint(7)
	emptyOutput := ""
	task := domain.DaemonTask{
		ID:       1,
		ServerID: &serverID,
		Task:     domain.DaemonTaskTypeServerStart,
		Status:   domain.DaemonTaskStatusError,
		Output:   &emptyOutput,
	}

	// ACT
	msgs := dialAndCollect(t, 1, task, true)

	// ASSERT
	require.Len(t, msgs, 2, "terminal task with empty output must emit task.status and task.complete only")

	types := []string{
		msgs[0]["type"].(string),
		msgs[1]["type"].(string),
	}
	assert.Contains(t, types, typeTaskStatus)
	assert.Contains(t, types, typeTaskComplete)
	assert.NotContains(t, types, typeTaskOutput, "empty output must not emit task.output")
}

func uintToStr(v uint) string {
	const base = 10

	if v == 0 {
		return "0"
	}

	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%base)
		v /= base
	}

	return string(buf[i:])
}

// ----- fakes -----

type fakeRBAC struct {
	can bool
	err error
}

func (f *fakeRBAC) Can(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return f.can, f.err
}

func (f *fakeRBAC) CanOneOf(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return f.can, f.err
}

func (f *fakeRBAC) CanForEntity(
	_ context.Context, _ uint, _ domain.EntityType, _ uint, _ []domain.AbilityName,
) (bool, error) {
	return f.can, f.err
}

func (f *fakeRBAC) GetRoles(_ context.Context, _ uint) ([]string, error) {
	return nil, nil
}

func (f *fakeRBAC) SetRolesToUser(_ context.Context, _ uint, _ []string) error {
	return nil
}

func (f *fakeRBAC) AllowUserAbilitiesForEntity(
	_ context.Context, _ uint, _ uint, _ domain.EntityType, _ []domain.AbilityName,
) error {
	return nil
}

func (f *fakeRBAC) RevokeOrForbidUserAbilitiesForEntity(
	_ context.Context, _ uint, _ uint, _ domain.EntityType, _ []domain.AbilityName,
) error {
	return nil
}

type findWithOutputCall struct {
	filter     filters.FindDaemonTask
	pagination *filters.Pagination
}

type fakeDaemonTaskRepo struct {
	mu                   sync.Mutex
	tasks                []domain.DaemonTask
	err                  error
	findWithOutputCalled []findWithOutputCall
}

func (f *fakeDaemonTaskRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.DaemonTask, error) {
	return nil, nil
}

func (f *fakeDaemonTaskRepo) Find(
	_ context.Context, _ *filters.FindDaemonTask, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.DaemonTask, error) {
	return nil, nil
}

func (f *fakeDaemonTaskRepo) FindWithOutput(
	_ context.Context,
	filter *filters.FindDaemonTask,
	_ []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.DaemonTask, error) {
	f.mu.Lock()
	if filter != nil {
		f.findWithOutputCalled = append(f.findWithOutputCalled, findWithOutputCall{
			filter:     *filter,
			pagination: pagination,
		})
	}
	f.mu.Unlock()

	return f.tasks, f.err
}

func (f *fakeDaemonTaskRepo) Count(_ context.Context, _ *filters.FindDaemonTask) (int, error) {
	return 0, nil
}

func (f *fakeDaemonTaskRepo) Save(_ context.Context, _ *domain.DaemonTask) error { return nil }
func (f *fakeDaemonTaskRepo) Delete(_ context.Context, _ uint) error             { return nil }

func (f *fakeDaemonTaskRepo) Exists(_ context.Context, _ *filters.FindDaemonTask) (bool, error) {
	return false, nil
}

func (f *fakeDaemonTaskRepo) AppendOutput(_ context.Context, _ uint, _ string) error {
	return nil
}

func (f *fakeDaemonTaskRepo) findWithOutputCalls() []findWithOutputCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]findWithOutputCall, len(f.findWithOutputCalled))
	copy(out, f.findWithOutputCalled)

	return out
}

type fakeServerRepo struct {
	mu      sync.Mutex
	servers []domain.Server
	err     error
}

func (f *fakeServerRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Server, error) {
	return nil, nil
}

func (f *fakeServerRepo) Find(
	_ context.Context, _ *filters.FindServer, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Server, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.servers, f.err
}

func (f *fakeServerRepo) Count(_ context.Context, _ *filters.FindServer) (int, error) {
	return 0, nil
}

func (f *fakeServerRepo) FindUserServers(
	_ context.Context, _ uint, _ *filters.FindServer, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Server, error) {
	return nil, nil
}

func (f *fakeServerRepo) Save(_ context.Context, _ *domain.Server) error           { return nil }
func (f *fakeServerRepo) SaveBulk(_ context.Context, _ []*domain.Server) error     { return nil }
func (f *fakeServerRepo) Delete(_ context.Context, _ uint) error                   { return nil }
func (f *fakeServerRepo) SoftDelete(_ context.Context, _ uint) error               { return nil }
func (f *fakeServerRepo) SetUserServers(_ context.Context, _ uint, _ []uint) error { return nil }

func (f *fakeServerRepo) UpdateServerStatuses(
	_ context.Context, _ uint, _ []repositories.ServerStatusUpdate,
) error {
	return nil
}

func (f *fakeServerRepo) Exists(_ context.Context, _ *filters.FindServer) (bool, error) {
	return false, nil
}

func (f *fakeServerRepo) Search(_ context.Context, _ string) ([]*domain.Server, error) {
	return nil, nil
}

type fakeResponder struct {
	mu      sync.Mutex
	errors  int
	lastErr error
}

func (r *fakeResponder) WriteError(_ context.Context, rw http.ResponseWriter, err error) {
	r.mu.Lock()
	r.errors++
	r.lastErr = err
	r.mu.Unlock()

	type httpError interface{ HTTPStatus() int }
	status := http.StatusInternalServerError
	if he, ok := err.(httpError); ok {
		status = he.HTTPStatus()
	}
	rw.WriteHeader(status)
	_, _ = rw.Write([]byte(err.Error()))
}

func (r *fakeResponder) Write(_ context.Context, rw http.ResponseWriter, _ any) {
	rw.WriteHeader(http.StatusOK)
}

func (r *fakeResponder) errorCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.errors
}
