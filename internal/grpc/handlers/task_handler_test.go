package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/google/uuid"
	"github.com/pkg/errors"
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

// capturePublisher records every message the handler broadcasts so tests can
// assert the realtime contract without a live pubsub backend.
type capturePublisher struct {
	mu       sync.Mutex
	messages []publishedMessage
	err      error
}

type publishedMessage struct {
	channel string
	msg     *pubsub.Message
}

func (p *capturePublisher) Publish(_ context.Context, channel string, msg *pubsub.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.messages = append(p.messages, publishedMessage{channel: channel, msg: msg})

	return p.err
}

func (p *capturePublisher) snapshot() []publishedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]publishedMessage(nil), p.messages...)
}

func (p *capturePublisher) byType(msgType string) []publishedMessage {
	out := make([]publishedMessage, 0, len(p.messages))
	for _, m := range p.snapshot() {
		if m.msg.Type == msgType {
			out = append(out, m)
		}
	}

	return out
}

var errPublisherDown = errors.New("pubsub backend down")

func TestHandleTaskOutput(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		output         *proto.TaskOutput
		publisherErr   error
		wantStoredText string
		wantPublished  int
		wantFinal      bool
	}{
		{
			name:           "empty_chunk_is_ignored",
			output:         &proto.TaskOutput{TaskId: 1, OutputChunk: nil},
			wantStoredText: "",
			wantPublished:  0,
		},
		{
			name:           "chunk_is_appended_and_broadcast",
			output:         &proto.TaskOutput{TaskId: 1, OutputChunk: []byte("installing...\n")},
			wantStoredText: "installing...\n",
			wantPublished:  1,
		},
		{
			name: "final_chunk_is_flagged",
			output: &proto.TaskOutput{
				TaskId: 1, OutputChunk: []byte("done\n"), IsFinal: true,
			},
			wantStoredText: "done\n",
			wantPublished:  1,
			wantFinal:      true,
		},
		{
			name:           "publish_error_does_not_lose_the_output",
			output:         &proto.TaskOutput{TaskId: 1, OutputChunk: []byte("chunk\n")},
			publisherErr:   errPublisherDown,
			wantStoredText: "chunk\n",
			wantPublished:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			task := &domain.DaemonTask{
				DedicatedServerID: 1,
				Task:              domain.DaemonTaskTypeServerInstall,
				Status:            domain.DaemonTaskStatusWorking,
				CreatedAt:         &now, UpdatedAt: &now,
			}
			repo := setupDaemonTaskRepo(t, task)
			publisher := &capturePublisher{err: tt.publisherErr}
			handler := NewTaskHandler(repo, inmemory.NewServerRepository(), publisher, nil, slog.Default())
			tt.output.TaskId = uint64(task.ID)

			// ACT
			err := handler.HandleTaskOutput(context.Background(), 1, tt.output)

			// ASSERT
			require.NoError(t, err, "a broadcast failure must not fail the daemon call")

			tasks, err := repo.FindWithOutput(
				context.Background(), &filters.FindDaemonTask{IDs: []uint{task.ID}}, nil, nil,
			)
			require.NoError(t, err)
			require.Len(t, tasks, 1)

			if tt.wantStoredText == "" {
				assert.Nil(t, tasks[0].Output, "an empty chunk must not touch the stored output")
			} else {
				require.NotNil(t, tasks[0].Output)
				assert.Equal(t, tt.wantStoredText, *tasks[0].Output, "output must be persisted verbatim")
			}

			published := publisher.byType(messages.TypeTaskOutput)
			require.Len(t, published, tt.wantPublished)

			if tt.wantPublished == 0 {
				return
			}

			assert.Equal(t, channels.BuildRealtimeTaskOutputChannel(uint64(task.ID)), published[0].channel)

			var payload messages.TaskOutputPayload
			require.NoError(t, json.Unmarshal(published[0].msg.Payload, &payload))
			assert.Equal(t, uint64(task.ID), payload.TaskID)
			assert.Equal(t, tt.wantStoredText, payload.Chunk, "the broadcast chunk must match what was stored")
			assert.Equal(t, tt.wantFinal, payload.IsFinal)
		})
	}
}

// Without a publisher wired in, output handling must still persist and must
// not panic — single-instance deployments run this way.
func TestHandleTaskOutput_WithoutPublisher(t *testing.T) {
	// ARRANGE
	now := time.Now()
	task := &domain.DaemonTask{
		DedicatedServerID: 1,
		Task:              domain.DaemonTaskTypeServerInstall,
		Status:            domain.DaemonTaskStatusWorking,
		CreatedAt:         &now, UpdatedAt: &now,
	}
	repo := setupDaemonTaskRepo(t, task)
	handler := NewTaskHandler(repo, inmemory.NewServerRepository(), nil, nil, slog.Default())

	// ACT
	err := handler.HandleTaskOutput(context.Background(), 1, &proto.TaskOutput{
		TaskId: uint64(task.ID), OutputChunk: []byte("line\n"),
	})

	// ASSERT
	require.NoError(t, err)

	tasks, err := repo.FindWithOutput(
		context.Background(), &filters.FindDaemonTask{IDs: []uint{task.ID}}, nil, nil,
	)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].Output)
	assert.Equal(t, "line\n", *tasks[0].Output)
}

func TestGetPendingTasks(t *testing.T) {
	now := time.Now()

	waitingForNode := &domain.DaemonTask{
		DedicatedServerID: 1, Task: domain.DaemonTaskTypeServerStart,
		Status: domain.DaemonTaskStatusWaiting, CreatedAt: &now, UpdatedAt: &now,
	}
	workingForNode := &domain.DaemonTask{
		DedicatedServerID: 1, Task: domain.DaemonTaskTypeServerStop,
		Status: domain.DaemonTaskStatusWorking, CreatedAt: &now, UpdatedAt: &now,
	}
	waitingForOtherNode := &domain.DaemonTask{
		DedicatedServerID: 2, Task: domain.DaemonTaskTypeServerRestart,
		Status: domain.DaemonTaskStatusWaiting, CreatedAt: &now, UpdatedAt: &now,
	}

	// ARRANGE
	repo := setupDaemonTaskRepo(t, waitingForNode, workingForNode, waitingForOtherNode)
	handler := NewTaskHandler(repo, inmemory.NewServerRepository(), nil, nil, slog.Default())

	// ACT
	tasks, err := handler.GetPendingTasks(context.Background(), 1)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, tasks, 1, "only waiting tasks owned by the node may be dispatched")
	assert.Equal(t, uint64(waitingForNode.ID), tasks[0].Id)
	assert.Equal(t, proto.DaemonTaskType_DAEMON_TASK_TYPE_SERVER_START, tasks[0].TaskType)
	assert.Equal(t, uint64(1), tasks[0].NodeId, "a task must only be dispatched to its owning node")
}

func TestGetPendingTasks_NoWork(t *testing.T) {
	// ARRANGE
	handler := NewTaskHandler(
		setupDaemonTaskRepo(t), inmemory.NewServerRepository(), nil, nil, slog.Default(),
	)

	// ACT
	tasks, err := handler.GetPendingTasks(context.Background(), 1)

	// ASSERT
	require.NoError(t, err)
	assert.Empty(t, tasks, "a node with no waiting work must get an empty list, not an error")
}

// A terminal status must reach subscribers as both a status update and a
// completion message, and a broken publisher must never fail the daemon call.
func TestHandleTaskStatusUpdate_PublishesRealtimeMessages(t *testing.T) {
	tests := []struct {
		name         string
		publisherErr error
	}{
		{name: "messages_are_broadcast"},
		{name: "publish_error_is_swallowed", publisherErr: errPublisherDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			now := time.Now()
			serverID := uint(1)
			task := &domain.DaemonTask{
				DedicatedServerID: 1, ServerID: &serverID,
				Task: domain.DaemonTaskTypeServerInstall, Status: domain.DaemonTaskStatusWorking,
				CreatedAt: &now, UpdatedAt: &now,
			}
			repo := setupDaemonTaskRepo(t, task)
			serverRepo := inmemory.NewServerRepository()
			require.NoError(t, serverRepo.Save(
				context.Background(), newTestServerForTask(domain.ServerInstalledStatusNotInstalled),
			))
			publisher := &capturePublisher{err: tt.publisherErr}
			handler := NewTaskHandler(repo, serverRepo, publisher, nil, slog.Default())

			// ACT
			err := handler.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
				TaskId:  uint64(task.ID),
				Status:  proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
				Message: "installed",
			})

			// ASSERT
			require.NoError(t, err, "a broadcast failure must not fail the daemon call")

			statusMsgs := publisher.byType(messages.TypeTaskStatus)
			require.Len(t, statusMsgs, 1)
			assert.Equal(t,
				channels.BuildRealtimeTaskStatusChannel(uint64(task.ID)), statusMsgs[0].channel)

			var statusPayload messages.TaskStatusPayload
			require.NoError(t, json.Unmarshal(statusMsgs[0].msg.Payload, &statusPayload))
			assert.Equal(t, uint64(task.ID), statusPayload.TaskID)
			assert.Equal(t, string(domain.DaemonTaskStatusSuccess), statusPayload.Status)
			assert.Equal(t, serverID, statusPayload.ServerID)
			assert.Equal(t, "installed", statusPayload.Message)

			completeMsgs := publisher.byType(messages.TypeTaskComplete)
			require.Len(t, completeMsgs, 1, "a terminal status must also emit a completion message")

			var completePayload messages.TaskCompletePayload
			require.NoError(t, json.Unmarshal(completeMsgs[0].msg.Payload, &completePayload))
			assert.Equal(t, uint64(task.ID), completePayload.TaskID)
			assert.Equal(t, string(domain.DaemonTaskStatusSuccess), completePayload.Status)
			assert.Equal(t, serverID, completePayload.ServerID)
		})
	}
}

// A non-terminal transition must not announce completion.
func TestHandleTaskStatusUpdate_NonTerminalDoesNotComplete(t *testing.T) {
	// ARRANGE
	now := time.Now()
	task := &domain.DaemonTask{
		DedicatedServerID: 1,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         &now, UpdatedAt: &now,
	}
	publisher := &capturePublisher{}
	handler := NewTaskHandler(
		setupDaemonTaskRepo(t, task), inmemory.NewServerRepository(), publisher, nil, slog.Default(),
	)

	// ACT
	err := handler.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
		TaskId: uint64(task.ID),
		Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
	})

	// ASSERT
	require.NoError(t, err)
	require.Len(t, publisher.byType(messages.TypeTaskStatus), 1)
	assert.Empty(t, publisher.byType(messages.TypeTaskComplete),
		"a task still in flight must not be announced as complete")
}
