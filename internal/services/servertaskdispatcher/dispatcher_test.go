package servertaskdispatcher

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	errTestFind          = errors.New("simulated find failure")
	errTestSenderSend    = errors.New("simulated sender send failure")
	errTestRepoCreate    = errors.New("simulated execution create failure")
	errTestRepoUpdate    = errors.New("simulated execution update failure")
	errTestRepoAppend    = errors.New("simulated execution append failure")
	errTestRepoAbandon   = errors.New("simulated mark abandoned failure")
	errTestRepoBuildSnap = errors.New("simulated build snapshot find failure")
)

// fakeSender records every SendTask call and can simulate failure.
type fakeSender struct {
	mu      sync.Mutex
	calls   []fakeSendCall
	sendErr error
}

type fakeSendCall struct {
	nodeID uint64
	msg    *proto.GatewayMessage
}

func (s *fakeSender) SendTask(_ context.Context, nodeID uint64, msg *proto.GatewayMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sendErr != nil {
		return s.sendErr
	}

	s.calls = append(s.calls, fakeSendCall{nodeID: nodeID, msg: msg})

	return nil
}

func (s *fakeSender) snapshot() []fakeSendCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]fakeSendCall, len(s.calls))
	copy(out, s.calls)

	return out
}

// fakeServerTaskRepo implements repositories.ServerTaskRepository.
// Only Find is exercised; the rest stay stubbed.
type fakeServerTaskRepo struct {
	mu sync.Mutex

	findResult []domain.ServerTask
	findErr    error
	findFilter *filters.FindServerTask
}

func (r *fakeServerTaskRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.ServerTask, error) {
	return nil, nil
}

func (r *fakeServerTaskRepo) Find(
	_ context.Context, filter *filters.FindServerTask, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.ServerTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.findFilter = filter
	if r.findErr != nil {
		return nil, r.findErr
	}

	out := make([]domain.ServerTask, len(r.findResult))
	copy(out, r.findResult)

	return out, nil
}

func (r *fakeServerTaskRepo) Save(_ context.Context, _ *domain.ServerTask) error { return nil }
func (r *fakeServerTaskRepo) BumpVersion(_ context.Context, _ uint) (uint64, error) {
	return 0, nil
}
func (r *fakeServerTaskRepo) SoftDelete(_ context.Context, _ uint) error { return nil }
func (r *fakeServerTaskRepo) Delete(_ context.Context, _ uint) error     { return nil }

func (r *fakeServerTaskRepo) capturedFilter() *filters.FindServerTask {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.findFilter
}

// fakeExecutionRepo implements repositories.ServerTaskExecutionRepository.
// Captures every Create, UpdateFinish, AppendOutputInline and MarkAbandoned call
// and can simulate per-method failures.
type fakeExecutionRepo struct {
	mu sync.Mutex

	createCalls []*domain.ServerTaskExecution
	createErr   error

	updateFinishCalls []updateFinishCall
	updateFinishErr   error

	appendOutputCalls []appendOutputCall
	appendOutputErr   error

	markAbandonedCalls []markAbandonedCall
	markAbandonedN     int
	markAbandonedErr   error
}

type updateFinishCall struct {
	id    xid.ID
	patch repositories.ServerTaskExecutionFinishPatch
}

type appendOutputCall struct {
	id       xid.ID
	chunk    []byte
	maxBytes int
}

type markAbandonedCall struct {
	nodeID  uint
	keepIDs []xid.ID
	reason  string
}

func (r *fakeExecutionRepo) Create(_ context.Context, exec *domain.ServerTaskExecution) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.createErr != nil {
		return r.createErr
	}

	clone := *exec
	r.createCalls = append(r.createCalls, &clone)

	return nil
}

func (r *fakeExecutionRepo) UpdateFinish(
	_ context.Context, executionID xid.ID, patch repositories.ServerTaskExecutionFinishPatch,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.updateFinishErr != nil {
		return r.updateFinishErr
	}

	r.updateFinishCalls = append(r.updateFinishCalls, updateFinishCall{id: executionID, patch: patch})

	return nil
}

func (r *fakeExecutionRepo) AppendOutputInline(
	_ context.Context, executionID xid.ID, chunk []byte, maxBytes int,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.appendOutputErr != nil {
		return r.appendOutputErr
	}

	chunkCopy := make([]byte, len(chunk))
	copy(chunkCopy, chunk)
	r.appendOutputCalls = append(r.appendOutputCalls, appendOutputCall{
		id:       executionID,
		chunk:    chunkCopy,
		maxBytes: maxBytes,
	})

	return nil
}

func (r *fakeExecutionRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.ServerTaskExecution, error) {
	return nil, nil
}

func (r *fakeExecutionRepo) Find(
	_ context.Context,
	_ *filters.FindServerTaskExecution,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.ServerTaskExecution, error) {
	return nil, nil
}

func (r *fakeExecutionRepo) MarkAbandoned(
	_ context.Context, nodeID uint, keepExecutionIDs []xid.ID, reason string,
) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.markAbandonedErr != nil {
		return 0, r.markAbandonedErr
	}

	keepCopy := make([]xid.ID, len(keepExecutionIDs))
	copy(keepCopy, keepExecutionIDs)
	r.markAbandonedCalls = append(r.markAbandonedCalls, markAbandonedCall{
		nodeID:  nodeID,
		keepIDs: keepCopy,
		reason:  reason,
	})

	return r.markAbandonedN, nil
}

func (r *fakeExecutionRepo) DeleteOlderThan(
	_ context.Context, _ domain.ServerTaskExecutionStatus, _ time.Time,
) (int, error) {
	return 0, nil
}

func (r *fakeExecutionRepo) snapshotCreate() []*domain.ServerTaskExecution {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]*domain.ServerTaskExecution, len(r.createCalls))
	copy(out, r.createCalls)

	return out
}

func (r *fakeExecutionRepo) snapshotUpdateFinish() []updateFinishCall {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]updateFinishCall, len(r.updateFinishCalls))
	copy(out, r.updateFinishCalls)

	return out
}

func (r *fakeExecutionRepo) snapshotAppendOutput() []appendOutputCall {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]appendOutputCall, len(r.appendOutputCalls))
	copy(out, r.appendOutputCalls)

	return out
}

func (r *fakeExecutionRepo) snapshotMarkAbandoned() []markAbandonedCall {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]markAbandonedCall, len(r.markAbandonedCalls))
	copy(out, r.markAbandonedCalls)

	return out
}

type testHarness struct {
	dispatcher *Dispatcher
	sender     *fakeSender
	serverRepo *fakeServerTaskRepo
	execRepo   *fakeExecutionRepo
	mem        *memory.Memory
	cleanup    func()
}

// newTestDispatcher wires a Dispatcher with hand-rolled fakes and a real
// in-memory pubsub so publish observations stay close to production wiring.
func newTestDispatcher(t *testing.T) *testHarness {
	t.Helper()

	mem := memory.New()
	sender := &fakeSender{}
	serverRepo := &fakeServerTaskRepo{}
	execRepo := &fakeExecutionRepo{}

	d := NewDispatcher(sender, serverRepo, execRepo, mem, discardLogger())

	cleanup := func() {
		_ = mem.Close()
	}

	return &testHarness{
		dispatcher: d,
		sender:     sender,
		serverRepo: serverRepo,
		execRepo:   execRepo,
		mem:        mem,
		cleanup:    cleanup,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// captureChannel installs a subscriber that buffers every message delivered
// to the given channel, returning the slice and its guarding mutex.
func captureChannel(t *testing.T, mem *memory.Memory, channel string) (*[]*pubsub.Message, *sync.Mutex) {
	t.Helper()

	var mu sync.Mutex
	got := make([]*pubsub.Message, 0)

	require.NoError(t,
		mem.Subscribe(context.Background(), channel, func(_ context.Context, msg *pubsub.Message) error {
			mu.Lock()
			defer mu.Unlock()

			got = append(got, msg)

			return nil
		}),
		"captureChannel subscribe should succeed",
	)

	return &got, &mu
}

// waitForOneMessage polls until the slice has at least one entry or the 200ms
// deadline passes. memory pubsub dispatches synchronously, but the poll covers
// future async behavior.
func waitForOneMessage(captured *[]*pubsub.Message, mu *sync.Mutex) bool {
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		l := len(*captured)
		mu.Unlock()
		if l >= 1 {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	return len(*captured) >= 1
}

func TestDispatcher_BuildSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("returns_only_enabled_and_non_deleted_tasks", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		// ACT
		_, err := h.dispatcher.BuildSnapshot(context.Background(), 42)

		// ASSERT
		require.NoError(t, err)
		filter := h.serverRepo.capturedFilter()
		require.NotNil(t, filter, "Find should have been invoked")
		assert.True(t, filter.OnlyEnabled, "filter.OnlyEnabled must be true so snapshot excludes paused tasks")
		assert.Equal(t, []uint{42}, filter.NodeIDs, "filter.NodeIDs must match the requested node")
	})

	t.Run("wraps_repo_find_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.serverRepo.findErr = errTestFind

		// ACT
		snapshot, err := h.dispatcher.BuildSnapshot(context.Background(), 1)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "load server tasks for snapshot",
			"repo errors must wrap with 'load server tasks for snapshot'")
		assert.Nil(t, snapshot, "no snapshot should be returned when repo errors")
	})

	t.Run("converts_tasks_via_DomainServerTasksToProtoList", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		nodeID := new(uint(7))
		h.serverRepo.findResult = []domain.ServerTask{
			{
				ID:       11,
				Command:  domain.ServerTaskCommandStart,
				ServerID: 100,
				NodeID:   nodeID,
				Enabled:  true,
				Version:  3,
			},
			{
				ID:       12,
				Command:  domain.ServerTaskCommandStop,
				ServerID: 101,
				NodeID:   nodeID,
				Enabled:  true,
				Version:  4,
			},
		}

		// ACT
		snapshot, err := h.dispatcher.BuildSnapshot(context.Background(), 7)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, snapshot)
		require.Len(t, snapshot.Tasks, 2, "two domain tasks should produce two proto tasks")
		assert.Equal(t, uint64(11), snapshot.Tasks[0].Id, "first proto task ID must mirror domain ID")
		assert.Equal(t, uint64(12), snapshot.Tasks[1].Id, "second proto task ID must mirror domain ID")
		assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_START, snapshot.Tasks[0].Command,
			"first proto task command must mirror domain command")
		assert.Equal(t, proto.ServerTaskCommand_SERVER_TASK_COMMAND_STOP, snapshot.Tasks[1].Command,
			"second proto task command must mirror domain command")
	})

	t.Run("sets_generated_at_to_now", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		before := time.Now()

		// ACT
		snapshot, err := h.dispatcher.BuildSnapshot(context.Background(), 1)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, snapshot.GeneratedAt, "GeneratedAt must be populated")

		ts := snapshot.GeneratedAt.AsTime()
		assert.WithinDuration(t, before, ts, 5*time.Second,
			"GeneratedAt must reflect roughly the current time at call site")
	})
}

func TestDispatcher_DispatchUpsert(t *testing.T) {
	t.Parallel()

	t.Run("returns_error_when_task_has_no_node_id", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		task := &domain.ServerTask{
			ID:       9,
			NodeID:   nil,
			ServerID: 100,
			Command:  domain.ServerTaskCommandStart,
		}

		// ACT
		err := h.dispatcher.DispatchUpsert(context.Background(), task)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "server task has no node_id; cannot dispatch",
			"error must mention the missing node_id reason")
		require.Empty(t, h.sender.snapshot(), "sender must not be called when node_id is missing")
	})

	t.Run("sends_upserted_delta_to_correct_node", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		task := &domain.ServerTask{
			ID:       21,
			NodeID:   new(uint(7)),
			ServerID: 100,
			Command:  domain.ServerTaskCommandStart,
			Version:  2,
		}

		// ACT
		err := h.dispatcher.DispatchUpsert(context.Background(), task)

		// ASSERT
		require.NoError(t, err)

		calls := h.sender.snapshot()
		require.Len(t, calls, 1, "exactly one SendTask call is expected")
		assert.Equal(t, uint64(7), calls[0].nodeID, "SendTask must target the task's node")

		delta := calls[0].msg.GetServerTaskDelta()
		require.NotNil(t, delta, "payload must be ServerTaskDelta")
		require.NotNil(t, delta.GetUpserted(), "delta must carry an Upserted variant")
		assert.Equal(t, uint64(21), delta.GetUpserted().Id, "upserted task ID must mirror the domain task ID")
		assert.NotEmpty(t, calls[0].msg.RequestId, "RequestId must be populated for tracing")
	})

	t.Run("swallows_sender_error_and_returns_nil", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.sender.sendErr = errTestSenderSend

		task := &domain.ServerTask{
			ID:       7,
			NodeID:   new(uint(3)),
			ServerID: 100,
			Command:  domain.ServerTaskCommandStart,
		}

		// ACT
		err := h.dispatcher.DispatchUpsert(context.Background(), task)

		// ASSERT
		require.NoError(t, err,
			"sender errors must be swallowed — daemon resyncs on reconnect")
	})
}

func TestDispatcher_DispatchDelete(t *testing.T) {
	t.Parallel()

	t.Run("sends_deleted_delta_with_id_and_version", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		// ACT
		err := h.dispatcher.DispatchDelete(context.Background(), 42, 7, 99)

		// ASSERT
		require.NoError(t, err)

		calls := h.sender.snapshot()
		require.Len(t, calls, 1, "exactly one SendTask call is expected")
		assert.Equal(t, uint64(7), calls[0].nodeID, "SendTask must target the supplied node")

		delta := calls[0].msg.GetServerTaskDelta()
		require.NotNil(t, delta, "payload must be ServerTaskDelta")
		deleted := delta.GetDeleted()
		require.NotNil(t, deleted, "delta must carry a Deleted variant")
		assert.Equal(t, uint64(42), deleted.Id, "deleted.Id must mirror the supplied task id")
		assert.Equal(t, uint64(99), deleted.Version, "deleted.Version must mirror the supplied version")
	})

	t.Run("swallows_sender_error_and_returns_nil", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.sender.sendErr = errTestSenderSend

		// ACT
		err := h.dispatcher.DispatchDelete(context.Background(), 42, 7, 99)

		// ASSERT
		require.NoError(t, err,
			"sender errors must be swallowed — daemon resyncs on reconnect")
	})
}

func TestDispatcher_HandleExecutionStarted(t *testing.T) {
	t.Parallel()

	t.Run("creates_execution_with_running_status", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		execID := xid.New()
		evt := &proto.ServerTaskExecutionStarted{
			ExecutionId: execID.String(),
			TaskId:      5,
			ServerId:    100,
			NodeId:      7,
			Command:     proto.ServerTaskCommand_SERVER_TASK_COMMAND_START,
			TaskVersion: 3,
		}

		// ACT
		err := h.dispatcher.HandleExecutionStarted(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		creates := h.execRepo.snapshotCreate()
		require.Len(t, creates, 1, "exactly one Create call is expected")

		exec := creates[0]
		assert.Equal(t, execID, exec.ExecutionID, "ExecutionID must mirror the proto event")
		assert.Equal(t, uint(5), exec.ServerTaskID, "ServerTaskID must mirror the proto TaskId")
		assert.Equal(t, uint(100), exec.ServerID, "ServerID must mirror the proto ServerId")
		assert.Equal(t, uint(7), exec.NodeID, "NodeID must mirror the proto NodeId")
		assert.Equal(t, domain.ServerTaskCommandStart, exec.Command, "Command must be converted from proto")
		assert.Equal(t, uint64(3), exec.TaskVersion, "TaskVersion must mirror the proto TaskVersion")
		assert.Equal(t, domain.ServerTaskExecutionStatusRunning, exec.Status,
			"status must always be Running for a fresh start event")
	})

	t.Run("uses_evt_started_at_when_provided", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		expected := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		evt := &proto.ServerTaskExecutionStarted{
			ExecutionId: xid.New().String(),
			TaskId:      5,
			NodeId:      7,
			StartedAt:   timestamppb.New(expected),
		}

		// ACT
		err := h.dispatcher.HandleExecutionStarted(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		creates := h.execRepo.snapshotCreate()
		require.Len(t, creates, 1)
		assert.True(t, creates[0].StartedAt.Equal(expected),
			"StartedAt must use the evt-provided timestamp")
	})

	t.Run("falls_back_to_now_when_started_at_nil", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		before := time.Now()
		evt := &proto.ServerTaskExecutionStarted{
			ExecutionId: xid.New().String(),
			TaskId:      5,
			NodeId:      7,
			StartedAt:   nil,
		}

		// ACT
		err := h.dispatcher.HandleExecutionStarted(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		creates := h.execRepo.snapshotCreate()
		require.Len(t, creates, 1)
		assert.WithinDuration(t, before, creates[0].StartedAt, 5*time.Second,
			"StartedAt must default to the wall clock when evt does not provide one")
	})

	t.Run("publishes_started_status", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		const taskID uint64 = 5
		execID := xid.New()
		captured, mu := captureChannel(t, h.mem, channels.BuildRealtimeServerTaskExecutionChannel(taskID))

		evt := &proto.ServerTaskExecutionStarted{
			ExecutionId: execID.String(),
			TaskId:      taskID,
			ServerId:    100,
			NodeId:      7,
		}

		// ACT
		err := h.dispatcher.HandleExecutionStarted(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		require.True(t, waitForOneMessage(captured, mu),
			"a status message should be published on the realtime channel")

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, *captured, 1)
		msg := (*captured)[0]
		assert.Equal(t, messages.TypeServerTaskExecutionStarted, msg.Type,
			"published message type must be the started variant")

		payload, parseErr := messages.ParsePayload[messages.ServerTaskExecutionStatusPayload](msg)
		require.NoError(t, parseErr)
		assert.Equal(t, execID.String(), payload.ExecutionID)
		assert.Equal(t, taskID, payload.TaskID)
		assert.Equal(t, uint64(100), payload.ServerID)
		assert.Equal(t, uint64(7), payload.NodeID)
		assert.Equal(t, string(domain.ServerTaskExecutionStatusRunning), payload.Status,
			"payload Status must reflect the Running status used at start")
	})

	t.Run("returns_error_when_execution_id_invalid", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		evt := &proto.ServerTaskExecutionStarted{
			ExecutionId: "not-a-real-xid",
			TaskId:      5,
			NodeId:      7,
		}

		// ACT
		err := h.dispatcher.HandleExecutionStarted(context.Background(), 7, evt)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse execution id",
			"invalid xid must surface as a 'parse execution id' error")
		require.Empty(t, h.execRepo.snapshotCreate(), "Create must not run for invalid execution id")
	})

	t.Run("wraps_repo_create_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		const taskID uint64 = 5
		captured, mu := captureChannel(t, h.mem, channels.BuildRealtimeServerTaskExecutionChannel(taskID))

		h.execRepo.createErr = errTestRepoCreate
		evt := &proto.ServerTaskExecutionStarted{
			ExecutionId: xid.New().String(),
			TaskId:      taskID,
			NodeId:      7,
		}

		// ACT
		err := h.dispatcher.HandleExecutionStarted(context.Background(), 7, evt)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "persist execution started",
			"Create errors must wrap with 'persist execution started'")

		mu.Lock()
		defer mu.Unlock()
		assert.Empty(t, *captured,
			"publisher must NOT be called when Create errors out")
	})

	t.Run("does_not_panic_when_publisher_is_nil", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		sender := &fakeSender{}
		serverRepo := &fakeServerTaskRepo{}
		execRepo := &fakeExecutionRepo{}
		d := NewDispatcher(sender, serverRepo, execRepo, nil, discardLogger())

		evt := &proto.ServerTaskExecutionStarted{
			ExecutionId: xid.New().String(),
			TaskId:      5,
			NodeId:      7,
		}

		// ACT
		require.NotPanics(t, func() {
			err := d.HandleExecutionStarted(context.Background(), 7, evt)
			require.NoError(t, err)
		}, "nil publisher must never trigger a panic")

		// ASSERT
		require.Len(t, execRepo.snapshotCreate(), 1,
			"Create must still run even when the publisher is nil")
	})
}

func TestDispatcher_HandleExecutionFinished(t *testing.T) {
	t.Parallel()

	t.Run("builds_finish_patch_with_all_fields", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		execID := xid.New()
		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId:       execID.String(),
			TaskId:            5,
			Status:            proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_FAILED,
			ExitCode:          137,
			ErrorMessage:      "oom",
			Duration:          durationpb.New(5 * time.Second),
			OutputInline:      []byte("hello"),
			OutputStreamed:    true,
			OutputStoragePath: "s3://bucket/path",
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		updates := h.execRepo.snapshotUpdateFinish()
		require.Len(t, updates, 1)
		patch := updates[0].patch

		assert.Equal(t, execID, updates[0].id, "UpdateFinish must target the parsed exec id")
		assert.Equal(t, domain.ServerTaskExecutionStatusFailed, patch.Status,
			"Status must convert to domain.FAILED")

		require.NotNil(t, patch.ExitCode, "ExitCode pointer must be populated when exit code is non-zero")
		assert.Equal(t, 137, *patch.ExitCode, "ExitCode must mirror the proto value")

		require.NotNil(t, patch.ErrorMessage, "ErrorMessage pointer must be set for non-empty input")
		assert.Equal(t, "oom", *patch.ErrorMessage)

		require.NotNil(t, patch.DurationMS, "DurationMS pointer must be set when duration is provided")
		assert.Equal(t, int64(5000), *patch.DurationMS, "DurationMS must mirror duration in milliseconds")

		require.NotNil(t, patch.OutputInline, "OutputInline pointer must be set for non-empty input")
		assert.Equal(t, "hello", *patch.OutputInline)

		require.NotNil(t, patch.OutputStoragePath, "OutputStoragePath must be set when streamed and path non-empty")
		assert.Equal(t, "s3://bucket/path", *patch.OutputStoragePath)
	})

	t.Run("sets_exit_code_when_zero_and_status_is_success", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId: xid.New().String(),
			TaskId:      5,
			Status:      proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
			ExitCode:    0,
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		updates := h.execRepo.snapshotUpdateFinish()
		require.Len(t, updates, 1)
		require.NotNil(t, updates[0].patch.ExitCode,
			"ExitCode pointer must be populated when status is SUCCESS even if value is 0")
		assert.Equal(t, 0, *updates[0].patch.ExitCode, "ExitCode value must be 0")
	})

	t.Run("leaves_exit_code_nil_when_zero_and_status_failed", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId: xid.New().String(),
			TaskId:      5,
			Status:      proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_FAILED,
			ExitCode:    0,
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		updates := h.execRepo.snapshotUpdateFinish()
		require.Len(t, updates, 1)
		assert.Nil(t, updates[0].patch.ExitCode,
			"ExitCode must stay nil when status is FAILED and exit code is the zero value")
	})

	t.Run("omits_error_message_when_empty", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId:  xid.New().String(),
			TaskId:       5,
			Status:       proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
			ErrorMessage: "",
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		updates := h.execRepo.snapshotUpdateFinish()
		require.Len(t, updates, 1)
		assert.Nil(t, updates[0].patch.ErrorMessage,
			"ErrorMessage must stay nil for empty input — the DB column is left untouched")
	})

	t.Run("omits_storage_path_when_not_streamed", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId:       xid.New().String(),
			TaskId:            5,
			Status:            proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
			OutputStreamed:    false,
			OutputStoragePath: "s3://bucket/orphan",
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		updates := h.execRepo.snapshotUpdateFinish()
		require.Len(t, updates, 1)
		assert.Nil(t, updates[0].patch.OutputStoragePath,
			"OutputStoragePath must be ignored when OutputStreamed is false")
	})

	t.Run("omits_storage_path_when_path_empty", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId:       xid.New().String(),
			TaskId:            5,
			Status:            proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
			OutputStreamed:    true,
			OutputStoragePath: "",
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		updates := h.execRepo.snapshotUpdateFinish()
		require.Len(t, updates, 1)
		assert.Nil(t, updates[0].patch.OutputStoragePath,
			"OutputStoragePath must stay nil when path string is empty even if streamed")
	})

	t.Run("truncates_output_inline_to_64KB", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		const inputLen = 70000
		input := strings.Repeat("a", inputLen-1) + "Z"
		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId:  xid.New().String(),
			TaskId:       5,
			Status:       proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
			OutputInline: []byte(input),
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		updates := h.execRepo.snapshotUpdateFinish()
		require.Len(t, updates, 1)
		require.NotNil(t, updates[0].patch.OutputInline)

		const maxBytes = 64 * 1024
		got := *updates[0].patch.OutputInline
		assert.Len(t, got, maxBytes, "truncated output must equal the configured maximum")
		assert.Equal(t, input[inputLen-maxBytes:], got, "truncated output must retain the tail of the input")
	})

	t.Run("uses_evt_finished_at_when_provided", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		expected := time.Date(2024, 7, 4, 8, 0, 0, 0, time.UTC)
		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId: xid.New().String(),
			TaskId:      5,
			Status:      proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
			FinishedAt:  timestamppb.New(expected),
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		updates := h.execRepo.snapshotUpdateFinish()
		require.Len(t, updates, 1)
		assert.True(t, updates[0].patch.FinishedAt.Equal(expected),
			"FinishedAt must reflect the evt-provided timestamp")
	})

	t.Run("falls_back_to_now_for_finished_at", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		before := time.Now()
		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId: xid.New().String(),
			TaskId:      5,
			Status:      proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
			FinishedAt:  nil,
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		updates := h.execRepo.snapshotUpdateFinish()
		require.Len(t, updates, 1)
		assert.WithinDuration(t, before, updates[0].patch.FinishedAt, 5*time.Second,
			"FinishedAt must fall back to the wall clock when evt does not provide one")
	})

	t.Run("publishes_finished_status_on_realtime_channel", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		const taskID uint64 = 9
		const nodeID uint64 = 7
		execID := xid.New()
		captured, mu := captureChannel(t, h.mem, channels.BuildRealtimeServerTaskExecutionChannel(taskID))

		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId:  execID.String(),
			TaskId:       taskID,
			Status:       proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_FAILED,
			ExitCode:     2,
			ErrorMessage: "boom",
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), nodeID, evt)

		// ASSERT
		require.NoError(t, err)
		require.True(t, waitForOneMessage(captured, mu),
			"a finished status message should be published on the realtime channel")

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, *captured, 1)
		msg := (*captured)[0]
		assert.Equal(t, messages.TypeServerTaskExecutionFinished, msg.Type,
			"published message type must be the finished variant")

		payload, parseErr := messages.ParsePayload[messages.ServerTaskExecutionStatusPayload](msg)
		require.NoError(t, parseErr)
		assert.Equal(t, execID.String(), payload.ExecutionID)
		assert.Equal(t, taskID, payload.TaskID)
		assert.Equal(t, nodeID, payload.NodeID, "payload NodeID must mirror the bound node")
		assert.Equal(t, string(domain.ServerTaskExecutionStatusFailed), payload.Status)
		require.NotNil(t, payload.ExitCode, "ExitCode must round-trip into the realtime payload")
		assert.Equal(t, int32(2), *payload.ExitCode)
		assert.Equal(t, "boom", payload.ErrorMessage, "ErrorMessage must round-trip into the realtime payload")
	})

	t.Run("sends_execution_ack_to_node", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		execID := xid.New()
		const nodeID uint64 = 7
		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId: execID.String(),
			TaskId:      5,
			Status:      proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), nodeID, evt)

		// ASSERT
		require.NoError(t, err)
		calls := h.sender.snapshot()
		require.Len(t, calls, 1, "exactly one SendTask call is expected (the ack)")
		assert.Equal(t, nodeID, calls[0].nodeID, "ack must be sent to the originating node")

		ack := calls[0].msg.GetServerTaskExecutionAck()
		require.NotNil(t, ack, "payload must be a ServerTaskExecutionAck")
		assert.Equal(t, execID.String(), ack.ExecutionId,
			"ack must reference the finished execution")
	})

	t.Run("returns_error_when_execution_id_invalid", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId: "garbage",
			TaskId:      5,
			Status:      proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse execution id",
			"invalid xid must surface as a 'parse execution id' error")
		require.Empty(t, h.execRepo.snapshotUpdateFinish(),
			"UpdateFinish must not run for invalid execution id")
		require.Empty(t, h.sender.snapshot(),
			"ack must not be sent for invalid execution id")
	})

	t.Run("wraps_repo_update_finish_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.execRepo.updateFinishErr = errTestRepoUpdate
		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId: xid.New().String(),
			TaskId:      5,
			Status:      proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "persist execution finish",
			"UpdateFinish errors must wrap with 'persist execution finish'")
		require.Empty(t, h.sender.snapshot(),
			"ack must NOT be sent when persistence fails")
	})

	t.Run("does_not_surface_publisher_or_sender_errors", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.sender.sendErr = errTestSenderSend

		evt := &proto.ServerTaskExecutionFinished{
			ExecutionId: xid.New().String(),
			TaskId:      5,
			Status:      proto.ServerTaskExecutionStatus_SERVER_TASK_EXECUTION_STATUS_SUCCESS,
		}

		// ACT
		err := h.dispatcher.HandleExecutionFinished(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err,
			"ack-send failures must not surface — only persistence failures abort the flow")
		require.Len(t, h.execRepo.snapshotUpdateFinish(), 1,
			"UpdateFinish must have run before the ack attempt failed")
	})
}

func TestDispatcher_HandleExecutionLog(t *testing.T) {
	t.Parallel()

	t.Run("early_returns_when_chunk_empty_and_not_final", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		execID := xid.New()
		captured, mu := captureChannel(t, h.mem, channels.BuildRealtimeServerTaskLogChannel(execID.String()))

		evt := &proto.ServerTaskExecutionLog{
			ExecutionId: execID.String(),
			Chunk:       nil,
			IsFinal:     false,
		}

		// ACT
		err := h.dispatcher.HandleExecutionLog(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		require.Empty(t, h.execRepo.snapshotAppendOutput(),
			"AppendOutputInline must not run for an empty non-final log")

		mu.Lock()
		defer mu.Unlock()
		assert.Empty(t, *captured,
			"publisher must not be called for an empty non-final log")
	})

	t.Run("appends_chunk_to_execution_output", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		execID := xid.New()
		evt := &proto.ServerTaskExecutionLog{
			ExecutionId: execID.String(),
			Chunk:       []byte("partial"),
			IsFinal:     false,
		}

		// ACT
		err := h.dispatcher.HandleExecutionLog(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		calls := h.execRepo.snapshotAppendOutput()
		require.Len(t, calls, 1, "exactly one AppendOutputInline call is expected")
		assert.Equal(t, execID, calls[0].id, "append must target the parsed execution id")
		assert.Equal(t, []byte("partial"), calls[0].chunk, "chunk bytes must reach the repo verbatim")
		assert.Equal(t, 64*1024, calls[0].maxBytes,
			"maxBytes must equal the inline-output cap")
	})

	t.Run("publishes_log_payload_on_realtime_log_channel", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		execID := xid.New()
		captured, mu := captureChannel(t, h.mem, channels.BuildRealtimeServerTaskLogChannel(execID.String()))

		evt := &proto.ServerTaskExecutionLog{
			ExecutionId: execID.String(),
			Sequence:    42,
			Chunk:       []byte("line"),
			IsFinal:     true,
		}

		// ACT
		err := h.dispatcher.HandleExecutionLog(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err)
		require.True(t, waitForOneMessage(captured, mu),
			"a log payload should be published on the realtime log channel")

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, *captured, 1)
		msg := (*captured)[0]
		assert.Equal(t, messages.TypeServerTaskExecutionLog, msg.Type,
			"published message type must be the execution-log variant")

		payload, parseErr := messages.ParsePayload[messages.ServerTaskExecutionLogPayload](msg)
		require.NoError(t, parseErr)
		assert.Equal(t, execID.String(), payload.ExecutionID, "payload ExecutionID must mirror the input")
		assert.Equal(t, uint64(42), payload.Sequence, "payload Sequence must mirror the input")
		assert.Equal(t, []byte("line"), payload.Chunk, "payload Chunk must mirror the input")
		assert.True(t, payload.IsFinal, "IsFinal=true must reach subscribers verbatim")
	})

	t.Run("returns_error_when_execution_id_invalid", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		evt := &proto.ServerTaskExecutionLog{
			ExecutionId: "not-xid",
			Chunk:       []byte("data"),
			IsFinal:     false,
		}

		// ACT
		err := h.dispatcher.HandleExecutionLog(context.Background(), 7, evt)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse execution id",
			"invalid xid must surface as a 'parse execution id' error")
		require.Empty(t, h.execRepo.snapshotAppendOutput(),
			"AppendOutputInline must not run for invalid execution id")
	})

	t.Run("swallows_append_error_and_continues_to_publish", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		execID := xid.New()
		captured, mu := captureChannel(t, h.mem, channels.BuildRealtimeServerTaskLogChannel(execID.String()))

		h.execRepo.appendOutputErr = errTestRepoAppend
		evt := &proto.ServerTaskExecutionLog{
			ExecutionId: execID.String(),
			Chunk:       []byte("data"),
		}

		// ACT
		err := h.dispatcher.HandleExecutionLog(context.Background(), 7, evt)

		// ASSERT
		require.NoError(t, err,
			"append errors must be swallowed — log fan-out is best-effort")
		require.True(t, waitForOneMessage(captured, mu),
			"the realtime log message must still be published even when append fails")
	})
}

func TestDispatcher_HandleResyncRequest(t *testing.T) {
	t.Parallel()

	t.Run("builds_snapshot_and_sends_to_node", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.serverRepo.findResult = []domain.ServerTask{{
			ID:       11,
			NodeID:   new(uint(7)),
			ServerID: 100,
			Command:  domain.ServerTaskCommandRestart,
			Enabled:  true,
		}}

		// ACT
		err := h.dispatcher.HandleResyncRequest(context.Background(), 7, &proto.ServerTaskResyncRequest{})

		// ASSERT
		require.NoError(t, err)
		calls := h.sender.snapshot()
		require.Len(t, calls, 1, "exactly one SendTask call is expected (the snapshot)")
		assert.Equal(t, uint64(7), calls[0].nodeID, "snapshot must be sent to the requesting node")

		snap := calls[0].msg.GetServerTaskSnapshot()
		require.NotNil(t, snap, "payload must be a ServerTaskSnapshot")
		require.Len(t, snap.Tasks, 1, "snapshot must include the matching server task")
		assert.Equal(t, uint64(11), snap.Tasks[0].Id, "snapshot task ID must mirror the domain task")
	})

	t.Run("wraps_build_snapshot_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.serverRepo.findErr = errTestRepoBuildSnap

		// ACT
		err := h.dispatcher.HandleResyncRequest(context.Background(), 7, &proto.ServerTaskResyncRequest{})

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "build snapshot for resync",
			"snapshot-build failures must wrap with 'build snapshot for resync'")
		require.Empty(t, h.sender.snapshot(),
			"sender must not be called when snapshot-build fails")
	})

	t.Run("wraps_sender_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.sender.sendErr = errTestSenderSend

		// ACT
		err := h.dispatcher.HandleResyncRequest(context.Background(), 7, &proto.ServerTaskResyncRequest{})

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "send snapshot after resync request",
			"sender failures must wrap with 'send snapshot after resync request' "+
				"(resync is the one method that surfaces send errors)")
	})
}

func TestDispatcher_ReconcileWorkingExecutions(t *testing.T) {
	t.Parallel()

	t.Run("defaults_reason_when_empty", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		// ACT
		n, err := h.dispatcher.ReconcileWorkingExecutions(context.Background(), 7, nil, "")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, 0, n)
		calls := h.execRepo.snapshotMarkAbandoned()
		require.Len(t, calls, 1)
		assert.Equal(t, domain.ExecutionAbandonedReasonDaemonRestart, calls[0].reason,
			"empty reason must default to the daemon-restart constant")
	})

	t.Run("passes_explicit_reason", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		// ACT
		_, err := h.dispatcher.ReconcileWorkingExecutions(context.Background(), 7, nil, "custom_reason")

		// ASSERT
		require.NoError(t, err)
		calls := h.execRepo.snapshotMarkAbandoned()
		require.Len(t, calls, 1)
		assert.Equal(t, "custom_reason", calls[0].reason,
			"non-empty reason must be passed through verbatim")
	})

	t.Run("skips_malformed_execution_ids", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		valid1 := xid.New().String()
		valid2 := xid.New().String()

		// ACT
		_, err := h.dispatcher.ReconcileWorkingExecutions(
			context.Background(), 7, []string{valid1, "not-xid", valid2}, "",
		)

		// ASSERT
		require.NoError(t, err)
		calls := h.execRepo.snapshotMarkAbandoned()
		require.Len(t, calls, 1)
		require.Len(t, calls[0].keepIDs, 2,
			"malformed ids must be silently dropped, keeping only parsable ids")
		assert.Equal(t, valid1, calls[0].keepIDs[0].String())
		assert.Equal(t, valid2, calls[0].keepIDs[1].String())
	})

	t.Run("returns_count_from_repo", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.execRepo.markAbandonedN = 4

		// ACT
		n, err := h.dispatcher.ReconcileWorkingExecutions(context.Background(), 7, nil, "")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, 4, n, "the repo-reported count must reach the caller unchanged")
	})

	t.Run("wraps_mark_abandoned_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.execRepo.markAbandonedErr = errTestRepoAbandon

		// ACT
		n, err := h.dispatcher.ReconcileWorkingExecutions(context.Background(), 7, nil, "")

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mark abandoned executions",
			"MarkAbandoned errors must wrap with 'mark abandoned executions'")
		assert.Equal(t, 0, n, "count must be zero when the repo errors")
	})
}

func TestDispatcher_CancelExecution(t *testing.T) {
	t.Parallel()

	t.Run("sends_cancel_message_to_node", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		const nodeID uint64 = 7
		const taskID uint64 = 5
		execID := xid.New().String()

		// ACT
		err := h.dispatcher.CancelExecution(context.Background(), execID, taskID, nodeID, "user_request")

		// ASSERT
		require.NoError(t, err)
		calls := h.sender.snapshot()
		require.Len(t, calls, 1)
		assert.Equal(t, nodeID, calls[0].nodeID, "cancel must be sent to the supplied node")

		cancel := calls[0].msg.GetServerTaskExecutionCancel()
		require.NotNil(t, cancel, "payload must be a ServerTaskExecutionCancel")
		assert.Equal(t, execID, cancel.ExecutionId, "ExecutionId must mirror the request")
		assert.Equal(t, taskID, cancel.TaskId, "TaskId must mirror the request")
		assert.Equal(t, "user_request", cancel.Reason, "Reason must mirror the request")
	})

	t.Run("wraps_sender_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		h := newTestDispatcher(t)
		defer h.cleanup()

		h.sender.sendErr = errTestSenderSend

		// ACT
		err := h.dispatcher.CancelExecution(context.Background(), xid.New().String(), 5, 7, "stop")

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "send execution cancel",
			"sender failures must wrap with 'send execution cancel'")
	})
}

func TestTruncateTail(t *testing.T) {
	t.Parallel()

	t.Run("returns_input_when_max_bytes_zero", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := "hello"

		// ACT
		got := truncateTail(input, 0)

		// ASSERT
		assert.Equal(t, input, got, "maxBytes=0 must leave the input intact")
	})

	t.Run("returns_input_when_max_bytes_negative", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := "hello"

		// ACT
		got := truncateTail(input, -1)

		// ASSERT
		assert.Equal(t, input, got, "negative maxBytes must leave the input intact")
	})

	t.Run("returns_input_when_shorter_than_max", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := "abc"

		// ACT
		got := truncateTail(input, 10)

		// ASSERT
		assert.Equal(t, input, got, "shorter inputs must pass through unchanged")
	})

	t.Run("returns_input_when_equal", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := "abc"

		// ACT
		got := truncateTail(input, 3)

		// ASSERT
		assert.Equal(t, input, got, "inputs equal to maxBytes must pass through unchanged")
	})

	t.Run("returns_tail_when_longer_than_max", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		input := "abcdef"

		// ACT
		got := truncateTail(input, 3)

		// ASSERT
		assert.Equal(t, "def", got, "longer inputs must be truncated to the tail")
	})

	t.Run("result_is_valid_utf8_when_cut_lands_mid_rune", func(t *testing.T) {
		t.Parallel()

		// "🚀" is 4 bytes; a byte cut that starts inside it must not yield
		// invalid UTF-8 (Postgres text columns reject invalid byte sequences).
		input := "ab🚀cd"

		for maxBytes := 1; maxBytes <= len(input); maxBytes++ {
			got := truncateTail(input, maxBytes)
			assert.True(t, utf8.ValidString(got),
				"truncateTail(%q, %d) = %q must be valid UTF-8", input, maxBytes, got)
			assert.True(t, strings.HasSuffix(input, got),
				"result must remain a suffix of the input")
		}
	})
}
