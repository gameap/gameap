package handlers

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDaemonTaskRepo(t *testing.T, tasks ...*domain.DaemonTask) *inmemory.DaemonTaskRepository {
	t.Helper()

	repo := inmemory.NewDaemonTaskRepository()
	for _, task := range tasks {
		require.NoError(t, repo.Save(context.Background(), task))
	}

	return repo
}

func newTestServerForTask(installed domain.ServerInstalledStatus) *domain.Server {
	return &domain.Server{
		ID: 1, UUID: uuid.New(), UUIDShort: "s1",
		Name: "Server", GameID: "cs", DSID: 1, GameModID: 1,
		ServerIP: "127.0.0.1", ServerPort: 27015, Dir: "/srv/s",
		Installed: installed,
	}
}

func TestHandleTaskStatusUpdate_ServerInstalledStatus(t *testing.T) {
	now := time.Now()
	serverID := uint(1)

	tests := []struct {
		name              string
		task              *domain.DaemonTask
		server            *domain.Server
		updateStatus      proto.DaemonTaskStatus
		wantInstalled     domain.ServerInstalledStatus
		wantServerUpdated bool
	}{
		{
			name: "gsinst_success_sets_installed",
			task: &domain.DaemonTask{
				DedicatedServerID: 1, ServerID: &serverID,
				Task: domain.DaemonTaskTypeServerInstall, Status: domain.DaemonTaskStatusWorking,
				CreatedAt: &now, UpdatedAt: &now,
			},
			server:            newTestServerForTask(domain.ServerInstalledStatusNotInstalled),
			updateStatus:      proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
			wantInstalled:     domain.ServerInstalledStatusInstalled,
			wantServerUpdated: true,
		},
		{
			name: "gsinst_working_sets_installation_in_progress",
			task: &domain.DaemonTask{
				DedicatedServerID: 1, ServerID: &serverID,
				Task: domain.DaemonTaskTypeServerInstall, Status: domain.DaemonTaskStatusWaiting,
				CreatedAt: &now, UpdatedAt: &now,
			},
			server:            newTestServerForTask(domain.ServerInstalledStatusNotInstalled),
			updateStatus:      proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
			wantInstalled:     domain.ServerInstalledStatusInstallationInProg,
			wantServerUpdated: true,
		},
		{
			name: "gsinst_error_sets_not_installed",
			task: &domain.DaemonTask{
				DedicatedServerID: 1, ServerID: &serverID,
				Task: domain.DaemonTaskTypeServerInstall, Status: domain.DaemonTaskStatusWorking,
				CreatedAt: &now, UpdatedAt: &now,
			},
			server:            newTestServerForTask(domain.ServerInstalledStatusInstallationInProg),
			updateStatus:      proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR,
			wantInstalled:     domain.ServerInstalledStatusNotInstalled,
			wantServerUpdated: true,
		},
		{
			name: "gsinst_canceled_sets_not_installed",
			task: &domain.DaemonTask{
				DedicatedServerID: 1, ServerID: &serverID,
				Task: domain.DaemonTaskTypeServerInstall, Status: domain.DaemonTaskStatusWorking,
				CreatedAt: &now, UpdatedAt: &now,
			},
			server:            newTestServerForTask(domain.ServerInstalledStatusInstallationInProg),
			updateStatus:      proto.DaemonTaskStatus_DAEMON_TASK_STATUS_CANCELED,
			wantInstalled:     domain.ServerInstalledStatusNotInstalled,
			wantServerUpdated: true,
		},
		{
			name: "gsdel_success_sets_not_installed",
			task: &domain.DaemonTask{
				DedicatedServerID: 1, ServerID: &serverID,
				Task: domain.DaemonTaskTypeServerDelete, Status: domain.DaemonTaskStatusWorking,
				CreatedAt: &now, UpdatedAt: &now,
			},
			server:            newTestServerForTask(domain.ServerInstalledStatusInstalled),
			updateStatus:      proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
			wantInstalled:     domain.ServerInstalledStatusNotInstalled,
			wantServerUpdated: true,
		},
		{
			name: "gsstart_success_does_not_change_installed",
			task: &domain.DaemonTask{
				DedicatedServerID: 1, ServerID: &serverID,
				Task: domain.DaemonTaskTypeServerStart, Status: domain.DaemonTaskStatusWorking,
				CreatedAt: &now, UpdatedAt: &now,
			},
			server:            newTestServerForTask(domain.ServerInstalledStatusInstalled),
			updateStatus:      proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
			wantInstalled:     domain.ServerInstalledStatusInstalled,
			wantServerUpdated: false,
		},
		{
			name: "task_without_server_id_skips_update",
			task: &domain.DaemonTask{
				DedicatedServerID: 1, ServerID: nil,
				Task: domain.DaemonTaskTypeServerInstall, Status: domain.DaemonTaskStatusWorking,
				CreatedAt: &now, UpdatedAt: &now,
			},
			server:            nil,
			updateStatus:      proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
			wantServerUpdated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskRepo := setupDaemonTaskRepo(t, tt.task)
			serverRepo := inmemory.NewServerRepository()

			if tt.server != nil {
				require.NoError(t, serverRepo.Save(context.Background(), tt.server))
			}

			handler := NewTaskHandler(taskRepo, serverRepo, nil, nil, slog.Default())

			err := handler.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
				TaskId: uint64(tt.task.ID),
				Status: tt.updateStatus,
			})
			require.NoError(t, err)

			if !tt.wantServerUpdated || tt.server == nil {
				return
			}

			servers, err := serverRepo.Find(
				context.Background(),
				&filters.FindServer{IDs: []uint{tt.server.ID}},
				nil, nil,
			)
			require.NoError(t, err)
			require.Len(t, servers, 1)
			assert.Equal(t, tt.wantInstalled, servers[0].Installed)
		})
	}
}

func TestReconcileWorkingTasks(t *testing.T) {
	now := time.Now()
	ctx := context.Background()

	type taskSpec struct {
		id                uint
		dedicatedServerID uint
		status            domain.DaemonTaskStatus
		taskType          domain.DaemonTaskType
		serverID          *uint
		output            *string
	}

	type wantState struct {
		id     uint
		status domain.DaemonTaskStatus
		// outputContains, when non-empty, asserts that the persisted output
		// contains this substring after reconciliation.
		outputContains string
	}

	serverID := uint(11)

	tests := []struct {
		name        string
		seed        []taskSpec
		nodeID      uint64
		inFlightIDs []uint64
		wantMarked  int
		wantState   []wantState
	}{
		{
			name: "missing_in_flight_marks_error_and_appends_output",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 1, status: domain.DaemonTaskStatusWorking, taskType: domain.DaemonTaskTypeCmdExec},
			},
			nodeID:      1,
			inFlightIDs: nil,
			wantMarked:  1,
			wantState: []wantState{
				{id: 1, status: domain.DaemonTaskStatusError, outputContains: AbandonedTaskMessage},
			},
		},
		{
			name: "task_present_in_flight_is_left_alone",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 1, status: domain.DaemonTaskStatusWorking, taskType: domain.DaemonTaskTypeCmdExec},
			},
			nodeID:      1,
			inFlightIDs: []uint64{1},
			wantMarked:  0,
			wantState: []wantState{
				{id: 1, status: domain.DaemonTaskStatusWorking},
			},
		},
		{
			name: "tasks_for_other_node_unchanged",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 1, status: domain.DaemonTaskStatusWorking, taskType: domain.DaemonTaskTypeCmdExec},
				{id: 2, dedicatedServerID: 2, status: domain.DaemonTaskStatusWorking, taskType: domain.DaemonTaskTypeCmdExec},
			},
			nodeID:      1,
			inFlightIDs: nil,
			wantMarked:  1,
			wantState: []wantState{
				{id: 1, status: domain.DaemonTaskStatusError, outputContains: AbandonedTaskMessage},
				{id: 2, status: domain.DaemonTaskStatusWorking},
			},
		},
		{
			name: "non_working_statuses_unaffected",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 1, status: domain.DaemonTaskStatusWaiting, taskType: domain.DaemonTaskTypeCmdExec},
				{id: 2, dedicatedServerID: 1, status: domain.DaemonTaskStatusSuccess, taskType: domain.DaemonTaskTypeCmdExec},
				{id: 3, dedicatedServerID: 1, status: domain.DaemonTaskStatusError, taskType: domain.DaemonTaskTypeCmdExec},
			},
			nodeID:      1,
			inFlightIDs: nil,
			wantMarked:  0,
			wantState: []wantState{
				{id: 1, status: domain.DaemonTaskStatusWaiting},
				{id: 2, status: domain.DaemonTaskStatusSuccess},
				{id: 3, status: domain.DaemonTaskStatusError},
			},
		},
		{
			name: "mixed_partial_in_flight",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 1, status: domain.DaemonTaskStatusWorking, taskType: domain.DaemonTaskTypeCmdExec},
				{id: 2, dedicatedServerID: 1, status: domain.DaemonTaskStatusWorking, taskType: domain.DaemonTaskTypeCmdExec},
				{id: 3, dedicatedServerID: 1, status: domain.DaemonTaskStatusWorking, taskType: domain.DaemonTaskTypeCmdExec},
			},
			nodeID:      1,
			inFlightIDs: []uint64{2},
			wantMarked:  2,
			wantState: []wantState{
				{id: 1, status: domain.DaemonTaskStatusError, outputContains: AbandonedTaskMessage},
				{id: 2, status: domain.DaemonTaskStatusWorking},
				{id: 3, status: domain.DaemonTaskStatusError, outputContains: AbandonedTaskMessage},
			},
		},
		{
			name:        "no_working_tasks_is_noop",
			seed:        nil,
			nodeID:      1,
			inFlightIDs: nil,
			wantMarked:  0,
		},
		{
			name: "appends_to_existing_output",
			seed: []taskSpec{
				{
					id: 1, dedicatedServerID: 1, status: domain.DaemonTaskStatusWorking,
					taskType: domain.DaemonTaskTypeCmdExec,
					output:   new("partial-progress\n"),
				},
			},
			nodeID:      1,
			inFlightIDs: nil,
			wantMarked:  1,
			wantState: []wantState{
				{id: 1, status: domain.DaemonTaskStatusError, outputContains: "partial-progress"},
			},
		},
		{
			name: "install_task_resets_server_installed_status",
			seed: []taskSpec{
				{
					id: 1, dedicatedServerID: 1, status: domain.DaemonTaskStatusWorking,
					taskType: domain.DaemonTaskTypeServerInstall,
					serverID: &serverID,
				},
			},
			nodeID:      1,
			inFlightIDs: nil,
			wantMarked:  1,
			wantState: []wantState{
				{id: 1, status: domain.DaemonTaskStatusError, outputContains: AbandonedTaskMessage},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskRepo := inmemory.NewDaemonTaskRepository()
			for _, spec := range tt.seed {
				task := &domain.DaemonTask{
					ID:                spec.id,
					DedicatedServerID: spec.dedicatedServerID,
					ServerID:          spec.serverID,
					Task:              spec.taskType,
					Status:            spec.status,
					Output:            spec.output,
					CreatedAt:         &now,
					UpdatedAt:         &now,
				}
				require.NoError(t, taskRepo.Save(ctx, task))
			}

			serverRepo := inmemory.NewServerRepository()
			require.NoError(t, serverRepo.Save(ctx, &domain.Server{
				ID: serverID, UUID: uuid.New(), UUIDShort: "s11",
				Name: "Server", GameID: "cs", DSID: 1, GameModID: 1,
				ServerIP: "127.0.0.1", ServerPort: 27015, Dir: "/srv/s",
				Installed: domain.ServerInstalledStatusInstallationInProg,
			}))

			handler := NewTaskHandler(taskRepo, serverRepo, nil, nil, slog.Default())

			marked, err := handler.ReconcileWorkingTasks(ctx, tt.nodeID, tt.inFlightIDs, "test_reason")
			require.NoError(t, err)
			assert.Equal(t, tt.wantMarked, marked)

			for _, want := range tt.wantState {
				tasks, err := taskRepo.FindWithOutput(ctx, &filters.FindDaemonTask{
					IDs: []uint{want.id},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, tasks, 1)

				assert.Equal(t, want.status, tasks[0].Status,
					"task %d expected status %s, got %s", want.id, want.status, tasks[0].Status)

				if want.outputContains != "" {
					require.NotNil(t, tasks[0].Output, "task %d expected output, got nil", want.id)
					assert.Contains(t, *tasks[0].Output, want.outputContains)
				}
			}

			if tt.name == "install_task_resets_server_installed_status" {
				servers, err := serverRepo.Find(ctx, &filters.FindServer{IDs: []uint{serverID}}, nil, nil)
				require.NoError(t, err)
				require.Len(t, servers, 1)
				assert.Equal(t, domain.ServerInstalledStatusNotInstalled, servers[0].Installed)
			}
		})
	}
}

func TestResolveInstalledStatus(t *testing.T) {
	tests := []struct {
		name       string
		taskType   domain.DaemonTaskType
		taskStatus domain.DaemonTaskStatus
		want       domain.ServerInstalledStatus
		wantOK     bool
	}{
		{
			name:       "gsinst_waiting_no_change",
			taskType:   domain.DaemonTaskTypeServerInstall,
			taskStatus: domain.DaemonTaskStatusWaiting,
			wantOK:     false,
		},
		{
			name:       "gsdel_working_no_change",
			taskType:   domain.DaemonTaskTypeServerDelete,
			taskStatus: domain.DaemonTaskStatusWorking,
			wantOK:     false,
		},
		{
			name:       "gsdel_error_no_change",
			taskType:   domain.DaemonTaskTypeServerDelete,
			taskStatus: domain.DaemonTaskStatusError,
			wantOK:     false,
		},
		{
			name:       "gsupd_success_no_change",
			taskType:   domain.DaemonTaskTypeServerUpdate,
			taskStatus: domain.DaemonTaskStatusSuccess,
			wantOK:     false,
		},
		{
			name:       "gsinst_success",
			taskType:   domain.DaemonTaskTypeServerInstall,
			taskStatus: domain.DaemonTaskStatusSuccess,
			want:       domain.ServerInstalledStatusInstalled,
			wantOK:     true,
		},
		{
			name:       "gsinst_working",
			taskType:   domain.DaemonTaskTypeServerInstall,
			taskStatus: domain.DaemonTaskStatusWorking,
			want:       domain.ServerInstalledStatusInstallationInProg,
			wantOK:     true,
		},
		{
			name:       "gsdel_success",
			taskType:   domain.DaemonTaskTypeServerDelete,
			taskStatus: domain.DaemonTaskStatusSuccess,
			want:       domain.ServerInstalledStatusNotInstalled,
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := resolveInstalledStatus(tt.taskType, tt.taskStatus)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
