package gettask

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func(*inmemory.DaemonTaskRepository) context.Context
		setupQuery     string
		expectedStatus int
		wantError      string
		expectTasks    int
	}{
		{
			name: "successful tasks retrieval",
			setupContext: func(taskRepo *inmemory.DaemonTaskRepository) context.Context {
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

				serverID := uint(10)
				task := &domain.DaemonTask{
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
					Data:              strPtr("{}"),
					Cmd:               strPtr("./start.sh"),
					Status:            domain.DaemonTaskStatusWaiting,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			setupQuery:     "",
			expectedStatus: http.StatusOK,
			expectTasks:    1,
		},
		{
			name: "filter by status waiting",
			setupContext: func(taskRepo *inmemory.DaemonTaskRepository) context.Context {
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

				serverID := uint(10)
				task1 := &domain.DaemonTask{
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusWaiting,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task1))

				task2 := &domain.DaemonTask{
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStop,
					Status:            domain.DaemonTaskStatusWorking,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task2))

				task3 := &domain.DaemonTask{
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerRestart,
					Status:            domain.DaemonTaskStatusSuccess,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task3))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			setupQuery:     "?filter[status]=waiting",
			expectedStatus: http.StatusOK,
			expectTasks:    1,
		},
		{
			name: "filter by status working",
			setupContext: func(taskRepo *inmemory.DaemonTaskRepository) context.Context {
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

				serverID := uint(10)
				task1 := &domain.DaemonTask{
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusWaiting,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task1))

				task2 := &domain.DaemonTask{
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStop,
					Status:            domain.DaemonTaskStatusWorking,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task2))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			setupQuery:     "?filter[status]=working",
			expectedStatus: http.StatusOK,
			expectTasks:    1,
		},
		{
			name: "only returns tasks for node",
			setupContext: func(taskRepo *inmemory.DaemonTaskRepository) context.Context {
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

				serverID := uint(10)
				task1 := &domain.DaemonTask{
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusWaiting,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task1))

				task2 := &domain.DaemonTask{
					DedicatedServerID: 2,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStop,
					Status:            domain.DaemonTaskStatusWaiting,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task2))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			setupQuery:     "",
			expectedStatus: http.StatusOK,
			expectTasks:    1,
		},
		{
			name: "daemon session not found",
			setupContext: func(_ *inmemory.DaemonTaskRepository) context.Context {
				return context.Background()
			},
			setupQuery:     "",
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
			expectTasks:    0,
		},
		{
			name: "daemon session with nil node",
			setupContext: func(_ *inmemory.DaemonTaskRepository) context.Context {
				daemonSession := &auth.DaemonSession{
					Node: nil,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			setupQuery:     "",
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
			expectTasks:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskRepo := inmemory.NewDaemonTaskRepository()
			responder := api.NewResponder()

			handler := NewHandler(taskRepo, responder)

			ctx := tt.setupContext(taskRepo)

			req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/tasks"+tt.setupQuery, nil)
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

			if tt.expectTasks > 0 {
				var response []TaskResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Len(t, response, tt.expectTasks)
			}
		})
	}
}

func TestHandler_ResponseStructure(t *testing.T) {
	taskRepo := inmemory.NewDaemonTaskRepository()
	responder := api.NewResponder()

	handler := NewHandler(taskRepo, responder)

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

	runAftID := uint(5)
	serverID := uint(10)
	data := "{\"test\": \"data\"}"
	cmd := "./start.sh"
	task := &domain.DaemonTask{
		RunAftID:          &runAftID,
		DedicatedServerID: 1,
		ServerID:          &serverID,
		Task:              domain.DaemonTaskTypeServerStart,
		Data:              &data,
		Cmd:               &cmd,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	require.NoError(t, taskRepo.Save(context.Background(), task))

	daemonSession := &auth.DaemonSession{
		Node: node,
	}
	ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

	req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/tasks", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response []TaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response, 1)

	taskResp := response[0]
	assert.Equal(t, task.ID, taskResp.ID)
	require.NotNil(t, taskResp.RunAftID)
	assert.Equal(t, runAftID, *taskResp.RunAftID)
	assert.Equal(t, uint(1), taskResp.DedicatedServerID)
	require.NotNil(t, taskResp.ServerID)
	assert.Equal(t, serverID, *taskResp.ServerID)
	assert.Equal(t, "gsstart", taskResp.Task)
	assert.Equal(t, data, taskResp.Data)
	assert.Equal(t, cmd, taskResp.Cmd)
	assert.Equal(t, "waiting", taskResp.Status)
	assert.Equal(t, 1, taskResp.StatusNum)
	assert.NotEmpty(t, taskResp.CreatedAt)
	assert.NotEmpty(t, taskResp.UpdatedAt)
}

func TestHandler_StatusNumMapping(t *testing.T) {
	tests := []struct {
		status       domain.DaemonTaskStatus
		expectedNum  int
		expectedName string
	}{
		{domain.DaemonTaskStatusWaiting, 1, "waiting"},
		{domain.DaemonTaskStatusWorking, 2, "working"},
		{domain.DaemonTaskStatusError, 3, "error"},
		{domain.DaemonTaskStatusSuccess, 4, "success"},
		{domain.DaemonTaskStatusCanceled, 5, "canceled"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			taskRepo := inmemory.NewDaemonTaskRepository()
			responder := api.NewResponder()

			handler := NewHandler(taskRepo, responder)

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

			serverID := uint(10)
			task := &domain.DaemonTask{
				DedicatedServerID: 1,
				ServerID:          &serverID,
				Task:              domain.DaemonTaskTypeServerStart,
				Status:            tt.status,
				CreatedAt:         &now,
				UpdatedAt:         &now,
			}
			require.NoError(t, taskRepo.Save(context.Background(), task))

			daemonSession := &auth.DaemonSession{
				Node: node,
			}
			ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

			req := httptest.NewRequest(http.MethodGet, "/gdaemon_api/tasks", nil)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var response []TaskResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.Len(t, response, 1)

			assert.Equal(t, tt.expectedNum, response[0].StatusNum)
			assert.Equal(t, tt.expectedName, response[0].Status)
		})
	}
}

func TestHandler_NewHandler(t *testing.T) {
	taskRepo := inmemory.NewDaemonTaskRepository()
	responder := api.NewResponder()

	handler := NewHandler(taskRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, taskRepo, handler.daemonTaskRepo)
	assert.Equal(t, responder, handler.responder)
}

func strPtr(s string) *string {
	return &s
}
