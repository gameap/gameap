// API Security Tests for OWASP API Security Top 10:2023.
// Category: API8:2023 — Security Misconfiguration.
//
// The request-context middleware is the single point that produces the
// correlation id and client IP every audit record is joined on. A weak
// sanitizer would let a client inject a forged/oversized correlation id
// (CWE-117 log injection) or evade IP attribution. These tests pin the
// sanitization, generation-fallback and metadata-capture contract.
//
// Reference: https://owasp.org/API-Security/editions/2023/

package audit_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureInfoHandler records the RequestInfo the middleware injected.
type captureInfoHandler struct {
	info *audit.RequestInfo
}

func (h *captureInfoHandler) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	h.info = audit.RequestInfoFromContext(r.Context())
}

// TestRequestContextMiddleware_RequestIDSanitization covers OWASP API8:2023.
// A short, log-safe inbound X-Request-Id is propagated verbatim; an empty,
// over-long or illegal-character value is replaced by a freshly generated id
// so a client can neither forge nor oversize the correlation key.
func TestRequestContextMiddleware_RequestIDSanitization(t *testing.T) {
	tests := []struct {
		name        string
		inbound     string
		setHeader   bool
		wantReused  bool
		wantReplace bool
	}{
		{
			name:       "valid_id_is_reused_and_echoed",
			inbound:    "abc-123_X.7",
			setHeader:  true,
			wantReused: true,
		},
		{
			name:        "empty_id_is_replaced",
			inbound:     "",
			setHeader:   true,
			wantReplace: true,
		},
		{
			name:        "absent_header_is_replaced",
			inbound:     "",
			setHeader:   false,
			wantReplace: true,
		},
		{
			name:        "too_long_id_is_replaced",
			inbound:     strings.Repeat("a", 65),
			setHeader:   true,
			wantReplace: true,
		},
		{
			name:        "id_with_space_is_replaced",
			inbound:     "a b",
			setHeader:   true,
			wantReplace: true,
		},
		{
			name:        "id_with_newline_is_replaced",
			inbound:     "a\nb",
			setHeader:   true,
			wantReplace: true,
		},
		{
			name:        "id_with_path_traversal_is_replaced",
			inbound:     "../x",
			setHeader:   true,
			wantReplace: true,
		},
		{
			name:       "max_length_id_is_reused",
			inbound:    strings.Repeat("a", 64),
			setHeader:  true,
			wantReused: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			next := &captureInfoHandler{}
			h := audit.NewRequestContextMiddleware("").Middleware(next)

			req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
			if tt.setHeader {
				req.Header.Set(audit.RequestIDHeader, tt.inbound)
			}
			rec := httptest.NewRecorder()

			// ACT
			h.ServeHTTP(rec, req)

			// ASSERT
			require.NotNil(t, next.info, "RequestInfo must be populated in the downstream context")
			gotID := next.info.RequestID
			respID := rec.Header().Get(audit.RequestIDHeader)

			assert.NotEmpty(t, gotID, "a correlation id must always be assigned")
			assert.Equal(t, gotID, respID, "the assigned id must be echoed in the response header")

			if tt.wantReused {
				assert.Equal(t, tt.inbound, gotID, "a valid inbound id must be reused verbatim")
			}
			if tt.wantReplace {
				assert.NotEqual(t, tt.inbound, gotID,
					"an unsafe inbound id must be replaced by a generated one")
			}
		})
	}
}

// TestRequestContextMiddleware_CapturesRequestMetadata covers OWASP API8:2023.
// The middleware must capture method/path/user-agent and the client IP so
// every downstream audit record is enrichable (OWASP ASVS §7.1.4).
func TestRequestContextMiddleware_CapturesRequestMetadata(t *testing.T) {
	// ARRANGE
	next := &captureInfoHandler{}
	h := audit.NewRequestContextMiddleware("").Middleware(next)

	req := httptest.NewRequest(http.MethodDelete, "/api/nodes/3", nil)
	req.Header.Set("User-Agent", "gameap-daemon/1.2")
	req.RemoteAddr = "198.51.100.9:44321"
	rec := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	require.NotNil(t, next.info)
	assert.Equal(t, http.MethodDelete, next.info.Method, "request method must be captured")
	assert.Equal(t, "/api/nodes/3", next.info.Path, "request path must be captured")
	assert.Equal(t, "gameap-daemon/1.2", next.info.UserAgent, "user agent must be captured")
	assert.Equal(t, "198.51.100.9", next.info.IP, "client IP must be the host part of RemoteAddr")
}

// TestRequestContextMiddleware_TrustedHeaderUsedForIP covers OWASP API8:2023.
// When configured behind a reverse proxy the middleware must attribute the
// client IP from the trusted header instead of the proxy's socket address.
func TestRequestContextMiddleware_TrustedHeaderUsedForIP(t *testing.T) {
	// ARRANGE
	next := &captureInfoHandler{}
	h := audit.NewRequestContextMiddleware("X-Real-IP").Middleware(next)

	req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	req.Header.Set("X-Real-IP", "203.0.113.42")
	rec := httptest.NewRecorder()

	// ACT
	h.ServeHTTP(rec, req)

	// ASSERT
	require.NotNil(t, next.info)
	assert.Equal(t, "203.0.113.42", next.info.IP,
		"the trusted proxy header must override RemoteAddr for client IP attribution")
}

// TestRequestContextMiddleware_RequestIDFromContextHelper covers OWASP API8:2023.
// RequestIDFromContext must surface the same id the middleware assigned, and
// return "" when the request never passed through the middleware.
func TestRequestContextMiddleware_RequestIDFromContextHelper(t *testing.T) {
	t.Run("returns_assigned_id_inside_handler", func(t *testing.T) {
		// ARRANGE
		var seen string
		h := audit.NewRequestContextMiddleware("").Middleware(
			http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = audit.RequestIDFromContext(r.Context())
			}),
		)
		req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
		req.Header.Set(audit.RequestIDHeader, "corr-001")
		rec := httptest.NewRecorder()

		// ACT
		h.ServeHTTP(rec, req)

		// ASSERT
		assert.Equal(t, "corr-001", seen, "helper must return the id captured by the middleware")
	})

	t.Run("returns_empty_without_middleware", func(t *testing.T) {
		// ARRANGE
		req := httptest.NewRequest(http.MethodGet, "/api/user", nil)

		// ACT
		got := audit.RequestIDFromContext(req.Context())

		// ASSERT
		assert.Empty(t, got, "no correlation id when the middleware did not run")
	})
}

// TestClientIP covers OWASP API8:2023.
// ClientIP is the single shared client-IP resolver (also used by the login
// rate limiter); its behaviour with/without a trusted header must be stable.
func TestClientIP(t *testing.T) {
	tests := []struct {
		name          string
		trustedHeader string
		headerName    string
		headerValue   string
		remoteAddr    string
		want          string
	}{
		{
			name:          "no_trusted_header_uses_remoteaddr_host",
			trustedHeader: "",
			remoteAddr:    "1.2.3.4:555",
			want:          "1.2.3.4",
		},
		{
			name:          "trusted_header_present_takes_first_of_list",
			trustedHeader: "X-Forwarded-For",
			headerName:    "X-Forwarded-For",
			headerValue:   "9.9.9.9, 10.0.0.1",
			remoteAddr:    "10.0.0.1:5555",
			want:          "9.9.9.9",
		},
		{
			name:          "trusted_header_configured_but_absent_falls_back_to_remoteaddr",
			trustedHeader: "X-Forwarded-For",
			remoteAddr:    "8.8.8.8:1234",
			want:          "8.8.8.8",
		},
		{
			name:          "remoteaddr_without_port_returned_as_is",
			trustedHeader: "",
			remoteAddr:    "192.0.2.55",
			want:          "192.0.2.55",
		},
		{
			name:          "trusted_header_single_value_no_comma",
			trustedHeader: "X-Real-IP",
			headerName:    "X-Real-IP",
			headerValue:   "203.0.113.7",
			remoteAddr:    "10.0.0.1:9999",
			want:          "203.0.113.7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.headerName != "" {
				req.Header.Set(tt.headerName, tt.headerValue)
			}

			// ACT
			got := audit.ClientIP(req, tt.trustedHeader)

			// ASSERT
			assert.Equal(t, tt.want, got, "resolved client IP mismatch")
		})
	}
}
