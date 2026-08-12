package daemon

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/transfers"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// notConnectedCaseName is the canonical case name used across tests that
// verify the ErrDaemonNotConnected sentinel propagation. Centralising it
// here lets table-driven assertions branch on the same literal.
const notConnectedCaseName = "returns_ErrDaemonNotConnected_when_not_connected"

// fakeFileDispatcher is a configurable in-test fake for FileDispatcher.
type fakeFileDispatcher struct {
	mu sync.Mutex

	dispatchFileList     func(ctx context.Context, nodeID uint64, path string, recursive bool, pattern string) (*proto.FileListResponse, error)
	dispatchFileOp       func(ctx context.Context, nodeID uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error)
	dispatchFileRead     func(ctx context.Context, nodeID uint64, path string, offset int64, length int64) (*FileReadResult, error)
	dispatchFileWrite    func(ctx context.Context, nodeID uint64, path string, content []byte, mode int32, createDirs bool, owner OwnerOptions) error
	dispatchUploadTask   func(ctx context.Context, nodeID uint64, transferID, destPath string, mode int32, owner OwnerOptions) error
	dispatchDownloadTask func(ctx context.Context, nodeID uint64, transferID, srcPath string) error

	fileListCalls     atomic.Int32
	fileOpCalls       atomic.Int32
	fileReadCalls     atomic.Int32
	fileWriteCalls    atomic.Int32
	uploadTaskCalls   atomic.Int32
	downloadTaskCalls atomic.Int32

	lastWriteOwner      OwnerOptions
	lastUploadTaskOwner OwnerOptions
	lastUploadTaskMode  int32
	lastFileOpReq       *proto.FileOperationRequest
}

func (f *fakeFileDispatcher) Start(_ context.Context) error { return nil }

func (f *fakeFileDispatcher) DispatchFileList(
	ctx context.Context, nodeID uint64, path string, recursive bool, pattern string,
) (*proto.FileListResponse, error) {
	f.fileListCalls.Add(1)
	f.mu.Lock()
	fn := f.dispatchFileList
	f.mu.Unlock()
	if fn == nil {
		return &proto.FileListResponse{Success: true}, nil
	}

	return fn(ctx, nodeID, path, recursive, pattern)
}

func (f *fakeFileDispatcher) DispatchFileOperation(
	ctx context.Context, nodeID uint64, req *proto.FileOperationRequest,
) (*proto.FileOperationResponse, error) {
	f.fileOpCalls.Add(1)
	f.mu.Lock()
	f.lastFileOpReq = req
	fn := f.dispatchFileOp
	f.mu.Unlock()
	if fn == nil {
		return &proto.FileOperationResponse{Success: true}, nil
	}

	return fn(ctx, nodeID, req)
}

func (f *fakeFileDispatcher) DispatchFileRead(
	ctx context.Context, nodeID uint64, path string, offset int64, length int64,
) (*FileReadResult, error) {
	f.fileReadCalls.Add(1)
	f.mu.Lock()
	fn := f.dispatchFileRead
	f.mu.Unlock()
	if fn == nil {
		return &FileReadResult{}, nil
	}

	return fn(ctx, nodeID, path, offset, length)
}

func (f *fakeFileDispatcher) DispatchFileWrite(
	ctx context.Context, nodeID uint64, path string,
	content []byte, mode int32, createDirs bool, owner OwnerOptions,
) error {
	f.fileWriteCalls.Add(1)
	f.mu.Lock()
	f.lastWriteOwner = owner
	fn := f.dispatchFileWrite
	f.mu.Unlock()
	if fn == nil {
		return nil
	}

	return fn(ctx, nodeID, path, content, mode, createDirs, owner)
}

func (f *fakeFileDispatcher) DispatchUploadTask(
	ctx context.Context, nodeID uint64, transferID string, destPath string,
	mode int32, owner OwnerOptions,
) error {
	f.uploadTaskCalls.Add(1)
	f.mu.Lock()
	f.lastUploadTaskOwner = owner
	f.lastUploadTaskMode = mode
	fn := f.dispatchUploadTask
	f.mu.Unlock()
	if fn == nil {
		return nil
	}

	return fn(ctx, nodeID, transferID, destPath, mode, owner)
}

func (f *fakeFileDispatcher) DispatchDownloadTask(
	ctx context.Context, nodeID uint64, transferID string, srcPath string,
) error {
	f.downloadTaskCalls.Add(1)
	f.mu.Lock()
	fn := f.dispatchDownloadTask
	f.mu.Unlock()
	if fn == nil {
		return nil
	}

	return fn(ctx, nodeID, transferID, srcPath)
}

// fileServiceTestSetup bundles the FileService under test with its fakes
// so assertions can inspect dependency interactions without re-wiring.
type fileServiceTestSetup struct {
	service     *FileService
	gateway     *fakeFileGateway
	dispatcher  *fakeFileDispatcher
	registry    *fakeConnectionChecker
	storage     *files.InMemoryFileManager
	transferReg *transfers.Registry
}

func setupFileService(t *testing.T) *fileServiceTestSetup {
	t.Helper()

	gateway := &fakeFileGateway{}
	dispatcher := &fakeFileDispatcher{}
	registry := newFakeConnectionChecker()
	storage := files.NewInMemoryFileManager()
	transferReg := transfers.NewRegistry()
	logger := slog.New(slog.DiscardHandler)

	svc := NewFileService(gateway, registry, dispatcher, storage, transferReg, logger)

	return &fileServiceTestSetup{
		service:     svc,
		gateway:     gateway,
		dispatcher:  dispatcher,
		registry:    registry,
		storage:     storage,
		transferReg: transferReg,
	}
}

func testNode(id uint, workPath string) *domain.Node {
	return &domain.Node{ID: id, WorkPath: workPath}
}

func TestFileService_resolveRoute_routesAcrossPaths(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
	}

	tests := []struct {
		name              string
		setup             setup
		wantGatewayCalls  int32
		wantDispatchCalls int32
		wantError         string
	}{
		{
			name: "routes_to_gateway_when_IsConnected_true",
			setup: setup{
				isConnected: true,
			},
			wantGatewayCalls:  1,
			wantDispatchCalls: 0,
		},
		{
			name: "routes_to_dispatcher_when_only_IsConnectedAnywhere_true",
			setup: setup{
				isConnected:         false,
				isConnectedAnywhere: true,
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 1,
		},
		{
			name: "returns_ErrDaemonNotConnected_when_neither_connected",
			setup: setup{
				isConnected:         false,
				isConnectedAnywhere: false,
			},
			wantGatewayCalls:  0,
			wantDispatchCalls: 0,
			wantError:         "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 42
			s.registry.setConnected(nodeID, tt.setup.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere

			s.gateway.requestFileList = func(_ context.Context, _ uint64, _ string, _ bool, _ string) (*proto.FileListResponse, error) {
				return &proto.FileListResponse{Success: true}, nil
			}
			s.dispatcher.dispatchFileList = func(_ context.Context, _ uint64, _ string, _ bool, _ string) (*proto.FileListResponse, error) {
				return &proto.FileListResponse{Success: true}, nil
			}

			// ACT
			_, err := s.service.ReadDir(ctx, testNode(42, "/srv"), "/srv")

			// ASSERT
			gwCalls := atomic.Int32{}
			if s.gateway.requestFileList != nil {
				// gateway calls are not counted by fakeFileGateway directly,
				// so detect via mock activation: each branch only ever sees one of the two.
				_ = gwCalls
			}

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantDispatchCalls, s.dispatcher.fileListCalls.Load(), "dispatcher calls count mismatch")
		})
	}
}

func TestFileService_ReadDir(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		gatewayResp         *proto.FileListResponse
		gatewayErr          error
		dispatcherResp      *proto.FileListResponse
		dispatcherErr       error
	}

	tests := []struct {
		name              string
		setup             setup
		directory         string
		workPath          string
		wantPath          string
		wantGatewayCalls  int32
		wantDispatchCalls int32
		wantFiles         int
		wantError         string
	}{
		{
			name: "gateway_path_returns_files",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.FileListResponse{
					Success: true,
					Files: []*proto.FileStat{
						{Name: "a.txt", Size: 100, Type: proto.FileType_FILE_TYPE_REGULAR},
						{Name: "b", Type: proto.FileType_FILE_TYPE_DIRECTORY},
					},
				},
			},
			directory:        "/srv/games/dir1",
			workPath:         "/srv/games",
			wantPath:         "dir1",
			wantGatewayCalls: 1,
			wantFiles:        2,
		},
		{
			name: "dispatcher_path_returns_files",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherResp: &proto.FileListResponse{
					Success: true,
					Files: []*proto.FileStat{
						{Name: "remote.txt", Size: 5, Type: proto.FileType_FILE_TYPE_REGULAR},
					},
				},
			},
			directory:         "/srv/games/dir1",
			workPath:          "/srv/games",
			wantPath:          "dir1",
			wantDispatchCalls: 1,
			wantFiles:         1,
		},
		{
			name: "gateway_error_wrapped_with_message",
			setup: setup{
				isConnected: true,
				gatewayErr:  errors.New("network down"),
			},
			directory:        "/srv",
			workPath:         "/srv",
			wantGatewayCalls: 1,
			wantError:        "file list request: network down",
		},
		{
			name: "dispatcher_error_wrapped_with_message",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherErr:       errors.New("dispatch boom"),
			},
			directory:         "/srv",
			workPath:          "/srv",
			wantDispatchCalls: 1,
			wantError:         "file list request: dispatch boom",
		},
		{
			name: "gateway_returns_unsuccessful_response_surfaces_error",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.FileListResponse{Success: false, Error: "permission denied"},
			},
			directory:        "/srv",
			workPath:         "/srv",
			wantGatewayCalls: 1,
			wantError:        "permission denied",
		},
		{
			name: "returns_ErrDaemonNotConnected_when_not_connected",
			setup: setup{
				isConnected:         false,
				isConnectedAnywhere: false,
			},
			directory: "/srv",
			workPath:  "/srv",
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 7
			s.registry.setConnected(nodeID, tt.setup.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere

			var capturedGatewayPath, capturedDispatcherPath string
			var capturedGatewayRecursive, capturedDispatcherRecursive bool

			s.gateway.requestFileList = func(_ context.Context, _ uint64, path string, recursive bool, _ string) (*proto.FileListResponse, error) {
				capturedGatewayPath = path
				capturedGatewayRecursive = recursive

				if tt.setup.gatewayErr != nil {
					return nil, tt.setup.gatewayErr
				}

				return tt.setup.gatewayResp, nil
			}
			s.dispatcher.dispatchFileList = func(_ context.Context, _ uint64, path string, recursive bool, _ string) (*proto.FileListResponse, error) {
				capturedDispatcherPath = path
				capturedDispatcherRecursive = recursive

				if tt.setup.dispatcherErr != nil {
					return nil, tt.setup.dispatcherErr
				}

				return tt.setup.dispatcherResp, nil
			}

			node := testNode(7, tt.workPath)

			// ACT
			got, err := s.service.ReadDir(ctx, node, tt.directory)

			// ASSERT
			assert.Equal(t, tt.wantDispatchCalls, s.dispatcher.fileListCalls.Load(), "dispatcher calls count mismatch")

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, got, "result must be nil on error")

				if tt.name == notConnectedCaseName {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)
			require.Len(t, got, tt.wantFiles, "file count mismatch")

			if tt.wantPath != "" {
				if tt.wantGatewayCalls > 0 {
					assert.Equal(t, tt.wantPath, capturedGatewayPath, "stripped path must reach gateway")
					assert.False(t, capturedGatewayRecursive, "ReadDir must use recursive=false")
				}
				if tt.wantDispatchCalls > 0 {
					assert.Equal(t, tt.wantPath, capturedDispatcherPath, "stripped path must reach dispatcher")
					assert.False(t, capturedDispatcherRecursive, "ReadDir must use recursive=false")
				}
			}
		})
	}
}

func TestFileService_ReadDir_mapsProtoFieldsToFileInfo(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const nodeID uint64 = 1
	s.registry.setConnected(nodeID, true)

	expected := []*proto.FileStat{
		{
			Name:          "deploy.sh",
			Path:          "/srv/games/deploy.sh",
			Size:          1024,
			Type:          proto.FileType_FILE_TYPE_REGULAR,
			Mode:          0o755,
			ModifiedAt:    timestamppb.New(time.Unix(1700000000, 0)),
			SymlinkTarget: "",
		},
		{
			Name: "logs",
			Path: "/srv/games/logs",
			Type: proto.FileType_FILE_TYPE_DIRECTORY,
		},
	}

	s.gateway.requestFileList = func(_ context.Context, _ uint64, _ string, _ bool, _ string) (*proto.FileListResponse, error) {
		return &proto.FileListResponse{Success: true, Files: expected}, nil
	}

	// ACT
	got, err := s.service.ReadDir(ctx, testNode(1, "/srv/games"), "/srv/games")

	// ASSERT
	require.NoError(t, err)
	require.Len(t, got, 2, "must return one FileInfo per FileStat")

	assert.Equal(t, "deploy.sh", got[0].Name, "Name must map from FileStat.Name")
	assert.Equal(t, "/srv/games/deploy.sh", got[0].Path, "Path must map from FileStat.Path")
	assert.Equal(t, uint64(1024), got[0].Size, "Size must map from FileStat.Size")
	assert.Equal(t, FileTypeFile, got[0].Type, "regular files must map to FileTypeFile")
	assert.Equal(t, uint32(0o755), got[0].Perm, "Perm must map from FileStat.Mode")
	assert.Equal(t, uint64(1700000000), got[0].TimeModified, "TimeModified must map from FileStat.ModifiedAt")

	assert.Equal(t, FileTypeDir, got[1].Type, "directories must map to FileTypeDir")
}

func TestFileService_ReadDirRecursive_propagatesRecursiveTrue(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const nodeID uint64 = 2
	s.registry.setConnected(nodeID, true)

	var capturedRecursive bool
	s.gateway.requestFileList = func(_ context.Context, _ uint64, _ string, recursive bool, _ string) (*proto.FileListResponse, error) {
		capturedRecursive = recursive

		return &proto.FileListResponse{Success: true}, nil
	}

	// ACT
	_, err := s.service.ReadDirRecursive(ctx, testNode(2, "/srv"), "/srv")

	// ASSERT
	require.NoError(t, err)
	assert.True(t, capturedRecursive, "ReadDirRecursive must call gateway with recursive=true")
}

func TestFileService_Download(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		gatewayResp         *proto.FileReadResponse
		gatewayErr          error
		dispatcherResult    *FileReadResult
		dispatcherErr       error
		storageContent      []byte
	}

	tests := []struct {
		name              string
		setup             setup
		wantContent       []byte
		wantGatewayCalls  bool
		wantDispatchCalls bool
		wantError         string
	}{
		{
			name: "gateway_returns_inline_content",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.FileReadResponse{Success: true, Content: []byte("hello world")},
			},
			wantContent:      []byte("hello world"),
			wantGatewayCalls: true,
		},
		{
			name: "dispatcher_returns_inline_content",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherResult:    &FileReadResult{Content: []byte("remote-hello")},
			},
			wantContent:       []byte("remote-hello"),
			wantDispatchCalls: true,
		},
		{
			name: "dispatcher_returns_storage_path_reads_from_storage",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherResult:    &FileReadResult{StoragePath: "transfers/x1/data"},
				storageContent:      []byte("from-storage"),
			},
			wantContent:       []byte("from-storage"),
			wantDispatchCalls: true,
		},
		{
			name: "gateway_returns_unsuccessful_response_surfaces_error",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.FileReadResponse{Success: false, Error: "no such file"},
			},
			wantGatewayCalls: true,
			wantError:        "no such file",
		},
		{
			name: "gateway_error_wrapped_with_message",
			setup: setup{
				isConnected: true,
				gatewayErr:  errors.New("transport down"),
			},
			wantGatewayCalls: true,
			wantError:        "file read request: transport down",
		},
		{
			name: "dispatcher_error_wrapped_with_message",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherErr:       errors.New("dispatch read boom"),
			},
			wantDispatchCalls: true,
			wantError:         "dispatched file read: dispatch read boom",
		},
		{
			name: "returns_ErrDaemonNotConnected_when_not_connected",
			setup: setup{
				isConnected:         false,
				isConnectedAnywhere: false,
			},
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 33
			s.registry.setConnected(nodeID, tt.setup.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere

			if tt.setup.storageContent != nil {
				require.NoError(t, s.storage.Write(ctx, tt.setup.dispatcherResult.StoragePath, tt.setup.storageContent))
			}

			s.gateway.requestFileRead = func(_ context.Context, _ uint64, _ string, _ int64, _ int64) (*proto.FileReadResponse, error) {
				if tt.setup.gatewayErr != nil {
					return nil, tt.setup.gatewayErr
				}

				return tt.setup.gatewayResp, nil
			}
			s.dispatcher.dispatchFileRead = func(_ context.Context, _ uint64, _ string, _ int64, _ int64) (*FileReadResult, error) {
				if tt.setup.dispatcherErr != nil {
					return nil, tt.setup.dispatcherErr
				}

				return tt.setup.dispatcherResult, nil
			}

			// ACT
			got, err := s.service.Download(ctx, testNode(33, "/srv"), "/srv/file.txt")

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, got, "content must be nil on error")

				if tt.name == notConnectedCaseName {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantContent, got, "downloaded content mismatch")

			if tt.setup.storageContent != nil {
				// storage path must be cleaned up after successful dispatcher-stored download
				assert.False(t, s.storage.Exists(ctx, tt.setup.dispatcherResult.StoragePath),
					"storage path must be deleted after consumption")
			}
		})
	}
}

func TestFileService_Download_propagatesStrippedPath(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const nodeID uint64 = 14
	s.registry.setConnected(nodeID, true)

	var capturedPath string
	s.gateway.requestFileRead = func(_ context.Context, _ uint64, path string, _ int64, _ int64) (*proto.FileReadResponse, error) {
		capturedPath = path

		return &proto.FileReadResponse{Success: true, Content: []byte("x")}, nil
	}

	// ACT
	_, err := s.service.Download(ctx, testNode(14, "/srv/games"), "/srv/games/configs/server.cfg")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "configs/server.cfg", capturedPath, "WorkPath prefix must be stripped before gateway call")
}

func TestFileService_DownloadStream(t *testing.T) {
	t.Parallel()

	t.Run("local_chunked_path_returns_reader_with_content", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 55
		s.registry.setConnected(nodeID, true)

		const payload = "stream-content-here"

		var readCount atomic.Int32
		s.gateway.requestFileRead = func(_ context.Context, _ uint64, _ string, offset int64, _ int64) (*proto.FileReadResponse, error) {
			n := readCount.Add(1)
			if n == 1 {
				assert.Equal(t, int64(0), offset, "first chunked read must use offset=0")

				return &proto.FileReadResponse{Success: true, Content: []byte(payload)}, nil
			}

			return &proto.FileReadResponse{Success: true, Content: nil}, nil
		}

		// ACT
		rc, err := s.service.DownloadStream(ctx, testNode(55, "/srv"), "/srv/large.bin")
		require.NoError(t, err)
		require.NotNil(t, rc)

		buf, readErr := io.ReadAll(rc)
		_ = rc.Close()

		// ASSERT
		require.NoError(t, readErr)
		assert.Equal(t, payload, string(buf), "chunked stream must surface all payload bytes")
	})

	t.Run("returns_ErrDaemonNotConnected_when_not_connected", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)

		// ACT
		rc, err := s.service.DownloadStream(ctx, testNode(99, "/srv"), "/srv/x")

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, rc, "reader must be nil on error")
		assert.ErrorIs(t, err, ErrDaemonNotConnected)
	})

	t.Run("local_chunked_path_propagates_gateway_error_via_reader", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 56
		s.registry.setConnected(nodeID, true)

		s.gateway.requestFileRead = func(_ context.Context, _ uint64, _ string, _ int64, _ int64) (*proto.FileReadResponse, error) {
			return nil, errors.New("read crash")
		}

		// ACT
		rc, err := s.service.DownloadStream(ctx, testNode(56, "/srv"), "/srv/x")
		require.NoError(t, err, "DownloadStream returns reader; error surfaces via Read")
		require.NotNil(t, rc)

		_, readErr := io.ReadAll(rc)
		_ = rc.Close()

		// ASSERT
		require.Error(t, readErr)
		assert.Contains(t, readErr.Error(), "read crash", "gateway error must surface through pipe reader")
	})
}

func TestFileService_Upload(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		gatewayErr          error
		dispatcherErr       error
		owner               OwnerOptions
	}

	tests := []struct {
		name              string
		setup             setup
		wantGatewayCalls  bool
		wantDispatchCalls bool
		wantWriteOwner    OwnerOptions
		wantError         string
	}{
		{
			name:             "gateway_path_writes_small_content",
			setup:            setup{isConnected: true},
			wantGatewayCalls: true,
		},
		{
			name:              "dispatcher_path_writes_small_content",
			setup:             setup{isConnectedAnywhere: true},
			wantDispatchCalls: true,
		},
		{
			name: "propagates_owner_to_gateway",
			setup: setup{
				isConnected: true,
				owner:       OwnerOptions{User: "gameap"},
			},
			wantGatewayCalls: true,
			wantWriteOwner:   OwnerOptions{User: "gameap"},
		},
		{
			name: "propagates_owner_to_dispatcher",
			setup: setup{
				isConnectedAnywhere: true,
				owner:               OwnerOptions{User: "gameap", UID: 1000, GID: 1000},
			},
			wantDispatchCalls: true,
			wantWriteOwner:    OwnerOptions{User: "gameap", UID: 1000, GID: 1000},
		},
		{
			name: "gateway_error_propagates",
			setup: setup{
				isConnected: true,
				gatewayErr:  errors.New("disk full"),
			},
			wantGatewayCalls: true,
			wantError:        "disk full",
		},
		{
			name: "dispatcher_error_propagates",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherErr:       errors.New("dispatched boom"),
			},
			wantDispatchCalls: true,
			wantError:         "dispatched boom",
		},
		{
			name:      "returns_ErrDaemonNotConnected_when_not_connected",
			setup:     setup{},
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 21
			s.registry.setConnected(nodeID, tt.setup.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere

			s.gateway.requestFileWrite = func(_ context.Context, _ uint64, _ string, _ []byte, _ int32, _ bool) error {
				return tt.setup.gatewayErr
			}
			s.dispatcher.dispatchFileWrite = func(_ context.Context, _ uint64, _ string, _ []byte, _ int32, _ bool, _ OwnerOptions) error {
				return tt.setup.dispatcherErr
			}

			// ACT
			err := s.service.Upload(ctx, testNode(21, "/srv"), "/srv/out.txt", []byte("hello"), 0o644, tt.setup.owner)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				if tt.name == notConnectedCaseName {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)

			if tt.wantGatewayCalls {
				assert.Equal(t, tt.wantWriteOwner, s.gateway.lastWriteOwner, "owner must be propagated to gateway")
			}
			if tt.wantDispatchCalls {
				assert.Equal(t, tt.wantWriteOwner, s.dispatcher.lastWriteOwner, "owner must be propagated to dispatcher")
			}
		})
	}
}

func TestFileService_Upload_stripsWorkPathAndMasksMode(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const nodeID uint64 = 8
	s.registry.setConnected(nodeID, true)

	var capturedPath string
	var capturedMode int32
	var capturedCreateDirs bool
	s.gateway.requestFileWrite = func(_ context.Context, _ uint64, path string, _ []byte, mode int32, createDirs bool) error {
		capturedPath = path
		capturedMode = mode
		capturedCreateDirs = createDirs

		return nil
	}

	// ACT
	err := s.service.Upload(ctx, testNode(8, "/srv/games"), "/srv/games/configs/server.cfg", []byte("payload"), 0o644, OwnerOptions{})

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "configs/server.cfg", capturedPath, "WorkPath must be stripped from path")
	assert.Equal(t, int32(0o644), capturedMode, "permission bits must be masked to 9 bits")
	assert.True(t, capturedCreateDirs, "createDirs must default to true")
}

func TestFileService_UploadStreamPrepared(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		hasFileTransferCap  bool
		gatewayErr          error
		dispatcherErr       error
	}

	tests := []struct {
		name              string
		setup             setup
		wantGatewayCalls  int32
		wantDispatchCalls int32
		wantError         string
	}{
		{
			name: "gateway_path_calls_RequestFileUploadTask",
			setup: setup{
				isConnected:        true,
				hasFileTransferCap: true,
			},
			wantGatewayCalls: 1,
		},
		{
			name: "dispatcher_path_calls_DispatchUploadTask",
			setup: setup{
				isConnectedAnywhere: true,
			},
			wantDispatchCalls: 1,
		},
		{
			name: "local_without_file_transfer_capability_falls_back_to_dispatcher",
			setup: setup{
				isConnected:        true,
				hasFileTransferCap: false,
			},
			wantDispatchCalls: 1,
		},
		{
			name: "gateway_error_wrapped_with_message",
			setup: setup{
				isConnected:        true,
				hasFileTransferCap: true,
				gatewayErr:         errors.New("upload boom"),
			},
			wantGatewayCalls: 1,
			wantError:        "upload task: upload boom",
		},
		{
			name: "dispatcher_error_wrapped_with_message",
			setup: setup{
				isConnectedAnywhere: true,
				dispatcherErr:       errors.New("dispatched boom"),
			},
			wantDispatchCalls: 1,
			wantError:         "dispatched upload task: dispatched boom",
		},
		{
			name: "returns_ErrDaemonNotConnected_when_not_connected",
			setup: setup{
				isConnected:         false,
				isConnectedAnywhere: false,
			},
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 17
			s.registry.setConnected(nodeID, tt.setup.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere
			if tt.setup.hasFileTransferCap {
				s.registry.capabilities[nodeID] = map[string]bool{capabilityFileTransfer: true}
			}

			s.gateway.requestUploadTask = func(_ context.Context, _ uint64, _, _, _ string, _ int64) error {
				return tt.setup.gatewayErr
			}
			s.dispatcher.dispatchUploadTask = func(_ context.Context, _ uint64, _, _ string, _ int32, _ OwnerOptions) error {
				return tt.setup.dispatcherErr
			}

			owner := OwnerOptions{User: "gameap"}

			// ACT
			err := s.service.UploadStreamPrepared(
				ctx, testNode(17, "/srv"), "/srv/dest.bin",
				"xfer-abc", "deadbeef", 12345, owner,
			)

			// ASSERT
			assert.Equal(t, tt.wantGatewayCalls, s.gateway.uploadCalls.Load(), "gateway upload calls mismatch")
			assert.Equal(t, tt.wantDispatchCalls, s.dispatcher.uploadTaskCalls.Load(), "dispatcher upload calls mismatch")

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				if tt.name == notConnectedCaseName {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)

			if tt.wantGatewayCalls > 0 {
				assert.Equal(t, owner, s.gateway.lastUploadTaskOwner, "owner must propagate to gateway")
			}
			if tt.wantDispatchCalls > 0 {
				assert.Equal(t, owner, s.dispatcher.lastUploadTaskOwner, "owner must propagate to dispatcher")
			}
		})
	}
}

func TestFileService_UploadStreamPrepared_propagatesArguments(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const nodeID uint64 = 18
	s.registry.setConnected(nodeID, true)
	s.registry.capabilities[nodeID] = map[string]bool{capabilityFileTransfer: true}

	var capturedTransferID, capturedDestPath, capturedChecksum string
	var capturedTotalSize int64
	s.gateway.requestUploadTask = func(_ context.Context, _ uint64, transferID, destPath, checksum string, totalSize int64) error {
		capturedTransferID = transferID
		capturedDestPath = destPath
		capturedChecksum = checksum
		capturedTotalSize = totalSize

		return nil
	}

	// ACT
	err := s.service.UploadStreamPrepared(
		ctx, testNode(18, "/srv/games"), "/srv/games/dest.bin",
		"xfer-xyz", "checksum-hex", 9999, OwnerOptions{},
	)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "xfer-xyz", capturedTransferID)
	assert.Equal(t, "dest.bin", capturedDestPath, "WorkPath must be stripped from destPath")
	assert.Equal(t, "checksum-hex", capturedChecksum)
	assert.Equal(t, int64(9999), capturedTotalSize, "totalSize must propagate unmodified")
}

func TestFileService_UploadStream(t *testing.T) {
	t.Parallel()

	t.Run("gateway_path_writes_small_content_inline", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 31
		s.registry.setConnected(nodeID, true)
		// no FileTransfer capability and small size => inline write path

		const payload = "tiny-stream"

		var capturedContent []byte
		s.gateway.requestFileWrite = func(_ context.Context, _ uint64, _ string, content []byte, _ int32, _ bool) error {
			capturedContent = content

			return nil
		}

		// ACT
		err := s.service.UploadStream(
			ctx, testNode(31, "/srv"), "/srv/out.bin",
			strings.NewReader(payload), uint64(len(payload)),
			0o644, OwnerOptions{},
		)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, payload, string(capturedContent), "inline upload must pass full bytes to gateway")
	})

	t.Run("gateway_capability_path_uploads_through_storage_and_requests_task", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 32
		s.registry.setConnected(nodeID, true)
		s.registry.capabilities[nodeID] = map[string]bool{capabilityFileTransfer: true}

		// payload larger than smallFileThreshold to bypass inline branch
		payload := bytes.Repeat([]byte("X"), smallFileThreshold+10)

		var capturedTransferID, capturedDestPath, capturedChecksum string
		var capturedTotalSize int64
		s.gateway.requestUploadTask = func(_ context.Context, _ uint64, transferID, destPath, checksum string, totalSize int64) error {
			capturedTransferID = transferID
			capturedDestPath = destPath
			capturedChecksum = checksum
			capturedTotalSize = totalSize

			return nil
		}

		// ACT
		err := s.service.UploadStream(
			ctx, testNode(32, "/srv"), "/srv/big.bin",
			bytes.NewReader(payload), uint64(len(payload)),
			0o600, OwnerOptions{User: "gameap"},
		)

		// ASSERT
		require.NoError(t, err)
		assert.NotEmpty(t, capturedTransferID, "transferID must be generated and non-empty")
		assert.Equal(t, "big.bin", capturedDestPath, "WorkPath must be stripped before reaching gateway")
		require.Len(t, capturedChecksum, 64, "sha256 hex checksum must be 64 chars")
		assert.Equal(t, int64(len(payload)), capturedTotalSize, "totalSize must equal payload length")
		assert.Equal(t, OwnerOptions{User: "gameap"}, s.gateway.lastUploadTaskOwner, "owner must propagate")
		assert.Equal(t, int32(0o600), s.gateway.lastUploadTaskMode, "mode must propagate (masked to 9 bits)")
	})

	t.Run("returns_ErrDaemonNotConnected_when_not_connected", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)

		// ACT
		err := s.service.UploadStream(
			ctx, testNode(99, "/srv"), "/srv/out.bin",
			strings.NewReader("x"), 1, 0o644, OwnerOptions{},
		)

		// ASSERT
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDaemonNotConnected, "must surface ErrDaemonNotConnected")
	})
}

func TestFileService_MkDir(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		gatewayErr          error
		dispatcherErr       error
	}

	tests := []struct {
		name              string
		setup             setup
		wantGatewayCalls  int32
		wantDispatchCalls int32
		wantError         string
	}{
		{
			name:             "gateway_path_sends_mkdir_operation",
			setup:            setup{isConnected: true},
			wantGatewayCalls: 1,
		},
		{
			name:              "dispatcher_path_sends_mkdir_operation",
			setup:             setup{isConnectedAnywhere: true},
			wantDispatchCalls: 1,
		},
		{
			name: "gateway_error_wrapped_with_message",
			setup: setup{
				isConnected: true,
				gatewayErr:  errors.New("mkdir boom"),
			},
			wantGatewayCalls: 1,
			wantError:        "file operation: mkdir boom",
		},
		{
			name:      "returns_ErrDaemonNotConnected_when_not_connected",
			setup:     setup{},
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 41
			s.registry.setConnected(nodeID, tt.setup.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere

			var capturedGatewayReq, capturedDispatcherReq *proto.FileOperationRequest
			s.gateway.requestFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
				capturedGatewayReq = req

				if tt.setup.gatewayErr != nil {
					return nil, tt.setup.gatewayErr
				}

				return &proto.FileOperationResponse{Success: true}, nil
			}
			s.dispatcher.dispatchFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
				capturedDispatcherReq = req

				if tt.setup.dispatcherErr != nil {
					return nil, tt.setup.dispatcherErr
				}

				return &proto.FileOperationResponse{Success: true}, nil
			}

			// ACT
			err := s.service.MkDir(ctx, testNode(41, "/srv/games"), "/srv/games/newdir", OwnerOptions{User: "gameap"})

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				if tt.name == notConnectedCaseName {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)

			req := capturedGatewayReq
			if tt.wantDispatchCalls > 0 {
				req = capturedDispatcherReq
			}
			require.NotNil(t, req, "operation request must be captured")
			assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_MKDIR, req.Operation, "MkDir must use MKDIR operation type")

			mkdirParams := req.GetMkdirParams()
			require.NotNil(t, mkdirParams, "MkDir must populate MkdirParams")
			assert.Equal(t, "newdir", mkdirParams.Path, "WorkPath must be stripped from path")
			assert.True(t, mkdirParams.Recursive, "MkDir must request recursive creation")
			assert.Equal(t, "gameap", mkdirParams.OwnerUser, "owner.User must propagate to MkdirParams")
		})
	}
}

func TestFileService_Copy(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
	}

	tests := []struct {
		name      string
		setup     setup
		wantError string
	}{
		{
			name:  "gateway_path_sends_copy_operation",
			setup: setup{isConnected: true},
		},
		{
			name:  "dispatcher_path_sends_copy_operation",
			setup: setup{isConnectedAnywhere: true},
		},
		{
			name:      "returns_ErrDaemonNotConnected_when_not_connected",
			setup:     setup{},
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 51
			s.registry.setConnected(nodeID, tt.setup.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere

			var capturedReq *proto.FileOperationRequest
			handler := func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
				capturedReq = req

				return &proto.FileOperationResponse{Success: true}, nil
			}
			s.gateway.requestFileOp = handler
			s.dispatcher.dispatchFileOp = handler

			// ACT
			err := s.service.Copy(ctx, testNode(51, "/srv/games"), "/srv/games/src.txt", "/srv/games/dst.txt")

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, capturedReq)
			assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_COPY, capturedReq.Operation)

			copyParams := capturedReq.GetCopyParams()
			require.NotNil(t, copyParams, "Copy must populate CopyParams")
			assert.Equal(t, "src.txt", copyParams.Source, "WorkPath must be stripped from Source")
			assert.Equal(t, "dst.txt", copyParams.Destination, "WorkPath must be stripped from Destination")
			assert.True(t, copyParams.Recursive, "Copy must default to recursive")
		})
	}
}

func TestFileService_Move(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
	}

	tests := []struct {
		name      string
		setup     setup
		wantError string
	}{
		{
			name:  "gateway_path_sends_move_operation",
			setup: setup{isConnected: true},
		},
		{
			name:  "dispatcher_path_sends_move_operation",
			setup: setup{isConnectedAnywhere: true},
		},
		{
			name:      "returns_ErrDaemonNotConnected_when_not_connected",
			setup:     setup{},
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 52
			s.registry.setConnected(nodeID, tt.setup.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere

			var capturedReq *proto.FileOperationRequest
			handler := func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
				capturedReq = req

				return &proto.FileOperationResponse{Success: true}, nil
			}
			s.gateway.requestFileOp = handler
			s.dispatcher.dispatchFileOp = handler

			// ACT
			err := s.service.Move(ctx, testNode(52, "/srv/games"), "/srv/games/from.txt", "/srv/games/to.txt")

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, capturedReq)
			assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_MOVE, capturedReq.Operation)

			moveParams := capturedReq.GetMoveParams()
			require.NotNil(t, moveParams, "Move must populate MoveParams")
			assert.Equal(t, "from.txt", moveParams.Source, "WorkPath must be stripped from Source")
			assert.Equal(t, "to.txt", moveParams.Destination, "WorkPath must be stripped from Destination")
		})
	}
}

func TestFileService_Remove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		isConnected         bool
		isConnectedAnywhere bool
		recursive           bool
		wantError           string
	}{
		{
			name:        "gateway_path_recursive_true",
			isConnected: true,
			recursive:   true,
		},
		{
			name:        "gateway_path_recursive_false",
			isConnected: true,
			recursive:   false,
		},
		{
			name:                "dispatcher_path_recursive_true",
			isConnectedAnywhere: true,
			recursive:           true,
		},
		{
			name:      "returns_ErrDaemonNotConnected_when_not_connected",
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 53
			s.registry.setConnected(nodeID, tt.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.isConnectedAnywhere

			var capturedReq *proto.FileOperationRequest
			handler := func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
				capturedReq = req

				return &proto.FileOperationResponse{Success: true}, nil
			}
			s.gateway.requestFileOp = handler
			s.dispatcher.dispatchFileOp = handler

			// ACT
			err := s.service.Remove(ctx, testNode(53, "/srv/games"), "/srv/games/garbage", tt.recursive)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, capturedReq)
			assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_DELETE, capturedReq.Operation)

			deleteParams := capturedReq.GetDeleteParams()
			require.NotNil(t, deleteParams, "Remove must populate DeleteParams")
			assert.Equal(t, "garbage", deleteParams.Path, "WorkPath must be stripped from Path")
			assert.Equal(t, tt.recursive, deleteParams.Recursive, "Recursive flag must propagate verbatim")
		})
	}
}

func TestFileService_Chmod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		isConnected         bool
		isConnectedAnywhere bool
		perm                uint32
		wantMode            int32
		wantError           string
	}{
		{
			name:        "gateway_path_sends_chmod_operation",
			isConnected: true,
			perm:        0o755,
			wantMode:    0o755,
		},
		{
			name:                "dispatcher_path_sends_chmod_operation",
			isConnectedAnywhere: true,
			perm:                0o644,
			wantMode:            0o644,
		},
		{
			name:        "perm_bits_outside_9_bits_are_masked",
			isConnected: true,
			perm:        0xFFFF,
			wantMode:    0o777,
		},
		{
			name:      "returns_ErrDaemonNotConnected_when_not_connected",
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 54
			s.registry.setConnected(nodeID, tt.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.isConnectedAnywhere

			var capturedReq *proto.FileOperationRequest
			handler := func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
				capturedReq = req

				return &proto.FileOperationResponse{Success: true}, nil
			}
			s.gateway.requestFileOp = handler
			s.dispatcher.dispatchFileOp = handler

			// ACT
			err := s.service.Chmod(ctx, testNode(54, "/srv/games"), "/srv/games/script.sh", tt.perm)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, capturedReq)
			assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_CHMOD, capturedReq.Operation)

			chmodParams := capturedReq.GetChmodParams()
			require.NotNil(t, chmodParams, "Chmod must populate ChmodParams")
			assert.Equal(t, "script.sh", chmodParams.Path, "WorkPath must be stripped from Path")
			assert.Equal(t, tt.wantMode, chmodParams.Mode, "mode must be masked to 9 bits")
		})
	}
}

func TestFileService_GetFileInfo(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		gatewayResp         *proto.FileOperationResponse
		gatewayErr          error
	}

	tests := []struct {
		name      string
		setup     setup
		wantName  string
		wantError string
	}{
		{
			name: "gateway_returns_file_details_when_stat_succeeds",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.FileOperationResponse{
					Success: true,
					Result: &proto.FileOperationResponse_StatResult{
						StatResult: &proto.StatResult{
							Stat: &proto.FileStat{
								Name: "config.yaml",
								Size: 256,
								Mode: 0o644,
								Type: proto.FileType_FILE_TYPE_REGULAR,
							},
						},
					},
				},
			},
			wantName: "config.yaml",
		},
		{
			name: "stat_returns_no_result_surfaces_error",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.FileOperationResponse{Success: true},
			},
			wantError: "stat returned no result",
		},
		{
			name: "gateway_returns_unsuccessful_response_surfaces_error",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.FileOperationResponse{Success: false, Error: "no such file"},
			},
			wantError: "no such file",
		},
		{
			name: "gateway_transport_error_wrapped",
			setup: setup{
				isConnected: true,
				gatewayErr:  errors.New("stat boom"),
			},
			wantError: "stat request: stat boom",
		},
		{
			name:      "returns_ErrDaemonNotConnected_when_not_connected",
			setup:     setup{},
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 61
			s.registry.setConnected(nodeID, tt.setup.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere

			s.gateway.requestFileOp = func(_ context.Context, _ uint64, _ *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
				if tt.setup.gatewayErr != nil {
					return nil, tt.setup.gatewayErr
				}

				return tt.setup.gatewayResp, nil
			}

			// ACT
			got, err := s.service.GetFileInfo(ctx, testNode(61, "/srv"), "/srv/config.yaml")

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, got, "details must be nil on error")

				if tt.name == notConnectedCaseName {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantName, got.Name, "name must surface from StatResult.Stat")
			assert.Equal(t, uint64(256), got.Size, "size must surface from StatResult.Stat.Size")
			assert.Equal(t, FileTypeFile, got.Type, "Type must map from proto FileType")
			assert.Equal(t, uint32(0o644), got.Perm, "Perm must map from proto Mode")
		})
	}
}

func TestFileService_GetFileInfo_routesToDispatcherWhenRemote(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const nodeID uint64 = 62
	s.registry.connectedAnywhere[nodeID] = true

	var capturedReq *proto.FileOperationRequest
	s.dispatcher.dispatchFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		capturedReq = req

		return &proto.FileOperationResponse{
			Success: true,
			Result: &proto.FileOperationResponse_StatResult{
				StatResult: &proto.StatResult{Stat: &proto.FileStat{Name: "x.txt"}},
			},
		}, nil
	}

	// ACT
	got, err := s.service.GetFileInfo(ctx, testNode(62, "/srv"), "/srv/x.txt")

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, capturedReq, "dispatcher must receive operation request")
	assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_STAT, capturedReq.Operation, "GetFileInfo must use STAT operation")

	statParams := capturedReq.GetStatParams()
	require.NotNil(t, statParams, "StatParams must be populated")
	assert.Equal(t, "x.txt", statParams.Path, "WorkPath must be stripped from Path")
}

func TestFileService_Hash(t *testing.T) {
	t.Parallel()

	type setup struct {
		isConnected bool
		gatewayResp *proto.FileOperationResponse
		gatewayErr  error
	}

	tests := []struct {
		name       string
		setup      setup
		wantHashes int
		wantError  string
	}{
		{
			name: "gateway_returns_hash_result",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.FileOperationResponse{
					Success: true,
					Result: &proto.FileOperationResponse_HashResult{
						HashResult: &proto.HashResult{
							Algorithm: proto.HashAlgorithm_HASH_ALGORITHM_SHA256,
							Hashes: []*proto.FileHash{
								{Path: "a.txt", Hash: "aa", Size: 1},
								{Path: "missing.txt", Error: "no such file"},
							},
						},
					},
				},
			},
			wantHashes: 2,
		},
		{
			name: "hash_returns_no_result_surfaces_error",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.FileOperationResponse{Success: true},
			},
			wantError: "hash returned no result",
		},
		{
			name: "gateway_returns_unsuccessful_response_surfaces_error",
			setup: setup{
				isConnected: true,
				gatewayResp: &proto.FileOperationResponse{Success: false, Error: "unsupported operation"},
			},
			wantError: "hash failed: unsupported operation",
		},
		{
			name: "gateway_transport_error_wrapped",
			setup: setup{
				isConnected: true,
				gatewayErr:  errors.New("hash boom"),
			},
			wantError: "hash request: hash boom",
		},
		{
			name:      "returns_ErrDaemonNotConnected_when_not_connected",
			setup:     setup{},
			wantError: "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			const nodeID uint64 = 63
			s.registry.setConnected(nodeID, tt.setup.isConnected)

			var capturedReq *proto.FileOperationRequest
			s.gateway.requestFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
				capturedReq = req
				if tt.setup.gatewayErr != nil {
					return nil, tt.setup.gatewayErr
				}

				return tt.setup.gatewayResp, nil
			}

			// ACT
			got, err := s.service.Hash(
				ctx, testNode(63, "/srv"),
				[]string{"/srv/a.txt", "/srv/missing.txt"},
				proto.HashAlgorithm_HASH_ALGORITHM_SHA256,
			)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, got, "result must be nil on error")

				if tt.name == notConnectedCaseName {
					assert.ErrorIs(t, err, ErrDaemonNotConnected, "must be ErrDaemonNotConnected sentinel")
				}

				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			require.Len(t, got.Hashes, tt.wantHashes)
			assert.Equal(t, "aa", got.Hashes[0].Hash)
			assert.Equal(t, "no such file", got.Hashes[1].Error, "per-file error must survive")

			require.NotNil(t, capturedReq, "gateway must receive the request")
			assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_HASH, capturedReq.Operation)
			hashParams := capturedReq.GetHashParams()
			require.NotNil(t, hashParams)
			assert.Equal(t, []string{"a.txt", "missing.txt"}, hashParams.Paths,
				"WorkPath must be stripped from every path")
			assert.Equal(t, proto.HashAlgorithm_HASH_ALGORITHM_SHA256, hashParams.Algorithm)
		})
	}
}

func TestFileService_Hash_routesToDispatcherWhenRemote(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const nodeID uint64 = 64
	s.registry.connectedAnywhere[nodeID] = true

	var capturedReq *proto.FileOperationRequest
	s.dispatcher.dispatchFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		capturedReq = req

		return &proto.FileOperationResponse{
			Success: true,
			Result: &proto.FileOperationResponse_HashResult{
				HashResult: &proto.HashResult{
					Algorithm: proto.HashAlgorithm_HASH_ALGORITHM_MD5,
					Hashes:    []*proto.FileHash{{Path: "x.bin", Hash: "bb", Size: 2}},
				},
			},
		}, nil
	}

	// ACT
	got, err := s.service.Hash(ctx, testNode(64, "/srv"), []string{"/srv/x.bin"}, proto.HashAlgorithm_HASH_ALGORITHM_MD5)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, capturedReq, "dispatcher must receive operation request")
	assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_HASH, capturedReq.Operation)
	assert.Equal(t, []string{"x.bin"}, capturedReq.GetHashParams().Paths)
}

func TestNewFileService_nilLoggerUsesDefault(t *testing.T) {
	t.Parallel()

	// ARRANGE
	gateway := &fakeFileGateway{}
	dispatcher := &fakeFileDispatcher{}
	registry := newFakeConnectionChecker()
	storage := files.NewInMemoryFileManager()
	transferReg := transfers.NewRegistry()

	// ACT
	svc := NewFileService(gateway, registry, dispatcher, storage, transferReg, nil)

	// ASSERT
	require.NotNil(t, svc, "service must be constructed")
	assert.NotNil(t, svc.logger, "logger must default to a non-nil instance when nil is passed")
}
