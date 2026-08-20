package nodesmetrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errRBACBoom = errors.New("rbac failure")
	errRepoDead = errors.New("repository unavailable")
)

func TestHandler_RejectsUnauthenticated(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/metrics", nil)

	h := NewHandler(nil, &fakeRBAC{}, &fakeNodes{}, nil, nil, &fakeResponder{})

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_RejectsNonAdmin(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/metrics", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 7},
	}))

	h := NewHandler(nil, &fakeRBAC{can: false}, &fakeNodes{}, nil, nil, &fakeResponder{})

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandler_RBACError_Returns500(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/metrics", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	h := NewHandler(nil, &fakeRBAC{err: errRBACBoom}, &fakeNodes{}, nil, nil, &fakeResponder{})

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandler_NodeRepoError_Returns500(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/nodes/metrics", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	h := NewHandler(nil, &fakeRBAC{can: true}, &fakeNodes{err: errRepoDead}, nil, nil, &fakeResponder{})

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestEnabledNodeIDs_FiltersDisabled(t *testing.T) {
	t.Parallel()
	provider := &fakeNodes{nodes: []domain.Node{
		{ID: 1, Enabled: true},
		{ID: 2, Enabled: false},
		{ID: 3, Enabled: true},
	}}

	h := &Handler{nodes: provider}

	ids, err := h.enabledNodeIDs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []uint64{1, 3}, ids)
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

type fakeNodes struct {
	nodes []domain.Node
	err   error
}

func (f *fakeNodes) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Node, error) {
	return f.nodes, f.err
}

type fakeResponder struct {
	mu sync.Mutex
}

func (r *fakeResponder) WriteError(_ context.Context, rw http.ResponseWriter, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

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
