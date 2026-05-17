package nodemetrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
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

func TestHandler_RejectsUnauthenticated(t *testing.T) {
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/1/metrics", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})

	responder := &fakeResponder{}
	h := NewHandler(nil, &fakeRBAC{}, &fakeNodeRepo{}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, 1, responder.errorCalls(), "WriteError must be called once for unauthenticated request")
}

func TestHandler_RejectsNonAdmin(t *testing.T) {
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/1/metrics", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 7},
	}))

	responder := &fakeResponder{}
	h := NewHandler(nil, &fakeRBAC{can: false}, &fakeNodeRepo{}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_RBACError_Returns500(t *testing.T) {
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/1/metrics", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	responder := &fakeResponder{}
	h := NewHandler(nil, &fakeRBAC{err: errRBACBoom}, &fakeNodeRepo{}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_MissingID_Returns400(t *testing.T) {
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/metrics", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	responder := &fakeResponder{}
	h := NewHandler(nil, &fakeRBAC{can: true}, &fakeNodeRepo{}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_InvalidID_Returns400(t *testing.T) {
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/abc/metrics", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	responder := &fakeResponder{}
	h := NewHandler(nil, &fakeRBAC{can: true}, &fakeNodeRepo{}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_UnknownNode_Returns404(t *testing.T) {
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/42/metrics", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "42"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	responder := &fakeResponder{}
	h := NewHandler(nil, &fakeRBAC{can: true}, &fakeNodeRepo{}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_NodeRepoError_Returns500(t *testing.T) {
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/1/metrics", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	responder := &fakeResponder{}
	h := NewHandler(nil, &fakeRBAC{can: true}, &fakeNodeRepo{err: errRepoDead}, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, 1, responder.errorCalls())
}

func TestHandler_ValidRequestPassesAuthz_NoErrorResponse(t *testing.T) {
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/1/metrics", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	responder := &fakeResponder{}
	repo := &fakeNodeRepo{
		nodes: []domain.Node{{ID: 1, Enabled: true}},
	}
	h := NewHandler(nil, &fakeRBAC{can: true}, repo, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	// ws.Accept fails on httptest.NewRecorder (no real connection), so the handler
	// silently returns. The auth gate passed, so no error response was written.
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "should not be 401")
	assert.NotEqual(t, http.StatusForbidden, rec.Code, "should not be 403")
	assert.NotEqual(t, http.StatusNotFound, rec.Code, "should not be 404")
	assert.NotEqual(t, http.StatusInternalServerError, rec.Code, "should not be 500")
	assert.Equal(t, 0, responder.errorCalls(), "WriteError must not be invoked on a valid request")
}

func TestHandler_VerifyNodeExists_FilterShape(t *testing.T) {
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/3/metrics", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "3"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	repo := &fakeNodeRepo{
		nodes: []domain.Node{{ID: 3, Enabled: true}},
	}
	responder := &fakeResponder{}
	h := NewHandler(nil, &fakeRBAC{can: true}, repo, ws.NewHub(nil), nil, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	require.Len(t, repo.findCalls(), 1, "node lookup must run exactly once for valid request")
	assert.Equal(t, []uint{3}, repo.findCalls()[0].IDs, "filter must scope by the requested node id")
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

type fakeNodeRepo struct {
	mu    sync.Mutex
	nodes []domain.Node
	err   error
	calls []filters.FindNode
}

func (f *fakeNodeRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Node, error) {
	return f.nodes, f.err
}

func (f *fakeNodeRepo) Find(
	_ context.Context, filter *filters.FindNode, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Node, error) {
	f.mu.Lock()
	if filter != nil {
		f.calls = append(f.calls, *filter)
	}
	f.mu.Unlock()

	return f.nodes, f.err
}

func (f *fakeNodeRepo) Save(_ context.Context, _ *domain.Node) error { return nil }
func (f *fakeNodeRepo) UpdateGDaemonAPIToken(_ context.Context, _ uint, _ string, _ time.Time) error {
	return nil
}
func (f *fakeNodeRepo) Delete(_ context.Context, _ uint) error { return nil }

func (f *fakeNodeRepo) findCalls() []filters.FindNode {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]filters.FindNode, len(f.calls))
	copy(out, f.calls)

	return out
}

type fakeResponder struct {
	mu     sync.Mutex
	errors int
}

func (r *fakeResponder) WriteError(_ context.Context, rw http.ResponseWriter, err error) {
	r.mu.Lock()
	r.errors++
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
