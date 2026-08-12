package gateway

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// silentLogger returns a slog.Logger that drops every record.
func silentLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// stubStream is a minimal session.Stream implementation that records sent
// gateway messages and serves Recv from a controllable channel. It is the
// gateway-package mirror of session.stubStream.
type stubStream struct {
	ctx     context.Context
	sentMu  sync.Mutex
	sent    []*proto.GatewayMessage
	sendErr error
	recvCh  chan *proto.DaemonMessage
}

func newStubStream(ctx context.Context) *stubStream {
	return &stubStream{
		ctx:    ctx,
		recvCh: make(chan *proto.DaemonMessage, 16),
	}
}

func (s *stubStream) Send(m *proto.GatewayMessage) error {
	if s.sendErr != nil {
		return s.sendErr
	}

	s.sentMu.Lock()
	defer s.sentMu.Unlock()
	s.sent = append(s.sent, m)

	return nil
}

func (s *stubStream) Recv() (*proto.DaemonMessage, error) {
	msg, ok := <-s.recvCh
	if !ok {
		return nil, io.EOF
	}

	return msg, nil
}

func (s *stubStream) Context() context.Context {
	return s.ctx
}

func (s *stubStream) Sent() []*proto.GatewayMessage {
	s.sentMu.Lock()
	defer s.sentMu.Unlock()

	return slices.Clone(s.sent)
}

// stubConnectServer satisfies proto.DaemonGateway_ConnectServer
// (grpc.BidiStreamingServer[DaemonMessage, GatewayMessage]) for in-process
// Connect tests. The unused grpc.ServerStream surface is satisfied by the
// embedded noopServerStream.
type stubConnectServer struct {
	noopServerStream

	ctx     context.Context
	sentMu  sync.Mutex
	sent    []*proto.GatewayMessage
	sendErr error
	recvCh  chan recvResult
}

type recvResult struct {
	msg *proto.DaemonMessage
	err error
}

func newStubConnectServer(ctx context.Context) *stubConnectServer {
	return &stubConnectServer{
		ctx:    ctx,
		recvCh: make(chan recvResult, 16),
	}
}

func (s *stubConnectServer) Context() context.Context {
	return s.ctx
}

func (s *stubConnectServer) Send(m *proto.GatewayMessage) error {
	if s.sendErr != nil {
		return s.sendErr
	}

	s.sentMu.Lock()
	defer s.sentMu.Unlock()
	s.sent = append(s.sent, m)

	return nil
}

func (s *stubConnectServer) Recv() (*proto.DaemonMessage, error) {
	r, ok := <-s.recvCh
	if !ok {
		return nil, io.EOF
	}

	return r.msg, r.err
}

// PushMessage queues a DaemonMessage for the next Recv call.
func (s *stubConnectServer) PushMessage(m *proto.DaemonMessage) {
	s.recvCh <- recvResult{msg: m}
}

// PushError queues an error for the next Recv call.
func (s *stubConnectServer) PushError(err error) {
	s.recvCh <- recvResult{err: err}
}

// CloseRecv signals EOF to subsequent Recv calls.
func (s *stubConnectServer) CloseRecv() {
	close(s.recvCh)
}

// Sent returns a snapshot of all messages that have been Send-called.
func (s *stubConnectServer) Sent() []*proto.GatewayMessage {
	s.sentMu.Lock()
	defer s.sentMu.Unlock()

	return slices.Clone(s.sent)
}

// noopServerStream provides default implementations of grpc.ServerStream's
// non-Context methods so that test stubs only need to override the methods
// they care about. None of these are exercised by the gateway code under
// test; they exist solely to satisfy the interface.
type noopServerStream struct{}

func (noopServerStream) SetHeader(metadata.MD) error  { return nil }
func (noopServerStream) SendHeader(metadata.MD) error { return nil }
func (noopServerStream) SetTrailer(metadata.MD)       {}
func (noopServerStream) Context() context.Context     { return context.Background() }
func (noopServerStream) SendMsg(any) error            { return nil }
func (noopServerStream) RecvMsg(any) error            { return nil }

// Compile-time assertion that stubConnectServer matches the generated
// stream type. This catches signature drift in the proto bindings.
var _ proto.DaemonGateway_ConnectServer = (*stubConnectServer)(nil)
var _ grpc.ServerStream = (*stubConnectServer)(nil)

// fakeAPIKeyVerifier implements APIKeyVerifier with controllable behavior.
type fakeAPIKeyVerifier struct {
	valid map[string]uint64 // apiKey -> nodeID
	err   error
}

func (f *fakeAPIKeyVerifier) Verify(apiKey string, nodeID uint64) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	wantNodeID, ok := f.valid[apiKey]

	return ok && wantNodeID == nodeID, nil
}

// fakeTaskHandler records calls to TaskHandler methods.
type fakeTaskHandler struct {
	mu              sync.Mutex
	statusUpdates   []*proto.TaskStatusUpdate
	outputs         []*proto.TaskOutput
	pendingTasks    []*proto.DaemonTask
	pendingErr      error
	statusUpdateErr error
	reconcileCalls  []reconcileCall
	reconcileMarked int
	reconcileErr    error
}

type reconcileCall struct {
	nodeID      uint64
	inFlightIDs []uint64
	reason      string
}

func (f *fakeTaskHandler) HandleTaskStatusUpdate(_ context.Context, _ uint64, update *proto.TaskStatusUpdate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusUpdates = append(f.statusUpdates, update)

	return f.statusUpdateErr
}

func (f *fakeTaskHandler) HandleTaskOutput(_ context.Context, _ uint64, output *proto.TaskOutput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outputs = append(f.outputs, output)

	return nil
}

func (f *fakeTaskHandler) GetPendingTasks(_ context.Context, _ uint64) ([]*proto.DaemonTask, error) {
	return f.pendingTasks, f.pendingErr
}

func (f *fakeTaskHandler) ReconcileWorkingTasks(
	_ context.Context, nodeID uint64, inFlightIDs []uint64, reason string,
) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reconcileCalls = append(f.reconcileCalls, reconcileCall{
		nodeID:      nodeID,
		inFlightIDs: inFlightIDs,
		reason:      reason,
	})

	return f.reconcileMarked, f.reconcileErr
}

func (f *fakeTaskHandler) ReconcileCalls() []reconcileCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.reconcileCalls)
}

func (f *fakeTaskHandler) StatusUpdates() []*proto.TaskStatusUpdate {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.statusUpdates)
}

func (f *fakeTaskHandler) Outputs() []*proto.TaskOutput {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.outputs)
}

// fakeTaskFlusher records FlushPending invocations.
type fakeTaskFlusher struct {
	mu       sync.Mutex
	flushed  []uint64
	flushErr error
	done     chan struct{}
}

func newFakeTaskFlusher() *fakeTaskFlusher {
	return &fakeTaskFlusher{done: make(chan struct{}, 8)}
}

func (f *fakeTaskFlusher) FlushPending(_ context.Context, nodeID uint64) error {
	f.mu.Lock()
	f.flushed = append(f.flushed, nodeID)
	f.mu.Unlock()

	select {
	case f.done <- struct{}{}:
	default:
	}

	return f.flushErr
}

func (f *fakeTaskFlusher) Flushed() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.flushed)
}

// fakeCommandHandler records command handler invocations.
type fakeCommandHandler struct {
	mu      sync.Mutex
	outputs []*proto.CommandOutput
	results []*proto.CommandResult
}

func (f *fakeCommandHandler) HandleCommandOutput(_ context.Context, _ uint64, output *proto.CommandOutput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outputs = append(f.outputs, output)

	return nil
}

func (f *fakeCommandHandler) HandleCommandResult(_ context.Context, _ uint64, result *proto.CommandResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, result)

	return nil
}

func (f *fakeCommandHandler) Outputs() []*proto.CommandOutput {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.outputs)
}

func (f *fakeCommandHandler) Results() []*proto.CommandResult {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.results)
}

// fakeServerStatusHandler records server status batches.
type fakeServerStatusHandler struct {
	mu       sync.Mutex
	batches  []*proto.ServerStatusBatch
	batchErr error
}

func (f *fakeServerStatusHandler) HandleServerStatuses(_ context.Context, _ uint64, batch *proto.ServerStatusBatch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, batch)

	return f.batchErr
}

func (f *fakeServerStatusHandler) Batches() []*proto.ServerStatusBatch {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.batches)
}

// fakeAttachHandler records attach handler invocations.
type fakeAttachHandler struct {
	mu      sync.Mutex
	started []*proto.AttachStarted
	outputs []*proto.AttachOutput
	closed  []*proto.AttachClosed
}

func (f *fakeAttachHandler) HandleAttachStarted(_ context.Context, _ uint64, started *proto.AttachStarted) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, started)

	return nil
}

func (f *fakeAttachHandler) HandleAttachOutput(_ context.Context, _ uint64, output *proto.AttachOutput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outputs = append(f.outputs, output)

	return nil
}

func (f *fakeAttachHandler) HandleAttachClosed(_ context.Context, _ uint64, closed *proto.AttachClosed) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, closed)

	return nil
}

// fakeMetricsHandler records metrics handler invocations.
type fakeMetricsHandler struct {
	mu        sync.Mutex
	responses []*proto.MetricsResponse
}

func (f *fakeMetricsHandler) HandleMetricsResponse(_ context.Context, _ uint64, _ string, resp *proto.MetricsResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = append(f.responses, resp)

	return nil
}

func (f *fakeMetricsHandler) Responses() []*proto.MetricsResponse {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.responses)
}

// serverTaskReconcileCall captures one invocation of
// ServerTaskHandler.ReconcileWorkingExecutions for assertion in register tests.
type serverTaskReconcileCall struct {
	nodeID      uint64
	inFlightIDs []string
	reason      string
}

// fakeServerTaskHandler records calls to ServerTaskHandler methods.
type fakeServerTaskHandler struct {
	mu sync.Mutex

	snapshotErr error
	snapshot    *proto.ServerTaskSnapshot

	started    []*proto.ServerTaskExecutionStarted
	startedErr error

	finished    []*proto.ServerTaskExecutionFinished
	finishedErr error

	logs   []*proto.ServerTaskExecutionLog
	logErr error

	resyncs   []*proto.ServerTaskResyncRequest
	resyncErr error

	reconcileCalls  []serverTaskReconcileCall
	reconcileMarked int
	reconcileErr    error
}

func (f *fakeServerTaskHandler) BuildSnapshot(_ context.Context, _ uint64) (*proto.ServerTaskSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.snapshot, f.snapshotErr
}

func (f *fakeServerTaskHandler) HandleExecutionStarted(
	_ context.Context, _ uint64, evt *proto.ServerTaskExecutionStarted,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, evt)

	return f.startedErr
}

func (f *fakeServerTaskHandler) HandleExecutionFinished(
	_ context.Context, _ uint64, evt *proto.ServerTaskExecutionFinished,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished = append(f.finished, evt)

	return f.finishedErr
}

func (f *fakeServerTaskHandler) HandleExecutionLog(
	_ context.Context, _ uint64, evt *proto.ServerTaskExecutionLog,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, evt)

	return f.logErr
}

func (f *fakeServerTaskHandler) HandleResyncRequest(
	_ context.Context, _ uint64, req *proto.ServerTaskResyncRequest,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resyncs = append(f.resyncs, req)

	return f.resyncErr
}

func (f *fakeServerTaskHandler) ReconcileWorkingExecutions(
	_ context.Context, nodeID uint64, inFlightExecIDs []string, reason string,
) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reconcileCalls = append(f.reconcileCalls, serverTaskReconcileCall{
		nodeID:      nodeID,
		inFlightIDs: inFlightExecIDs,
		reason:      reason,
	})

	return f.reconcileMarked, f.reconcileErr
}

func (f *fakeServerTaskHandler) Started() []*proto.ServerTaskExecutionStarted {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.started)
}

func (f *fakeServerTaskHandler) Finished() []*proto.ServerTaskExecutionFinished {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.finished)
}

func (f *fakeServerTaskHandler) Logs() []*proto.ServerTaskExecutionLog {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.logs)
}

func (f *fakeServerTaskHandler) Resyncs() []*proto.ServerTaskResyncRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.resyncs)
}

func (f *fakeServerTaskHandler) ReconcileCalls() []serverTaskReconcileCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.reconcileCalls)
}

// serviceDeps bundles the in-memory repositories used by the gateway.Service.
type serviceDeps struct {
	registry          *session.Registry
	nodeRepo          *inmemory.NodeRepository
	serverRepo        *inmemory.ServerRepository
	serverSettingRepo *inmemory.ServerSettingRepository
	daemonTaskRepo    *inmemory.DaemonTaskRepository
	gameRepo          *inmemory.GameRepository
	gameModRepo       *inmemory.GameModRepository
	apiKeyVerifier    *fakeAPIKeyVerifier
	taskHandler       *fakeTaskHandler
	taskFlusher       *fakeTaskFlusher
	commandHandler    *fakeCommandHandler
	serverHandler     *fakeServerStatusHandler
	attachHandler     *fakeAttachHandler
	metricsHandler    *fakeMetricsHandler
	serverTaskHandler *fakeServerTaskHandler
}

// newServiceWithDeps assembles a *Service backed entirely by in-memory
// repositories and recording fake collaborators.  Tests can mutate any
// returned dependency before calling Service methods.
func newServiceWithDeps(t *testing.T) (*Service, *serviceDeps) {
	t.Helper()

	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	deps := &serviceDeps{
		registry:          session.NewRegistry(ps, "test-instance", silentLogger()),
		nodeRepo:          inmemory.NewNodeRepository(),
		serverRepo:        inmemory.NewServerRepository(),
		serverSettingRepo: inmemory.NewServerSettingRepository(),
		daemonTaskRepo:    inmemory.NewDaemonTaskRepository(),
		gameRepo:          inmemory.NewGameRepository(),
		gameModRepo:       inmemory.NewGameModRepository(),
		apiKeyVerifier:    &fakeAPIKeyVerifier{valid: map[string]uint64{}},
		taskHandler:       &fakeTaskHandler{},
		taskFlusher:       newFakeTaskFlusher(),
		commandHandler:    &fakeCommandHandler{},
		serverHandler:     &fakeServerStatusHandler{},
		attachHandler:     &fakeAttachHandler{},
		metricsHandler:    &fakeMetricsHandler{},
		serverTaskHandler: &fakeServerTaskHandler{},
	}

	svc := NewService(
		deps.registry,
		deps.nodeRepo,
		deps.serverRepo,
		deps.serverSettingRepo,
		deps.daemonTaskRepo,
		deps.gameRepo,
		deps.gameModRepo,
		deps.apiKeyVerifier,
		deps.taskHandler,
		deps.taskFlusher,
		deps.commandHandler,
		deps.serverHandler,
		deps.attachHandler,
		deps.metricsHandler,
		deps.serverTaskHandler,
		nil,
		silentLogger(),
	)

	return svc, deps
}

// connectAndRegisterSession opens a fresh stubStream and registers a
// session for node id 1 directly with the registry. Returns the stub for
// inspection. Tests that need multiple sessions should call the registry
// directly.
func connectAndRegisterSession(t *testing.T, deps *serviceDeps, capabilities ...string) *stubStream {
	t.Helper()

	const nodeID uint64 = 1
	stream := newStubStream(context.Background())
	sess := session.NewSession(nodeID, stream, "v1", capabilities, func() {})
	if err := deps.registry.Register(context.Background(), sess); err != nil {
		t.Fatalf("registry.Register: %v", err)
	}
	t.Cleanup(func() {
		_ = deps.registry.Unregister(context.Background(), nodeID)
	})

	return stream
}

// errSentinel is a typed sentinel for tests that need a non-nil err113-safe error.
var errSentinel = errors.New("sentinel test error")

type archiveProgressCall struct {
	nodeID   uint64
	progress *proto.ArchiveProgress
}

type fakeArchiveProgressHandler struct {
	mu    sync.Mutex
	calls []archiveProgressCall
}

func (f *fakeArchiveProgressHandler) HandleArchiveProgress(
	_ context.Context, nodeID uint64, progress *proto.ArchiveProgress,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, archiveProgressCall{nodeID: nodeID, progress: progress})

	return nil
}

func (f *fakeArchiveProgressHandler) Calls() []archiveProgressCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.calls)
}
