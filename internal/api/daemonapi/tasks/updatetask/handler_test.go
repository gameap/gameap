package updatetask

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
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupContext   func(*inmemory.DaemonTaskRepository) context.Context
		taskID         string
		requestBody    any
		expectedStatus int
		wantError      string
		validateTask   func(*testing.T, *domain.DaemonTask)
	}{
		{
			name: "successful update to working status",
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
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
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
			taskID: "1",
			requestBody: map[string]any{
				"status": 2,
			},
			expectedStatus: http.StatusOK,
			validateTask: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				assert.Equal(t, domain.DaemonTaskStatusWorking, task.Status)
			},
		},
		{
			name: "successful update to success status",
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
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusWorking,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			taskID: "1",
			requestBody: map[string]any{
				"status": 4,
			},
			expectedStatus: http.StatusOK,
			validateTask: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				assert.Equal(t, domain.DaemonTaskStatusSuccess, task.Status)
			},
		},
		{
			name: "successful update to error status",
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
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusWorking,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(context.Background(), task))

				daemonSession := &auth.DaemonSession{
					Node: node,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			taskID: "1",
			requestBody: map[string]any{
				"status": 3,
			},
			expectedStatus: http.StatusOK,
			validateTask: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				assert.Equal(t, domain.DaemonTaskStatusError, task.Status)
			},
		},
		{
			name: "successful update to canceled status",
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
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
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
			taskID: "1",
			requestBody: map[string]any{
				"status": 5,
			},
			expectedStatus: http.StatusOK,
			validateTask: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				assert.Equal(t, domain.DaemonTaskStatusCanceled, task.Status)
			},
		},
		{
			name: "daemon task not found",
			setupContext: func(_ *inmemory.DaemonTaskRepository) context.Context {
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
				"status": 2,
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "daemon task not found",
		},
		{
			name: "daemon task belongs to different node",
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
					ID:                1,
					DedicatedServerID: 2,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
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
			taskID: "1",
			requestBody: map[string]any{
				"status": 2,
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "daemon task not found",
		},
		{
			name: "invalid task ID",
			setupContext: func(_ *inmemory.DaemonTaskRepository) context.Context {
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
				"status": 2,
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid task ID",
		},
		{
			name: "daemon session not found",
			setupContext: func(_ *inmemory.DaemonTaskRepository) context.Context {
				return context.Background()
			},
			taskID: "1",
			requestBody: map[string]any{
				"status": 2,
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
		},
		{
			name: "daemon session with nil node",
			setupContext: func(_ *inmemory.DaemonTaskRepository) context.Context {
				daemonSession := &auth.DaemonSession{
					Node: nil,
				}

				return auth.ContextWithDaemonSession(context.Background(), daemonSession)
			},
			taskID: "1",
			requestBody: map[string]any{
				"status": 2,
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "daemon session not found",
		},
		{
			name: "invalid JSON body",
			setupContext: func(_ *inmemory.DaemonTaskRepository) context.Context {
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
			name: "empty status",
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
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
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
			taskID:         "1",
			requestBody:    map[string]any{},
			expectedStatus: http.StatusBadRequest,
			wantError:      "empty status",
		},
		{
			name: "invalid status value",
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
					ID:                1,
					DedicatedServerID: 1,
					ServerID:          &serverID,
					Task:              domain.DaemonTaskTypeServerStart,
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
			taskID: "1",
			requestBody: map[string]any{
				"status": 99,
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskRepo := inmemory.NewDaemonTaskRepository()
			responder := api.NewResponder()

			handler := NewHandler(
				taskRepo,
				responder,
			)

			ctx := tt.setupContext(taskRepo)

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest(http.MethodPut, "/gdaemon_api/tasks/"+tt.taskID, bytes.NewReader(body))
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"gdaemon_task": tt.taskID})
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
				var response updateTaskResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "success", response.Message)
			}

			if tt.validateTask != nil {
				tasks, err := taskRepo.FindAll(ctx, nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)
				tt.validateTask(t, &tasks[0])
			}
		})
	}
}

func TestHandler_ResponseStructure(t *testing.T) {
	taskRepo := inmemory.NewDaemonTaskRepository()
	responder := api.NewResponder()

	handler := NewHandler(
		taskRepo,
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

	serverID := uint(10)
	task := &domain.DaemonTask{
		ID:                1,
		DedicatedServerID: 1,
		ServerID:          &serverID,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	require.NoError(t, taskRepo.Save(context.Background(), task))

	daemonSession := &auth.DaemonSession{
		Node: node,
	}
	ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

	requestBody := map[string]any{
		"status": 2,
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/gdaemon_api/tasks/1", bytes.NewReader(body))
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"gdaemon_task": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response updateTaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.Equal(t, "success", response.Message)
}

func TestHandler_NewHandler(t *testing.T) {
	taskRepo := inmemory.NewDaemonTaskRepository()
	responder := api.NewResponder()

	handler := NewHandler(
		taskRepo,
		responder,
	)

	require.NotNil(t, handler)
	assert.Equal(t, taskRepo, handler.daemonTaskRepo)
	assert.Equal(t, responder, handler.responder)
}

func TestUpdateTaskInput_Validate(t *testing.T) {
	tests := []struct {
		name      string
		input     *updateTaskInput
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid status waiting",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(1)),
			},
			wantError: false,
		},
		{
			name: "valid status working",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(2)),
			},
			wantError: false,
		},
		{
			name: "valid status error",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(3)),
			},
			wantError: false,
		},
		{
			name: "valid status success",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(4)),
			},
			wantError: false,
		},
		{
			name: "valid status canceled",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(5)),
			},
			wantError: false,
		},
		{
			name: "empty status",
			input: &updateTaskInput{
				Status: nil,
			},
			wantError: true,
			errorMsg:  "empty status",
		},
		{
			name: "invalid status zero",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(0)),
			},
			wantError: true,
			errorMsg:  "invalid status",
		},
		{
			name: "invalid status negative",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(-1)),
			},
			wantError: true,
			errorMsg:  "invalid status",
		},
		{
			name: "invalid status too large",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(99)),
			},
			wantError: true,
			errorMsg:  "invalid status",
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

func TestUpdateTaskInput_ToStatus(t *testing.T) {
	tests := []struct {
		name       string
		input      *updateTaskInput
		wantStatus domain.DaemonTaskStatus
	}{
		{
			name: "status 1 maps to waiting",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(1)),
			},
			wantStatus: domain.DaemonTaskStatusWaiting,
		},
		{
			name: "status 2 maps to working",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(2)),
			},
			wantStatus: domain.DaemonTaskStatusWorking,
		},
		{
			name: "status 3 maps to error",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(3)),
			},
			wantStatus: domain.DaemonTaskStatusError,
		},
		{
			name: "status 4 maps to success",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(4)),
			},
			wantStatus: domain.DaemonTaskStatusSuccess,
		},
		{
			name: "status 5 maps to canceled",
			input: &updateTaskInput{
				Status: lo.ToPtr(flexible.Int(5)),
			},
			wantStatus: domain.DaemonTaskStatusCanceled,
		},
		{
			name: "nil status returns empty string",
			input: &updateTaskInput{
				Status: nil,
			},
			wantStatus: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.input.ToStatus()
			assert.Equal(t, tt.wantStatus, status)
		})
	}
}
