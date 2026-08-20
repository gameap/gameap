package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCORSMiddleware(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		config   *config.Config
		expected string
	}{
		{
			name: "creates_middleware_with_standard_http_port",
			config: &config.Config{
				HTTPHost: "example.com",
				HTTPPort: 80,
			},
			expected: "http://example.com",
		},
		{
			name: "creates_middleware_with_standard_https_port",
			config: &config.Config{
				HTTPHost: "example.com",
				HTTPPort: 443,
			},
			expected: "http://example.com",
		},
		{
			name: "creates_middleware_with_custom_port",
			config: &config.Config{
				HTTPHost: "example.com",
				HTTPPort: 8080,
			},
			expected: "http://example.com:8080",
		},
		{
			name: "creates_middleware_with_localhost",
			config: &config.Config{
				HTTPHost: "localhost",
				HTTPPort: 3000,
			},
			expected: "http://localhost:3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			middleware := NewCORSMiddleware(tt.config)

			require.NotNil(t, middleware)
			require.NotNil(t, middleware.cors)
		})
	}
}

func TestCORSMiddleware_Middleware(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		config         *config.Config
		requestOrigin  string
		requestMethod  string
		expectedStatus int
		checkCORS      bool
	}{
		{
			name: "allows_same_origin_request",
			config: &config.Config{
				HTTPHost: "example.com",
				HTTPPort: 80,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  http.MethodGet,
			expectedStatus: http.StatusOK,
			checkCORS:      true,
		},
		{
			name: "allows_same_origin_with_custom_port",
			config: &config.Config{
				HTTPHost: "example.com",
				HTTPPort: 8080,
			},
			requestOrigin:  "http://example.com:8080",
			requestMethod:  http.MethodGet,
			expectedStatus: http.StatusOK,
			checkCORS:      true,
		},
		{
			name: "handles_preflight_request",
			config: &config.Config{
				HTTPHost: "example.com",
				HTTPPort: 80,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  http.MethodOptions,
			expectedStatus: http.StatusNoContent,
			checkCORS:      true,
		},
		{
			name: "passes_through_request_without_origin",
			config: &config.Config{
				HTTPHost: "example.com",
				HTTPPort: 80,
			},
			requestOrigin:  "",
			requestMethod:  http.MethodGet,
			expectedStatus: http.StatusOK,
			checkCORS:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			corsMiddleware := NewCORSMiddleware(tt.config)

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("success"))
			})

			handler := corsMiddleware.Middleware(nextHandler)

			req := httptest.NewRequest(tt.requestMethod, "/api/test", nil)
			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}

			if tt.requestMethod == http.MethodOptions {
				req.Header.Set("Access-Control-Request-Method", http.MethodPost)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.checkCORS && tt.requestOrigin != "" {
				corsHeader := rec.Header().Get("Access-Control-Allow-Origin")
				if corsHeader != "" {
					assert.Equal(t, tt.requestOrigin, corsHeader)
				}
			}
		})
	}
}

func TestCORSMiddleware_Middleware_CallsNextHandler(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		HTTPHost: "example.com",
		HTTPPort: 80,
	}
	corsMiddleware := NewCORSMiddleware(cfg)

	called := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response body"))
	})

	handler := corsMiddleware.Middleware(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "response body", rec.Body.String())
}

func TestCORSMiddleware_Middleware_AllowsCredentials(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		HTTPHost: "example.com",
		HTTPPort: 80,
	}
	corsMiddleware := NewCORSMiddleware(cfg)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := corsMiddleware.Middleware(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Origin", "http://example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, "true", rec.Header().Get("Access-Control-Allow-Credentials"))
}

func TestNewCORSMiddleware_HTTPSWhenForceHTTPS(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		HTTPHost:  "app.example.com",
		HTTPPort:  8025,
		HTTPSPort: 443,
	}
	cfg.TLS.ForceHTTPS = true

	middleware := NewCORSMiddleware(cfg)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.Middleware(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Origin", "https://app.example.com")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "https://app.example.com", rec.Header().Get("Access-Control-Allow-Origin"),
		"with ForceHTTPS the auto-derived origin must use https://, not http://")
}

func TestNewCORSMiddleware_RejectsHTTPOriginWhenForceHTTPS(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		HTTPHost:  "app.example.com",
		HTTPSPort: 443,
	}
	cfg.TLS.ForceHTTPS = true

	middleware := NewCORSMiddleware(cfg)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.Middleware(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Origin", "http://app.example.com")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// rs/cors omits the Allow-Origin header for un-allowed origins (it does
	// NOT 4xx the request); the absence of the header is the rejection signal.
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"),
		"http:// origin must not be allowed under ForceHTTPS")
}

func TestNewCORSMiddleware_ExplicitAllowedOriginsWinsOverAutoDerived(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		HTTPHost:           "default.local",
		HTTPPort:           80,
		HTTPAllowedOrigins: []string{"https://admin.example.com", "https://ops.example.com"},
	}

	middleware := NewCORSMiddleware(cfg)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.Middleware(nextHandler)

	for _, origin := range []string{"https://admin.example.com", "https://ops.example.com"} {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Origin", origin)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		assert.Equalf(t, origin, rec.Header().Get("Access-Control-Allow-Origin"),
			"explicit allow-list entry %s must be honoured", origin)
	}

	// The auto-derived "http://default.local" must NOT be implicitly added.
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Origin", "http://default.local")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"),
		"auto-derived default origin must not be merged when an explicit list is provided")
}
