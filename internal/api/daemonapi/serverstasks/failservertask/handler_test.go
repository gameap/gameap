package failservertask

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
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name               string
		setupContext       func(*inmemory.ServerTaskRepository, *inmemory.ServerRepository) context.Context
		taskID             string
		requestBody        any
		expectedStatus     int
		wantError          string
		validateServerTask func(*testing.T, *inmemory.ServerTaskFailRepository, uint)
	}{
		{
			name: "successful server task fail creation",
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
			taskID: "1",
			requestBody: map[string]any{
				"output": "Task execution failed with error: connection timeout",
			},
			expectedStatus: http.StatusOK,
			validateServerTask: func(t *testing.T, failRepo *inmemory.ServerTaskFailRepository, taskID uint) {
				t.Helper()
				fails, err := failRepo.Find(context.Background(), nil, nil, nil)
				require.NoError(t, err)
				require.Len(t, fails, 1)
				assert.Equal(t, taskID, fails[0].ServerTaskID)
				assert.Equal(t, "Task execution failed with error: connection timeout", fails[0].Output)
				assert.NotNil(t, fails[0].CreatedAt)
				assert.NotNil(t, fails[0].UpdatedAt)
			},
		},
		{
			name: "server task not found",
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
			taskID: "999",
			requestBody: map[string]any{
				"output": "Some output",
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server task not found",
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
			taskID: "1",
			requestBody: map[string]any{
				"output": "Some output",
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server task not found",
		},
		{
			name: "daemon session not found",
			setupContext: func(_ *inmemory.ServerTaskRepository, _ *inmemory.ServerRepository) context.Context {
				return context.Background()
			},
			taskID: "1",
			requestBody: map[string]any{
				"output": "Some output",
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
		},
		{
			name: "daemon session with nil node",
			setupContext: func(_ *inmemory.ServerTaskRepository, _ *inmemory.ServerRepository) context.Context {
				daemonSession := &auth.DaemonSession{
					Node: nil,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			taskID: "1",
			requestBody: map[string]any{
				"output": "Some output",
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
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
			taskID: "invalid",
			requestBody: map[string]any{
				"output": "Some output",
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid task ID",
		},
		{
			name: "invalid request body - malformed JSON",
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
			taskID:         "1",
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request",
		},
		{
			name: "invalid input - empty output",
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
					DSID:       1,
					ServerIP:   "172.18.0.5",
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
			taskID: "1",
			requestBody: map[string]any{
				"output": "",
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "output is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			taskRepo := inmemory.NewServerTaskRepository(serverRepo)
			failRepo := inmemory.NewServerTaskFailRepository()
			responder := api.NewResponder()

			handler := NewHandler(taskRepo, failRepo, serverRepo, responder)

			ctx := tt.setupContext(taskRepo, serverRepo)

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/servers_tasks/"+tt.taskID+"/fail", bytes.NewReader(body))
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

			if tt.validateServerTask != nil {
				tt.validateServerTask(t, failRepo, 1)
			}
		})
	}
}

func TestHandler_ResponseStructure(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	taskRepo := inmemory.NewServerTaskRepository(serverRepo)
	failRepo := inmemory.NewServerTaskFailRepository()
	responder := api.NewResponder()

	handler := NewHandler(taskRepo, failRepo, serverRepo, responder)

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
	task := &domain.ServerTask{
		Command:      domain.ServerTaskCommandStop,
		ServerID:     10,
		Repeat:       0,
		RepeatPeriod: 600 * time.Second,
		Counter:      1396,
		ExecuteDate:  executeDate,
		CreatedAt:    &now,
		UpdatedAt:    &now,
	}
	require.NoError(t, taskRepo.Save(context.Background(), task))

	daemonSession := &auth.DaemonSession{
		Node: node,
	}
	ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

	requestBody := map[string]any{
		"output": "Server failed to stop: process not responding",
	}
	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/servers_tasks/1/fail", bytes.NewReader(body))
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"server_task": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response failServerTaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, "success", response.Message)

	fails, err := failRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, fails, 1)
	assert.Equal(t, uint(1), fails[0].ServerTaskID)
	assert.Equal(t, "Server failed to stop: process not responding", fails[0].Output)
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	taskRepo := inmemory.NewServerTaskRepository(serverRepo)
	failRepo := inmemory.NewServerTaskFailRepository()
	responder := api.NewResponder()

	handler := NewHandler(taskRepo, failRepo, serverRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, taskRepo, handler.serverTaskRepo)
	assert.Equal(t, failRepo, handler.serverTaskFailRepo)
	assert.Equal(t, serverRepo, handler.serverRepo)
	assert.Equal(t, responder, handler.responder)
}
