package middlewares

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTTPSRedirectMiddleware(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		httpsPort        uint16
		canonicalHosts   []string
		requestHost      string
		requestPath      string
		hasTLS           bool
		expectedStatus   int
		expectedLocation string
	}{
		{
			name:             "redirects_http_to_https_default_port",
			httpsPort:        443,
			requestHost:      "example.com",
			requestPath:      "/api/test",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://example.com/api/test",
		},
		{
			name:             "redirects_http_to_https_custom_port",
			httpsPort:        8443,
			requestHost:      "example.com",
			requestPath:      "/api/test",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://example.com:8443/api/test",
		},
		{
			name:             "redirects_http_to_https_with_query_params",
			httpsPort:        443,
			requestHost:      "example.com",
			requestPath:      "/api/test?foo=bar&baz=qux",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://example.com/api/test?foo=bar&baz=qux",
		},
		{
			name:             "redirects_strips_http_port_from_host",
			httpsPort:        443,
			requestHost:      "example.com:8080",
			requestPath:      "/api/test",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://example.com/api/test",
		},
		{
			name:             "passes_through_https_requests",
			httpsPort:        443,
			requestHost:      "example.com",
			requestPath:      "/api/test",
			hasTLS:           true,
			expectedStatus:   http.StatusOK,
			expectedLocation: "",
		},
		{
			name:             "redirects_root_path",
			httpsPort:        443,
			requestHost:      "example.com",
			requestPath:      "/",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://example.com/",
		},
		{
			name:             "redirects_with_ipv4_host",
			httpsPort:        443,
			requestHost:      "192.168.1.1",
			requestPath:      "/api",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://192.168.1.1/api",
		},
		{
			name:             "redirects_with_ipv4_host_and_port",
			httpsPort:        8443,
			requestHost:      "192.168.1.1:8080",
			requestPath:      "/api",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://192.168.1.1:8443/api",
		},
		{
			name:             "redirects_unknown_host_to_canonical_name",
			httpsPort:        443,
			canonicalHosts:   []string{"panel.example.com", "alt.example.com"},
			requestHost:      "attacker.example.net",
			requestPath:      "/api/test",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://panel.example.com/api/test",
		},
		{
			name:             "keeps_host_listed_among_canonical_names",
			httpsPort:        443,
			canonicalHosts:   []string{"panel.example.com", "alt.example.com"},
			requestHost:      "alt.example.com",
			requestPath:      "/api/test",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://alt.example.com/api/test",
		},
		{
			name:             "matches_canonical_name_case_insensitively",
			httpsPort:        443,
			canonicalHosts:   []string{"panel.example.com"},
			requestHost:      "Panel.Example.COM",
			requestPath:      "/api/test",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://Panel.Example.COM/api/test",
		},
		{
			name:             "redirects_unknown_host_to_first_non_wildcard_canonical_name",
			httpsPort:        443,
			canonicalHosts:   []string{"*.example.com", "panel.example.com"},
			requestHost:      "attacker.example.net",
			requestPath:      "/api/test",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://panel.example.com/api/test",
		},
		{
			name:             "keeps_request_host_when_canonical_names_are_wildcard_only",
			httpsPort:        443,
			canonicalHosts:   []string{"*.example.com"},
			requestHost:      "panel.example.com",
			requestPath:      "/api/test",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://panel.example.com/api/test",
		},
		{
			name:             "keeps_unknown_host_when_canonical_names_are_wildcard_only",
			httpsPort:        443,
			canonicalHosts:   []string{"*.example.com"},
			requestHost:      "attacker.example.net",
			requestPath:      "/api/test",
			hasTLS:           false,
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://attacker.example.net/api/test",
		},
		{
			name:             "passes_through_acme_challenge_over_http",
			httpsPort:        443,
			requestHost:      "example.com",
			requestPath:      "/.well-known/acme-challenge/abc123",
			hasTLS:           false,
			expectedStatus:   http.StatusOK,
			expectedLocation: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := HTTPSRedirectMiddleware(tt.httpsPort, tt.canonicalHosts)
			handler := middleware(nextHandler)

			req := httptest.NewRequest(http.MethodGet, tt.requestPath, nil)
			req.Host = tt.requestHost

			if tt.hasTLS {
				req.TLS = &tls.ConnectionState{}
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.expectedLocation != "" {
				assert.Equal(t, tt.expectedLocation, rec.Header().Get("Location"))
			}
		})
	}
}

func TestHTTPSRedirectMiddleware_PreservesRequestMethod(t *testing.T) {
	t.Parallel()
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := HTTPSRedirectMiddleware(443, nil)
			handler := middleware(nextHandler)

			req := httptest.NewRequest(method, "/api/test", nil)
			req.Host = "example.com"

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusMovedPermanently, rec.Code)
			assert.Equal(t, "https://example.com/api/test", rec.Header().Get("Location"))
		})
	}
}

func TestHTTPSRedirectMiddleware_CallsNextHandler_WhenTLS(t *testing.T) {
	t.Parallel()
	called := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	middleware := HTTPSRedirectMiddleware(443, nil)
	handler := middleware(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Host = "example.com"
	req.TLS = &tls.ConnectionState{}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "success", rec.Body.String())
}
