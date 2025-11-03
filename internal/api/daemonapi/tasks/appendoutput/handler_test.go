package appendoutput

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
			name: "successful append output",
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
					Output:            lo.ToPtr("Previous output\n"),
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
				"output": "New output line\n",
			},
			expectedStatus: http.StatusOK,
			validateTask: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				assert.NotNil(t, task.Output)
				assert.Equal(t, "Previous output\nNew output line\n", *task.Output)
			},
		},
		{
			name: "successful append to empty output",
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
					Output:            nil,
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
				"output": "First output line\n",
			},
			expectedStatus: http.StatusOK,
			validateTask: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				assert.NotNil(t, task.Output)
				assert.Equal(t, "First output line\n", *task.Output)
			},
		},
		{
			name: "successful append with empty output",
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
					Output:            lo.ToPtr("Existing output"),
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
				"output": "",
			},
			expectedStatus: http.StatusOK,
			validateTask: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				assert.NotNil(t, task.Output)
				assert.Equal(t, "Existing output", *task.Output)
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
				"output": "Some output",
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
				"output": "Some output",
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
				"output": "Some output",
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
				"output": "Some output",
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
				"output": "Some output",
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

			req := httptest.NewRequest(http.MethodPut, "/gdaemon_api/tasks/"+tt.taskID+"/output", bytes.NewReader(body))
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
				var response appendOutputResponse
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
		Status:            domain.DaemonTaskStatusWorking,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	require.NoError(t, taskRepo.Save(context.Background(), task))

	daemonSession := &auth.DaemonSession{
		Node: node,
	}
	ctx := auth.ContextWithDaemonSession(context.Background(), daemonSession)

	requestBody := map[string]any{
		"output": "Test output",
	}

	body, err := json.Marshal(requestBody)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/gdaemon_api/tasks/1/output", bytes.NewReader(body))
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"gdaemon_task": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response appendOutputResponse
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

func TestAppendOutputInput_Validate(t *testing.T) {
	tests := []struct {
		name      string
		input     *appendOutputInput
		wantError bool
	}{
		{
			name: "valid input with output",
			input: &appendOutputInput{
				Output: "Some output",
			},
			wantError: false,
		},
		{
			name: "valid input with empty output",
			input: &appendOutputInput{
				Output: "",
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()

			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
