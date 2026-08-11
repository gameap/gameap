package daemon

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type archiveCancelCall struct {
	nodeID      uint64
	operationID string
	reason      string
}

type fakeArchiveGateway struct {
	mu             sync.Mutex
	requestArchive func(ctx context.Context, nodeID uint64, req *proto.ArchiveRequest) (*proto.ArchiveResponse, error)
	requests       []*proto.ArchiveRequest
	cancels        []archiveCancelCall
}

func (f *fakeArchiveGateway) RequestArchive(
	ctx context.Context, nodeID uint64, req *proto.ArchiveRequest,
) (*proto.ArchiveResponse, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	fn := f.requestArchive
	f.mu.Unlock()

	if fn == nil {
		return &proto.ArchiveResponse{RequestId: req.GetRequestId(), Success: true}, nil
	}

	return fn(ctx, nodeID, req)
}

func (f *fakeArchiveGateway) RequestArchiveCancel(nodeID uint64, operationID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancels = append(f.cancels, archiveCancelCall{nodeID: nodeID, operationID: operationID, reason: reason})

	return nil
}

func (f *fakeArchiveGateway) Requests() []*proto.ArchiveRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]*proto.ArchiveRequest(nil), f.requests...)
}

func (f *fakeArchiveGateway) Cancels() []archiveCancelCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]archiveCancelCall(nil), f.cancels...)
}

func (f *fakeConnectionChecker) setArchiveCapability(nodeID uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.capabilities[nodeID] == nil {
		f.capabilities[nodeID] = make(map[string]bool)
	}
	f.capabilities[nodeID][capabilityArchive] = true
}

type archiveTestSetup struct {
	service  *ArchiveService
	gateway  *fakeArchiveGateway
	registry *fakeConnectionChecker
	pubsub   pubsub.PubSub
}

func setupArchiveService(t *testing.T, ps pubsub.PubSub, instanceID string) *archiveTestSetup {
	t.Helper()

	gateway := &fakeArchiveGateway{}
	registry := newFakeConnectionChecker()
	logger := slog.New(slog.DiscardHandler)

	svc := NewArchiveService(ps, gateway, registry, instanceID,
		ArchiveLimits{MaxTotalBytes: 1 << 30, MaxFiles: 1000}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, svc.Start(ctx))

	return &archiveTestSetup{
		service:  svc,
		gateway:  gateway,
		registry: registry,
		pubsub:   ps,
	}
}

// eventRecorder collects pub/sub messages of one channel pattern.
type eventRecorder struct {
	mu   sync.Mutex
	msgs []*pubsub.Message
}

func recordChannel(t *testing.T, ps pubsub.PubSub, pattern string) *eventRecorder {
	t.Helper()

	rec := &eventRecorder{}
	require.NoError(t, ps.Subscribe(context.Background(), pattern, func(_ context.Context, msg *pubsub.Message) error {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.msgs = append(rec.msgs, msg)

		return nil
	}))

	return rec
}

func (r *eventRecorder) Messages() []*pubsub.Message {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]*pubsub.Message(nil), r.msgs...)
}

func TestArchiveService_StartCreate_LocalHappyPath(t *testing.T) {
	// ARRANGE
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })
	s := setupArchiveService(t, ps, "inst-a")

	const nodeID uint64 = 1
	const serverID uint = 77
	s.registry.setConnected(nodeID, true)
	s.registry.setArchiveCapability(nodeID)

	fmEvents := recordChannel(t, ps, channels.RealtimeFMArchiveAll)

	s.gateway.requestArchive = func(_ context.Context, gwNodeID uint64, req *proto.ArchiveRequest) (*proto.ArchiveResponse, error) {
		require.NoError(t, s.service.HandleArchiveProgress(context.Background(), gwNodeID, &proto.ArchiveProgress{
			RequestId:      req.GetRequestId(),
			FilesProcessed: 1,
			FilesTotal:     3,
			BytesProcessed: 100,
			CurrentEntry:   "maps/de_dust2.bsp",
		}))

		return &proto.ArchiveResponse{
			RequestId:      req.GetRequestId(),
			Success:        true,
			FilesProcessed: 3,
			BytesProcessed: 4096,
			ArchiveSize:    2048,
			Format:         proto.ArchiveFormat_ARCHIVE_FORMAT_ZIP,
			Skipped:        []string{"srv.sock"},
			SkippedCount:   1,
		}, nil
	}

	// ACT
	opID, err := s.service.StartCreate(testContext(t), testNode(1, "/srv"), CreateArchiveParams{
		ArchivePath: "/srv/backups/maps.zip",
		BasePath:    "/srv/backups",
		Sources:     []string{"/srv/backups/maps"},
		Format:      proto.ArchiveFormat_ARCHIVE_FORMAT_ZIP,
		Overwrite:   true,
		Owner:       OwnerOptions{User: "gameap"},
		Options: ArchiveStartOptions{
			ServerID:  serverID,
			Initiator: "user:5",
			Timeout:   time.Minute,
		},
	})

	// ASSERT
	require.NoError(t, err)
	require.NotEmpty(t, opID)

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := s.service.WaitCompletion(waitCtx, opID)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, uint32(3), result.FilesProcessed)
	assert.Equal(t, uint64(2048), result.ArchiveSize)
	assert.Equal(t, "zip", result.Format)
	assert.Equal(t, []string{"srv.sock"}, result.Skipped)

	snapshot, ok := s.service.GetSnapshot(opID)
	require.True(t, ok)
	assert.Equal(t, ArchiveOpDone, snapshot.Status)
	assert.Equal(t, ArchiveKindCreate, snapshot.Kind)
	assert.Equal(t, "user:5", snapshot.Initiator)
	assert.Equal(t, serverID, snapshot.ServerID)
	assert.Equal(t, uint32(1), snapshot.Progress.FilesProcessed, "last progress must be recorded")
	require.NotNil(t, snapshot.Result)

	requests := s.gateway.Requests()
	require.Len(t, requests, 1)
	assert.Equal(t, opID, requests[0].GetRequestId())
	assert.Equal(t, time.Minute, requests[0].GetTimeout().AsDuration())
	create := requests[0].GetCreate()
	require.NotNil(t, create)
	assert.Equal(t, "backups/maps.zip", create.ArchivePath, "WorkPath must be stripped")
	assert.Equal(t, "backups", create.BasePath)
	assert.Equal(t, []string{"backups/maps"}, create.Sources)
	assert.Equal(t, "gameap", create.OwnerUser)
	assert.Equal(t, uint64(1<<30), create.MaxTotalBytes, "config limits must fill unset params")
	assert.Equal(t, uint32(1000), create.MaxFiles)

	require.Eventually(t, func() bool {
		return len(fmEvents.Messages()) >= 2
	}, 2*time.Second, 10*time.Millisecond, "server-scoped channel must receive progress and complete")

	fmMsgs := fmEvents.Messages()
	assert.Equal(t, messages.TypeArchiveProgress, fmMsgs[0].Type)
	assert.Equal(t, messages.TypeArchiveComplete, fmMsgs[1].Type)

	progress, err := messages.ParsePayload[messages.ArchiveProgressEventPayload](fmMsgs[0])
	require.NoError(t, err)
	assert.Equal(t, opID, progress.OperationID)
	assert.Equal(t, serverID, progress.ServerID)
	assert.Equal(t, "create", progress.Kind)
	assert.Equal(t, "maps/de_dust2.bsp", progress.CurrentEntry)
}

func TestArchiveService_StartCreate_NoArchiveCapability(t *testing.T) {
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })
	s := setupArchiveService(t, ps, "inst-a")

	const nodeID uint64 = 2
	s.registry.setConnected(nodeID, true)

	opID, err := s.service.StartCreate(testContext(t), testNode(2, "/srv"), CreateArchiveParams{
		ArchivePath: "/srv/a.zip",
		BasePath:    "/srv",
		Sources:     []string{"/srv/a"},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrArchiveNotSupported)
	assert.Contains(t, err.Error(), "node does not support archive operations")
	assert.Empty(t, opID)
	assert.Empty(t, s.gateway.Requests(), "gateway must not be called")

	_, ok := s.service.GetSnapshot(opID)
	assert.False(t, ok, "no registry entry may survive a rejected start")
}

func TestArchiveService_StartExtract_NotConnected(t *testing.T) {
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })
	s := setupArchiveService(t, ps, "inst-a")

	_, err := s.service.StartExtract(testContext(t), testNode(3, "/srv"), ExtractArchiveParams{
		ArchivePath: "/srv/a.zip",
		Destination: "/srv/out",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDaemonNotConnected)
}

func TestArchiveService_RemoteStart_TwoInstances(t *testing.T) {
	// ARRANGE: one shared pub/sub, instance A initiates, instance B owns the
	// daemon session.
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })
	a := setupArchiveService(t, ps, "inst-a")
	b := setupArchiveService(t, ps, "inst-b")

	const nodeID uint64 = 5
	a.registry.connectedAnywhere[nodeID] = true
	b.registry.setConnected(nodeID, true)
	b.registry.setArchiveCapability(nodeID)

	b.gateway.requestArchive = func(_ context.Context, gwNodeID uint64, req *proto.ArchiveRequest) (*proto.ArchiveResponse, error) {
		require.NoError(t, b.service.HandleArchiveProgress(context.Background(), gwNodeID, &proto.ArchiveProgress{
			RequestId:      req.GetRequestId(),
			FilesProcessed: 2,
		}))

		return &proto.ArchiveResponse{
			RequestId:      req.GetRequestId(),
			Success:        true,
			FilesProcessed: 9,
			Format:         proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ,
		}, nil
	}

	// ACT
	opID, err := a.service.StartExtract(testContext(t), testNode(5, "/srv"), ExtractArchiveParams{
		ArchivePath:       "/srv/mod.tar.gz",
		Destination:       "/srv/mods",
		CreateDestination: true,
		Options: ArchiveStartOptions{
			ServerID:  12,
			Initiator: "user:9",
		},
	})

	// ASSERT
	require.NoError(t, err, "instance B must ack the dispatched start")
	require.NotEmpty(t, opID)

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := a.service.WaitCompletion(waitCtx, opID)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, uint32(9), result.FilesProcessed)
	assert.Equal(t, "tar_gz", result.Format)

	requests := b.gateway.Requests()
	require.Len(t, requests, 1, "owning instance must drive the operation")
	assert.Equal(t, opID, requests[0].GetRequestId())
	require.NotNil(t, requests[0].GetExtract())
	assert.Equal(t, "mod.tar.gz", requests[0].GetExtract().ArchivePath)

	require.Eventually(t, func() bool {
		snapshot, ok := a.service.GetSnapshot(opID)

		return ok && snapshot.Status == ArchiveOpDone && snapshot.Progress.FilesProcessed == 2
	}, 2*time.Second, 10*time.Millisecond,
		"initiator's registry must see progress and completion via pub/sub")

	assert.Empty(t, a.gateway.Requests(), "initiating instance must not touch its gateway")
}

func TestArchiveService_RemoteStart_CapabilityErrorMapsToSentinel(t *testing.T) {
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })
	a := setupArchiveService(t, ps, "inst-a")
	b := setupArchiveService(t, ps, "inst-b")

	const nodeID uint64 = 6
	a.registry.connectedAnywhere[nodeID] = true
	b.registry.setConnected(nodeID, true)

	_, err := a.service.StartCreate(testContext(t), testNode(6, "/srv"), CreateArchiveParams{
		ArchivePath: "/srv/a.zip",
		BasePath:    "/srv",
		Sources:     []string{"/srv/x"},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrArchiveNotSupported,
		"remote capability rejection must map back to the sentinel")
	assert.Empty(t, b.gateway.Requests())
}

func TestArchiveService_Cancel(t *testing.T) {
	t.Run("local_session_sends_gateway_cancel", func(t *testing.T) {
		ps := memory.New()
		t.Cleanup(func() { _ = ps.Close() })
		s := setupArchiveService(t, ps, "inst-a")

		const nodeID uint64 = 7
		s.registry.setConnected(nodeID, true)

		err := s.service.Cancel(testContext(t), testNode(7, "/srv"), "op-1", "canceled by user")

		require.NoError(t, err)
		cancels := s.gateway.Cancels()
		require.Len(t, cancels, 1)
		assert.Equal(t, nodeID, cancels[0].nodeID)
		assert.Equal(t, "op-1", cancels[0].operationID)
		assert.Equal(t, "canceled by user", cancels[0].reason)
	})

	t.Run("remote_session_dispatches_cancel", func(t *testing.T) {
		ps := memory.New()
		t.Cleanup(func() { _ = ps.Close() })
		a := setupArchiveService(t, ps, "inst-a")
		b := setupArchiveService(t, ps, "inst-b")

		const nodeID uint64 = 8
		a.registry.connectedAnywhere[nodeID] = true
		b.registry.setConnected(nodeID, true)

		err := a.service.Cancel(testContext(t), testNode(8, "/srv"), "op-2", "test reason")

		require.NoError(t, err)
		cancels := b.gateway.Cancels()
		require.Len(t, cancels, 1)
		assert.Equal(t, "op-2", cancels[0].operationID)
		assert.Equal(t, "test reason", cancels[0].reason)
		assert.Empty(t, a.gateway.Cancels())
	})

	t.Run("not_connected_anywhere_returns_sentinel", func(t *testing.T) {
		ps := memory.New()
		t.Cleanup(func() { _ = ps.Close() })
		s := setupArchiveService(t, ps, "inst-a")

		err := s.service.Cancel(testContext(t), testNode(9, "/srv"), "op-3", "r")

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDaemonNotConnected)
	})
}

func TestArchiveService_NodeScopedOpSkipsServerChannel(t *testing.T) {
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })
	s := setupArchiveService(t, ps, "inst-a")

	const nodeID uint64 = 10
	s.registry.setConnected(nodeID, true)
	s.registry.setArchiveCapability(nodeID)

	fmEvents := recordChannel(t, ps, channels.RealtimeFMArchiveAll)
	opEvents := recordChannel(t, ps, channels.RealtimeArchiveOpAll)

	opID, err := s.service.StartCreate(testContext(t), testNode(10, "/srv"), CreateArchiveParams{
		ArchivePath: "/srv/plugin.tar",
		BasePath:    "/srv",
		Sources:     []string{"/srv/data"},
		Options:     ArchiveStartOptions{ServerID: 0, Initiator: "plugin:3"},
	})
	require.NoError(t, err)

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = s.service.WaitCompletion(waitCtx, opID)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(opEvents.Messages()) >= 1
	}, 2*time.Second, 10*time.Millisecond, "per-op channel must receive the completion")

	assert.Empty(t, fmEvents.Messages(),
		"node-scoped operations must never leak onto server-scoped channels")
}

func TestArchiveService_WaitCompletion_UnknownOperation(t *testing.T) {
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })
	s := setupArchiveService(t, ps, "inst-a")

	_, err := s.service.WaitCompletion(testContext(t), "nope")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrArchiveOperationNotFound)
}

func TestArchiveService_SweepExpired(t *testing.T) {
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })
	s := setupArchiveService(t, ps, "inst-a")

	const nodeID uint64 = 11
	s.registry.setConnected(nodeID, true)
	s.registry.setArchiveCapability(nodeID)

	opID, err := s.service.StartCreate(testContext(t), testNode(11, "/srv"), CreateArchiveParams{
		ArchivePath: "/srv/a.zip",
		BasePath:    "/srv",
		Sources:     []string{"/srv/x"},
	})
	require.NoError(t, err)

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = s.service.WaitCompletion(waitCtx, opID)
	require.NoError(t, err)

	_, ok := s.service.GetSnapshot(opID)
	require.True(t, ok, "completed entry must stay within retention")

	s.service.sweepExpired(time.Now().Add(archiveOpRetention + time.Minute))

	_, ok = s.service.GetSnapshot(opID)
	assert.False(t, ok, "expired entry must be swept")
}

func TestArchiveFormatAPINames(t *testing.T) {
	format, ok := ArchiveFormatFromAPIName("tar_zstd")
	require.True(t, ok)
	assert.Equal(t, proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_ZSTD, format)

	_, ok = ArchiveFormatFromAPIName("nonsense")
	assert.False(t, ok)

	assert.Equal(t, "7z", ArchiveFormatToAPIName(proto.ArchiveFormat_ARCHIVE_FORMAT_7Z))
	assert.Empty(t, ArchiveFormatToAPIName(proto.ArchiveFormat_ARCHIVE_FORMAT_UNSPECIFIED))
}
