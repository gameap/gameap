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
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
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
	mu sync.Mutex
	// attempts records every Publish call; messages only the delivered ones.
	attempts []publishedMessage
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

	// Every call is an attempt; only a call that returns nil delivered
	// anything, so a failing backend must not look like a broadcast.
	p.attempts = append(p.attempts, publishedMessage{channel: channel, msg: msg})
	if p.err != nil {
		return p.err
	}

	p.messages = append(p.messages, publishedMessage{channel: channel, msg: msg})

	return nil
}

func (p *capturePublisher) snapshot() []publishedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]publishedMessage(nil), p.messages...)
}

func (p *capturePublisher) attemptSnapshot() []publishedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]publishedMessage(nil), p.attempts...)
}

func filterByType(msgs []publishedMessage, msgType string) []publishedMessage {
	out := make([]publishedMessage, 0, len(msgs))
	for _, m := range msgs {
		if m.msg.Type == msgType {
			out = append(out, m)
		}
	}

	return out
}

// byType returns messages the publisher actually delivered.
func (p *capturePublisher) byType(msgType string) []publishedMessage {
	return filterByType(p.snapshot(), msgType)
}

// attemptsByType returns messages the handler tried to publish, delivered or not.
func (p *capturePublisher) attemptsByType(msgType string) []publishedMessage {
	return filterByType(p.attemptSnapshot(), msgType)
}

var errPublisherDown = errors.New("pubsub backend down")

func TestHandleTaskOutput(t *testing.T) {
	t.Parallel()
	now := time.Now()

	tests := []struct {
		name           string
		output         *proto.TaskOutput
		publisherErr   error
		wantStoredText string
		// wantAttempts is how often the handler tried to broadcast the chunk;
		// wantDelivered how many of those the publisher accepted.
		wantAttempts  int
		wantDelivered int
		wantFinal     bool
	}{
		{
			name:           "empty_chunk_is_ignored",
			output:         &proto.TaskOutput{TaskId: 1, OutputChunk: nil},
			wantStoredText: "",
			wantAttempts:   0,
			wantDelivered:  0,
		},
		{
			name:           "chunk_is_appended_and_broadcast",
			output:         &proto.TaskOutput{TaskId: 1, OutputChunk: []byte("installing...\n")},
			wantStoredText: "installing...\n",
			wantAttempts:   1,
			wantDelivered:  1,
		},
		{
			name: "final_chunk_is_flagged",
			output: &proto.TaskOutput{
				TaskId: 1, OutputChunk: []byte("done\n"), IsFinal: true,
			},
			wantStoredText: "done\n",
			wantAttempts:   1,
			wantDelivered:  1,
			wantFinal:      true,
		},
		{
			name:           "publish_error_does_not_lose_the_output",
			output:         &proto.TaskOutput{TaskId: 1, OutputChunk: []byte("chunk\n")},
			publisherErr:   errPublisherDown,
			wantStoredText: "chunk\n",
			wantAttempts:   1,
			wantDelivered:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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

			attempted := publisher.attemptsByType(messages.TypeTaskOutput)
			require.Len(t, attempted, tt.wantAttempts)
			require.Len(t, publisher.byType(messages.TypeTaskOutput), tt.wantDelivered,
				"a publisher that errors must not count as a delivery")

			if tt.wantAttempts == 0 {
				return
			}

			assert.Equal(t, channels.BuildRealtimeTaskOutputChannel(uint64(task.ID)), attempted[0].channel)

			var payload messages.TaskOutputPayload
			require.NoError(t, json.Unmarshal(attempted[0].msg.Payload, &payload))
			assert.Equal(t, uint64(task.ID), payload.TaskID)
			assert.Equal(t, tt.wantStoredText, payload.Chunk, "the broadcast chunk must match what was stored")
			assert.Equal(t, tt.wantFinal, payload.IsFinal)
		})
	}
}

// Output arrives as a stream of chunks: each one must be appended to what is
// already stored, not replace it, and each must be broadcast in order.
func TestHandleTaskOutput_AppendsChunksInOrder(t *testing.T) {
	t.Parallel()
	// ARRANGE
	now := time.Now()
	task := &domain.DaemonTask{
		DedicatedServerID: 1,
		Task:              domain.DaemonTaskTypeServerInstall,
		Status:            domain.DaemonTaskStatusWorking,
		CreatedAt:         &now, UpdatedAt: &now,
	}
	repo := setupDaemonTaskRepo(t, task)
	publisher := &capturePublisher{}
	handler := NewTaskHandler(repo, inmemory.NewServerRepository(), publisher, nil, slog.Default())

	chunks := []string{"downloading...\n", "unpacking...\n"}

	// ACT
	for _, chunk := range chunks {
		require.NoError(t, handler.HandleTaskOutput(context.Background(), 1, &proto.TaskOutput{
			TaskId:      uint64(task.ID),
			OutputChunk: []byte(chunk),
		}))
	}

	// ASSERT
	tasks, err := repo.FindWithOutput(
		context.Background(), &filters.FindDaemonTask{IDs: []uint{task.ID}}, nil, nil,
	)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].Output)
	assert.Equal(t, chunks[0]+chunks[1], *tasks[0].Output,
		"a later chunk must append to the stored output, not replace it")

	published := publisher.byType(messages.TypeTaskOutput)
	require.Len(t, published, len(chunks))

	for i, chunk := range chunks {
		var payload messages.TaskOutputPayload
		require.NoError(t, json.Unmarshal(published[i].msg.Payload, &payload))
		assert.Equal(t, chunk, payload.Chunk, "chunk %d must be broadcast in order", i)
		assert.False(t, payload.IsFinal, "neither chunk was marked final")
	}
}

// Without a publisher wired in, output handling must still persist and must
// not panic — single-instance deployments run this way.
func TestHandleTaskOutput_WithoutPublisher(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	tests := []struct {
		name         string
		status       proto.DaemonTaskStatus
		wantStatus   domain.DaemonTaskStatus
		publisherErr error
	}{
		{
			name:       "success_is_broadcast",
			status:     proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
			wantStatus: domain.DaemonTaskStatusSuccess,
		},
		{
			name:       "error_is_broadcast",
			status:     proto.DaemonTaskStatus_DAEMON_TASK_STATUS_ERROR,
			wantStatus: domain.DaemonTaskStatusError,
		},
		{
			name:       "canceled_is_broadcast",
			status:     proto.DaemonTaskStatus_DAEMON_TASK_STATUS_CANCELED,
			wantStatus: domain.DaemonTaskStatusCanceled,
		},
		{
			name:         "publish_error_is_swallowed",
			status:       proto.DaemonTaskStatus_DAEMON_TASK_STATUS_SUCCESS,
			wantStatus:   domain.DaemonTaskStatusSuccess,
			publisherErr: errPublisherDown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
				Status:  tt.status,
				Message: "installed",
			})

			// ASSERT
			require.NoError(t, err, "a broadcast failure must not fail the daemon call")

			wantDelivered := 1
			if tt.publisherErr != nil {
				wantDelivered = 0
			}

			statusMsgs := publisher.attemptsByType(messages.TypeTaskStatus)
			require.Len(t, statusMsgs, 1)
			require.Len(t, publisher.byType(messages.TypeTaskStatus), wantDelivered,
				"a publisher that errors must not count as a delivery")
			assert.Equal(t,
				channels.BuildRealtimeTaskStatusChannel(uint64(task.ID)), statusMsgs[0].channel)

			var statusPayload messages.TaskStatusPayload
			require.NoError(t, json.Unmarshal(statusMsgs[0].msg.Payload, &statusPayload))
			assert.Equal(t, uint64(task.ID), statusPayload.TaskID)
			assert.Equal(t, string(tt.wantStatus), statusPayload.Status)
			assert.Equal(t, serverID, statusPayload.ServerID)
			assert.Equal(t, "installed", statusPayload.Message)

			completeMsgs := publisher.attemptsByType(messages.TypeTaskComplete)
			require.Len(t, completeMsgs, 1, "every terminal status must emit a completion message")
			require.Len(t, publisher.byType(messages.TypeTaskComplete), wantDelivered)

			var completePayload messages.TaskCompletePayload
			require.NoError(t, json.Unmarshal(completeMsgs[0].msg.Payload, &completePayload))
			assert.Equal(t, uint64(task.ID), completePayload.TaskID)
			assert.Equal(t, string(tt.wantStatus), completePayload.Status)
			assert.Equal(t, serverID, completePayload.ServerID)
		})
	}
}

// A non-terminal transition must not announce completion.
func TestHandleTaskStatusUpdate_NonTerminalDoesNotComplete(t *testing.T) {
	t.Parallel()
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
