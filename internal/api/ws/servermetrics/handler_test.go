package servermetrics

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/metrics"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errRBACBoom = errors.New("rbac failure")
	errRepoDead = errors.New("repository unavailable")
)

func TestServerIDFilter(t *testing.T) {
	const wantedServer uint = 7

	tests := []struct {
		name   string
		series *proto.MetricSeries
		want   bool
	}{
		{
			name: "matching_server_id__pass",
			series: &proto.MetricSeries{
				Name:   "gameap_server_cpu",
				Labels: map[string]string{"server_id": "7"},
			},
			want: true,
		},
		{
			name: "different_server_id__drop",
			series: &proto.MetricSeries{
				Name:   "gameap_server_cpu",
				Labels: map[string]string{"server_id": "8"},
			},
			want: false,
		},
		{
			name: "no_server_id_label__drop",
			series: &proto.MetricSeries{
				Name:   "gameap_node_cpu",
				Labels: map[string]string{"host": "node-a"},
			},
			want: false,
		},
		{
			name: "empty_server_id__drop",
			series: &proto.MetricSeries{
				Name:   "gameap_server_cpu",
				Labels: map[string]string{"server_id": ""},
			},
			want: false,
		},
		{
			name: "nil_labels__drop",
			series: &proto.MetricSeries{
				Name: "gameap_node_loadavg",
			},
			want: false,
		},
	}

	filter := serverIDFilter(wantedServer)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, filter(tc.series))
		})
	}
}

func TestHandler_ServeHTTP_RejectsRequestsBeforeUpgrade(t *testing.T) {
	authedSession := &auth.Session{User: &domain.User{ID: 1}}

	tests := []struct {
		name           string
		session        *auth.Session
		muxVars        map[string]string
		repo           *fakeServerRepo
		rbac           *fakeRBAC
		wantStatus     int
		wantError      string
		wantErrorCalls int
	}{
		{
			name:           "missing_session_returns_401",
			session:        nil,
			muxVars:        map[string]string{"server": "1"},
			repo:           &fakeServerRepo{servers: []domain.Server{{ID: 1, DSID: 1}}},
			rbac:           &fakeRBAC{can: true},
			wantStatus:     http.StatusUnauthorized,
			wantError:      "user not authenticated",
			wantErrorCalls: 1,
		},
		{
			name:           "session_without_user_returns_401",
			session:        &auth.Session{},
			muxVars:        map[string]string{"server": "1"},
			repo:           &fakeServerRepo{servers: []domain.Server{{ID: 1, DSID: 1}}},
			rbac:           &fakeRBAC{can: true},
			wantStatus:     http.StatusUnauthorized,
			wantError:      "user not authenticated",
			wantErrorCalls: 1,
		},
		{
			name:           "non_numeric_server_param_returns_400",
			session:        authedSession,
			muxVars:        map[string]string{"server": "abc"},
			repo:           &fakeServerRepo{servers: []domain.Server{{ID: 1, DSID: 1}}},
			rbac:           &fakeRBAC{can: true},
			wantStatus:     http.StatusBadRequest,
			wantError:      "invalid server id",
			wantErrorCalls: 1,
		},
		{
			name:           "missing_server_param_returns_400",
			session:        authedSession,
			muxVars:        nil,
			repo:           &fakeServerRepo{servers: []domain.Server{{ID: 1, DSID: 1}}},
			rbac:           &fakeRBAC{can: true},
			wantStatus:     http.StatusBadRequest,
			wantError:      "invalid server id",
			wantErrorCalls: 1,
		},
		{
			name:           "server_not_found_returns_404",
			session:        authedSession,
			muxVars:        map[string]string{"server": "1"},
			repo:           &fakeServerRepo{servers: nil},
			rbac:           &fakeRBAC{can: true},
			wantStatus:     http.StatusNotFound,
			wantError:      "server not found",
			wantErrorCalls: 1,
		},
		{
			name:           "server_repo_error_returns_500",
			session:        authedSession,
			muxVars:        map[string]string{"server": "1"},
			repo:           &fakeServerRepo{err: errRepoDead},
			rbac:           &fakeRBAC{can: true},
			wantStatus:     http.StatusInternalServerError,
			wantError:      "repository unavailable",
			wantErrorCalls: 1,
		},
		{
			name:           "admin_check_error_returns_500",
			session:        authedSession,
			muxVars:        map[string]string{"server": "1"},
			repo:           &fakeServerRepo{servers: []domain.Server{{ID: 1, DSID: 1}}},
			rbac:           &fakeRBAC{err: errRBACBoom},
			wantStatus:     http.StatusInternalServerError,
			wantError:      "rbac failure",
			wantErrorCalls: 1,
		},
		{
			name:           "ability_check_error_returns_500",
			session:        authedSession,
			muxVars:        map[string]string{"server": "1"},
			repo:           &fakeServerRepo{servers: []domain.Server{{ID: 1, DSID: 1}}},
			rbac:           &fakeRBAC{can: false, canForEntityErr: errRBACBoom},
			wantStatus:     http.StatusInternalServerError,
			wantError:      "rbac failure",
			wantErrorCalls: 1,
		},
		{
			name:           "non_admin_without_server_ability_returns_403",
			session:        authedSession,
			muxVars:        map[string]string{"server": "1"},
			repo:           &fakeServerRepo{servers: []domain.Server{{ID: 1, DSID: 1}}},
			rbac:           &fakeRBAC{can: false},
			wantStatus:     http.StatusForbidden,
			wantError:      "user does not have required permissions",
			wantErrorCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/1/metrics", nil)
			if tt.muxVars != nil {
				req = mux.SetURLVars(req, tt.muxVars)
			}
			if tt.session != nil {
				req = req.WithContext(auth.ContextWithSession(req.Context(), tt.session))
			}

			responder := &fakeResponder{}
			h := newTestHandler(tt.repo, tt.rbac, &fakeHub{}, responder)

			h.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code, "status code mismatch")
			assert.Equal(t, tt.wantErrorCalls, responder.errorCalls(), "WriteError must be invoked exactly the expected number of times")
			assert.Contains(t, rec.Body.String(), tt.wantError, "response body must contain expected error substring")
		})
	}
}

func TestHandler_ServeHTTP_BlocksOnRecorder_BeforeUpgrade(t *testing.T) {
	// ARRANGE
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/3/metrics", nil)
	req = mux.SetURLVars(req, map[string]string{"server": "3"})
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 5},
	}))

	repo := &fakeServerRepo{servers: []domain.Server{{ID: 3, DSID: 9}}}
	responder := &fakeResponder{}
	h := newTestHandler(repo, &fakeRBAC{can: true}, &fakeHub{}, responder)

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	// All authz gates passed; ws.Accept fails on httptest.NewRecorder (no hijack
	// support). The handler logs the warning and returns silently — no error
	// response is written, and the recorder ends up with the upgrade-attempt
	// status (the recorder's default is 200 because no WriteHeader(non-success)
	// was called by the handler before Accept).
	assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "must not return 401")
	assert.NotEqual(t, http.StatusForbidden, rec.Code, "must not return 403")
	assert.NotEqual(t, http.StatusNotFound, rec.Code, "must not return 404")
	assert.NotEqual(t, http.StatusInternalServerError, rec.Code, "must not return 500")
	assert.Equal(t, 0, responder.errorCalls(), "WriteError must not be invoked on a request that passed auth")

	require.Len(t, repo.findCalls(), 1, "server lookup must run exactly once for an authorized request")
	assert.Equal(t, []uint{3}, repo.findCalls()[0].IDs, "server lookup must scope by the requested id")
}

func TestHandler_ServeHTTP_SuccessfulUpgrade(t *testing.T) {
	// ARRANGE
	const serverID uint = 4
	const dsID uint = 11

	repo := &fakeServerRepo{servers: []domain.Server{{ID: serverID, DSID: dsID}}}
	rbac := &fakeRBAC{can: true}
	hub := &fakeHub{sub: newFakeSub()}
	responder := &fakeResponder{}
	h := newTestHandler(repo, rbac, hub, responder)

	httpSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		r = mux.SetURLVars(r, map[string]string{"server": "4"})
		r = r.WithContext(auth.ContextWithSession(r.Context(), &auth.Session{
			User: &domain.User{ID: 1},
		}))
		h.ServeHTTP(rw, r)
	}))
	t.Cleanup(httpSrv.Close)

	// ACT
	dialCtx, dialCancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer dialCancel()

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http")
	conn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err, "WebSocket upgrade must succeed once auth gates pass")
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })

	// ASSERT
	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel()

	msgType, raw, err := conn.Read(readCtx)
	require.NoError(t, err, "client must receive the initial metrics frame from the server")
	assert.Equal(t, websocket.MessageText, msgType, "metrics frames are JSON text frames")

	var frame struct {
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal(raw, &frame))
	assert.Equal(t, "metrics.replay.done", frame.Type, "Pump must emit a replay-done frame after subscribe")

	// The replay-done frame is emitted only after Subscribe returns, so
	// receiving it guarantees the hub call happened; checking earlier races
	// with the server goroutine that runs Pump after the upgrade.
	require.True(t, hub.subscribed.Load(), "metrics hub must receive a Subscribe call after successful upgrade")
	assert.Equal(t, uint64(dsID), hub.lastNodeID.Load(), "Pump must subscribe using the server's daemon id")

	require.Equal(t, 0, responder.errorCalls(), "no error response must be written on the happy path")
}

func TestHandler_ServeHTTP_SubscribeError_ReportsErrorFrame(t *testing.T) {
	// ARRANGE
	repo := &fakeServerRepo{servers: []domain.Server{{ID: 1, DSID: 7}}}
	hub := &fakeHub{subErr: errors.New("hub down")}
	responder := &fakeResponder{}
	h := newTestHandler(repo, &fakeRBAC{can: true}, hub, responder)

	httpSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		r = mux.SetURLVars(r, map[string]string{"server": "1"})
		r = r.WithContext(auth.ContextWithSession(r.Context(), &auth.Session{
			User: &domain.User{ID: 1},
		}))
		h.ServeHTTP(rw, r)
	}))
	t.Cleanup(httpSrv.Close)

	// ACT
	dialCtx, dialCancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer dialCancel()

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http")
	conn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err, "WebSocket upgrade still succeeds before Subscribe runs")
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })

	// ASSERT
	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel()

	_, raw, err := conn.Read(readCtx)
	require.NoError(t, err, "client must receive the error frame Pump emits when Subscribe fails")

	var frame struct {
		Type    string         `json:"type"`
		Payload map[string]any `json:"payload"`
	}
	require.NoError(t, json.Unmarshal(raw, &frame))
	assert.Equal(t, "metrics.error", frame.Type, "Subscribe failure must surface as a metrics.error frame")
	assert.Equal(t, "hub down", frame.Payload["error"], "error frame must carry the underlying cause")
}

// ----- handler builder -----

func newTestHandler(
	repo repositories.ServerRepository,
	rbac base.RBAC,
	metricsHub metrics.Hub,
	responder base.Responder,
) *Handler {
	h := NewHandler(metricsHub, repo, rbac, ws.NewHub(silentLogger()), nil, responder)
	h.logger = silentLogger()

	return h
}

func silentLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// ----- fakes -----

type fakeRBAC struct {
	can             bool
	err             error
	canForEntity    bool
	canForEntityErr error
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
	return f.canForEntity, f.canForEntityErr
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

type fakeServerRepo struct {
	mu      sync.Mutex
	servers []domain.Server
	err     error
	calls   []filters.FindServer
}

func (f *fakeServerRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Server, error) {
	return f.servers, f.err
}

func (f *fakeServerRepo) Find(
	_ context.Context, filter *filters.FindServer, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Server, error) {
	f.mu.Lock()
	if filter != nil {
		f.calls = append(f.calls, *filter)
	}
	f.mu.Unlock()

	return f.servers, f.err
}

func (f *fakeServerRepo) Count(_ context.Context, _ *filters.FindServer) (int, error) {
	return len(f.servers), f.err
}

func (f *fakeServerRepo) FindUserServers(
	_ context.Context, _ uint, _ *filters.FindServer, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Server, error) {
	return f.servers, f.err
}

func (f *fakeServerRepo) Save(_ context.Context, _ *domain.Server) error           { return nil }
func (f *fakeServerRepo) SaveBulk(_ context.Context, _ []*domain.Server) error     { return nil }
func (f *fakeServerRepo) Delete(_ context.Context, _ uint) error                   { return nil }
func (f *fakeServerRepo) SoftDelete(_ context.Context, _ uint) error               { return nil }
func (f *fakeServerRepo) SetUserServers(_ context.Context, _ uint, _ []uint) error { return nil }
func (f *fakeServerRepo) Exists(_ context.Context, _ *filters.FindServer) (bool, error) {
	return len(f.servers) > 0, f.err
}

func (f *fakeServerRepo) Search(_ context.Context, _ string) ([]*domain.Server, error) {
	return nil, nil
}

func (f *fakeServerRepo) UpdateServerStatuses(
	_ context.Context, _ uint, _ []repositories.ServerStatusUpdate,
) error {
	return nil
}

func (f *fakeServerRepo) findCalls() []filters.FindServer {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]filters.FindServer, len(f.calls))
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
	var he httpError
	if errors.As(err, &he) {
		status = he.HTTPStatus()
	}
	rw.WriteHeader(status)
	_, _ = io.WriteString(rw, err.Error())
}

func (r *fakeResponder) Write(_ context.Context, rw http.ResponseWriter, _ any) {
	rw.WriteHeader(http.StatusOK)
}

func (r *fakeResponder) errorCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.errors
}

type fakeHub struct {
	sub        metrics.Subscription
	replay     []*proto.MetricsResponse
	subErr     error
	subscribed atomic.Bool
	lastNodeID atomic.Uint64
}

func (f *fakeHub) Start(_ context.Context) error { return nil }

func (f *fakeHub) Subscribe(
	_ context.Context, nodeID uint64, _ time.Duration,
) (metrics.Subscription, []*proto.MetricsResponse, error) {
	f.subscribed.Store(true)
	f.lastNodeID.Store(nodeID)
	if f.subErr != nil {
		return nil, nil, f.subErr
	}

	return f.sub, f.replay, nil
}

func (f *fakeHub) GetHistory(
	_ context.Context, _ uint64, _ time.Duration,
) (*proto.MetricsResponse, error) {
	return nil, nil
}

func (f *fakeHub) Stop() {}

type fakeSub struct {
	ch     chan *proto.MetricsResponse
	closed atomic.Bool
}

func newFakeSub() *fakeSub {
	return &fakeSub{ch: make(chan *proto.MetricsResponse, 8)}
}

func (f *fakeSub) Samples() <-chan *proto.MetricsResponse { return f.ch }

func (f *fakeSub) Close() {
	if f.closed.CompareAndSwap(false, true) {
		close(f.ch)
	}
}
