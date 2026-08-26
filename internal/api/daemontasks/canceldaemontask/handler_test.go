package canceldaemontask

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturingPublisher struct {
	mu       sync.Mutex
	messages []*pubsub.Message
	channels []string
}

func (p *capturingPublisher) Publish(_ context.Context, channel string, msg *pubsub.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.messages = append(p.messages, msg)
	p.channels = append(p.channels, channel)

	return nil
}

func newWaitingTask() *domain.DaemonTask {
	now := time.Now()
	serverID := uint(10)

	return &domain.DaemonTask{
		ID:                1,
		DedicatedServerID: 1,
		ServerID:          &serverID,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
}

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupRepo      func(*inmemory.DaemonTaskRepository)
		taskID         string
		expectedStatus int
		wantError      string
		wantPublished  bool
		validateTask   func(*testing.T, *domain.DaemonTask)
	}{
		{
			name: "successful_cancel_of_waiting_task",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				task := newWaitingTask()
				require.NoError(t, r.Save(context.Background(), task))
			},
			taskID:         "1",
			expectedStatus: http.StatusOK,
			wantPublished:  true,
			validateTask: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				assert.Equal(t, domain.DaemonTaskStatusCanceled, task.Status)
			},
		},
		{
			name: "task_in_working_status_returns_422",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				task := newWaitingTask()
				task.Status = domain.DaemonTaskStatusWorking
				require.NoError(t, r.Save(context.Background(), task))
			},
			taskID:         "1",
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "cancel_fail_cannot_be_canceled",
			validateTask: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				assert.Equal(t, domain.DaemonTaskStatusWorking, task.Status)
			},
		},
		{
			name: "task_in_canceled_status_returns_422",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				task := newWaitingTask()
				task.Status = domain.DaemonTaskStatusCanceled
				require.NoError(t, r.Save(context.Background(), task))
			},
			taskID:         "1",
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "cancel_fail_cannot_be_canceled",
		},
		{
			name: "task_in_success_status_returns_422",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				task := newWaitingTask()
				task.Status = domain.DaemonTaskStatusSuccess
				require.NoError(t, r.Save(context.Background(), task))
			},
			taskID:         "1",
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "cancel_fail_cannot_be_canceled",
		},
		{
			name:           "task_not_found_returns_404",
			setupRepo:      func(_ *inmemory.DaemonTaskRepository) {},
			taskID:         "999",
			expectedStatus: http.StatusNotFound,
			wantError:      "daemon task not found",
		},
		{
			name:           "invalid_task_id_returns_400",
			setupRepo:      func(_ *inmemory.DaemonTaskRepository) {},
			taskID:         "invalid",
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid task id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			taskRepo := inmemory.NewDaemonTaskRepository()
			responder := api.NewResponder()
			publisher := &capturingPublisher{}

			tt.setupRepo(taskRepo)

			handler := NewHandler(taskRepo, publisher, responder)

			req := httptest.NewRequest(http.MethodPost, "/api/gdaemon_tasks/"+tt.taskID+"/cancel", nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.taskID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			require.Equal(t, tt.expectedStatus, w.Code, "body: %s", w.Body.String())

			if tt.wantError != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.Equal(t, "error", resp["status"])
				errMsg, ok := resp["error"].(string)
				require.True(t, ok, "error field must be a string, got %T", resp["error"])
				assert.Contains(t, errMsg, tt.wantError)
			} else {
				var resp cancelDaemonTaskResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.Equal(t, "success", resp.Message)
			}

			if tt.validateTask != nil {
				tasks, err := taskRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)
				tt.validateTask(t, &tasks[0])
			}

			if tt.wantPublished {
				require.Len(t, publisher.messages, 2)
				assert.Equal(t, messages.TypeTaskStatus, publisher.messages[0].Type)
				assert.Equal(t, messages.TypeTaskComplete, publisher.messages[1].Type)

				expectedChannel := channels.BuildRealtimeTaskStatusChannel(1)
				assert.Equal(t, expectedChannel, publisher.channels[0])
				assert.Equal(t, expectedChannel, publisher.channels[1])
			} else {
				assert.Empty(t, publisher.messages)
			}
		})
	}
}

func TestHandler_ServeHTTP_WithoutPublisher(t *testing.T) {
	t.Parallel()

	taskRepo := inmemory.NewDaemonTaskRepository()
	responder := api.NewResponder()

	require.NoError(t, taskRepo.Save(context.Background(), newWaitingTask()))

	handler := NewHandler(taskRepo, nil, responder)

	req := httptest.NewRequest(http.MethodPost, "/api/gdaemon_tasks/1/cancel", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp cancelDaemonTaskResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Message)

	tasks, err := taskRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, domain.DaemonTaskStatusCanceled, tasks[0].Status)
}

func TestHandler_NewHandler(t *testing.T) {
	t.Parallel()

	taskRepo := inmemory.NewDaemonTaskRepository()
	responder := api.NewResponder()
	publisher := &capturingPublisher{}

	handler := NewHandler(taskRepo, publisher, responder)

	require.NotNil(t, handler)
	assert.Equal(t, taskRepo, handler.daemonTaskRepo)
	assert.Equal(t, publisher, handler.publisher)
	assert.Equal(t, responder, handler.responder)
}
