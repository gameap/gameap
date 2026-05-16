package audit

import (
	"net"
	"net/http"
	"strings"

	"github.com/gameap/gameap/pkg/idgen"
)

const (
	// RequestIDHeader is the inbound/outbound correlation header.
	RequestIDHeader = "X-Request-Id"

	// maxRequestIDLen caps an externally supplied request ID. Longer or
	// malformed values are replaced with a freshly generated ID so a client
	// cannot inject a log-forging or oversized correlation identifier.
	maxRequestIDLen = 64
)

// RequestContextMiddleware assigns or propagates a correlation ID and
// captures per-request metadata (client IP, user agent, method, path) into
// the request context, so every downstream audit event is automatically
// enriched and joinable on request_id. It is the single global integration
// point — wrap the root handler with it once.
type RequestContextMiddleware struct {
	clientIPHeader string
}

// NewRequestContextMiddleware builds the middleware. clientIPHeader, when
// non-empty, is the trusted reverse-proxy header to read the real client IP
// from (e.g. X-Real-IP); empty means use RemoteAddr.
func NewRequestContextMiddleware(clientIPHeader string) *RequestContextMiddleware {
	return &RequestContextMiddleware{clientIPHeader: clientIPHeader}
}

func (m *RequestContextMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := sanitizeRequestID(r.Header.Get(RequestIDHeader))
		if requestID == "" {
			requestID = idgen.New()
		}

		info := &RequestInfo{
			RequestID: requestID,
			IP:        ClientIP(r, m.clientIPHeader),
			UserAgent: r.UserAgent(),
			Method:    r.Method,
			Path:      r.URL.Path,
		}

		w.Header().Set(RequestIDHeader, requestID)

		next.ServeHTTP(w, r.WithContext(ContextWithRequestInfo(r.Context(), info)))
	})
}

// sanitizeRequestID accepts an externally supplied correlation ID only if
// it is short and limited to an unambiguous, log-safe character set;
// anything else yields "" so the caller generates a fresh ID instead.
func sanitizeRequestID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > maxRequestIDLen {
		return ""
	}
	for _, c := range v {
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '.', c == '_', c == '-':
		default:
			return ""
		}
	}

	return v
}

// ClientIP returns the best-effort client IP. When trustedHeader is set
// (operator runs behind a reverse proxy) its first value is used; otherwise
// the host part of RemoteAddr is returned. This is the single shared
// implementation — the login rate limiter uses it too.
func ClientIP(r *http.Request, trustedHeader string) string {
	if trustedHeader != "" {
		if v := strings.TrimSpace(r.Header.Get(trustedHeader)); v != "" {
			// X-Forwarded-For may carry a comma-separated list — keep first.
			if idx := strings.IndexByte(v, ','); idx >= 0 {
				v = strings.TrimSpace(v[:idx])
			}

			return v
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
