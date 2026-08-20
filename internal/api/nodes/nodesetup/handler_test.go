package nodesetup

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/enrollment"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResponder records WriteError/Write calls so the test can assert on
// status code, error text, and the structured response body the handler
// passes to base.Responder. Mirrors the pattern used in
// internal/api/ws/console/handler_test.go.
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

func (r *fakeResponder) successCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.successes
}

func (r *fakeResponder) lastError() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastErr
}

func (r *fakeResponder) lastResult() any {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.lastSuccess
}

// authedRequest returns a GET /api/nodes/setup request whose context carries
// an authenticated Session, mirroring what the auth middleware would inject
// in production.
func authedRequest(t *testing.T) *http.Request {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/setup", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), &auth.Session{
		User: &domain.User{ID: 1},
	}))

	return req
}

// newEnrollmentService builds an enrollment.Service backed by an in-memory
// cache. The other repositories are nil because the handler under test only
// drives SetupKeyManager().Generate() and never touches the node/cert/svc
// paths.
func newEnrollmentService(t *testing.T, c cache.Cache) *enrollment.Service {
	t.Helper()

	if c == nil {
		c = cache.NewInMemory()
	}
	keyManager := enrollment.NewSetupKeyManager(c, "")

	return enrollment.NewService(keyManager, nil, nil, nil)
}

// failingCache returns the configured error from Set, forcing
// SetupKeyManager.Generate() to surface a "failed to store setup key" error.
type failingCache struct {
	setErr error
}

func (f *failingCache) Get(_ context.Context, _ string) (any, error) {
	return nil, cache.ErrNotFound
}

func (f *failingCache) Set(_ context.Context, _ string, _ any, _ ...cache.Option) error {
	return f.setErr
}

func (f *failingCache) Delete(_ context.Context, _ string) error { return nil }
func (f *failingCache) Clear(_ context.Context) error            { return nil }

var errCacheUnavailable = errors.New("cache unavailable")

// -----------------------------------------------------------------------------
// ServeHTTP tests
// -----------------------------------------------------------------------------

func TestHandler_ServeHTTP_unauthenticated_returns401(t *testing.T) {
	t.Parallel()
	// ARRANGE
	c := cache.NewInMemory()
	svc := newEnrollmentService(t, c)
	responder := &fakeResponder{}
	h := NewHandler(responder, "panel.example.com", svc, 31718, "", 0, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/nodes/setup", nil)
	rec := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, 1, responder.errorCalls(), "exactly one WriteError must be emitted")
	assert.Equal(t, 0, responder.successCalls(), "Write must not be called for unauthenticated requests")
	require.Error(t, responder.lastError())
	assert.Contains(t, responder.lastError().Error(), "user not authenticated")

	// Confirm SetupKeyManager.Generate was never invoked: the in-memory cache
	// must still report "not found" for the setup-key entry. This pins the
	// "no side effects on unauthenticated requests" behaviour without
	// asserting on internal call counts.
	_, err := c.Get(req.Context(), enrollment.SetupKeyCacheKey)
	assert.ErrorIs(t, err, cache.ErrNotFound,
		"Generate must not be called when the session is unauthenticated")
}

func TestHandler_ServeHTTP_keyManagerError_propagates(t *testing.T) {
	t.Parallel()
	// ARRANGE
	svc := newEnrollmentService(t, &failingCache{setErr: errCacheUnavailable})
	responder := &fakeResponder{}
	h := NewHandler(responder, "panel.example.com", svc, 31718, "", 0, nil)

	req := authedRequest(t)
	rec := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	assert.Equal(t, 1, responder.errorCalls())
	assert.Equal(t, 0, responder.successCalls())
	require.Error(t, responder.lastError())
	assert.Contains(t, responder.lastError().Error(), "failed to generate setup key",
		"the handler must wrap the underlying error with a clear message")
	assert.Contains(t, responder.lastError().Error(), "cache unavailable",
		"the underlying cache error must remain chained")
}

func TestHandler_ServeHTTP_success_writesResponse(t *testing.T) {
	t.Parallel()
	// ARRANGE
	c := cache.NewInMemory()
	svc := newEnrollmentService(t, c)
	responder := &fakeResponder{}
	h := NewHandler(responder, "panel.example.com", svc, 31718, "", 0, nil)

	req := authedRequest(t)
	rec := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	require.Equal(t, 1, responder.successCalls(),
		"a happy-path setup request must produce exactly one Write")
	require.Equal(t, 0, responder.errorCalls(),
		"no errors should be reported on the happy path")
	require.Equal(t, http.StatusOK, rec.Code)

	resp, ok := responder.lastResult().(setupResponse)
	require.True(t, ok, "responder must receive a setupResponse value")

	assert.True(t, resp.GRPCEnabled, "GRPC must be advertised as enabled")
	require.NotEmpty(t, resp.Token, "a setup token must be generated")

	// The exact token value is implementation detail of CryptoRandomString;
	// we treat the generated Token as ground truth and verify every other
	// field is composed from it. This keeps the test refactor-resistant.
	token := resp.Token

	expectedConnectURL := enrollment.FormatConnectURL("panel.example.com", 31718, token)
	assert.Equal(t, expectedConnectURL, resp.ConnectURL)
	assert.True(t, strings.HasPrefix(resp.ConnectURL, "grpc://panel.example.com:31718/"),
		"ConnectURL must be a grpc:// URL with the resolved host and port")
	assert.Contains(t, resp.ConnectURL, token,
		"the setup token must appear in the connect URL path")

	expectedBaseURL := "http://panel.example.com"
	expectedSetupLink := expectedBaseURL + "/nodes/setup/" + token
	assert.Equal(t, expectedSetupLink, resp.Link,
		"Link must be baseURL + /nodes/setup/<token>")
	assert.Equal(t, expectedSetupLink, resp.SetupLink,
		"SetupLink must mirror Link")
	assert.Equal(t, expectedBaseURL, resp.Host)

	assert.Equal(t, "bash <(curl -s '"+expectedSetupLink+"')", resp.LinuxCmd)
	assert.Equal(t, "gameapctl daemon install --connect="+expectedConnectURL, resp.WindowsCmd)

	// The generated token must also be persisted in the cache so that a
	// subsequent enrollment call can validate it. This is the only durable
	// side effect of Generate that the handler relies on.
	stored, err := c.Get(req.Context(), enrollment.SetupKeyCacheKey)
	require.NoError(t, err)
	assert.Equal(t, token, stored, "the generated key must be stored in the cache")
}

func TestHandler_ServeHTTP_usesGRPCExtPort_whenSet(t *testing.T) {
	t.Parallel()
	// Pins the dual-port behaviour: when grpcExtPort is non-zero it overrides
	// the in-cluster grpcPort. The previous test fixed extPort=0 so this case
	// covers the other branch.
	// ARRANGE
	svc := newEnrollmentService(t, nil)
	responder := &fakeResponder{}
	h := NewHandler(responder, "panel.example.com", svc, 31718, "", 9090, nil)

	req := authedRequest(t)
	rec := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	require.Equal(t, 1, responder.successCalls())
	resp, ok := responder.lastResult().(setupResponse)
	require.True(t, ok)
	assert.Contains(t, resp.ConnectURL, ":9090/",
		"a non-zero grpcExtPort must take precedence over grpcPort")
	assert.NotContains(t, resp.ConnectURL, ":31718/",
		"the in-cluster grpcPort must not appear when extPort overrides it")
}

func TestHandler_ServeHTTP_warnsWhenHostNotCoveredByCert(t *testing.T) {
	t.Parallel()
	// ARRANGE
	svc := newEnrollmentService(t, nil)
	responder := &fakeResponder{}
	h := NewHandler(responder, "panel.example.com", svc, 31718, "", 0,
		func(string) bool { return false })

	req := authedRequest(t)
	rec := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	require.Equal(t, 1, responder.successCalls())
	resp, ok := responder.lastResult().(setupResponse)
	require.True(t, ok)

	require.Len(t, resp.Warnings, 1)
	assert.Contains(t, resp.Warnings[0], "panel.example.com")
	assert.Contains(t, resp.Warnings[0], "GRPC_EXTERNAL_HOST")
}

func TestHandler_ServeHTTP_noWarningsWhenHostCovered(t *testing.T) {
	t.Parallel()
	// ARRANGE
	svc := newEnrollmentService(t, nil)
	responder := &fakeResponder{}
	h := NewHandler(responder, "panel.example.com", svc, 31718, "", 0,
		func(string) bool { return true })

	req := authedRequest(t)
	rec := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	require.Equal(t, 1, responder.successCalls())
	resp, ok := responder.lastResult().(setupResponse)
	require.True(t, ok)
	assert.Empty(t, resp.Warnings)
}

// -----------------------------------------------------------------------------
// resolveGRPCHost tests
// -----------------------------------------------------------------------------

func TestHandler_resolveGRPCHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		panelHost     string
		grpcExtHost   string
		forwardedHost string
		requestHost   string
		want          string
	}{
		{
			name:          "grpcExtHost_wins_over_panelHost_and_request",
			grpcExtHost:   "grpc.example.com",
			panelHost:     "panel.example.com",
			forwardedHost: "forwarded.example.com",
			requestHost:   "req.example.com",
			want:          "grpc.example.com",
		},
		{
			name:          "panelHost_used_when_no_extHost",
			grpcExtHost:   "",
			panelHost:     "panel.example.com",
			forwardedHost: "forwarded.example.com",
			requestHost:   "req.example.com",
			want:          "panel.example.com",
		},
		{
			name:          "falls_back_to_X_Forwarded_Host_when_panelHost_empty",
			grpcExtHost:   "",
			panelHost:     "",
			forwardedHost: "fw.example.com",
			requestHost:   "req.example.com",
			want:          "fw.example.com",
		},
		{
			name:          "falls_back_to_request_Host_when_no_forwarded",
			grpcExtHost:   "",
			panelHost:     "",
			forwardedHost: "",
			requestHost:   "req.example.com",
			want:          "req.example.com",
		},
		{
			name:      "strips_http_prefix",
			panelHost: "http://panel.example.com",
			want:      "panel.example.com",
		},
		{
			name:      "strips_https_prefix",
			panelHost: "https://panel.example.com",
			want:      "panel.example.com",
		},
		{
			name:      "strips_port_from_host",
			panelHost: "panel.example.com:8080",
			want:      "panel.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			h := NewHandler(&fakeResponder{}, tt.panelHost, nil, 31718, tt.grpcExtHost, 0, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/nodes/setup", nil)
			if tt.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			}
			if tt.requestHost != "" {
				req.Host = tt.requestHost
			}

			// ACT
			got := h.resolveGRPCHost(req)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

// -----------------------------------------------------------------------------
// detectBaseURL tests
// -----------------------------------------------------------------------------

func TestHandler_detectBaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		panelHost      string
		forwardedHost  string
		forwardedProto string
		requestHost    string
		withTLS        bool
		want           string
	}{
		{
			name:        "https_when_TLS_present",
			withTLS:     true,
			requestHost: "secure.example.com",
			want:        "https://secure.example.com",
		},
		{
			name:           "https_when_X_Forwarded_Proto_https",
			forwardedProto: "https",
			requestHost:    "req.example.com",
			want:           "https://req.example.com",
		},
		{
			name:        "http_default",
			requestHost: "req.example.com",
			want:        "http://req.example.com",
		},
		{
			name:          "panelHost_used_for_host",
			panelHost:     "panel.example.com",
			forwardedHost: "ignored.example.com",
			requestHost:   "alsoignored.example.com",
			want:          "http://panel.example.com",
		},
		{
			name:          "falls_back_to_X_Forwarded_Host_then_request_Host",
			panelHost:     "",
			forwardedHost: "fw.example.com",
			requestHost:   "req.example.com",
			want:          "http://fw.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			h := NewHandler(&fakeResponder{}, tt.panelHost, nil, 31718, "", 0, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/nodes/setup", nil)
			if tt.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tt.forwardedHost)
			}
			if tt.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwardedProto)
			}
			if tt.requestHost != "" {
				req.Host = tt.requestHost
			}
			if tt.withTLS {
				req.TLS = &tls.ConnectionState{}
			}

			// ACT
			got := h.detectBaseURL(req)

			// ASSERT
			assert.Equal(t, tt.want, got)
		})
	}
}

// -----------------------------------------------------------------------------
// NewHandler dependency assembly
// -----------------------------------------------------------------------------

func TestNewHandler_assemblesDependencies(t *testing.T) {
	t.Parallel()
	// ARRANGE
	responder := &fakeResponder{}
	svc := newEnrollmentService(t, nil)

	// ACT
	h := NewHandler(responder, "panel.example.com", svc, 31718, "grpc.example.com", 9090, nil)

	// ASSERT
	require.NotNil(t, h)
	assert.Same(t, responder, h.responder, "the configured responder must be retained")
	assert.Equal(t, "panel.example.com", h.panelHost)
	assert.Same(t, svc, h.enrollmentSvc, "the configured enrollment service must be retained")
	assert.Equal(t, uint16(31718), h.grpcPort)
	assert.Equal(t, "grpc.example.com", h.grpcExtHost)
	assert.Equal(t, uint16(9090), h.grpcExtPort)
}
