package patchservers

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
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name            string
		setupContext    func(*inmemory.ServerRepository) context.Context
		requestBody     any
		expectedStatus  int
		wantError       string
		validateServers func(*testing.T, *inmemory.ServerRepository)
	}{
		{
			name: "successful bulk update of all fields",
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

				serverUUID1 := uuid.New()
				server1 := &domain.Server{
					ID:            1,
					UUID:          serverUUID1,
					UUIDShort:     serverUUID1.String()[:8],
					Enabled:       true,
					Installed:     0,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "test",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server1))

				serverUUID2 := uuid.New()
				server2 := &domain.Server{
					ID:            2,
					UUID:          serverUUID2,
					UUIDShort:     serverUUID2.String()[:8],
					Enabled:       true,
					Installed:     0,
					Blocked:       false,
					Name:          "Test Server 2",
					GameID:        "test",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27016,
					Dir:           "/servers/test2",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server2))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			requestBody: []map[string]any{
				{
					"id":                 1,
					"installed":          2,
					"process_active":     1,
					"last_process_check": "2024-01-15T10:30:00Z",
				},
				{
					"id":                 2,
					"process_active":     1,
					"last_process_check": "2024-01-15T10:30:00Z",
				},
			},
			expectedStatus: http.StatusOK,
			validateServers: func(t *testing.T, serverRepo *inmemory.ServerRepository) {
				t.Helper()
				servers, err := serverRepo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 2)

				server1 := lo.Filter(servers, func(s domain.Server, _ int) bool { return s.ID == 1 })[0]
				assert.Equal(t, domain.ServerInstalledStatus(2), server1.Installed)
				assert.True(t, server1.ProcessActive)
				require.NotNil(t, server1.LastProcessCheck)
				assert.Equal(t, "2024-01-15T10:30:00Z", server1.LastProcessCheck.Format(time.RFC3339))

				server2 := lo.Filter(servers, func(s domain.Server, _ int) bool { return s.ID == 2 })[0]
				assert.Equal(t, domain.ServerInstalledStatus(0), server2.Installed)
				assert.True(t, server2.ProcessActive)
				require.NotNil(t, server2.LastProcessCheck)
				assert.Equal(t, "2024-01-15T10:30:00Z", server2.LastProcessCheck.Format(time.RFC3339))
			},
		},
		{
			name: "successful update with only installed field",
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

				serverUUID1 := uuid.New()
				server1 := &domain.Server{
					ID:            1,
					UUID:          serverUUID1,
					UUIDShort:     serverUUID1.String()[:8],
					Enabled:       true,
					Installed:     0,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "test",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server1))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			requestBody: []map[string]any{
				{
					"id":        1,
					"installed": 1,
				},
			},
			expectedStatus: http.StatusOK,
			validateServers: func(t *testing.T, serverRepo *inmemory.ServerRepository) {
				t.Helper()
				servers, err := serverRepo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)

				assert.Equal(t, domain.ServerInstalledStatusInstalled, servers[0].Installed)
				assert.False(t, servers[0].ProcessActive)
				assert.Nil(t, servers[0].LastProcessCheck)
			},
		},
		{
			name: "successful update with only process_active field",
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

				serverUUID1 := uuid.New()
				server1 := &domain.Server{
					ID:            1,
					UUID:          serverUUID1,
					UUIDShort:     serverUUID1.String()[:8],
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "test",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server1))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			requestBody: []map[string]any{
				{
					"id":             1,
					"process_active": 1,
				},
			},
			expectedStatus: http.StatusOK,
			validateServers: func(t *testing.T, serverRepo *inmemory.ServerRepository) {
				t.Helper()
				servers, err := serverRepo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)

				assert.Equal(t, domain.ServerInstalledStatusInstalled, servers[0].Installed)
				assert.True(t, servers[0].ProcessActive)
			},
		},
		{
			name: "successful update with only last_process_check field",
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

				serverUUID1 := uuid.New()
				oldCheckTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
				server1 := &domain.Server{
					ID:               1,
					UUID:             serverUUID1,
					UUIDShort:        serverUUID1.String()[:8],
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Test Server 1",
					GameID:           "test",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					Dir:              "/servers/test1",
					ProcessActive:    true,
					LastProcessCheck: &oldCheckTime,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server1))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			requestBody: []map[string]any{
				{
					"id":                 1,
					"last_process_check": "2024-01-15T10:30:00Z",
				},
			},
			expectedStatus: http.StatusOK,
			validateServers: func(t *testing.T, serverRepo *inmemory.ServerRepository) {
				t.Helper()
				servers, err := serverRepo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)

				assert.Equal(t, domain.ServerInstalledStatusInstalled, servers[0].Installed)
				assert.True(t, servers[0].ProcessActive)
				require.NotNil(t, servers[0].LastProcessCheck)
				assert.Equal(t, "2024-01-15T10:30:00Z", servers[0].LastProcessCheck.Format(time.RFC3339))
			},
		},
		{
			name: "empty request body returns success",
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
			requestBody:    []map[string]any{},
			expectedStatus: http.StatusOK,
		},
		{
			name: "skip servers not belonging to the node",
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

				serverUUID1 := uuid.New()
				server1 := &domain.Server{
					ID:            1,
					UUID:          serverUUID1,
					UUIDShort:     serverUUID1.String()[:8],
					Enabled:       true,
					Installed:     0,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "test",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server1))

				serverUUID2 := uuid.New()
				server2 := &domain.Server{
					ID:            2,
					UUID:          serverUUID2,
					UUIDShort:     serverUUID2.String()[:8],
					Enabled:       true,
					Installed:     0,
					Blocked:       false,
					Name:          "Test Server 2",
					GameID:        "test",
					DSID:          2,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27016,
					Dir:           "/servers/test2",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server2))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			requestBody: []map[string]any{
				{
					"id":        1,
					"installed": 1,
				},
				{
					"id":        2,
					"installed": 1,
				},
			},
			expectedStatus: http.StatusOK,
			validateServers: func(t *testing.T, serverRepo *inmemory.ServerRepository) {
				t.Helper()
				servers, err := serverRepo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 2)

				server1 := lo.Filter(servers, func(s domain.Server, _ int) bool { return s.ID == 1 })[0]
				assert.Equal(t, domain.ServerInstalledStatusInstalled, server1.Installed)

				server2 := lo.Filter(servers, func(s domain.Server, _ int) bool { return s.ID == 2 })[0]
				assert.Equal(t, domain.ServerInstalledStatus(0), server2.Installed)
			},
		},
		{
			name: "skip non-existent servers",
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

				serverUUID1 := uuid.New()
				server1 := &domain.Server{
					ID:            1,
					UUID:          serverUUID1,
					UUIDShort:     serverUUID1.String()[:8],
					Enabled:       true,
					Installed:     0,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "test",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server1))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			requestBody: []map[string]any{
				{
					"id":        1,
					"installed": 1,
				},
				{
					"id":        999,
					"installed": 1,
				},
			},
			expectedStatus: http.StatusOK,
			validateServers: func(t *testing.T, serverRepo *inmemory.ServerRepository) {
				t.Helper()
				servers, err := serverRepo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)

				assert.Equal(t, domain.ServerInstalledStatusInstalled, servers[0].Installed)
			},
		},
		{
			name: "daemon session not found",
			setupContext: func(_ *inmemory.ServerRepository) context.Context {
				return context.Background()
			},
			requestBody: []map[string]any{
				{
					"id":        1,
					"installed": 1,
				},
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
			requestBody: []map[string]any{
				{
					"id":        1,
					"installed": 1,
				},
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
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request",
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
			requestBody: []map[string]any{
				{
					"id":        0,
					"installed": 1,
				},
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "server ID is required",
		},
		{
			name: "invalid installed status - negative",
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
			requestBody: []map[string]any{
				{
					"id":        1,
					"installed": -1,
				},
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid installed status",
		},
		{
			name: "invalid installed status - too large",
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
			requestBody: []map[string]any{
				{
					"id":        1,
					"installed": 1000000000,
				},
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid installed status",
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

			req := httptest.NewRequest(http.MethodPatch, "/gdaemon_api/servers", bytes.NewReader(body))
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
			} else {
				var response bulkUpdateServerResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "success", response.Message)
			}

			if tt.validateServers != nil {
				tt.validateServers(t, serverRepo)
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

	requestBody := []map[string]any{
		{
			"id":                 1,
			"installed":          1,
			"process_active":     1,
			"last_process_check": "2024-01-15T10:30:00Z",
		},
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/gdaemon_api/servers", bytes.NewReader(body))
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response bulkUpdateServerResponse
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

func TestBulkUpdateServerInput_Validate(t *testing.T) {
	tests := []struct {
		name      string
		input     *bulkUpdateServerInput
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid input with all fields",
			input: &bulkUpdateServerInput{
				ID:               1,
				Installed:        lo.ToPtr(1),
				ProcessActive:    lo.ToPtr(flexible.Bool(true)),
				LastProcessCheck: lo.ToPtr(flexible.Time{Time: time.Now()}),
			},
			wantError: false,
		},
		{
			name: "valid input with installed only",
			input: &bulkUpdateServerInput{
				ID:        1,
				Installed: lo.ToPtr(1),
			},
			wantError: false,
		},
		{
			name: "valid input with process_active only",
			input: &bulkUpdateServerInput{
				ID:            1,
				ProcessActive: lo.ToPtr(flexible.Bool(false)),
			},
			wantError: false,
		},
		{
			name: "valid input with last_process_check only",
			input: &bulkUpdateServerInput{
				ID:               1,
				LastProcessCheck: lo.ToPtr(flexible.Time{Time: time.Now()}),
			},
			wantError: false,
		},
		{
			name: "valid input with ID only",
			input: &bulkUpdateServerInput{
				ID: 1,
			},
			wantError: false,
		},
		{
			name: "valid installed - zero",
			input: &bulkUpdateServerInput{
				ID:        1,
				Installed: lo.ToPtr(0),
			},
			wantError: false,
		},
		{
			name: "valid installed - large value",
			input: &bulkUpdateServerInput{
				ID:        1,
				Installed: lo.ToPtr(2),
			},
			wantError: false,
		},
		{
			name: "invalid - missing server ID",
			input: &bulkUpdateServerInput{
				ID:        0,
				Installed: lo.ToPtr(1),
			},
			wantError: true,
			errorMsg:  "server ID is required",
		},
		{
			name: "invalid installed status - negative",
			input: &bulkUpdateServerInput{
				ID:        1,
				Installed: lo.ToPtr(-1),
			},
			wantError: true,
			errorMsg:  "invalid installed status",
		},
		{
			name: "invalid installed status - too large",
			input: &bulkUpdateServerInput{
				ID:        1,
				Installed: lo.ToPtr(20),
			},
			wantError: true,
			errorMsg:  "invalid installed status",
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
