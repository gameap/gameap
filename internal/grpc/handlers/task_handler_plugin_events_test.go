package handlers

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePluginTaskEvents struct {
	mu       sync.Mutex
	events   []pluginproto.EventType
	statuses []string
	taskIDs  []uint
}

func (f *fakePluginTaskEvents) DispatchTaskEventAsync(
	_ context.Context,
	eventType pluginproto.EventType,
	taskID, _ uint,
	_ *uint,
	_, status string,
	_ map[string]string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.events = append(f.events, eventType)
	f.statuses = append(f.statuses, status)
	f.taskIDs = append(f.taskIDs, taskID)
}

func TestHandleTaskStatusUpdate_PluginTaskEvents(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		updateStatus proto.DaemonTaskStatus
		wantEvents   []pluginproto.EventType
		wantStatuses []string
	}{
		{
			name:         "success_dispatches_completed",
			updateStatus: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
			wantEvents:   []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_COMPLETED},
			wantStatuses: []string{string(domain.DaemonTaskStatusSuccess)},
		},
		{
			name:         "error_dispatches_task_failure_event",
			updateStatus: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR,
			wantEvents:   []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_FAILED},
			wantStatuses: []string{string(domain.DaemonTaskStatusError)},
		},
		{
			name:         "working_dispatches_nothing",
			updateStatus: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
			wantEvents:   nil,
			wantStatuses: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &domain.DaemonTask{
				DedicatedServerID: 1,
				Task:              domain.DaemonTaskTypeServerStart,
				Status:            domain.DaemonTaskStatusWorking,
				CreatedAt:         &now,
				UpdatedAt:         &now,
			}
			repo := setupDaemonTaskRepo(t, task)
			pluginEvents := &fakePluginTaskEvents{}
			handler := NewTaskHandler(repo, nil, nil, pluginEvents, slog.Default())

			err := handler.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
				TaskId: uint64(task.ID),
				Status: tt.updateStatus,
			})

			require.NoError(t, err)
			assert.Equal(t, tt.wantEvents, pluginEvents.events)
			assert.Equal(t, tt.wantStatuses, pluginEvents.statuses)
		})
	}
}

func TestHandleTaskStatusUpdate_duplicate_terminal_update_dispatches_once(t *testing.T) {
	now := time.Now()
	task := &domain.DaemonTask{
		DedicatedServerID: 1,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWorking,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	repo := setupDaemonTaskRepo(t, task)
	pluginEvents := &fakePluginTaskEvents{}
	handler := NewTaskHandler(repo, nil, nil, pluginEvents, slog.Default())

	update := &proto.TaskStatusUpdate{
		TaskId: uint64(task.ID),
		Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
	}

	require.NoError(t, handler.HandleTaskStatusUpdate(context.Background(), 1, update))
	require.NoError(t, handler.HandleTaskStatusUpdate(context.Background(), 1, update))

	assert.Equal(t,
		[]pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_COMPLETED},
		pluginEvents.events,
		"a re-delivered terminal update must not emit a duplicate plugin event")
}

func TestHandleTaskStatusUpdate_concurrent_duplicate_terminal_updates_dispatch_once(t *testing.T) {
	now := time.Now()
	task := &domain.DaemonTask{
		DedicatedServerID: 1,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWorking,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	repo := setupDaemonTaskRepo(t, task)
	pluginEvents := &fakePluginTaskEvents{}
	handler := NewTaskHandler(repo, nil, nil, pluginEvents, slog.Default())

	const workers = 8
	start := make(chan struct{})
	errs := make(chan error, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			<-start

			errs <- handler.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
				TaskId: uint64(task.ID),
				Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
			})
		}()
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t,
		[]pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_COMPLETED},
		pluginEvents.events,
		"concurrent duplicate terminal updates must emit exactly one plugin event")
}

func TestHandleTaskStatusUpdate_terminal_transition_between_statuses_dispatches(t *testing.T) {
	now := time.Now()
	task := &domain.DaemonTask{
		DedicatedServerID: 1,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWorking,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	repo := setupDaemonTaskRepo(t, task)
	pluginEvents := &fakePluginTaskEvents{}
	handler := NewTaskHandler(repo, nil, nil, pluginEvents, slog.Default())

	require.NoError(t, handler.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
		TaskId: uint64(task.ID),
		Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
	}))
	require.NoError(t, handler.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
		TaskId: uint64(task.ID),
		Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR,
	}))

	assert.Equal(t,
		[]pluginproto.EventType{
			pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_COMPLETED,
			pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_FAILED,
		},
		pluginEvents.events,
		"a genuine transition between terminal statuses is a new event")
}

func TestReconcileWorkingTasks_dispatches_task_failure_event(t *testing.T) {
	now := time.Now()
	task := &domain.DaemonTask{
		DedicatedServerID: 3,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWorking,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
	repo := setupDaemonTaskRepo(t, task)
	pluginEvents := &fakePluginTaskEvents{}
	handler := NewTaskHandler(repo, nil, nil, pluginEvents, slog.Default())

	marked, err := handler.ReconcileWorkingTasks(context.Background(), 3, nil, "daemon_restart")

	require.NoError(t, err)
	require.Equal(t, 1, marked)
	require.Equal(t, []pluginproto.EventType{pluginproto.EventType_EVENT_TYPE_DAEMON_TASK_FAILED}, pluginEvents.events)
	assert.Equal(t, []string{string(domain.DaemonTaskStatusError)}, pluginEvents.statuses)
	assert.Equal(t, []uint{task.ID}, pluginEvents.taskIDs)
}
