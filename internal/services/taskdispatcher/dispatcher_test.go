package taskdispatcher

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errTestPersist    = errors.New("simulated persist failure")
	errTestRepoFind   = errors.New("simulated repo find failure")
	errTestRepoSave   = errors.New("simulated repo save failure")
	errTestStreamSend = errors.New("simulated stream send failure")
	errTestRepoAppend = errors.New("simulated append output failure")
)

// fakeStream implements session.Stream. The dispatcher only uses Send; Recv and
// Context are stubbed so the Stream contract is satisfied.
type fakeStream struct {
	mu      sync.Mutex
	sent    []*proto.GatewayMessage
	sendErr error
	ctx     context.Context //nolint:containedctx // test stub for the Stream interface
}

func newFakeStream() *fakeStream {
	return &fakeStream{ctx: context.Background()}
}

func (s *fakeStream) Send(msg *proto.GatewayMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sendErr != nil {
		return s.sendErr
	}

	s.sent = append(s.sent, msg)

	return nil
}

func (s *fakeStream) Recv() (*proto.DaemonMessage, error) {
	<-s.ctx.Done()

	return nil, s.ctx.Err()
}

func (s *fakeStream) Context() context.Context {
	return s.ctx
}

func (s *fakeStream) Sent() []*proto.GatewayMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*proto.GatewayMessage, len(s.sent))
	copy(out, s.sent)

	return out
}

func (s *fakeStream) setSendErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sendErr = err
}

// fakeDaemonTaskRepo implements repositories.DaemonTaskRepository.
type fakeDaemonTaskRepo struct {
	mu sync.Mutex

	findResult []domain.DaemonTask
	findErr    error
	findFilter *filters.FindDaemonTask

	saveErr   error
	saveCalls int
	lastSaved *domain.DaemonTask

	appendOutputErr   error
	appendOutputCalls []appendOutputCall
}

type appendOutputCall struct {
	ID     uint
	Output string
}

func (r *fakeDaemonTaskRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.DaemonTask, error) {
	return nil, nil
}

func (r *fakeDaemonTaskRepo) Find(
	_ context.Context, filter *filters.FindDaemonTask, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.DaemonTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.findFilter = filter
	if r.findErr != nil {
		return nil, r.findErr
	}

	out := make([]domain.DaemonTask, len(r.findResult))
	copy(out, r.findResult)

	return out, nil
}

func (r *fakeDaemonTaskRepo) FindWithOutput(
	_ context.Context, _ *filters.FindDaemonTask, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.DaemonTask, error) {
	return nil, nil
}

func (r *fakeDaemonTaskRepo) Count(_ context.Context, _ *filters.FindDaemonTask) (int, error) {
	return 0, nil
}

func (r *fakeDaemonTaskRepo) Save(_ context.Context, task *domain.DaemonTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.saveCalls++
	if r.saveErr != nil {
		return r.saveErr
	}

	clone := *task
	r.lastSaved = &clone

	return nil
}

func (r *fakeDaemonTaskRepo) Delete(_ context.Context, _ uint) error {
	return nil
}

func (r *fakeDaemonTaskRepo) Exists(_ context.Context, _ *filters.FindDaemonTask) (bool, error) {
	return false, nil
}

func (r *fakeDaemonTaskRepo) AppendOutput(_ context.Context, id uint, output string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.appendOutputErr != nil {
		return r.appendOutputErr
	}

	r.appendOutputCalls = append(r.appendOutputCalls, appendOutputCall{ID: id, Output: output})

	return nil
}

func (r *fakeDaemonTaskRepo) snapshot() (saveCalls int, lastSaved *domain.DaemonTask, appendCalls []appendOutputCall) {
	r.mu.Lock()
	defer r.mu.Unlock()

	calls := make([]appendOutputCall, len(r.appendOutputCalls))
	copy(calls, r.appendOutputCalls)

	var lastSavedCopy *domain.DaemonTask
	if r.lastSaved != nil {
		clone := *r.lastSaved
		lastSavedCopy = &clone
	}

	return r.saveCalls, lastSavedCopy, calls
}

// fakeServerRepo implements repositories.ServerRepository.
type fakeServerRepo struct {
	findResult []domain.Server
	findErr    error
	findFilter *filters.FindServer
}

func (r *fakeServerRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Server, error) {
	return nil, nil
}

func (r *fakeServerRepo) Find(
	_ context.Context, filter *filters.FindServer, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Server, error) {
	r.findFilter = filter

	return r.findResult, r.findErr
}

func (r *fakeServerRepo) Count(_ context.Context, _ *filters.FindServer) (int, error) {
	return 0, nil
}

func (r *fakeServerRepo) FindUserServers(
	_ context.Context, _ uint, _ *filters.FindServer, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Server, error) {
	return nil, nil
}

func (r *fakeServerRepo) Save(_ context.Context, _ *domain.Server) error           { return nil }
func (r *fakeServerRepo) SaveBulk(_ context.Context, _ []*domain.Server) error     { return nil }
func (r *fakeServerRepo) Delete(_ context.Context, _ uint) error                   { return nil }
func (r *fakeServerRepo) SoftDelete(_ context.Context, _ uint) error               { return nil }
func (r *fakeServerRepo) SetUserServers(_ context.Context, _ uint, _ []uint) error { return nil }
func (r *fakeServerRepo) Exists(_ context.Context, _ *filters.FindServer) (bool, error) {
	return false, nil
}
func (r *fakeServerRepo) Search(_ context.Context, _ string) ([]*domain.Server, error) {
	return nil, nil
}
func (r *fakeServerRepo) UpdateServerStatuses(
	_ context.Context, _ uint, _ []repositories.ServerStatusUpdate,
) error {
	return nil
}

// fakeServerSettingRepo implements repositories.ServerSettingRepository.
type fakeServerSettingRepo struct {
	findResult []domain.ServerSetting
	findErr    error
	findFilter *filters.FindServerSetting
}

func (r *fakeServerSettingRepo) Find(
	_ context.Context, filter *filters.FindServerSetting, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.ServerSetting, error) {
	r.findFilter = filter

	return r.findResult, r.findErr
}

func (r *fakeServerSettingRepo) Save(_ context.Context, _ *domain.ServerSetting) error { return nil }
func (r *fakeServerSettingRepo) Delete(_ context.Context, _ uint) error                { return nil }

// fakeGameRepo implements repositories.GameRepository.
type fakeGameRepo struct {
	findResult []domain.Game
	findErr    error
}

func (r *fakeGameRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Game, error) {
	return nil, nil
}

func (r *fakeGameRepo) Find(
	_ context.Context, _ *filters.FindGame, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Game, error) {
	return r.findResult, r.findErr
}

func (r *fakeGameRepo) Save(_ context.Context, _ *domain.Game) error { return nil }
func (r *fakeGameRepo) Delete(_ context.Context, _ string) error     { return nil }

// fakeGameModRepo implements repositories.GameModRepository.
type fakeGameModRepo struct {
	findResult []domain.GameMod
	findErr    error
	findFilter *filters.FindGameMod
}

func (r *fakeGameModRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.GameMod, error) {
	return nil, nil
}

func (r *fakeGameModRepo) Find(
	_ context.Context, filter *filters.FindGameMod, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.GameMod, error) {
	r.findFilter = filter

	return r.findResult, r.findErr
}

func (r *fakeGameModRepo) Save(_ context.Context, _ *domain.GameMod) error { return nil }
func (r *fakeGameModRepo) Delete(_ context.Context, _ uint) error          { return nil }

// fakeNodeRepo implements repositories.NodeRepository.
type fakeNodeRepo struct {
	findResult []domain.Node
	findErr    error
	findFilter *filters.FindNode
}

func (r *fakeNodeRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Node, error) {
	return nil, nil
}

func (r *fakeNodeRepo) Find(
	_ context.Context, filter *filters.FindNode, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.Node, error) {
	r.findFilter = filter

	return r.findResult, r.findErr
}

func (r *fakeNodeRepo) Save(_ context.Context, _ *domain.Node) error { return nil }
func (r *fakeNodeRepo) UpdateGDaemonAPIToken(_ context.Context, _ uint, _ string, _ time.Time) error {
	return nil
}
func (r *fakeNodeRepo) Delete(_ context.Context, _ uint) error { return nil }

type testRepos struct {
	daemonTask    *fakeDaemonTaskRepo
	server        *fakeServerRepo
	serverSetting *fakeServerSettingRepo
	game          *fakeGameRepo
	gameMod       *fakeGameModRepo
	node          *fakeNodeRepo
}

type testHarness struct {
	dispatcher *Dispatcher
	mem        *memory.Memory
	registry   *session.Registry
	repos      testRepos
	cleanup    func()
}

// newTestDispatcher constructs a Dispatcher backed by a real in-memory pubsub
// and registry plus hand-rolled repository fakes. The returned cleanup function
// must be deferred to release goroutines and close the pubsub.
func newTestDispatcher(t *testing.T) *testHarness {
	t.Helper()

	mem := memory.New()
	ctx, cancel := context.WithCancel(context.Background())

	registry := session.NewRegistry(mem, "test-instance", discardLogger())
	require.NoError(t, registry.Start(ctx), "registry.Start should succeed")

	repos := testRepos{
		daemonTask:    &fakeDaemonTaskRepo{},
		server:        &fakeServerRepo{},
		serverSetting: &fakeServerSettingRepo{},
		game:          &fakeGameRepo{},
		gameMod:       &fakeGameModRepo{},
		node:          &fakeNodeRepo{},
	}

	d := NewDispatcher(
		registry,
		repos.daemonTask,
		repos.server,
		repos.serverSetting,
		repos.game,
		repos.gameMod,
		repos.node,
		mem,
		discardLogger(),
	)

	cleanup := func() {
		cancel()
		_ = mem.Close()
	}

	return &testHarness{
		dispatcher: d,
		mem:        mem,
		registry:   registry,
		repos:      repos,
		cleanup:    cleanup,
	}
}

// registerSession installs a Session for nodeID with the supplied stream.
func registerSession(t *testing.T, registry *session.Registry, nodeID uint64, stream session.Stream) {
	t.Helper()

	sess := session.NewSession(nodeID, stream, "1.0.0", []string{}, func() {})
	require.NoError(t, registry.Register(context.Background(), sess), "registry.Register should succeed")
}

// captureChannel subscribes a collector handler on the exact channel and
// returns a thread-safe slice plus its mutex. Use waitForMessages to await
// async publishes from the dispatcher.
func captureChannel(t *testing.T, mem *memory.Memory, channel string) (*[]*pubsub.Message, *sync.Mutex) {
	t.Helper()

	var mu sync.Mutex
	got := make([]*pubsub.Message, 0)

	require.NoError(t, mem.Subscribe(context.Background(), channel, func(_ context.Context, msg *pubsub.Message) error {
		mu.Lock()
		defer mu.Unlock()

		got = append(got, msg)

		return nil
	}), "captureChannel subscribe should succeed")

	return &got, &mu
}

// waitForMessages polls the captured slice until it contains at least n entries
// or the deadline passes. memory.Memory dispatches synchronously so this is
// usually instant, but the helper guards against future async behavior.
func waitForMessages(captured *[]*pubsub.Message, mu *sync.Mutex, n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		l := len(*captured)
		mu.Unlock()
		if l >= n {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	return len(*captured) >= n
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestDispatch_PersistenceErrorPropagates(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	h.repos.daemonTask.saveErr = errTestPersist
	task := &domain.DaemonTask{
		DedicatedServerID: 1,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
	}

	// ACT
	err := h.dispatcher.Dispatch(context.Background(), task)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persist task", "err should wrap with 'persist task'")
}

func TestHandleTaskStatusUpdate_IgnoresUnknownTaskID(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	h.repos.daemonTask.findResult = nil

	// ACT
	err := h.dispatcher.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
		TaskId: 99,
		Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
	})

	// ASSERT
	require.NoError(t, err)
	saveCalls, _, _ := h.repos.daemonTask.snapshot()
	assert.Equal(t, 0, saveCalls, "Save must not be called when task is unknown")
}

func TestHandleTaskStatusUpdate_WrongNodeIsIgnored(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	h.repos.daemonTask.findResult = []domain.DaemonTask{{
		ID:                5,
		DedicatedServerID: 99,
		Status:            domain.DaemonTaskStatusWaiting,
	}}

	// ACT
	err := h.dispatcher.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
		TaskId: 5,
		Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
	})

	// ASSERT
	require.NoError(t, err)
	saveCalls, _, _ := h.repos.daemonTask.snapshot()
	assert.Equal(t, 0, saveCalls, "Save must not be called for status updates from a different node")
}

func TestHandleTaskOutput_EmptyChunkIsNoop(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	// ACT
	err := h.dispatcher.HandleTaskOutput(context.Background(), 1, &proto.TaskOutput{
		TaskId:      5,
		OutputChunk: nil,
	})

	// ASSERT
	require.NoError(t, err)
	_, _, appendCalls := h.repos.daemonTask.snapshot()
	require.Empty(t, appendCalls, "AppendOutput must not be called for empty chunks")
}

func TestDispatch_PersistsTaskWhenSessionDisconnected(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	task := &domain.DaemonTask{
		DedicatedServerID: 7,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
	}

	// ACT
	err := h.dispatcher.Dispatch(context.Background(), task)

	// ASSERT
	require.NoError(t, err, "Dispatch must succeed even with no session — task is queued")
	saveCalls, _, _ := h.repos.daemonTask.snapshot()
	assert.GreaterOrEqual(t, saveCalls, 1, "task should be persisted at least once")
}

func TestDispatch_WithConnectedSessionSendsToStream(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const nodeID uint64 = 42
	stream := newFakeStream()
	registerSession(t, h.registry, nodeID, stream)

	task := &domain.DaemonTask{
		ID:                123,
		DedicatedServerID: uint(nodeID),
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
	}

	// ACT
	err := h.dispatcher.Dispatch(context.Background(), task)

	// ASSERT
	require.NoError(t, err)

	sent := stream.Sent()
	require.NotEmpty(t, sent, "the connected session should have received at least one message")

	var found bool
	for _, msg := range sent {
		if t := msg.GetTask(); t != nil && t.Id == 123 {
			found = true

			break
		}
	}
	assert.True(t, found, "stream should receive the dispatched task with matching ID")
}

func TestFlushPending_NoSessionReturnsError(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	// ACT
	err := h.dispatcher.FlushPending(context.Background(), 99)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found", "missing session must surface a 'session not found' error")
}

func TestFlushPending_SendsWaitingTasksAndFlipsToWorking(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const nodeID uint64 = 11
	stream := newFakeStream()
	registerSession(t, h.registry, nodeID, stream)

	h.repos.daemonTask.findResult = []domain.DaemonTask{
		{ID: 1, DedicatedServerID: uint(nodeID), Status: domain.DaemonTaskStatusWaiting, Task: domain.DaemonTaskTypeServerStart},
		{ID: 2, DedicatedServerID: uint(nodeID), Status: domain.DaemonTaskStatusWaiting, Task: domain.DaemonTaskTypeServerStop},
	}

	// ACT
	err := h.dispatcher.FlushPending(context.Background(), nodeID)

	// ASSERT
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(stream.Sent()), 2, "stream should receive at least one message per pending task")

	_, lastSaved, _ := h.repos.daemonTask.snapshot()
	require.NotNil(t, lastSaved, "Save should have been invoked")
	assert.Equal(t, domain.DaemonTaskStatusWorking, lastSaved.Status, "task status should flip to Working after dispatch")
}

func TestFlushPending_FilterShape(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const nodeID uint64 = 17
	stream := newFakeStream()
	registerSession(t, h.registry, nodeID, stream)

	// ACT
	err := h.dispatcher.FlushPending(context.Background(), nodeID)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, h.repos.daemonTask.findFilter, "Find should have been invoked")
	assert.Equal(t, []uint{uint(nodeID)}, h.repos.daemonTask.findFilter.DedicatedServerIDs,
		"filter.DedicatedServerIDs must equal the node being flushed")
	assert.Equal(t, []domain.DaemonTaskStatus{domain.DaemonTaskStatusWaiting}, h.repos.daemonTask.findFilter.Statuses,
		"filter.Statuses must restrict to Waiting tasks only")
}

func TestFlushPending_StreamSendErrorPropagates(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const nodeID uint64 = 21
	stream := newFakeStream()
	stream.setSendErr(errTestStreamSend)
	registerSession(t, h.registry, nodeID, stream)

	h.repos.daemonTask.findResult = []domain.DaemonTask{
		{ID: 5, DedicatedServerID: uint(nodeID), Status: domain.DaemonTaskStatusWaiting, Task: domain.DaemonTaskTypeServerStart},
	}

	// ACT
	err := h.dispatcher.FlushPending(context.Background(), nodeID)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "send pending task", "stream send failure must wrap as 'send pending task'")
}

func TestGetPendingTasks_ReturnsProtoTasks(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	h.repos.daemonTask.findResult = []domain.DaemonTask{
		{ID: 10, DedicatedServerID: 3, Status: domain.DaemonTaskStatusWaiting, Task: domain.DaemonTaskTypeServerStart},
		{ID: 11, DedicatedServerID: 3, Status: domain.DaemonTaskStatusWaiting, Task: domain.DaemonTaskTypeServerStop},
	}

	// ACT
	got, err := h.dispatcher.GetPendingTasks(context.Background(), 3)

	// ASSERT
	require.NoError(t, err)
	require.Len(t, got, 2, "two domain tasks should produce two proto tasks")
	assert.Equal(t, uint64(10), got[0].Id, "proto task IDs must mirror domain task IDs in order")
	assert.Equal(t, uint64(11), got[1].Id)
}

func TestGetPendingTasks_RepoErrorPropagates(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	h.repos.daemonTask.findErr = errTestRepoFind

	// ACT
	got, err := h.dispatcher.GetPendingTasks(context.Background(), 3)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "find pending tasks", "repo errors must wrap with 'find pending tasks'")
	assert.Nil(t, got, "no proto tasks should be returned when the repo errors")
}

func TestHandleTaskStatusUpdate_UpdatesStatusAndPublishes(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const taskID uint64 = 5
	const nodeID uint64 = 1

	h.repos.daemonTask.findResult = []domain.DaemonTask{{
		ID:                uint(taskID),
		DedicatedServerID: uint(nodeID),
		Status:            domain.DaemonTaskStatusWaiting,
	}}

	captured, mu := captureChannel(t, h.mem, channels.BuildRealtimeTaskStatusChannel(taskID))

	// ACT
	err := h.dispatcher.HandleTaskStatusUpdate(context.Background(), nodeID, &proto.TaskStatusUpdate{
		TaskId:  taskID,
		Status:  proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
		Message: "running now",
	})

	// ASSERT
	require.NoError(t, err)

	_, lastSaved, _ := h.repos.daemonTask.snapshot()
	require.NotNil(t, lastSaved, "Save should be invoked")
	assert.Equal(t, domain.DaemonTaskStatusWorking, lastSaved.Status, "status must be persisted from the update")

	require.True(t, waitForMessages(captured, mu, 1, 200*time.Millisecond),
		"a TaskStatus message should be published on the realtime channel")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *captured, 1)
	msg := (*captured)[0]
	assert.Equal(t, "task.status", msg.Type, "published message type must be task.status")

	payload, parseErr := messages.ParsePayload[messages.TaskStatusPayload](msg)
	require.NoError(t, parseErr)
	assert.Equal(t, taskID, payload.TaskID, "payload TaskID must mirror the update")
	assert.Equal(t, string(domain.DaemonTaskStatusWorking), payload.Status,
		"payload Status must reflect the persisted domain status")
	assert.Equal(t, "running now", payload.Message, "payload Message must mirror the update")
}

func TestHandleTaskStatusUpdate_RepoErrorPropagates(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	h.repos.daemonTask.findErr = errTestRepoFind

	// ACT
	err := h.dispatcher.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
		TaskId: 5,
		Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
	})

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "find task", "repo Find errors must wrap with 'find task'")
}

func TestHandleTaskStatusUpdate_SaveErrorPropagates(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	h.repos.daemonTask.findResult = []domain.DaemonTask{{
		ID:                5,
		DedicatedServerID: 1,
		Status:            domain.DaemonTaskStatusWaiting,
	}}
	h.repos.daemonTask.saveErr = errTestRepoSave

	// ACT
	err := h.dispatcher.HandleTaskStatusUpdate(context.Background(), 1, &proto.TaskStatusUpdate{
		TaskId: 5,
		Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
	})

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update task status", "Save errors must wrap with 'update task status'")
}

func TestHandleTaskOutput_AppendsAndPublishes(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const taskID uint64 = 5
	captured, mu := captureChannel(t, h.mem, channels.BuildRealtimeTaskOutputChannel(taskID))

	// ACT
	err := h.dispatcher.HandleTaskOutput(context.Background(), 1, &proto.TaskOutput{
		TaskId:      taskID,
		OutputChunk: []byte("hello"),
		IsFinal:     false,
	})

	// ASSERT
	require.NoError(t, err)

	_, _, appendCalls := h.repos.daemonTask.snapshot()
	require.Len(t, appendCalls, 1, "exactly one AppendOutput call expected")
	assert.Equal(t, uint(taskID), appendCalls[0].ID, "AppendOutput must receive the matching task ID")
	assert.Equal(t, "hello", appendCalls[0].Output, "AppendOutput must receive the chunk verbatim")

	require.True(t, waitForMessages(captured, mu, 1, 200*time.Millisecond),
		"a TaskOutput message should be published on the realtime channel")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *captured, 1)
	payload, parseErr := messages.ParsePayload[messages.TaskOutputPayload]((*captured)[0])
	require.NoError(t, parseErr)
	assert.Equal(t, taskID, payload.TaskID, "payload TaskID must mirror the input")
	assert.Equal(t, "hello", payload.Chunk, "payload Chunk must mirror the OutputChunk")
	assert.False(t, payload.IsFinal, "payload IsFinal must mirror the input")
}

func TestHandleTaskOutput_IsFinalPropagates(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const taskID uint64 = 6
	captured, mu := captureChannel(t, h.mem, channels.BuildRealtimeTaskOutputChannel(taskID))

	// ACT
	err := h.dispatcher.HandleTaskOutput(context.Background(), 1, &proto.TaskOutput{
		TaskId:      taskID,
		OutputChunk: []byte("done"),
		IsFinal:     true,
	})

	// ASSERT
	require.NoError(t, err)
	require.True(t, waitForMessages(captured, mu, 1, 200*time.Millisecond))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, *captured, 1)
	payload, parseErr := messages.ParsePayload[messages.TaskOutputPayload]((*captured)[0])
	require.NoError(t, parseErr)
	assert.True(t, payload.IsFinal, "IsFinal=true must reach subscribers verbatim")
}

func TestHandleTaskOutput_AppendErrorPropagates(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	h.repos.daemonTask.appendOutputErr = errTestRepoAppend

	// ACT
	err := h.dispatcher.HandleTaskOutput(context.Background(), 1, &proto.TaskOutput{
		TaskId:      5,
		OutputChunk: []byte("hello"),
	})

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "append task output", "AppendOutput errors must wrap with 'append task output'")
}

func TestCancelTask_MarksCanceledAndSignalsNode(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const taskID uint64 = 77
	const nodeID uint64 = 4

	h.repos.daemonTask.findResult = []domain.DaemonTask{{
		ID:                uint(taskID),
		DedicatedServerID: uint(nodeID),
		Status:            domain.DaemonTaskStatusWorking,
	}}

	stream := newFakeStream()
	registerSession(t, h.registry, nodeID, stream)

	// ACT
	err := h.dispatcher.CancelTask(context.Background(), taskID, "user requested")

	// ASSERT
	require.NoError(t, err)

	sent := stream.Sent()
	var cancelMsg *proto.TaskCancel
	for _, msg := range sent {
		if c := msg.GetTaskCancel(); c != nil {
			cancelMsg = c

			break
		}
	}
	require.NotNil(t, cancelMsg, "stream should receive a TaskCancel message")
	assert.Equal(t, taskID, cancelMsg.TaskId, "TaskCancel.TaskId must mirror the cancel request")
	assert.Equal(t, "user requested", cancelMsg.Reason, "TaskCancel.Reason must mirror the cancel request")

	_, lastSaved, _ := h.repos.daemonTask.snapshot()
	require.NotNil(t, lastSaved)
	assert.Equal(t, domain.DaemonTaskStatusCanceled, lastSaved.Status, "task should be saved with Canceled status")
}

func TestCancelTask_TaskNotFoundReturnsError(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	h.repos.daemonTask.findResult = nil

	// ACT
	err := h.dispatcher.CancelTask(context.Background(), 999, "no-op")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "task not found", "missing task must surface 'task not found'")
}

func TestCancelTask_FindErrorPropagates(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	h.repos.daemonTask.findErr = errTestRepoFind

	// ACT
	err := h.dispatcher.CancelTask(context.Background(), 777, "no-op")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "find task", "repo find errors must wrap with 'find task'")
}

func TestCancelTask_SaveErrorPropagates(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const taskID uint64 = 81
	h.repos.daemonTask.findResult = []domain.DaemonTask{{
		ID:                uint(taskID),
		DedicatedServerID: 1,
		Status:            domain.DaemonTaskStatusWorking,
	}}
	h.repos.daemonTask.saveErr = errTestRepoSave

	// ACT
	err := h.dispatcher.CancelTask(context.Background(), taskID, "stop now")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update task status", "save errors must wrap with 'update task status'")
}

func TestCancelTask_StreamSendFailureDoesNotAbort(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const taskID uint64 = 88
	const nodeID uint64 = 9

	h.repos.daemonTask.findResult = []domain.DaemonTask{{
		ID:                uint(taskID),
		DedicatedServerID: uint(nodeID),
		Status:            domain.DaemonTaskStatusWorking,
	}}

	stream := newFakeStream()
	stream.setSendErr(errTestStreamSend)
	registerSession(t, h.registry, nodeID, stream)

	// ACT
	err := h.dispatcher.CancelTask(context.Background(), taskID, "force")

	// ASSERT
	require.NoError(t, err, "cancel should still succeed even when the daemon notification fails")
	_, lastSaved, _ := h.repos.daemonTask.snapshot()
	require.NotNil(t, lastSaved)
	assert.Equal(t, domain.DaemonTaskStatusCanceled, lastSaved.Status,
		"task must still be marked Canceled when the stream send fails")
}

func TestDispatch_LoadsServerAndSendsConfigUpdate(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const nodeID uint64 = 5
	const serverID uint = 100

	stream := newFakeStream()
	registerSession(t, h.registry, nodeID, stream)

	h.repos.server.findResult = []domain.Server{{
		ID:        serverID,
		DSID:      uint(nodeID),
		GameID:    "csgo",
		GameModID: 7,
		Name:      "matchserver",
	}}
	h.repos.gameMod.findResult = []domain.GameMod{{
		ID:       7,
		GameCode: "csgo",
		Name:     "default",
	}}
	h.repos.node.findResult = []domain.Node{{
		ID: uint(nodeID),
		OS: domain.NodeOSLinux,
	}}
	h.repos.serverSetting.findResult = []domain.ServerSetting{{
		ID:       1,
		ServerID: serverID,
		Name:     "autostart",
		Value:    domain.NewServerSettingValue(true),
	}}
	h.repos.game.findResult = []domain.Game{{
		Code: "csgo",
		Name: "Counter-Strike",
	}}

	sid := serverID
	task := &domain.DaemonTask{
		ID:                123,
		DedicatedServerID: uint(nodeID),
		ServerID:          &sid,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
	}

	// ACT
	err := h.dispatcher.Dispatch(context.Background(), task)

	// ASSERT
	require.NoError(t, err)

	var hasConfigUpdate, hasTask bool
	for _, msg := range stream.Sent() {
		if cu := msg.GetServerConfigUpdate(); cu != nil {
			hasConfigUpdate = true
			assert.NotNil(t, cu.Server, "ServerConfigUpdate.Server must be populated")
			assert.NotNil(t, cu.Game, "ServerConfigUpdate.Game must be populated when game lookup succeeds")
			assert.NotNil(t, cu.GameMod, "ServerConfigUpdate.GameMod must be populated when game mod lookup succeeds")
		}
		if t := msg.GetTask(); t != nil && t.Id == 123 {
			hasTask = true
		}
	}
	assert.True(t, hasConfigUpdate, "the daemon should receive a ServerConfigUpdate before the task")
	assert.True(t, hasTask, "the daemon should also receive the task itself")
}

func TestDispatch_ServerConfigLookupFilters(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	const serverID uint = 301
	const nodeID uint = 41

	h.repos.server.findResult = []domain.Server{{
		ID:        serverID,
		DSID:      nodeID,
		GameModID: 55,
	}}

	sid := serverID
	task := &domain.DaemonTask{
		ID:                901,
		DedicatedServerID: nodeID,
		ServerID:          &sid,
		Task:              domain.DaemonTaskTypeServerStart,
		Status:            domain.DaemonTaskStatusWaiting,
	}

	// ACT
	err := h.dispatcher.Dispatch(context.Background(), task)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, h.repos.server.findFilter, "server lookup must be executed")
	assert.Equal(t, []uint{serverID}, h.repos.server.findFilter.IDs, "server lookup must filter by task.ServerID")

	require.NotNil(t, h.repos.gameMod.findFilter, "game mod lookup must be executed")
	assert.Equal(t, []uint{55}, h.repos.gameMod.findFilter.IDs, "game mod lookup must filter by server.GameModID")

	require.NotNil(t, h.repos.node.findFilter, "node lookup must be executed")
	assert.Equal(t, []uint{nodeID}, h.repos.node.findFilter.IDs, "node lookup must filter by server.DSID")

	require.NotNil(t, h.repos.serverSetting.findFilter, "server setting lookup must be executed")
	assert.Equal(t, []uint{serverID}, h.repos.serverSetting.findFilter.ServerIDs,
		"server setting lookup must filter by task.ServerID")
}

func TestPublishTaskStatus_NilPublisher_NoOp(t *testing.T) {
	// ARRANGE
	h := newTestDispatcher(t)
	defer h.cleanup()

	d := NewDispatcher(
		h.registry,
		h.repos.daemonTask,
		h.repos.server,
		h.repos.serverSetting,
		h.repos.game,
		h.repos.gameMod,
		h.repos.node,
		nil,
		discardLogger(),
	)

	const taskID uint64 = 5
	const nodeID uint64 = 1

	h.repos.daemonTask.findResult = []domain.DaemonTask{{
		ID:                uint(taskID),
		DedicatedServerID: uint(nodeID),
		Status:            domain.DaemonTaskStatusWaiting,
	}}

	// ACT
	require.NotPanics(t, func() {
		err := d.HandleTaskStatusUpdate(context.Background(), nodeID, &proto.TaskStatusUpdate{
			TaskId: taskID,
			Status: proto.DaemonTaskStatus_DAEMON_TASK_STATUS_WORKING,
		})
		require.NoError(t, err)
	}, "nil publisher must never trigger a panic")

	// ASSERT
	saveCalls, _, _ := h.repos.daemonTask.snapshot()
	assert.GreaterOrEqual(t, saveCalls, 1, "Save must still be invoked when publisher is nil")
}
