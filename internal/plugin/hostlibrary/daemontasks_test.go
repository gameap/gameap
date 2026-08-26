package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/common"
	"github.com/gameap/gameap/pkg/plugin/sdk/daemontasks"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonTasksService_FindDaemonTasks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setupRepo func(*inmemory.DaemonTaskRepository)
		request   *daemontasks.FindDaemonTasksRequest
		wantTotal int
		wantIDs   []uint
	}{
		{
			name: "no_filter_returns_all",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				_ = r.Save(context.Background(), &domain.DaemonTask{
					DedicatedServerID: 1,
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusWaiting,
				})
				_ = r.Save(context.Background(), &domain.DaemonTask{
					DedicatedServerID: 1,
					Task:              domain.DaemonTaskTypeServerStop,
					Status:            domain.DaemonTaskStatusSuccess,
				})
				_ = r.Save(context.Background(), &domain.DaemonTask{
					DedicatedServerID: 2,
					Task:              domain.DaemonTaskTypeServerRestart,
					Status:            domain.DaemonTaskStatusWorking,
				})
			},
			request:   &daemontasks.FindDaemonTasksRequest{},
			wantTotal: 3,
			wantIDs:   []uint{1, 2, 3},
		},
		{
			name: "filter_by_ids",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 1, Task: domain.DaemonTaskTypeServerStart, Status: domain.DaemonTaskStatusWaiting})
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 1, Task: domain.DaemonTaskTypeServerStop, Status: domain.DaemonTaskStatusWaiting})
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 2, Task: domain.DaemonTaskTypeServerRestart, Status: domain.DaemonTaskStatusWaiting})
			},
			request: &daemontasks.FindDaemonTasksRequest{
				Filter: &daemontasks.DaemonTaskFilter{Ids: []uint64{1, 3}},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 3},
		},
		{
			name: "filter_by_node_ids",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 1, Task: domain.DaemonTaskTypeServerStart, Status: domain.DaemonTaskStatusWaiting})
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 1, Task: domain.DaemonTaskTypeServerStop, Status: domain.DaemonTaskStatusWaiting})
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 2, Task: domain.DaemonTaskTypeServerRestart, Status: domain.DaemonTaskStatusWaiting})
			},
			request: &daemontasks.FindDaemonTasksRequest{
				Filter: &daemontasks.DaemonTaskFilter{NodeIds: []uint64{1}},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 2},
		},
		{
			name: "filter_by_server_ids",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				_ = r.Save(context.Background(), &domain.DaemonTask{
					DedicatedServerID: 1,
					ServerID:          new(uint(10)),
					Task:              domain.DaemonTaskTypeServerStart,
					Status:            domain.DaemonTaskStatusWaiting,
				})
				_ = r.Save(context.Background(), &domain.DaemonTask{
					DedicatedServerID: 1,
					ServerID:          new(uint(20)),
					Task:              domain.DaemonTaskTypeServerStop,
					Status:            domain.DaemonTaskStatusWaiting,
				})
				_ = r.Save(context.Background(), &domain.DaemonTask{
					DedicatedServerID: 2,
					ServerID:          new(uint(10)),
					Task:              domain.DaemonTaskTypeServerRestart,
					Status:            domain.DaemonTaskStatusWaiting,
				})
			},
			request: &daemontasks.FindDaemonTasksRequest{
				Filter: &daemontasks.DaemonTaskFilter{ServerIds: []uint64{10}},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 3},
		},
		{
			name: "filter_by_statuses",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 1, Task: domain.DaemonTaskTypeServerStart, Status: domain.DaemonTaskStatusWaiting})
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 1, Task: domain.DaemonTaskTypeServerStop, Status: domain.DaemonTaskStatusSuccess})
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 2, Task: domain.DaemonTaskTypeServerRestart, Status: domain.DaemonTaskStatusError})
			},
			request: &daemontasks.FindDaemonTasksRequest{
				Filter: &daemontasks.DaemonTaskFilter{
					Statuses: []proto.DaemonTaskStatus{
						proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WAITING,
						proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR,
					},
				},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 3},
		},
		{
			name: "filter_by_task_types",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 1, Task: domain.DaemonTaskTypeServerStart, Status: domain.DaemonTaskStatusWaiting})
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 1, Task: domain.DaemonTaskTypeServerStop, Status: domain.DaemonTaskStatusWaiting})
				_ = r.Save(context.Background(), &domain.DaemonTask{DedicatedServerID: 2, Task: domain.DaemonTaskTypeServerStart, Status: domain.DaemonTaskStatusWaiting})
			},
			request: &daemontasks.FindDaemonTasksRequest{
				Filter: &daemontasks.DaemonTaskFilter{
					TaskTypes: []proto.DaemonTaskType{proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START},
				},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 3},
		},
		{
			name: "pagination_applied",
			setupRepo: func(r *inmemory.DaemonTaskRepository) {
				for i := 1; i <= 10; i++ {
					_ = r.Save(context.Background(), &domain.DaemonTask{
						DedicatedServerID: 1,
						Task:              domain.DaemonTaskTypeServerStart,
						Status:            domain.DaemonTaskStatusWaiting,
					})
				}
			},
			request: &daemontasks.FindDaemonTasksRequest{
				Pagination: &common.Pagination{Limit: 3, Offset: 2},
			},
			wantTotal: 3,
			wantIDs:   []uint{3, 4, 5},
		},
		{
			name:      "empty_repository_returns_empty",
			setupRepo: func(_ *inmemory.DaemonTaskRepository) {},
			request:   &daemontasks.FindDaemonTasksRequest{},
			wantTotal: 0,
			wantIDs:   []uint{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewDaemonTaskRepository()
			tt.setupRepo(repo)

			svc := NewDaemonTasksService(repo, nil, allowAllGuard(testPluginID))
			resp, err := svc.FindDaemonTasks(context.Background(), tt.request)

			require.NoError(t, err)
			assert.Equal(t, int32(tt.wantTotal), resp.Total)
			require.Len(t, resp.Tasks, tt.wantTotal)

			for i, wantID := range tt.wantIDs {
				assert.Equal(t, uint64(wantID), resp.Tasks[i].Id)
			}
		})
	}
}

func TestDaemonTasksService_CreateDaemonTask(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		request     *daemontasks.CreateDaemonTaskRequest
		wantError   string
		wantSuccess bool
	}{
		{
			name: "invalid_task_type_returns_error",
			request: &daemontasks.CreateDaemonTaskRequest{
				NodeId:   1,
				TaskType: proto.DaemonTaskType_DAEMON_TASK_TYPE_UNSPECIFIED,
			},
			wantError:   "invalid task type",
			wantSuccess: false,
		},
		{
			name: "valid_task_created",
			request: &daemontasks.CreateDaemonTaskRequest{
				NodeId:   1,
				ServerId: new(uint64(10)),
				TaskType: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START,
			},
			wantSuccess: true,
		},
		{
			name: "task_with_run_after",
			request: &daemontasks.CreateDaemonTaskRequest{
				NodeId:     1,
				ServerId:   new(uint64(10)),
				TaskType:   proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_STOP,
				RunAfterId: new(uint64(5)),
			},
			wantSuccess: true,
		},
		{
			name: "task_with_cmd",
			request: &daemontasks.CreateDaemonTaskRequest{
				NodeId:   1,
				TaskType: proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC,
				Cmd:      new("echo hello"),
			},
			wantSuccess: true,
		},
		{
			name: "server_install_task",
			request: &daemontasks.CreateDaemonTaskRequest{
				NodeId:   1,
				ServerId: new(uint64(5)),
				TaskType: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_INSTALL,
			},
			wantSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewDaemonTaskRepository()
			svc := NewDaemonTasksService(repo, nil, allowAllGuard(testPluginID))

			resp, err := svc.CreateDaemonTask(context.Background(), tt.request)

			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.Nil(t, resp.Error)
			assert.Greater(t, resp.TaskId, uint64(0))
		})
	}
}

type fakeTaskDispatcher struct {
	dispatched []*domain.DaemonTask
	err        error
}

func (f *fakeTaskDispatcher) Dispatch(_ context.Context, task *domain.DaemonTask) error {
	if f.err != nil {
		return f.err
	}

	if task.ID == 0 {
		task.ID = uint(len(f.dispatched) + 1)
	}
	f.dispatched = append(f.dispatched, task)

	return nil
}

func TestDaemonTasksService_CreateDaemonTask_Dispatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		dispatcher     *fakeTaskDispatcher
		request        *daemontasks.CreateDaemonTaskRequest
		wantError      string
		wantSuccess    bool
		wantDispatched int
		wantSaved      int
		wantTaskFields func(*testing.T, *domain.DaemonTask)
	}{
		{
			name:       "dispatcher_called_when_provided",
			dispatcher: &fakeTaskDispatcher{},
			request: &daemontasks.CreateDaemonTaskRequest{
				NodeId:   8,
				TaskType: proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC,
				Cmd:      new("echo hello"),
			},
			wantSuccess:    true,
			wantDispatched: 1,
			wantSaved:      0,
			wantTaskFields: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				assert.Equal(t, uint(8), task.DedicatedServerID)
				assert.Equal(t, domain.DaemonTaskTypeCmdExec, task.Task)
				assert.Equal(t, domain.DaemonTaskStatusWaiting, task.Status)
				require.NotNil(t, task.Cmd)
				assert.Equal(t, "echo hello", *task.Cmd)
				assert.Nil(t, task.ServerID)
			},
		},
		{
			name:       "dispatcher_passes_server_id_when_set",
			dispatcher: &fakeTaskDispatcher{},
			request: &daemontasks.CreateDaemonTaskRequest{
				NodeId:   1,
				ServerId: new(uint64(42)),
				TaskType: proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START,
			},
			wantSuccess:    true,
			wantDispatched: 1,
			wantTaskFields: func(t *testing.T, task *domain.DaemonTask) {
				t.Helper()
				require.NotNil(t, task.ServerID)
				assert.Equal(t, uint(42), *task.ServerID)
			},
		},
		{
			name:       "dispatcher_error_returns_error_response",
			dispatcher: &fakeTaskDispatcher{err: errors.New("registry unavailable")},
			request: &daemontasks.CreateDaemonTaskRequest{
				NodeId:   1,
				TaskType: proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC,
				Cmd:      new("ls"),
			},
			wantError:      "registry unavailable",
			wantSuccess:    false,
			wantDispatched: 0,
			wantSaved:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewDaemonTaskRepository()
			svc := NewDaemonTasksService(repo, tt.dispatcher, allowAllGuard(testPluginID))

			resp, err := svc.CreateDaemonTask(context.Background(), tt.request)

			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
			} else {
				assert.Nil(t, resp.Error)
			}

			require.Len(t, tt.dispatcher.dispatched, tt.wantDispatched)

			saved, err := repo.Find(context.Background(), nil, nil, nil)
			require.NoError(t, err)
			assert.Len(t, saved, tt.wantSaved, "repo.Save should not be called on dispatcher path")

			if tt.wantTaskFields != nil && tt.wantDispatched > 0 {
				tt.wantTaskFields(t, tt.dispatcher.dispatched[0])
			}
		})
	}
}

func TestStatusConversion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		protoStatus proto.DaemonTaskStatus
		wantDomain  domain.DaemonTaskStatus
	}{
		{
			name:        "waiting_status",
			protoStatus: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WAITING,
			wantDomain:  domain.DaemonTaskStatusWaiting,
		},
		{
			name:        "working_status",
			protoStatus: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
			wantDomain:  domain.DaemonTaskStatusWorking,
		},
		{
			name:        "error_status",
			protoStatus: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR,
			wantDomain:  domain.DaemonTaskStatusError,
		},
		{
			name:        "success_status",
			protoStatus: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
			wantDomain:  domain.DaemonTaskStatusSuccess,
		},
		{
			name:        "canceled_status",
			protoStatus: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_CANCELED,
			wantDomain:  domain.DaemonTaskStatusCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := convertProtoStatusesToDomain([]proto.DaemonTaskStatus{tt.protoStatus})
			require.Len(t, result, 1)
			assert.Equal(t, tt.wantDomain, result[0])
		})
	}
}

func TestTypeConversion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		protoType  proto.DaemonTaskType
		wantDomain domain.DaemonTaskType
	}{
		{
			name:       "server_start",
			protoType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START,
			wantDomain: domain.DaemonTaskTypeServerStart,
		},
		{
			name:       "server_stop",
			protoType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_STOP,
			wantDomain: domain.DaemonTaskTypeServerStop,
		},
		{
			name:       "server_restart",
			protoType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_RESTART,
			wantDomain: domain.DaemonTaskTypeServerRestart,
		},
		{
			name:       "server_update",
			protoType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_UPDATE,
			wantDomain: domain.DaemonTaskTypeServerUpdate,
		},
		{
			name:       "server_install",
			protoType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_INSTALL,
			wantDomain: domain.DaemonTaskTypeServerInstall,
		},
		{
			name:       "server_delete",
			protoType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_DELETE,
			wantDomain: domain.DaemonTaskTypeServerDelete,
		},
		{
			name:       "server_move",
			protoType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_MOVE,
			wantDomain: domain.DaemonTaskTypeServerMove,
		},
		{
			name:       "cmd_exec",
			protoType:  proto.DaemonTaskType_DAEMON_TASK_TYPE_CMD_EXEC,
			wantDomain: domain.DaemonTaskTypeCmdExec,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := convertProtoTypesToDomain([]proto.DaemonTaskType{tt.protoType})
			require.Len(t, result, 1)
			assert.Equal(t, tt.wantDomain, result[0])
		})
	}
}

func TestConvertDaemonTaskToProto(t *testing.T) {
	t.Parallel()
	task := &domain.DaemonTask{
		ID:                1,
		DedicatedServerID: 10,
		ServerID:          new(uint(20)),
		RunAftID:          new(uint(5)),
		Task:              domain.DaemonTaskTypeServerStart,
		Cmd:               new("./start.sh"),
		Output:            new("Server started"),
		Status:            domain.DaemonTaskStatusSuccess,
	}

	result := convertDaemonTaskToProto(task)

	assert.Equal(t, uint64(1), result.Id)
	assert.Equal(t, uint64(10), result.NodeId)
	require.NotNil(t, result.ServerId)
	assert.Equal(t, uint64(20), *result.ServerId)
	require.NotNil(t, result.RunAfterId)
	assert.Equal(t, uint64(5), *result.RunAfterId)
	assert.Equal(t, proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START, result.TaskType)
	require.NotNil(t, result.Cmd)
	assert.Equal(t, "./start.sh", *result.Cmd)
	require.NotNil(t, result.Output)
	assert.Equal(t, "Server started", *result.Output)
	assert.Equal(t, proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS, result.Status)
}

func TestConvertDaemonTaskToProto_NilOptionalFields(t *testing.T) {
	t.Parallel()
	task := &domain.DaemonTask{
		ID:                1,
		DedicatedServerID: 10,
		Task:              domain.DaemonTaskTypeServerStop,
		Status:            domain.DaemonTaskStatusWaiting,
	}

	result := convertDaemonTaskToProto(task)

	assert.Equal(t, uint64(1), result.Id)
	assert.Nil(t, result.ServerId)
	assert.Nil(t, result.RunAfterId)
	assert.Nil(t, result.Cmd)
	assert.Nil(t, result.Output)
}

func TestNewDaemonTasksHostLibraryFactory(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewDaemonTaskRepository()
	factory := NewDaemonTasksHostLibraryFactory(repo, nil, NewGuard(stubPermissionChecker{allowed: true}))

	lib, ok := factory.Create(42).(*DaemonTasksHostLibrary)
	require.True(t, ok)
	require.NotNil(t, lib.impl)
	assert.Equal(t, uint64(42), lib.impl.guard.PluginID(), "factory must bind the plugin id")
}
