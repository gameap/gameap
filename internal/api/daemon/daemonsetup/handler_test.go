package daemonsetup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	daemonbase "github.com/gameap/gameap/internal/api/daemon/base"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		setupEnv       func(t *testing.T)
		setupCache     func(cache.Cache)
		panelHost      string
		expectedStatus int
		wantError      bool
		validateResp   func(*testing.T, string)
	}{
		{
			name:  "successful setup with env token",
			token: "test-env-token",
			setupEnv: func(t *testing.T) {
				t.Helper()
				t.Setenv("DAEMON_SETUP_TOKEN", "test-env-token")
			},
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, "export createToken=")
				assert.Contains(t, resp, "export panelHost=http://panel.example.com")
				assert.Contains(t, resp, "curl -sL https://raw.githubusercontent.com/gameap/auto-install-scripts/master/install-gdaemon.sh | bash --")
			},
		},
		{
			name:  "successful setup with cache token",
			token: "cached-token",
			setupCache: func(c cache.Cache) {
				err := c.Set(context.Background(), daemonbase.AutoSetupTokenCacheKey, "cached-token", cache.WithExpiration(300*time.Second))
				require.NoError(t, err)
			},
			panelHost:      "https://panel.example.com",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp string) {
				t.Helper()

				assert.Contains(t, resp, "export createToken=")
				assert.Contains(t, resp, "export panelHost=http://panel.example.com")
				assert.Contains(t, resp, "curl -sL https://raw.githubusercontent.com/gameap/auto-install-scripts/master/install-gdaemon.sh")
			},
		},
		{
			name:           "invalid token",
			token:          "wrong-token",
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusForbidden,
			wantError:      true,
		},
		{
			name:  "token mismatch with env",
			token: "wrong-token",
			setupEnv: func(t *testing.T) {
				t.Helper()
				t.Setenv("DAEMON_SETUP_TOKEN", "correct-token")
			},
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusForbidden,
			wantError:      true,
		},
		{
			name:  "token mismatch with cache",
			token: "wrong-token",
			setupCache: func(c cache.Cache) {
				err := c.Set(context.Background(), daemonbase.AutoSetupTokenCacheKey, "correct-token", cache.WithExpiration(300*time.Second))
				require.NoError(t, err)
			},
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusForbidden,
			wantError:      true,
		},
		{
			name:           "token not found in cache",
			token:          "some-token",
			setupCache:     func(_ cache.Cache) {},
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusForbidden,
			wantError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheInstance := cache.NewInMemory()
			responder := api.NewResponder()
			handler := NewHandler(cacheInstance, responder, tt.panelHost)

			if tt.setupCache != nil {
				tt.setupCache(cacheInstance)
			}

			if tt.setupEnv != nil {
				tt.setupEnv(t)
			}

			req := httptest.NewRequest(http.MethodGet, "/gdaemon/setup/"+tt.token, nil)
			req = mux.SetURLVars(req, map[string]string{"token": tt.token})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError {
				assert.Contains(t, w.Body.String(), "error")
			}

			if tt.validateResp != nil {
				tt.validateResp(t, w.Body.String())
			}
		})
	}
}

func TestHandler_CreateTokenStoredInCache(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	responder := api.NewResponder()
	handler := NewHandler(cacheInstance, responder, "panel.example.com")

	t.Setenv("DAEMON_SETUP_TOKEN", "test-token")

	req := httptest.NewRequest(http.MethodGet, "/gdaemon/setup/test-token", nil)
	req = mux.SetURLVars(req, map[string]string{"token": "test-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	val, err := cacheInstance.Get(context.Background(), daemonbase.AutoCreateTokenCacheKey)
	require.NoError(t, err)
	assert.NotNil(t, val)

	tokenStr, ok := val.(string)
	require.True(t, ok)
	assert.NotEmpty(t, tokenStr)
}

func TestHandler_SetupTokenDeletedFromCache(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	responder := api.NewResponder()
	handler := NewHandler(cacheInstance, responder, "panel.example.com")

	err := cacheInstance.Set(context.Background(), daemonbase.AutoSetupTokenCacheKey, "test-token", cache.WithExpiration(300*time.Second))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/gdaemon/setup/test-token", nil)
	req = mux.SetURLVars(req, map[string]string{"token": "test-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	val, err := cacheInstance.Get(context.Background(), daemonbase.AutoSetupTokenCacheKey)
	if err == nil {
		assert.Nil(t, val)
	}
}

func TestHandler_HostDetection(t *testing.T) {
	tests := []struct {
		name             string
		panelHost        string
		headers          map[string]string
		host             string
		wantHostInScript string
	}{
		{
			name:             "uses configured panel host",
			panelHost:        "configured.example.com",
			host:             "request.example.com",
			wantHostInScript: "http://configured.example.com",
		},
		{
			name:             "detects from request host without configured host",
			panelHost:        "",
			host:             "detected.example.com",
			wantHostInScript: "http://detected.example.com",
		},
		{
			name:      "uses X-Forwarded-Host header",
			panelHost: "",
			headers: map[string]string{
				"X-Forwarded-Host":  "forwarded.example.com",
				"X-Forwarded-Proto": "https",
			},
			host:             "request.example.com",
			wantHostInScript: "https://forwarded.example.com",
		},
		{
			name:             "strips http prefix from panel host",
			panelHost:        "http://panel.example.com",
			host:             "request.example.com",
			wantHostInScript: "http://panel.example.com",
		},
		{
			name:             "strips https prefix from panel host",
			panelHost:        "https://panel.example.com",
			host:             "request.example.com",
			wantHostInScript: "http://panel.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheInstance := cache.NewInMemory()
			responder := api.NewResponder()
			handler := NewHandler(cacheInstance, responder, tt.panelHost)

			t.Setenv("DAEMON_SETUP_TOKEN", "test-token")

			req := httptest.NewRequest(http.MethodGet, "/gdaemon/setup/test-token", nil)
			req = mux.SetURLVars(req, map[string]string{"token": "test-token"})
			req.Host = tt.host

			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			body := w.Body.String()
			assert.Contains(t, body, "export panelHost="+tt.wantHostInScript)
		})
	}
}

func TestHandler_ResponseContentType(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	responder := api.NewResponder()
	handler := NewHandler(cacheInstance, responder, "panel.example.com")

	t.Setenv("DAEMON_SETUP_TOKEN", "test-token")

	req := httptest.NewRequest(http.MethodGet, "/gdaemon/setup/test-token", nil)
	req = mux.SetURLVars(req, map[string]string{"token": "test-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
}

func TestHandler_BuildSetupScript(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	responder := api.NewResponder()
	handler := NewHandler(cacheInstance, responder, "panel.example.com")

	script := handler.buildSetupScript("test-create-token", "http://panel.example.com")

	assert.Contains(t, script, "export createToken=test-create-token")
	assert.Contains(t, script, "export panelHost=http://panel.example.com")
	assert.Contains(t, script, "curl -sL https://raw.githubusercontent.com/gameap/auto-install-scripts/master/install-gdaemon.sh | bash --")

	lines := strings.Split(script, "\n")
	assert.GreaterOrEqual(t, len(lines), 3)
}

func TestHandler_EnvTokenPriority(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	responder := api.NewResponder()
	handler := NewHandler(cacheInstance, responder, "panel.example.com")

	err := cacheInstance.Set(context.Background(), daemonbase.AutoSetupTokenCacheKey, "cache-token", cache.WithExpiration(300*time.Second))
	require.NoError(t, err)

	t.Setenv("DAEMON_SETUP_TOKEN", "env-token")

	req := httptest.NewRequest(http.MethodGet, "/gdaemon/setup/env-token", nil)
	req = mux.SetURLVars(req, map[string]string{"token": "env-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_CacheTokenUsedWhenEnvEmpty(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	responder := api.NewResponder()
	handler := NewHandler(cacheInstance, responder, "panel.example.com")

	err := cacheInstance.Set(context.Background(), daemonbase.AutoSetupTokenCacheKey, "cache-token", cache.WithExpiration(300*time.Second))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/gdaemon/setup/cache-token", nil)
	req = mux.SetURLVars(req, map[string]string{"token": "cache-token"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
