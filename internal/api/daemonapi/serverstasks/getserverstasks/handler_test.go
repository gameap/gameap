package getserverstasks

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func(*inmemory.ServerTaskRepository, *inmemory.ServerRepository) context.Context
		expectedStatus int
		wantError      string
		expectTasks    int
	}{
		{
			name: "successful tasks retrieval for node",
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
			expectedStatus: http.StatusOK,
			expectTasks:    1,
		},
		{
			name: "multiple tasks for same node",
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

				server1 := &domain.Server{
					ID:         10,
					Enabled:    true,
					Installed:  domain.ServerInstalledStatusInstalled,
					Name:       "Test Server 1",
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
				require.NoError(t, serverRepo.Save(context.Background(), server1))

				server2 := &domain.Server{
					ID:         11,
					Enabled:    true,
					Installed:  domain.ServerInstalledStatusInstalled,
					Name:       "Test Server 2",
					UUID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
					UUIDShort:  "550e8401",
					GameID:     "rust",
					DSID:       1,
					ServerIP:   "172.18.0.5",
					ServerPort: 27025,
					Dir:        "/srv/gameap/servers/server2",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server2))

				executeDate := now.Add(24 * time.Hour)
				task1 := &domain.ServerTask{
					Command:      domain.ServerTaskCommandStart,
					ServerID:     10,
					Repeat:       0,
					RepeatPeriod: 3600 * time.Second,
					Counter:      0,
					ExecuteDate:  executeDate,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task1))

				task2 := &domain.ServerTask{
					Command:      domain.ServerTaskCommandStop,
					ServerID:     11,
					Repeat:       0,
					RepeatPeriod: 600 * time.Second,
					Counter:      5,
					ExecuteDate:  executeDate,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task2))

				task3 := &domain.ServerTask{
					Command:      domain.ServerTaskCommandReinstall,
					ServerID:     10,
					Repeat:       1,
					RepeatPeriod: 7200 * time.Second,
					Counter:      10,
					ExecuteDate:  executeDate,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task3))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			expectedStatus: http.StatusOK,
			expectTasks:    3,
		},
		{
			name: "only returns tasks for authenticated node",
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

				server1 := &domain.Server{
					ID:         10,
					Enabled:    true,
					Installed:  domain.ServerInstalledStatusInstalled,
					Name:       "Test Server 1",
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
				require.NoError(t, serverRepo.Save(context.Background(), server1))

				server2 := &domain.Server{
					ID:         11,
					Enabled:    true,
					Installed:  domain.ServerInstalledStatusInstalled,
					Name:       "Test Server 2",
					UUID:       uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"),
					UUIDShort:  "550e8401",
					GameID:     "rust",
					DSID:       2,
					ServerIP:   "172.18.0.6",
					ServerPort: 27015,
					Dir:        "/srv/gameap/servers/server2",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server2))

				executeDate := now.Add(24 * time.Hour)
				task1 := &domain.ServerTask{
					Command:      domain.ServerTaskCommandStart,
					ServerID:     10,
					Repeat:       0,
					RepeatPeriod: 3600 * time.Second,
					Counter:      0,
					ExecuteDate:  executeDate,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task1))

				task2 := &domain.ServerTask{
					Command:      domain.ServerTaskCommandStop,
					ServerID:     11,
					Repeat:       0,
					RepeatPeriod: 600 * time.Second,
					Counter:      0,
					ExecuteDate:  executeDate,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task2))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			expectedStatus: http.StatusOK,
			expectTasks:    1,
		},
		{
			name: "returns empty array when no tasks",
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
			expectedStatus: http.StatusOK,
			expectTasks:    0,
		},
		{
			name: "daemon session not found",
			setupContext: func(_ *inmemory.ServerTaskRepository, _ *inmemory.ServerRepository) context.Context {
				return context.Background()
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
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			taskRepo := inmemory.NewServerTaskRepository(serverRepo)
			responder := api.NewResponder()

			handler := NewHandler(taskRepo, serverRepo, responder)

			ctx := tt.setupContext(taskRepo, serverRepo)

			req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/servers_tasks", nil)
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

			if tt.expectedStatus == http.StatusOK {
				var response []ServerTaskResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Len(t, response, tt.expectTasks)
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

	executeDate := time.Date(2029, 9, 21, 18, 10, 11, 0, time.UTC)
	payload := "{\"test\": \"data\"}"
	task := &domain.ServerTask{
		Command:      domain.ServerTaskCommandReinstall,
		ServerID:     10,
		Repeat:       0,
		RepeatPeriod: 3600 * time.Second,
		Counter:      0,
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

	req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/servers_tasks", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []ServerTaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 1)

	taskResp := response[0]
	assert.Equal(t, task.ID, taskResp.ID)
	assert.Equal(t, "reinstall", taskResp.Command)
	assert.Equal(t, uint(10), taskResp.ServerID)
	assert.Equal(t, uint8(0), taskResp.Repeat)
	assert.Equal(t, 3600, taskResp.RepeatPeriod)
	assert.Equal(t, uint(0), taskResp.Counter)
	assert.Equal(t, "2029-09-21 18:10:11", taskResp.ExecuteDate)
	require.NotNil(t, taskResp.Payload)
	assert.Equal(t, payload, *taskResp.Payload)
	assert.NotEmpty(t, taskResp.CreatedAt)
	assert.NotEmpty(t, taskResp.UpdatedAt)
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
