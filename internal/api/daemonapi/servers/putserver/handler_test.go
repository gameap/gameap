package putserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func(*inmemory.ServerRepository) context.Context
		serverID       string
		requestBody    any
		expectedStatus int
		wantError      string
		validateServer func(*testing.T, *domain.Server)
	}{
		{ //nolint:dupl
			name: "successful update of all fields",
			setupContext: func(serverRepo *inmemory.ServerRepository) context.Context {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				serverUUID := uuid.New()
				server := &domain.Server{
					ID:            1,
					UUID:          serverUUID,
					UUIDShort:     serverUUID.String()[:8],
					Enabled:       true,
					Installed:     0,
					Blocked:       false,
					Name:          "Test Server",
					GameID:        "test",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/servers/test",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			serverID: "1",
			requestBody: map[string]any{
				"installed":          1,
				"process_active":     true,
				"last_process_check": "2024-01-15T10:30:00Z",
			},
			expectedStatus: http.StatusOK,
			validateServer: func(t *testing.T, server *domain.Server) {
				t.Helper()
				assert.Equal(t, domain.ServerInstalledStatusInstalled, server.Installed)
				assert.True(t, server.ProcessActive)
				require.NotNil(t, server.LastProcessCheck)
				assert.Equal(t, "2024-01-15T10:30:00Z", server.LastProcessCheck.Format(time.RFC3339))
			},
		},
		{
			name: "successful update of installed only",
			setupContext: func(serverRepo *inmemory.ServerRepository) context.Context {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				serverUUID := uuid.New()
				server := &domain.Server{
					ID:            1,
					UUID:          serverUUID,
					UUIDShort:     serverUUID.String()[:8],
					Enabled:       true,
					Installed:     0,
					Blocked:       false,
					Name:          "Test Server",
					GameID:        "test",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/servers/test",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			serverID: "1",
			requestBody: map[string]any{
				"installed": 2,
			},
			expectedStatus: http.StatusOK,
			validateServer: func(t *testing.T, server *domain.Server) {
				t.Helper()
				assert.Equal(t, domain.ServerInstalledStatusInstallationInProg, server.Installed)
				assert.False(t, server.ProcessActive)
				assert.Nil(t, server.LastProcessCheck)
			},
		},
		{
			name: "successful update of process_active only",
			setupContext: func(serverRepo *inmemory.ServerRepository) context.Context {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				serverUUID := uuid.New()
				server := &domain.Server{
					ID:            1,
					UUID:          serverUUID,
					UUIDShort:     serverUUID.String()[:8],
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server",
					GameID:        "test",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/servers/test",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			serverID: "1",
			requestBody: map[string]any{
				"process_active": true,
			},
			expectedStatus: http.StatusOK,
			validateServer: func(t *testing.T, server *domain.Server) {
				t.Helper()
				assert.Equal(t, domain.ServerInstalledStatusInstalled, server.Installed)
				assert.True(t, server.ProcessActive)
			},
		},
		{
			name: "successful update of last_process_check only",
			setupContext: func(serverRepo *inmemory.ServerRepository) context.Context {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				serverUUID := uuid.New()
				oldCheckTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				server := &domain.Server{
					ID:               1,
					UUID:             serverUUID,
					UUIDShort:        serverUUID.String()[:8],
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Test Server",
					GameID:           "test",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					Dir:              "/servers/test",
					ProcessActive:    true,
					LastProcessCheck: &oldCheckTime,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			serverID: "1",
			requestBody: map[string]any{
				"last_process_check": "2024-01-15T10:30:00Z",
			},
			expectedStatus: http.StatusOK,
			validateServer: func(t *testing.T, server *domain.Server) {
				t.Helper()
				assert.Equal(t, domain.ServerInstalledStatusInstalled, server.Installed)
				assert.True(t, server.ProcessActive)
				require.NotNil(t, server.LastProcessCheck)
				assert.Equal(t, "2024-01-15T10:30:00Z", server.LastProcessCheck.Format(time.RFC3339))
			},
		},
		{
			name: "empty request body does not update anything",
			setupContext: func(serverRepo *inmemory.ServerRepository) context.Context {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				serverUUID := uuid.New()
				checkTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				server := &domain.Server{
					ID:               1,
					UUID:             serverUUID,
					UUIDShort:        serverUUID.String()[:8],
					Enabled:          true,
					Installed:        50,
					Blocked:          false,
					Name:             "Test Server",
					GameID:           "test",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					Dir:              "/servers/test",
					ProcessActive:    true,
					LastProcessCheck: &checkTime,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			serverID:       "1",
			requestBody:    map[string]any{},
			expectedStatus: http.StatusOK,
			validateServer: func(t *testing.T, server *domain.Server) {
				t.Helper()
				assert.Equal(t, domain.ServerInstalledStatus(50), server.Installed)
				assert.True(t, server.ProcessActive)
				require.NotNil(t, server.LastProcessCheck)
				assert.Equal(t, "2024-01-01T00:00:00Z", server.LastProcessCheck.Format(time.RFC3339))
			},
		},
		{
			name: "server not found",
			setupContext: func(_ *inmemory.ServerRepository) context.Context {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			serverID: "999",
			requestBody: map[string]any{
				"installed": 1,
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name: "server belongs to different node",
			setupContext: func(serverRepo *inmemory.ServerRepository) context.Context {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				serverUUID := uuid.New()
				server := &domain.Server{
					ID:            1,
					UUID:          serverUUID,
					UUIDShort:     serverUUID.String()[:8],
					Enabled:       true,
					Installed:     0,
					Blocked:       false,
					Name:          "Test Server",
					GameID:        "test",
					DSID:          2,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/servers/test",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			serverID: "1",
			requestBody: map[string]any{
				"installed": 1,
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name: "invalid server ID",
			setupContext: func(_ *inmemory.ServerRepository) context.Context {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			serverID: "invalid",
			requestBody: map[string]any{
				"installed": 1,
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server ID",
		},
		{
			name: "daemon session not found",
			setupContext: func(_ *inmemory.ServerRepository) context.Context {
				return context.Background()
			},
			serverID: "1",
			requestBody: map[string]any{
				"installed": 1,
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
		},
		{
			name: "daemon session with nil node",
			setupContext: func(_ *inmemory.ServerRepository) context.Context {
				daemonSession := &auth.DaemonSession{
					Node: nil,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			serverID: "1",
			requestBody: map[string]any{
				"installed": 1,
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
		},
		{
			name: "invalid JSON body",
			setupContext: func(_ *inmemory.ServerRepository) context.Context {
				now := time.Now()
				node := &domain.Node{
					ID:                  1,
					Enabled:             true,
					Name:                "test-node",
					OS:                  "linux",
					Location:            "Montenegro",
					IPs:                 []string{"172.18.0.5"},
					WorkPath:            "/srv/gameap",
					GdaemonHost:         "172.18.0.5",
					GdaemonPort:         31717,
					GdaemonAPIKey:       "test-api-key",
					GdaemonServerCert:   "certs/root.crt",
					ClientCertificateID: 1,
					PreferInstallMethod: "auto",
					CreatedAt:           &now,
					UpdatedAt:           &now,
				}

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			serverID:       "1",
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			responder := api.NewResponder()

			handler := NewHandler(
				serverRepo,
				responder,
			)

			ctx := tt.setupContext(serverRepo)

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPut, "/gdaemon_api/servers/"+tt.serverID, bytes.NewReader(body))
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"server": tt.serverID})
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
			} else {
				var response updateServerResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "success", response.Message)
			}

			if tt.validateServer != nil {
				servers, err := serverRepo.Find(ctx, nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)
				tt.validateServer(t, &servers[0])
			}
		})
	}
}

func TestHandler_ResponseStructure(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()

	handler := NewHandler(
		serverRepo,
		responder,
	)

	now := time.Now()
	node := &domain.Node{
		ID:                  1,
		Enabled:             true,
		Name:                "test-node",
		OS:                  "linux",
		Location:            "Montenegro",
		IPs:                 []string{"172.18.0.5"},
		WorkPath:            "/srv/gameap",
		GdaemonHost:         "172.18.0.5",
		GdaemonPort:         31717,
		GdaemonAPIKey:       "test-api-key",
		GdaemonServerCert:   "certs/root.crt",
		ClientCertificateID: 1,
		PreferInstallMethod: "auto",
		CreatedAt:           &now,
		UpdatedAt:           &now,
	}

	serverUUID := uuid.New()
	server := &domain.Server{
		ID:            1,
		UUID:          serverUUID,
		UUIDShort:     serverUUID.String()[:8],
		Enabled:       true,
		Installed:     0,
		Blocked:       false,
		Name:          "Test Server",
		GameID:        "test",
		DSID:          1,
		GameModID:     1,
		ServerIP:      "127.0.0.1",
		ServerPort:    27015,
		Dir:           "/servers/test",
		ProcessActive: false,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))

	daemonSession := &auth.DaemonSession{
		Node: node,
	}
	ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

	requestBody := map[string]any{
		"installed":          1,
		"process_active":     true,
		"last_process_check": "2024-01-15T10:30:00Z",
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/gdaemon_api/servers/1", bytes.NewReader(body))
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response updateServerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, "success", response.Message)
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()

	handler := NewHandler(
		serverRepo,
		responder,
	)

	require.NotNil(t, handler)
	assert.Equal(t, serverRepo, handler.serverRepo)
	assert.Equal(t, responder, handler.responder)
}

func TestUpdateServerInput_Validate(t *testing.T) {
	tests := []struct {
		name      string
		input     *updateServerInput
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid input with all fields",
			input: &updateServerInput{
				Installed:        lo.ToPtr(1),
				ProcessActive:    lo.ToPtr(flexible.Bool(true)),
				LastProcessCheck: lo.ToPtr(flexible.Time{Time: time.Now()}),
			},
			wantError: false,
		},
		{
			name: "valid input with installed only",
			input: &updateServerInput{
				Installed: lo.ToPtr(1),
			},
			wantError: false,
		},
		{
			name: "valid input with process_active only",
			input: &updateServerInput{
				ProcessActive: lo.ToPtr(flexible.Bool(false)),
			},
			wantError: false,
		},
		{
			name: "valid input with last_process_check only",
			input: &updateServerInput{
				LastProcessCheck: lo.ToPtr(flexible.Time{Time: time.Now()}),
			},
			wantError: false,
		},
		{
			name:      "valid empty input",
			input:     &updateServerInput{},
			wantError: false,
		},
		{
			name: "valid installed - zero",
			input: &updateServerInput{
				Installed: lo.ToPtr(0),
			},
			wantError: false,
		},
		{
			name: "valid installed - large value",
			input: &updateServerInput{
				Installed: lo.ToPtr(2),
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
