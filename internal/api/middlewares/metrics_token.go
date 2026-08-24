package middlewares

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gameap/gameap/internal/audit"
)

// MetricsTokenMiddleware guards the Prometheus scrape endpoint with a static
// bearer token (OWASP API2:2023 Broken Authentication: the endpoint is
// outside the session-based API, so it gets its own credential). A wrong or
// missing token answers 401 with no detail and is recorded in the audit log;
// the comparison is constant-time.
type MetricsTokenMiddleware struct {
	token []byte
	audit audit.Logger
}

func NewMetricsTokenMiddleware(token string, auditLogger audit.Logger) *MetricsTokenMiddleware {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &MetricsTokenMiddleware{
		token: []byte(token),
		audit: auditLogger,
	}
}

func (m *MetricsTokenMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(m.token) == 0 || !m.authorized(r) {
			audit.TokenRejected(r.Context(), m.audit, "metrics_token_invalid")

			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)

			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *MetricsTokenMiddleware) authorized(r *http.Request) bool {
	scheme, presented, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return false
	}

	presented = strings.TrimSpace(presented)

	return subtle.ConstantTimeCompare([]byte(presented), m.token) == 1
}
