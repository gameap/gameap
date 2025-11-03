package nodesetup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	daemonbase "github.com/gameap/gameap/internal/api/daemon/base"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser1 = domain.User{
	ID:    1,
	Login: "admin",
	Email: "admin@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		setupEnv       func(t *testing.T)
		setupCache     func(cache.Cache)
		panelHost      string
		expectedStatus int
		wantError      string
		validateResp   func(*testing.T, setupResponse)
	}{
		{
			name: "successful setup without env token",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			panelHost:      "panel.example.com",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp setupResponse) {
				t.Helper()

				assert.NotEmpty(t, resp.Token)
				assert.NotEmpty(t, resp.Link)
				assert.Equal(t, "http://panel.example.com", resp.Host)
				assert.Contains(t, resp.Link, "http://panel.example.com/gdaemon/setup/")
			},
		},
		{
			name: "successful setup with env token",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupEnv: func(t *testing.T) {
				t.Helper()

				t.Setenv("DAEMON_SETUP_TOKEN", "test-env-token")
			},
			panelHost:      "https://panel.example.com",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp setupResponse) {
				t.Helper()

				assert.Equal(t, "test-env-token", resp.Token)
				assert.NotEmpty(t, resp.Link)
				assert.Equal(t, "http://panel.example.com", resp.Host)
				assert.Contains(t, resp.Link, "http://panel.example.com/gdaemon/setup/test-env-token")
			},
		},
		{
			name:           "user not authenticated",
			panelHost:      "https://panel.example.com",
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name: "cache clears old setup token",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupCache: func(c cache.Cache) {
				err := c.Set(context.Background(), daemonbase.AutoSetupTokenCacheKey, "old-token", cache.WithExpiration(300*time.Second))
				require.NoError(t, err)
			},
			panelHost:      "https://panel.example.com",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp setupResponse) {
				t.Helper()

				assert.NotEmpty(t, resp.Token)
				assert.NotEqual(t, "old-token", resp.Token)
			},
		},
		{
			name: "creates and stores create token in cache",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			panelHost:      "https://panel.example.com",
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp setupResponse) {
				t.Helper()

				assert.NotEmpty(t, resp.Token)
			},
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

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/setup", nil)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}

			if tt.validateResp != nil {
				var resp setupResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				tt.validateResp(t, resp)
			}
		})
	}
}

func TestHandler_SetupTokenNotFromEnv(t *testing.T) {
	cacheInstance := cache.NewInMemory()
	responder := api.NewResponder()
	handler := NewHandler(cacheInstance, responder, "https://panel.example.com")

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/setup", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	val, err := cacheInstance.Get(context.Background(), daemonbase.AutoSetupTokenCacheKey)
	require.NoError(t, err)
	assert.NotNil(t, val)

	tokenStr, ok := val.(string)
	require.True(t, ok)
	assert.NotEmpty(t, tokenStr)
}

func TestHandler_HostDetection(t *testing.T) {
	tests := []struct {
		name       string
		panelHost  string
		headers    map[string]string
		host       string
		wantHost   string
		wantInLink string
	}{
		{
			name:       "uses configured panel host",
			panelHost:  "configured.example.com",
			host:       "request.example.com",
			wantHost:   "http://configured.example.com",
			wantInLink: "http://configured.example.com/gdaemon/setup/",
		},
		{
			name:       "detects from request host without configured host",
			panelHost:  "",
			host:       "detected.example.com",
			wantHost:   "http://detected.example.com",
			wantInLink: "http://detected.example.com/gdaemon/setup/",
		},
		{
			name:      "uses X-Forwarded-Host header",
			panelHost: "",
			headers: map[string]string{
				"X-Forwarded-Host":  "forwarded.example.com",
				"X-Forwarded-Proto": "https",
			},
			host:       "request.example.com",
			wantHost:   "https://forwarded.example.com",
			wantInLink: "https://forwarded.example.com/gdaemon/setup/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cacheInstance := cache.NewInMemory()
			responder := api.NewResponder()
			handler := NewHandler(cacheInstance, responder, tt.panelHost)

			session := &auth.Session{
				Login: "admin",
				Email: "admin@example.com",
				User:  &testUser1,
			}
			ctx := auth.ContextWithSession(context.Background(), session)

			req := httptest.NewRequest(http.MethodGet, "/api/dedicated_servers/setup", nil)
			req = req.WithContext(ctx)
			req.Host = tt.host

			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var resp setupResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

			assert.Equal(t, tt.wantHost, resp.Host)
			assert.Contains(t, resp.Link, tt.wantInLink)
		})
	}
}

func TestNewSetupResponse(t *testing.T) {
	token := "test-token"
	host := "https://example.com"

	resp := newSetupResponse(token, host)

	assert.Equal(t, "test-token", resp.Token)
	assert.Equal(t, "https://example.com", resp.Host)
	assert.Equal(t, "https://example.com/gdaemon/setup/test-token", resp.Link)
}
