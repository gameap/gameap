// Security tests for the Prometheus scrape endpoint guard.
//
// OWASP API Security Top 10:2023 — API2:2023 Broken Authentication (the
// endpoint is reachable without a panel session, so it must refuse every
// request that does not present the configured bearer token, and must stay
// closed when no token is configured).
package middlewares_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/api/middlewares"
	"github.com/gameap/gameap/internal/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingAudit struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *recordingAudit) Record(_ context.Context, e audit.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recordingAudit) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.events)
}

func TestMetricsTokenMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		configured    string
		authorization string
		wantStatus    int
		wantAudit     int
	}{
		{
			name:          "valid_token_passes",
			configured:    "s3cr3t-token",
			authorization: "Bearer s3cr3t-token",
			wantStatus:    http.StatusOK,
		},
		{
			name:          "scheme_is_case_insensitive",
			configured:    "s3cr3t-token",
			authorization: "bearer s3cr3t-token",
			wantStatus:    http.StatusOK,
		},
		{
			name:          "wrong_token",
			configured:    "s3cr3t-token",
			authorization: "Bearer s3cr3t-tokeN",
			wantStatus:    http.StatusUnauthorized,
			wantAudit:     1,
		},
		{
			name:          "token_prefix_is_not_enough",
			configured:    "s3cr3t-token",
			authorization: "Bearer s3cr3t",
			wantStatus:    http.StatusUnauthorized,
			wantAudit:     1,
		},
		{
			name:          "missing_header",
			configured:    "s3cr3t-token",
			authorization: "",
			wantStatus:    http.StatusUnauthorized,
			wantAudit:     1,
		},
		{
			name:          "wrong_scheme",
			configured:    "s3cr3t-token",
			authorization: "Basic czNjcjN0LXRva2Vu",
			wantStatus:    http.StatusUnauthorized,
			wantAudit:     1,
		},
		{
			name:          "empty_configured_token_refuses_even_an_empty_bearer",
			configured:    "",
			authorization: "Bearer ",
			wantStatus:    http.StatusUnauthorized,
			wantAudit:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			auditLog := &recordingAudit{}
			served := false
			handler := middlewares.NewMetricsTokenMiddleware(tt.configured, auditLog).Middleware(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					served = true
					w.WriteHeader(http.StatusOK)
				}))

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantStatus == http.StatusOK, served)
			assert.Equal(t, tt.wantAudit, auditLog.count())

			if tt.wantStatus == http.StatusUnauthorized {
				assert.Equal(t, `Bearer realm="metrics"`, rec.Header().Get("WWW-Authenticate"))
				if tt.configured != "" {
					assert.NotContains(t, rec.Body.String(), tt.configured, "the response must not leak the token")
				}

				event := auditLog.events[0]
				assert.Equal(t, audit.EventAuthTokenRejected, event.Type)
				assert.Equal(t, "metrics_token_invalid", event.Reason)
			}
		})
	}
}

func TestNewMetricsTokenMiddleware_nil_audit_is_safe(t *testing.T) {
	t.Parallel()

	handler := middlewares.NewMetricsTokenMiddleware("x", nil).Middleware(http.NotFoundHandler())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
