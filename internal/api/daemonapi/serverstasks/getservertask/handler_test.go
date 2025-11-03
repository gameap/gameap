package getservertask

import (
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
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func(*inmemory.ServerTaskRepository, *inmemory.ServerRepository) context.Context
		taskID         string
		expectedStatus int
		wantError      string
		expectTask     bool
	}{
		{
			name: "successful task retrieval",
			setupContext: func(taskRepo *inmemory.ServerTaskRepository, serverRepo *inmemory.ServerRepository) context.Context {
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

				server := &domain.Server{
					ID:               10,
					Enabled:          true,
					Installed:        domain.ServerInstalledStatusInstalled,
					Blocked:          false,
					Name:             "Test Server",
					UUID:             uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
					UUIDShort:        "550e8400",
					GameID:           "rust",
					DSID:             1,
					ServerIP:         "172.18.0.5",
					ServerPort:       27015,
					Dir:              "/srv/gameap/servers/server1",
					ProcessActive:    false,
					LastProcessCheck: &now,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				executeDate := now.Add(24 * time.Hour)
				task := &domain.ServerTask{
					Command:      domain.ServerTaskCommandStart,
					ServerID:     10,
					Repeat:       0,
					RepeatPeriod: 3600 * time.Second,
					Counter:      0,
					ExecuteDate:  executeDate,
					Payload:      nil,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			taskID:         "1",
			expectedStatus: http.StatusOK,
			expectTask:     true,
		},
		{
			name: "task not found",
			setupContext: func(_ *inmemory.ServerTaskRepository, _ *inmemory.ServerRepository) context.Context {
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
			taskID:         "999",
			expectedStatus: http.StatusNotFound,
			wantError:      "server task not found",
			expectTask:     false,
		},
		{
			name: "task belongs to different node - authorization failure",
			setupContext: func(taskRepo *inmemory.ServerTaskRepository, serverRepo *inmemory.ServerRepository) context.Context {
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

				server := &domain.Server{
					ID:         10,
					Enabled:    true,
					Installed:  domain.ServerInstalledStatusInstalled,
					Name:       "Test Server",
					UUID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
					UUIDShort:  "550e8400",
					GameID:     "rust",
					DSID:       2,
					ServerIP:   "172.18.0.6",
					ServerPort: 27015,
					Dir:        "/srv/gameap/servers/server1",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				executeDate := now.Add(24 * time.Hour)
				task := &domain.ServerTask{
					Command:      domain.ServerTaskCommandStart,
					ServerID:     10,
					Repeat:       0,
					RepeatPeriod: 3600 * time.Second,
					Counter:      0,
					ExecuteDate:  executeDate,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			taskID:         "1",
			expectedStatus: http.StatusNotFound,
			wantError:      "server task not found",
			expectTask:     false,
		},
		{
			name: "daemon session not found",
			setupContext: func(_ *inmemory.ServerTaskRepository, _ *inmemory.ServerRepository) context.Context {
				return context.Background()
			},
			taskID:         "1",
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
			expectTask:     false,
		},
		{
			name: "daemon session with nil node",
			setupContext: func(_ *inmemory.ServerTaskRepository, _ *inmemory.ServerRepository) context.Context {
				daemonSession := &auth.DaemonSession{
					Node: nil,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			taskID:         "1",
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
			expectTask:     false,
		},
		{
			name: "invalid task ID",
			setupContext: func(_ *inmemory.ServerTaskRepository, _ *inmemory.ServerRepository) context.Context {
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
			taskID:         "invalid",
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid task ID",
			expectTask:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			taskRepo := inmemory.NewServerTaskRepository(serverRepo)
			responder := api.NewResponder()

			handler := NewHandler(taskRepo, serverRepo, responder)

			ctx := tt.setupContext(taskRepo, serverRepo)

			req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/servers_tasks/"+tt.taskID, nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"server_task": tt.taskID})
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

			if tt.expectTask {
				var response ServerTaskResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotZero(t, response.ID)
				assert.NotEmpty(t, response.Command)
			}
		})
	}
}

func TestHandler_ResponseStructure(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	taskRepo := inmemory.NewServerTaskRepository(serverRepo)
	responder := api.NewResponder()

	handler := NewHandler(taskRepo, serverRepo, responder)

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

	server := &domain.Server{
		ID:         10,
		Enabled:    true,
		Installed:  domain.ServerInstalledStatusInstalled,
		Name:       "Test Server",
		UUID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		UUIDShort:  "550e8400",
		GameID:     "rust",
		DSID:       1,
		ServerIP:   "172.18.0.5",
		ServerPort: 27015,
		Dir:        "/srv/gameap/servers/server1",
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))

	executeDate := time.Date(2025, 10, 17, 19, 59, 53, 0, time.UTC)
	payload := "{\"test\": \"data\"}"
	task := &domain.ServerTask{
		Command:      domain.ServerTaskCommandStop,
		ServerID:     10,
		Repeat:       0,
		RepeatPeriod: 600 * time.Second,
		Counter:      1396,
		ExecuteDate:  executeDate,
		Payload:      &payload,
		CreatedAt:    &now,
		UpdatedAt:    &now,
	}
	require.NoError(t, taskRepo.Save(context.Background(), task))

	daemonSession := &auth.DaemonSession{
		Node: node,
	}
	ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

	req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/servers_tasks/1", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"server_task": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response ServerTaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, task.ID, response.ID)
	assert.Equal(t, "stop", response.Command)
	assert.Equal(t, uint(10), response.ServerID)
	assert.Equal(t, uint8(0), response.Repeat)
	assert.Equal(t, 600, response.RepeatPeriod)
	assert.Equal(t, uint(1396), response.Counter)
	assert.Equal(t, "2025-10-17 19:59:53", response.ExecuteDate)
	require.NotNil(t, response.Payload)
	assert.Equal(t, payload, *response.Payload)
	assert.NotEmpty(t, response.CreatedAt)
	assert.NotEmpty(t, response.UpdatedAt)
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	taskRepo := inmemory.NewServerTaskRepository(serverRepo)
	responder := api.NewResponder()

	handler := NewHandler(taskRepo, serverRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, taskRepo, handler.serverTaskRepo)
	assert.Equal(t, serverRepo, handler.serverRepo)
	assert.Equal(t, responder, handler.responder)
}
