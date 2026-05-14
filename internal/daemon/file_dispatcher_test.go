package daemon

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/gameap/gameap/pkg/proto"
)

// fakeFileGateway is a configurable in-test fake for FileGateway.
type fakeFileGateway struct {
	mu sync.Mutex

	requestFileList     func(ctx context.Context, nodeID uint64, path string, recursive bool, pattern string) (*proto.FileListResponse, error)
	requestFileRead     func(ctx context.Context, nodeID uint64, path string, offset int64, length int64) (*proto.FileReadResponse, error)
	requestFileWrite    func(ctx context.Context, nodeID uint64, path string, content []byte, mode int32, createDirs bool) error
	requestFileOp       func(ctx context.Context, nodeID uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error)
	requestUploadTask   func(ctx context.Context, nodeID uint64, transferID, destPath, checksum string, totalSize int64) error
	requestDownloadTask func(ctx context.Context, nodeID uint64, transferID, srcPath string) error

	uploadCalls   atomic.Int32
	downloadCalls atomic.Int32

	lastWriteOwner      OwnerOptions
	lastUploadTaskOwner OwnerOptions
	lastUploadTaskMode  int32
}

func (f *fakeFileGateway) RequestFileList(
	ctx context.Context, nodeID uint64, path string, recursive bool, pattern string,
) (*proto.FileListResponse, error) {
	f.mu.Lock()
	fn := f.requestFileList
	f.mu.Unlock()
	if fn == nil {
		return &proto.FileListResponse{Success: true}, nil
	}

	return fn(ctx, nodeID, path, recursive, pattern)
}

func (f *fakeFileGateway) RequestFileRead(
	ctx context.Context, nodeID uint64, path string, offset int64, length int64,
) (*proto.FileReadResponse, error) {
	f.mu.Lock()
	fn := f.requestFileRead
	f.mu.Unlock()
	if fn == nil {
		return &proto.FileReadResponse{Success: true}, nil
	}

	return fn(ctx, nodeID, path, offset, length)
}

func (f *fakeFileGateway) RequestFileWrite(
	ctx context.Context, nodeID uint64, path string,
	content []byte, mode int32, createDirs bool, owner OwnerOptions,
) error {
	f.mu.Lock()
	f.lastWriteOwner = owner
	fn := f.requestFileWrite
	f.mu.Unlock()
	if fn == nil {
		return nil
	}

	return fn(ctx, nodeID, path, content, mode, createDirs)
}

func (f *fakeFileGateway) RequestFileOperation(
	ctx context.Context, nodeID uint64, req *proto.FileOperationRequest,
) (*proto.FileOperationResponse, error) {
	f.mu.Lock()
	fn := f.requestFileOp
	f.mu.Unlock()
	if fn == nil {
		return &proto.FileOperationResponse{Success: true}, nil
	}

	return fn(ctx, nodeID, req)
}

func (f *fakeFileGateway) RequestFileUploadTask(
	ctx context.Context, nodeID uint64, transferID, destPath, checksum string,
	totalSize int64, mode int32, owner OwnerOptions,
) error {
	f.uploadCalls.Add(1)
	f.mu.Lock()
	f.lastUploadTaskOwner = owner
	f.lastUploadTaskMode = mode
	fn := f.requestUploadTask
	f.mu.Unlock()
	if fn == nil {
		return nil
	}

	return fn(ctx, nodeID, transferID, destPath, checksum, totalSize)
}

func (f *fakeFileGateway) RequestFileDownloadTask(
	ctx context.Context, nodeID uint64, transferID, srcPath string,
) error {
	f.downloadCalls.Add(1)
	f.mu.Lock()
	fn := f.requestDownloadTask
	f.mu.Unlock()
	if fn == nil {
		return nil
	}

	return fn(ctx, nodeID, transferID, srcPath)
}

// fakeConnectionChecker is a configurable in-test fake for ConnectionChecker.
type fakeConnectionChecker struct {
	mu                  sync.RWMutex
	connected           map[uint64]bool
	connectedAnywhere   map[uint64]bool
	capabilities        map[uint64]map[string]bool
	defaultIsConnected  bool
	defaultIsAnywhere   bool
	defaultHasCapabilty bool
}

func newFakeConnectionChecker() *fakeConnectionChecker {
	return &fakeConnectionChecker{
		connected:         make(map[uint64]bool),
		connectedAnywhere: make(map[uint64]bool),
		capabilities:      make(map[uint64]map[string]bool),
	}
}

func (f *fakeConnectionChecker) setConnected(nodeID uint64, value bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected[nodeID] = value
}

func (f *fakeConnectionChecker) setConnectedAnywhere(nodeID uint64, value bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connectedAnywhere[nodeID] = value
}

//nolint:unparam // capability arg is generic by design; tests currently only use capabilityFileTransfer
func (f *fakeConnectionChecker) setCapability(nodeID uint64, capability string, value bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.capabilities[nodeID] == nil {
		f.capabilities[nodeID] = make(map[string]bool)
	}
	f.capabilities[nodeID][capability] = value
}

func (f *fakeConnectionChecker) IsConnected(nodeID uint64) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if v, ok := f.connected[nodeID]; ok {
		return v
	}

	return f.defaultIsConnected
}

func (f *fakeConnectionChecker) IsConnectedAnywhere(nodeID uint64) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if v, ok := f.connectedAnywhere[nodeID]; ok {
		return v
	}

	return f.defaultIsAnywhere
}

func (f *fakeConnectionChecker) HasCapability(nodeID uint64, capability string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if caps, ok := f.capabilities[nodeID]; ok {
		if v, ok := caps[capability]; ok {
			return v
		}
	}

	return f.defaultHasCapabilty
}

type dispatcherTestSetup struct {
	dispatcher FileDispatcher
	gateway    *fakeFileGateway
	registry   *fakeConnectionChecker
	storage    files.StreamFileManager
	pubsub     pubsub.PubSub
	instanceID string
}

func setupDispatcher(t *testing.T) *dispatcherTestSetup {
	t.Helper()

	gateway := &fakeFileGateway{}
	registry := newFakeConnectionChecker()
	storage := files.NewInMemoryFileManager()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	logger := slog.Default()
	instanceID := "test-instance"

	dispatcher := NewFileDispatcher(ps, gateway, registry, storage, instanceID, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	require.NoError(t, dispatcher.Start(ctx))

	return &dispatcherTestSetup{
		dispatcher: dispatcher,
		gateway:    gateway,
		registry:   registry,
		storage:    storage,
		pubsub:     ps,
		instanceID: instanceID,
	}
}

func TestNewFileDispatcher_NilLoggerUsesDefault(t *testing.T) {
	// ARRANGE
	gateway := &fakeFileGateway{}
	registry := newFakeConnectionChecker()
	storage := files.NewInMemoryFileManager()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	// ACT
	dispatcher := NewFileDispatcher(ps, gateway, registry, storage, "id", nil)

	// ASSERT
	require.NotNil(t, dispatcher)
}

func TestDispatchFileList_Success(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 7
	s.registry.setConnected(nodeID, true)

	expectedFiles := []*proto.FileStat{
		{Name: "a.txt", Size: 100, Type: proto.FileType_FILE_TYPE_REGULAR},
		{Name: "b.txt", Size: 200, Type: proto.FileType_FILE_TYPE_REGULAR},
	}

	var capturedNodeID uint64
	var capturedPath string
	var capturedRecursive bool
	var capturedPattern string

	s.gateway.requestFileList = func(_ context.Context, n uint64, path string, recursive bool, pattern string) (*proto.FileListResponse, error) {
		capturedNodeID = n
		capturedPath = path
		capturedRecursive = recursive
		capturedPattern = pattern

		return &proto.FileListResponse{Success: true, Files: expectedFiles}, nil
	}

	// ACT
	resp, err := s.dispatcher.DispatchFileList(testContext(t), nodeID, "/srv/dir", true, "*.txt")

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	require.Len(t, resp.Files, 2)
	assert.Equal(t, "a.txt", resp.Files[0].Name)
	assert.Equal(t, "b.txt", resp.Files[1].Name)

	assert.Equal(t, nodeID, capturedNodeID, "gateway must receive the same nodeID")
	assert.Equal(t, "/srv/dir", capturedPath, "path must be propagated to gateway")
	assert.True(t, capturedRecursive, "recursive flag must be propagated")
	assert.Equal(t, "*.txt", capturedPattern, "pattern must be propagated")
}

func TestDispatchFileList_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 7
	s.registry.setConnected(nodeID, true)

	s.gateway.requestFileList = func(_ context.Context, _ uint64, _ string, _ bool, _ string) (*proto.FileListResponse, error) {
		return nil, errors.New("gateway boom")
	}

	// ACT
	resp, err := s.dispatcher.DispatchFileList(testContext(t), nodeID, "/x", false, "")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "gateway boom", "gateway error must propagate via pubsub response")
}

func TestDispatchFileRead_Success(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 8
	s.registry.setConnected(nodeID, true)

	expectedContent := []byte("hello world")
	s.gateway.requestFileRead = func(_ context.Context, _ uint64, path string, offset int64, length int64) (*proto.FileReadResponse, error) {
		assert.Equal(t, "/srv/file.txt", path)
		assert.Equal(t, int64(0), offset)
		assert.Equal(t, int64(11), length)

		return &proto.FileReadResponse{Success: true, Content: expectedContent}, nil
	}

	// ACT
	result, err := s.dispatcher.DispatchFileRead(testContext(t), nodeID, "/srv/file.txt", 0, 11)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, expectedContent, result.Content)
	assert.Empty(t, result.StoragePath, "no storage path expected for inline read response")
}

func TestDispatchFileRead_FailureFromDaemon(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 8
	s.registry.setConnected(nodeID, true)

	s.gateway.requestFileRead = func(_ context.Context, _ uint64, _ string, _ int64, _ int64) (*proto.FileReadResponse, error) {
		return &proto.FileReadResponse{Success: false, Error: "permission denied"}, nil
	}

	// ACT
	result, err := s.dispatcher.DispatchFileRead(testContext(t), nodeID, "/srv/file.txt", 0, 0)

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "file read failed", "error must surface daemon-side failure")
	assert.Contains(t, err.Error(), "permission denied")
}

func TestDispatchFileWrite_Success(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 9
	s.registry.setConnected(nodeID, true)

	var capturedPath string
	var capturedContent []byte
	var capturedMode int32
	var capturedCreateDirs bool

	s.gateway.requestFileWrite = func(_ context.Context, _ uint64, path string, content []byte, mode int32, createDirs bool) error {
		capturedPath = path
		capturedContent = content
		capturedMode = mode
		capturedCreateDirs = createDirs

		return nil
	}

	// ACT
	err := s.dispatcher.DispatchFileWrite(
		testContext(t), nodeID, "/srv/out.txt", []byte("payload"), 0o644, true, OwnerOptions{},
	)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "/srv/out.txt", capturedPath)
	assert.Equal(t, []byte("payload"), capturedContent)
	assert.Equal(t, int32(0o644), capturedMode)
	assert.True(t, capturedCreateDirs)
}

func TestDispatchFileWrite_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 9
	s.registry.setConnected(nodeID, true)

	s.gateway.requestFileWrite = func(_ context.Context, _ uint64, _ string, _ []byte, _ int32, _ bool) error {
		return errors.New("disk full")
	}

	// ACT
	err := s.dispatcher.DispatchFileWrite(
		testContext(t), nodeID, "/srv/out.txt", []byte("payload"), 0o644, true, OwnerOptions{},
	)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

func TestDispatchFileOperation_Success(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 10
	s.registry.setConnected(nodeID, true)

	expectedStat := &proto.FileStat{
		Name:       "x.txt",
		Size:       42,
		Type:       proto.FileType_FILE_TYPE_REGULAR,
		ModifiedAt: timestamppb.New(time.Unix(1700000000, 0)),
	}

	s.gateway.requestFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_STAT, req.Operation)

		return &proto.FileOperationResponse{
			Success: true,
			Result: &proto.FileOperationResponse_StatResult{
				StatResult: &proto.StatResult{Stat: expectedStat},
			},
		}, nil
	}

	req := &proto.FileOperationRequest{
		Operation: proto.FileOperationType_FILE_OPERATION_TYPE_STAT,
		Parameters: &proto.FileOperationRequest_StatParams{
			StatParams: &proto.StatParams{Path: "x.txt"},
		},
	}

	// ACT
	resp, err := s.dispatcher.DispatchFileOperation(testContext(t), nodeID, req)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
	require.NotNil(t, resp.GetStatResult())
	assert.Equal(t, "x.txt", resp.GetStatResult().Stat.Name)
}

func TestDispatchUploadTask_Success(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 11
	s.registry.setConnected(nodeID, true)

	require.NoError(t, s.storage.Write(testContext(t), transferPrefix+"xfer-1/data", []byte("uploaded")))

	var capturedTransferID string
	var capturedDestPath string
	s.gateway.requestUploadTask = func(_ context.Context, _ uint64, transferID, destPath, _ string, _ int64) error {
		capturedTransferID = transferID
		capturedDestPath = destPath

		return nil
	}

	// ACT
	err := s.dispatcher.DispatchUploadTask(testContext(t), nodeID, "xfer-1", "/srv/dest.txt", 0, OwnerOptions{})

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "xfer-1", capturedTransferID)
	assert.Equal(t, "/srv/dest.txt", capturedDestPath)

	// Storage should have the data deleted on successful upload
	assert.False(t, s.storage.Exists(testContext(t), transferPrefix+"xfer-1/data"),
		"upload data must be deleted from storage after successful upload")
}

func TestDispatchDownloadTask_Success(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 12
	s.registry.setConnected(nodeID, true)

	var capturedTransferID string
	var capturedSrcPath string
	s.gateway.requestDownloadTask = func(_ context.Context, _ uint64, transferID, srcPath string) error {
		capturedTransferID = transferID
		capturedSrcPath = srcPath

		return nil
	}

	// ACT
	err := s.dispatcher.DispatchDownloadTask(testContext(t), nodeID, "xfer-2", "/srv/source.bin")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "xfer-2", capturedTransferID)
	assert.Equal(t, "/srv/source.bin", capturedSrcPath)
	assert.Equal(t, int32(1), s.gateway.downloadCalls.Load(), "download gateway must be called once")
}

func TestDispatchUploadTask_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 11
	s.registry.setConnected(nodeID, true)
	s.gateway.requestUploadTask = func(_ context.Context, _ uint64, _, _, _ string, _ int64) error {
		return errors.New("upload boom")
	}

	// ACT
	err := s.dispatcher.DispatchUploadTask(testContext(t), nodeID, "xfer-x", "/srv/dest", 0, OwnerOptions{})

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload boom")
}

func TestDispatchDownloadTask_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 12
	s.registry.setConnected(nodeID, true)
	s.gateway.requestDownloadTask = func(_ context.Context, _ uint64, _, _ string) error {
		return errors.New("download boom")
	}

	// ACT
	err := s.dispatcher.DispatchDownloadTask(testContext(t), nodeID, "xfer-y", "/srv/src")

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download boom")
}

func TestDispatch_NotConnected_TimesOut(t *testing.T) {
	// When the registry says the daemon is not connected, no goroutine spawns
	// to publish a response. The dispatcher's wait will surface as context
	// cancellation when the caller cancels.

	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 99
	s.registry.setConnected(nodeID, false)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// ACT
	resp, err := s.dispatcher.DispatchFileList(ctx, nodeID, "/x", false, "")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "dispatcher must surface context deadline when no daemon answers")
}

func TestDispatch_ContextCancelled(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 100

	// Block in the gateway so dispatcher waits for response.
	startedCh := make(chan struct{}, 1)
	releaseCh := make(chan struct{})
	s.registry.setConnected(nodeID, true)
	s.gateway.requestFileList = func(ctx context.Context, _ uint64, _ string, _ bool, _ string) (*proto.FileListResponse, error) {
		startedCh <- struct{}{}
		select {
		case <-releaseCh:
		case <-ctx.Done():
		}

		return &proto.FileListResponse{Success: true}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan error, 1)
	go func() {
		_, err := s.dispatcher.DispatchFileList(ctx, nodeID, "/x", false, "")
		resultCh <- err
	}()

	// Wait for the gateway to be invoked, then cancel
	select {
	case <-startedCh:
	case <-time.After(2 * time.Second):
		cancel()
		close(releaseCh)
		t.Fatal("gateway was not invoked within 2s")
	}

	// ACT
	cancel()
	close(releaseCh)

	// ASSERT
	select {
	case err := <-resultCh:
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled, "cancelled context must propagate")
	case <-time.After(2 * time.Second):
		t.Fatal("DispatchFileList did not return after cancellation")
	}
}

func TestStart_DispatcherSubscribesAndDeliversResponses(t *testing.T) {
	// Sanity check that Start has wired up the request-handler subscription
	// (verified indirectly by every other Dispatch* test, but exercise once
	// to guarantee Start returns no error and routes one request end-to-end).

	// ARRANGE
	s := setupDispatcher(t)
	const nodeID uint64 = 1
	s.registry.setConnected(nodeID, true)
	s.gateway.requestFileList = func(_ context.Context, _ uint64, _ string, _ bool, _ string) (*proto.FileListResponse, error) {
		return &proto.FileListResponse{Success: true}, nil
	}

	// ACT
	resp, err := s.dispatcher.DispatchFileList(testContext(t), nodeID, "/x", false, "")

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
}

func TestExecFile_PayloadParseError(t *testing.T) {
	tests := []struct {
		name string
		op   string
	}{
		{name: "file_list", op: "file_list"},
		{name: "file_read", op: "file_read"},
		{name: "file_write", op: "file_write"},
		{name: "file_operation", op: "file_operation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			s := setupDispatcher(t)
			d := s.dispatcher.(*fileDispatcher)

			// ACT
			resp := d.executeFileRequest(testContext(t), parsePayloadHelper(tt.op, []byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9}))

			// ASSERT
			assert.NotEmpty(t, resp.Error, "malformed payload must yield error response")
		})
	}
}

func TestExec_GatewayErrorReturnsPayloadError(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	d := s.dispatcher.(*fileDispatcher)

	s.gateway.requestFileList = func(_ context.Context, _ uint64, _ string, _ bool, _ string) (*proto.FileListResponse, error) {
		return nil, errors.New("gw err")
	}

	req := &proto.FileListRequest{Path: "/x"}
	data, err := req.MarshalVT()
	require.NoError(t, err)

	// ACT
	resp := d.execFileList(testContext(t), parsePayloadHelper("file_list", data))

	// ASSERT
	assert.Equal(t, "gw err", resp.Error)
}

func TestExecuteFileRequest_UnsupportedOperation(t *testing.T) {
	// ARRANGE
	s := setupDispatcher(t)
	d := s.dispatcher.(*fileDispatcher)

	// ACT
	resp := d.executeFileRequest(testContext(t), parsePayloadHelper("nonsense", nil))

	// ASSERT
	assert.Contains(t, resp.Error, "unsupported operation: nonsense")
}

func parsePayloadHelper(op string, data []byte) messages.DaemonFileRequestPayload {
	return messages.DaemonFileRequestPayload{
		NodeID:    1,
		RequestID: "req-test",
		Operation: op,
		Data:      data,
	}
}
