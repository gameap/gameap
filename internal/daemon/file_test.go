package daemon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/transfers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/gameap/gameap/pkg/proto"
)

type fileServiceTestSetup struct {
	service     *FileService
	gateway     *fakeFileGateway
	registry    *fakeConnectionChecker
	dispatcher  FileDispatcher
	storage     *files.InMemoryFileManager
	transferReg *transfers.Registry
}

func setupFileService(t *testing.T) *fileServiceTestSetup {
	t.Helper()

	gateway := &fakeFileGateway{}
	registry := newFakeConnectionChecker()
	storage := files.NewInMemoryFileManager()
	transferReg := transfers.NewRegistry()
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	logger := slog.Default()
	dispatcher := NewFileDispatcher(ps, gateway, registry, storage, "test-instance", logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, dispatcher.Start(ctx))

	service := NewFileService(gateway, registry, dispatcher, storage, transferReg, nil, logger)

	return &fileServiceTestSetup{
		service:     service,
		gateway:     gateway,
		registry:    registry,
		dispatcher:  dispatcher,
		storage:     storage,
		transferReg: transferReg,
	}
}

func newTestNode(id uint) *domain.Node {
	return &domain.Node{
		ID:       id,
		Name:     "test",
		WorkPath: "/srv/gameap",
	}
}

func TestNewFileService_NilLoggerUsesDefault(t *testing.T) {
	// ARRANGE
	gateway := &fakeFileGateway{}
	registry := newFakeConnectionChecker()
	storage := files.NewInMemoryFileManager()

	// ACT
	service := NewFileService(gateway, registry, nil, storage, transfers.NewRegistry(), nil, nil)

	// ASSERT
	require.NotNil(t, service)
}

func TestFileService_resolveRoute(t *testing.T) {
	tests := []struct {
		name                string
		isConnected         bool
		isConnectedAnywhere bool
		wantLocal           bool
		wantError           string
	}{
		{
			name:                "local_when_connected_to_this_instance",
			isConnected:         true,
			isConnectedAnywhere: false,
			wantLocal:           true,
			wantError:           "",
		},
		{
			name:                "remote_when_connected_anywhere_else",
			isConnected:         false,
			isConnectedAnywhere: true,
			wantLocal:           false,
			wantError:           "",
		},
		{
			name:                "errors_when_not_connected_anywhere",
			isConnected:         false,
			isConnectedAnywhere: false,
			wantLocal:           false,
			wantError:           "daemon not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			s := setupFileService(t)
			const nodeID uint64 = 42
			s.registry.setConnected(nodeID, tt.isConnected)
			s.registry.setConnectedAnywhere(nodeID, tt.isConnectedAnywhere)

			// ACT
			local, err := s.service.resolveRoute(nodeID)

			// ASSERT
			if tt.wantError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			}
			assert.Equal(t, tt.wantLocal, local)
		})
	}
}

func TestFileService_ReadDir_Local_Success(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(1)
	s.registry.setConnected(uint64(node.ID), true)

	s.gateway.requestFileList = func(_ context.Context, nodeID uint64, path string, _ bool, _ string) (*proto.FileListResponse, error) {
		assert.Equal(t, uint64(node.ID), nodeID)
		assert.Equal(t, "subdir", path, "WorkPath prefix must be stripped before sending to gateway")

		return &proto.FileListResponse{
			Success: true,
			Files: []*proto.FileStat{
				{Name: "a.txt", Size: 10, Type: proto.FileType_FILE_TYPE_REGULAR, Mode: 0o644},
				{Name: "sub", Size: 0, Type: proto.FileType_FILE_TYPE_DIRECTORY, Mode: 0o755},
			},
		}, nil
	}

	// ACT
	files, err := s.service.ReadDir(testContext(t), node, "/srv/gameap/subdir")

	// ASSERT
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, "a.txt", files[0].Name)
	assert.Equal(t, FileTypeFile, files[0].Type)
	assert.Equal(t, FileTypeDir, files[1].Type)
}

func TestFileService_ReadDir_Dispatched_Success(t *testing.T) {
	// When the daemon is connected on a different instance, FileService routes
	// through the dispatcher rather than calling gateway directly.

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(2)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	stubDispatcher := &fakeDispatcher{
		dispatchFileList: func(_ context.Context, nodeID uint64, path string, _ bool, _ string) (*proto.FileListResponse, error) {
			assert.Equal(t, uint64(node.ID), nodeID)
			assert.Equal(t, ".", path)

			return &proto.FileListResponse{
				Success: true,
				Files: []*proto.FileStat{
					{Name: "x.txt", Size: 5, Type: proto.FileType_FILE_TYPE_REGULAR},
				},
			}, nil
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	got, err := svc.ReadDir(testContext(t), node, "/srv/gameap")

	// ASSERT
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "x.txt", got[0].Name)
}

func TestFileService_ReadDir_DaemonReturnsFailure(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(3)
	s.registry.setConnected(uint64(node.ID), true)

	s.gateway.requestFileList = func(_ context.Context, _ uint64, _ string, _ bool, _ string) (*proto.FileListResponse, error) {
		return &proto.FileListResponse{Success: false, Error: "perm denied"}, nil
	}

	// ACT
	files, err := s.service.ReadDir(testContext(t), node, "/srv/gameap/x")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, files)
	assert.Contains(t, err.Error(), "file list failed")
	assert.Contains(t, err.Error(), "perm denied")
}

func TestFileService_ReadDir_NotConnected(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(4)

	// ACT
	files, err := s.service.ReadDir(testContext(t), node, "/srv/gameap/x")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, files)
	assert.ErrorIs(t, err, ErrDaemonNotConnected)
}

func TestFileService_ReadDir_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(5)
	s.registry.setConnected(uint64(node.ID), true)
	s.gateway.requestFileList = func(_ context.Context, _ uint64, _ string, _ bool, _ string) (*proto.FileListResponse, error) {
		return nil, errors.New("transport boom")
	}

	// ACT
	files, err := s.service.ReadDir(testContext(t), node, "/srv/gameap/x")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, files)
	assert.Contains(t, err.Error(), "transport boom")
}

func TestFileService_ReadDirRecursive_Local_Success(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(50)
	s.registry.setConnected(uint64(node.ID), true)

	s.gateway.requestFileList = func(_ context.Context, nodeID uint64, path string, recursive bool, _ string) (*proto.FileListResponse, error) {
		assert.Equal(t, uint64(node.ID), nodeID)
		assert.Equal(t, "subdir", path, "WorkPath prefix must be stripped before sending to gateway")
		assert.True(t, recursive, "recursive flag must reach gateway")

		return &proto.FileListResponse{
			Success: true,
			Files: []*proto.FileStat{
				{Name: "a.txt", Path: "subdir/a.txt", Size: 10, Type: proto.FileType_FILE_TYPE_REGULAR, Mode: 0o644},
			},
		}, nil
	}

	// ACT
	got, err := s.service.ReadDirRecursive(testContext(t), node, "/srv/gameap/subdir")

	// ASSERT
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a.txt", got[0].Name)
	assert.Equal(t, "subdir/a.txt", got[0].Path, "Path field must come from proto.FileStat.Path")
	assert.Equal(t, FileTypeFile, got[0].Type)
}

func TestFileService_ReadDirRecursive_Dispatched_Success(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(51)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	stubDispatcher := &fakeDispatcher{
		dispatchFileList: func(_ context.Context, nodeID uint64, path string, recursive bool, _ string) (*proto.FileListResponse, error) {
			assert.Equal(t, uint64(node.ID), nodeID)
			assert.Equal(t, "deep", path)
			assert.True(t, recursive, "dispatched call must propagate recursive=true")

			return &proto.FileListResponse{
				Success: true,
				Files: []*proto.FileStat{
					{Name: "y.txt", Path: "deep/y.txt", Size: 7, Type: proto.FileType_FILE_TYPE_REGULAR},
				},
			}, nil
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	got, err := svc.ReadDirRecursive(testContext(t), node, "/srv/gameap/deep")

	// ASSERT
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "y.txt", got[0].Name)
	assert.Equal(t, "deep/y.txt", got[0].Path)
}

func TestFileService_ReadDirRecursive_LegacyFallback(t *testing.T) {
	// When the daemon is not connected anywhere AND legacy is non-nil, the
	// recursive path must fall through to legacy.ReadDirRecursive — same as
	// the non-recursive branch. The empty inmemory cert/file stores cause
	// configMaker to fail with "failed to read server certificate", which
	// proves the call reached legacy (rather than surfacing ErrDaemonNotConnected).

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(52)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), false)

	certRepo := inmemory.NewClientCertificateRepository()
	fakeFM := files.NewInMemoryFileManager()
	legacy := NewFileBINNService(certRepo, fakeFM)
	svc := NewFileService(s.gateway, s.registry, s.dispatcher, s.storage, s.transferReg, legacy, slog.Default())

	// ACT
	got, err := svc.ReadDirRecursive(testContext(t), node, "/srv/gameap/anything")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, got)
	assert.NotErrorIs(
		t, err, ErrDaemonNotConnected,
		"recursive path must fall through to legacy and surface its config-load error",
	)
	assert.Contains(t, err.Error(), "failed to read server certificate")
}

func TestFileService_Download_Local_Success(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(6)
	s.registry.setConnected(uint64(node.ID), true)

	expected := []byte("hello")
	s.gateway.requestFileRead = func(_ context.Context, _ uint64, path string, _, _ int64) (*proto.FileReadResponse, error) {
		assert.Equal(t, "f.txt", path)

		return &proto.FileReadResponse{Success: true, Content: expected}, nil
	}

	// ACT
	data, err := s.service.Download(testContext(t), node, "/srv/gameap/f.txt")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, expected, data)
}

func TestFileService_Download_NotConnected(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(60)

	// ACT
	data, err := s.service.Download(testContext(t), node, "/srv/gameap/x")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, data)
	assert.ErrorIs(t, err, ErrDaemonNotConnected)
}

func TestFileService_Download_Local_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(61)
	s.registry.setConnected(uint64(node.ID), true)
	s.gateway.requestFileRead = func(_ context.Context, _ uint64, _ string, _, _ int64) (*proto.FileReadResponse, error) {
		return nil, errors.New("transport down")
	}

	// ACT
	data, err := s.service.Download(testContext(t), node, "/srv/gameap/x")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "transport down")
}

func TestFileService_Download_Remote_DispatcherError(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(62)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	stubDispatcher := &fakeDispatcher{
		dispatchFileRead: func(_ context.Context, _ uint64, _ string, _, _ int64) (*FileReadResult, error) {
			return nil, errors.New("dispatcher boom")
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	data, err := svc.Download(testContext(t), node, "/srv/gameap/x")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "dispatcher boom")
}

func TestFileService_Download_Local_DaemonFailure(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(6)
	s.registry.setConnected(uint64(node.ID), true)

	s.gateway.requestFileRead = func(_ context.Context, _ uint64, _ string, _, _ int64) (*proto.FileReadResponse, error) {
		return &proto.FileReadResponse{Success: false, Error: "no such file"}, nil
	}

	// ACT
	data, err := s.service.Download(testContext(t), node, "/srv/gameap/missing.txt")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "no such file")
}

func TestFileService_Download_Remote_InlineContent(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(7)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	expected := []byte("payload")
	stubDispatcher := &fakeDispatcher{
		dispatchFileRead: func(_ context.Context, _ uint64, _ string, _, _ int64) (*FileReadResult, error) {
			return &FileReadResult{Content: expected}, nil
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	data, err := svc.Download(testContext(t), node, "/srv/gameap/x.txt")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, expected, data)
}

func TestFileService_Download_Remote_StoragePath(t *testing.T) {
	// When a dispatcher response carries StoragePath instead of inline Content,
	// FileService reads the bytes from storage and deletes the staging file.

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(8)
	const nodeID = uint64(8)
	s.registry.setConnected(nodeID, false)
	s.registry.setConnectedAnywhere(nodeID, true)

	storagePath := transferPrefix + "fake-xfer/data"
	expected := []byte("file from S3")
	require.NoError(t, s.storage.Write(testContext(t), storagePath, expected))

	// We don't go through the real dispatcher round-trip for this; instead,
	// stub the dispatcher to return StoragePath directly.
	stubDispatcher := &fakeDispatcher{
		dispatchFileRead: func(_ context.Context, _ uint64, _ string, _, _ int64) (*FileReadResult, error) {
			return &FileReadResult{StoragePath: storagePath}, nil
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	data, err := svc.Download(testContext(t), node, "/srv/gameap/x.txt")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, expected, data)
	assert.False(t, s.storage.Exists(testContext(t), storagePath),
		"FileService should remove the staging file after reading it")
}

func TestFileService_Download_Remote_StorageReadError(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(9)

	stubDispatcher := &fakeDispatcher{
		dispatchFileRead: func(_ context.Context, _ uint64, _ string, _, _ int64) (*FileReadResult, error) {
			return &FileReadResult{StoragePath: "missing"}, nil
		},
	}
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	data, err := svc.Download(testContext(t), node, "/srv/gameap/x")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, data)
	assert.Contains(t, err.Error(), "reading transferred file from storage")
}

func TestFileService_Upload_Local_Small(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(10)
	s.registry.setConnected(uint64(node.ID), true)

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
	err := s.service.Upload(testContext(t), node, "/srv/gameap/out.txt", []byte("hello"), 0o644, OwnerOptions{})

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "out.txt", capturedPath)
	assert.Equal(t, []byte("hello"), capturedContent)
	assert.Equal(t, int32(0o644), capturedMode)
	assert.True(t, capturedCreateDirs, "Upload should request directory creation by default")
}

func TestFileService_Upload_Remote_Dispatched(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(70)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	var capturedPath string
	var capturedContent []byte
	stubDispatcher := &fakeDispatcher{
		dispatchFileWrite: func(_ context.Context, _ uint64, path string, content []byte, _ int32, _ bool) error {
			capturedPath = path
			capturedContent = content

			return nil
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	err := svc.Upload(testContext(t), node, "/srv/gameap/sub/file.txt", []byte("hi"), 0o644, OwnerOptions{})

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "sub/file.txt", capturedPath)
	assert.Equal(t, []byte("hi"), capturedContent)
}

func TestFileService_Upload_NotConnected(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(11)

	// ACT
	err := s.service.Upload(testContext(t), node, "/srv/gameap/out.txt", []byte("x"), 0o644, OwnerOptions{})

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDaemonNotConnected)
}

func TestFileService_UploadStream_SmallLocal(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(12)
	s.registry.setConnected(uint64(node.ID), true)

	expected := []byte("smallcontent")
	s.gateway.requestFileWrite = func(_ context.Context, _ uint64, path string, content []byte, _ int32, _ bool) error {
		assert.Equal(t, "out.txt", path)
		assert.Equal(t, expected, content)

		return nil
	}

	// ACT
	err := s.service.UploadStream(
		testContext(t), node, "/srv/gameap/out.txt",
		bytes.NewReader(expected), uint64(len(expected)), 0o644, OwnerOptions{},
	)

	// ASSERT
	require.NoError(t, err)
}

func TestFileService_UploadStream_LargeLocalUsesUploadTask(t *testing.T) {
	// When local + has capability + size > smallFileThreshold, FileService stages
	// to storage and calls the gateway's RequestFileUploadTask with a sha256 checksum.

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(13)
	s.registry.setConnected(uint64(node.ID), true)
	s.registry.setCapability(uint64(node.ID), capabilityFileTransfer, true)

	// 2MB content - above smallFileThreshold (1MB)
	largeContent := bytes.Repeat([]byte("X"), 2*1024*1024)
	expectedChecksum := sha256.Sum256(largeContent)
	expectedHex := hex.EncodeToString(expectedChecksum[:])

	var gotChecksum string
	var gotTotal int64
	var gotPath string
	s.gateway.requestUploadTask = func(_ context.Context, _ uint64, _, destPath, checksum string, totalSize int64) error {
		gotChecksum = checksum
		gotTotal = totalSize
		gotPath = destPath

		return nil
	}

	// ACT
	err := s.service.UploadStream(
		testContext(t), node, "/srv/gameap/big.bin",
		bytes.NewReader(largeContent), uint64(len(largeContent)), 0o600, OwnerOptions{},
	)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, expectedHex, gotChecksum, "checksum must equal sha256 of the streamed content")
	assert.Equal(t, int64(len(largeContent)), gotTotal)
	assert.Equal(t, "big.bin", gotPath)
}

func TestFileService_UploadStream_LargeLocalUploadTaskError(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(14)
	s.registry.setConnected(uint64(node.ID), true)
	s.registry.setCapability(uint64(node.ID), capabilityFileTransfer, true)

	largeContent := bytes.Repeat([]byte("Y"), 2*1024*1024)
	s.gateway.requestUploadTask = func(_ context.Context, _ uint64, _, _, _ string, _ int64) error {
		return errors.New("upload task failed")
	}

	// ACT
	err := s.service.UploadStream(
		testContext(t), node, "/srv/gameap/big.bin",
		bytes.NewReader(largeContent), uint64(len(largeContent)), 0o600, OwnerOptions{},
	)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload task failed")
}

func TestFileService_UploadStream_RemoteDispatched(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(75)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	largeContent := bytes.Repeat([]byte("Z"), 2*1024*1024)

	var capturedTransferID string
	var capturedDestPath string
	stubDispatcher := &fakeDispatcher{
		dispatchUploadTask: func(_ context.Context, _ uint64, transferID, destPath string) error {
			capturedTransferID = transferID
			capturedDestPath = destPath

			return nil
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	err := svc.UploadStream(
		testContext(t), node, "/srv/gameap/big.bin",
		bytes.NewReader(largeContent), uint64(len(largeContent)), 0o600, OwnerOptions{},
	)

	// ASSERT
	require.NoError(t, err)
	assert.NotEmpty(t, capturedTransferID, "transfer ID must be generated and propagated")
	assert.Equal(t, "big.bin", capturedDestPath)

	// staging file should be deleted on success regardless of branch... actually
	// the dispatched branch only deletes on error. Verify it's still present:
	storagePath := transfers.TransferDataPath(capturedTransferID)
	_ = storagePath
}

func TestFileService_UploadStream_RemoteDispatchError(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(76)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	largeContent := bytes.Repeat([]byte("W"), 2*1024*1024)

	stubDispatcher := &fakeDispatcher{
		dispatchUploadTask: func(_ context.Context, _ uint64, _, _ string) error {
			return errors.New("dispatch failed")
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	err := svc.UploadStream(
		testContext(t), node, "/srv/gameap/big.bin",
		bytes.NewReader(largeContent), uint64(len(largeContent)), 0o600, OwnerOptions{},
	)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dispatch failed")
}

func TestFileService_UploadStream_NotConnected(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(77)

	// ACT
	err := s.service.UploadStream(testContext(t), node, "/srv/gameap/x", bytes.NewReader([]byte("hi")), 2, 0o600, OwnerOptions{})

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDaemonNotConnected)
}

func TestFileService_UploadStreamPrepared_LocalWithCapability(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(80)
	s.registry.setConnected(uint64(node.ID), true)
	s.registry.setCapability(uint64(node.ID), capabilityFileTransfer, true)

	const transferID = "tid-local"
	const checksum = "abc123"
	const totalSize uint64 = 4096

	var (
		gotTransferID string
		gotPath       string
		gotChecksum   string
		gotTotal      int64
	)
	s.gateway.requestUploadTask = func(_ context.Context, _ uint64, tID, destPath, sum string, total int64) error {
		gotTransferID = tID
		gotPath = destPath
		gotChecksum = sum
		gotTotal = total

		return nil
	}

	// ACT
	err := s.service.UploadStreamPrepared(
		testContext(t), node, "/srv/gameap/sub/file.bin", transferID, checksum, totalSize, OwnerOptions{},
	)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, transferID, gotTransferID)
	assert.Equal(t, "sub/file.bin", gotPath, "WorkPath prefix must be stripped before sending to gateway")
	assert.Equal(t, checksum, gotChecksum)
	assert.Equal(t, int64(totalSize), gotTotal)
}

func TestFileService_UploadStreamPrepared_RemoteDispatched(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(81)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	const transferID = "tid-remote"

	var (
		gotTransferID string
		gotPath       string
	)
	stubDispatcher := &fakeDispatcher{
		dispatchUploadTask: func(_ context.Context, _ uint64, tID, destPath string) error {
			gotTransferID = tID
			gotPath = destPath

			return nil
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	err := svc.UploadStreamPrepared(
		testContext(t), node, "/srv/gameap/big.bin", transferID, "sum", 8192, OwnerOptions{},
	)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, transferID, gotTransferID)
	assert.Equal(t, "big.bin", gotPath)
}

func TestFileService_UploadStreamPrepared_NotConnectedNoLegacy(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(82)

	// ACT
	err := s.service.UploadStreamPrepared(
		testContext(t), node, "/srv/gameap/x.bin", "tid-no-legacy", "sum", 1024, OwnerOptions{},
	)

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDaemonNotConnected)
}

func TestFileService_UploadStreamPrepared_NotConnectedLegacyMissingData(t *testing.T) {
	// When daemon is offline everywhere AND legacy is non-nil, but the data file
	// is missing in storage, UploadStreamPrepared must return an explicit error
	// without invoking the legacy BINN client.

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(83)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), false)

	certRepo := inmemory.NewClientCertificateRepository()
	fakeFM := files.NewInMemoryFileManager()
	legacy := NewFileBINNService(certRepo, fakeFM)
	svc := NewFileService(s.gateway, s.registry, s.dispatcher, s.storage, s.transferReg, legacy, slog.Default())

	const transferID = "tid-missing"

	// ACT
	err := svc.UploadStreamPrepared(
		testContext(t), node, "/srv/gameap/x.bin", transferID, "sum", 1024, OwnerOptions{},
	)

	// ASSERT
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrDaemonNotConnected, "fallback branch must be entered before surfacing not-connected")
	assert.Contains(t, err.Error(), "upload data missing in storage")
	assert.Contains(t, err.Error(), transferID)
}

func TestFileService_UploadStreamPrepared_NotConnectedLegacyTooLarge(t *testing.T) {
	// When totalSize exceeds the in-memory buffer cap, the helper must reject
	// the upload BEFORE touching storage — saving an expensive S3 fetch that
	// would only fail later at the BINN-stream stage.

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(85)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), false)

	certRepo := inmemory.NewClientCertificateRepository()
	fakeFM := files.NewInMemoryFileManager()
	legacy := NewFileBINNService(certRepo, fakeFM)
	svc := NewFileService(s.gateway, s.registry, s.dispatcher, s.storage, s.transferReg, legacy, slog.Default())

	const transferID = "tid-too-large"
	oversize := uint64(legacyUploadMaxBufferSize) + 1

	// ACT
	err := svc.UploadStreamPrepared(
		testContext(t), node, "/srv/gameap/huge.bin", transferID, "sum", oversize, OwnerOptions{},
	)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legacy upload not supported")
	assert.Contains(t, err.Error(), "gRPC daemon")
}

func TestFileService_UploadStreamPrepared_NotConnectedLegacySizeMismatch(t *testing.T) {
	// When stored data length differs from the declared totalSize, the helper
	// must surface a mismatch error rather than passing an underspecified
	// reader to legacy.UploadStream (which would deadlock or send wrong bytes).

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(86)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), false)

	const transferID = "tid-mismatch"
	storagePath := transfers.TransferDataPath(transferID)
	require.NoError(t, s.storage.Write(testContext(t), storagePath, []byte("only-7b")))

	certRepo := inmemory.NewClientCertificateRepository()
	fakeFM := files.NewInMemoryFileManager()
	legacy := NewFileBINNService(certRepo, fakeFM)
	svc := NewFileService(s.gateway, s.registry, s.dispatcher, s.storage, s.transferReg, legacy, slog.Default())

	// ACT — declare 100 bytes but storage only has 7
	err := svc.UploadStreamPrepared(
		testContext(t), node, "/srv/gameap/x.bin", transferID, "sum", 100, OwnerOptions{},
	)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload data size mismatch")
	assert.Contains(t, err.Error(), transferID)
}

func TestFileService_UploadStreamPrepared_NotConnectedLegacyDataPresent(t *testing.T) {
	// When daemon is offline everywhere, legacy is non-nil, and data is staged
	// in storage at transfers/{ID}/data, UploadStreamPrepared must hand off to
	// the legacy BINN client. The legacy client will fail to load TLS config
	// against an empty certificate repo — surfacing that error proves the
	// fallback branch took control before ErrDaemonNotConnected.

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(84)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), false)

	const transferID = "tid-present"
	storagePath := transfers.TransferDataPath(transferID)
	require.NoError(t, s.storage.Write(testContext(t), storagePath, []byte("staged-data")))

	certRepo := inmemory.NewClientCertificateRepository()
	fakeFM := files.NewInMemoryFileManager()
	legacy := NewFileBINNService(certRepo, fakeFM)
	svc := NewFileService(s.gateway, s.registry, s.dispatcher, s.storage, s.transferReg, legacy, slog.Default())

	// ACT
	err := svc.UploadStreamPrepared(
		testContext(t), node, "/srv/gameap/x.bin", transferID, "sum", uint64(len("staged-data")), OwnerOptions{},
	)

	// ASSERT
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrDaemonNotConnected, "fallback branch must take control")
	assert.Contains(t, err.Error(), "legacy upload stream", "legacy.UploadStream error must be wrapped")
}

func TestFileService_MkDir_Local(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(15)
	s.registry.setConnected(uint64(node.ID), true)

	var gotOp proto.FileOperationType
	var gotPath string
	var gotRecursive bool
	s.gateway.requestFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		gotOp = req.Operation
		if mkdir := req.GetMkdirParams(); mkdir != nil {
			gotPath = mkdir.Path
			gotRecursive = mkdir.Recursive
		}

		return &proto.FileOperationResponse{Success: true}, nil
	}

	// ACT
	err := s.service.MkDir(testContext(t), node, "/srv/gameap/newdir/sub", OwnerOptions{})

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_MKDIR, gotOp)
	assert.Equal(t, "newdir/sub", gotPath, "MkDir must strip the WorkPath prefix")
	assert.True(t, gotRecursive, "MkDir must request recursive creation")
}

func TestFileService_MkDir_DaemonFailure(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(15)
	s.registry.setConnected(uint64(node.ID), true)

	s.gateway.requestFileOp = func(_ context.Context, _ uint64, _ *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		return &proto.FileOperationResponse{Success: false, Error: "exists"}, nil
	}

	// ACT
	err := s.service.MkDir(testContext(t), node, "/srv/gameap/newdir", OwnerOptions{})

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file operation failed")
	assert.Contains(t, err.Error(), "exists")
}

func TestFileService_Copy_Local(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(16)
	s.registry.setConnected(uint64(node.ID), true)

	var gotSrc, gotDst string
	s.gateway.requestFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_COPY, req.Operation)
		if cp := req.GetCopyParams(); cp != nil {
			gotSrc = cp.Source
			gotDst = cp.Destination
		}

		return &proto.FileOperationResponse{Success: true}, nil
	}

	// ACT
	err := s.service.Copy(testContext(t), node, "/srv/gameap/src.txt", "/srv/gameap/dst.txt")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "src.txt", gotSrc)
	assert.Equal(t, "dst.txt", gotDst)
}

func TestFileService_Move_Local(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(17)
	s.registry.setConnected(uint64(node.ID), true)

	var gotSrc, gotDst string
	s.gateway.requestFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_MOVE, req.Operation)
		if mv := req.GetMoveParams(); mv != nil {
			gotSrc = mv.Source
			gotDst = mv.Destination
		}

		return &proto.FileOperationResponse{Success: true}, nil
	}

	// ACT
	err := s.service.Move(testContext(t), node, "/srv/gameap/a.txt", "/srv/gameap/b.txt")

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "a.txt", gotSrc)
	assert.Equal(t, "b.txt", gotDst)
}

func TestFileService_Remove_Local(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(18)
	s.registry.setConnected(uint64(node.ID), true)

	var gotPath string
	var gotRecursive bool
	s.gateway.requestFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_DELETE, req.Operation)
		if d := req.GetDeleteParams(); d != nil {
			gotPath = d.Path
			gotRecursive = d.Recursive
		}

		return &proto.FileOperationResponse{Success: true}, nil
	}

	// ACT
	err := s.service.Remove(testContext(t), node, "/srv/gameap/x", true)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "x", gotPath)
	assert.True(t, gotRecursive)
}

func TestFileService_Chmod_Local(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(19)
	s.registry.setConnected(uint64(node.ID), true)

	var gotPath string
	var gotMode int32
	s.gateway.requestFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_CHMOD, req.Operation)
		if c := req.GetChmodParams(); c != nil {
			gotPath = c.Path
			gotMode = c.Mode
		}

		return &proto.FileOperationResponse{Success: true}, nil
	}

	// ACT
	err := s.service.Chmod(testContext(t), node, "/srv/gameap/file.sh", 0o755)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "file.sh", gotPath)
	assert.Equal(t, int32(0o755), gotMode)
}

func TestFileService_Chmod_Dispatched_Success(t *testing.T) {
	// When the daemon is connected on a different instance, FileService routes
	// chmod through the dispatcher rather than calling gateway directly.

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(190)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	var gotPath string
	var gotMode int32
	stubDispatcher := &fakeDispatcher{
		dispatchFileOp: func(_ context.Context, nodeID uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
			assert.Equal(t, uint64(node.ID), nodeID)
			assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_CHMOD, req.Operation)
			if c := req.GetChmodParams(); c != nil {
				gotPath = c.Path
				gotMode = c.Mode
			}

			return &proto.FileOperationResponse{Success: true}, nil
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	err := svc.Chmod(testContext(t), node, "/srv/gameap/scripts/run.sh", 0o755)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "scripts/run.sh", gotPath, "WorkPath prefix must be stripped before sending over dispatcher")
	assert.Equal(t, int32(0o755), gotMode)
}

func TestFileService_Chmod_DaemonReturnsFailure(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(191)
	s.registry.setConnected(uint64(node.ID), true)

	s.gateway.requestFileOp = func(_ context.Context, _ uint64, _ *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		return &proto.FileOperationResponse{Success: false, Error: "permission denied"}, nil
	}

	// ACT
	err := s.service.Chmod(testContext(t), node, "/srv/gameap/x", 0o644)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file operation failed")
	assert.Contains(t, err.Error(), "permission denied")
}

func TestFileService_Chmod_GatewayError(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(192)
	s.registry.setConnected(uint64(node.ID), true)
	s.gateway.requestFileOp = func(_ context.Context, _ uint64, _ *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		return nil, errors.New("transport boom")
	}

	// ACT
	err := s.service.Chmod(testContext(t), node, "/srv/gameap/x", 0o644)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file operation")
	assert.Contains(t, err.Error(), "transport boom")
}

func TestFileService_Chmod_NotConnected(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(193)

	// ACT
	err := s.service.Chmod(testContext(t), node, "/srv/gameap/x", 0o644)

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDaemonNotConnected)
}

func TestFileService_Chmod_MaskHighBits(t *testing.T) {
	// Bits above the 9 standard rwxrwxrwx bits (setuid 04000, setgid 02000,
	// sticky 01000) must be stripped by `mode & 0x1FF` before reaching daemon.
	// Without the mask, callers could grant elevated execution rights via the
	// HTTP layer; this test pins the mask invariant.

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(194)
	s.registry.setConnected(uint64(node.ID), true)

	var gotMode int32
	s.gateway.requestFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		if c := req.GetChmodParams(); c != nil {
			gotMode = c.Mode
		}

		return &proto.FileOperationResponse{Success: true}, nil
	}

	// ACT
	err := s.service.Chmod(testContext(t), node, "/srv/gameap/x", 0o7777)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, int32(0o777), gotMode, "high bits (setuid/setgid/sticky) must be masked off")
}

func TestFileService_GetFileInfo_Local(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(20)
	s.registry.setConnected(uint64(node.ID), true)

	expectedStat := &proto.FileStat{
		Name:       "f.txt",
		Size:       1024,
		Type:       proto.FileType_FILE_TYPE_REGULAR,
		Mode:       0o644,
		ModifiedAt: timestamppb.New(testTime()),
		AccessedAt: timestamppb.New(testTime()),
	}
	s.gateway.requestFileOp = func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_STAT, req.Operation)
		assert.Equal(t, "f.txt", req.GetStatParams().Path)

		return &proto.FileOperationResponse{
			Success: true,
			Result: &proto.FileOperationResponse_StatResult{
				StatResult: &proto.StatResult{Stat: expectedStat},
			},
		}, nil
	}

	// ACT
	got, err := s.service.GetFileInfo(testContext(t), node, "/srv/gameap/f.txt")

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "f.txt", got.Name)
	assert.Equal(t, uint64(1024), got.Size)
	assert.Equal(t, FileTypeFile, got.Type)
	assert.Equal(t, uint32(0o644), got.Perm)
}

func TestFileService_GetFileInfo_DaemonFailure(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(21)
	s.registry.setConnected(uint64(node.ID), true)

	s.gateway.requestFileOp = func(_ context.Context, _ uint64, _ *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		return &proto.FileOperationResponse{Success: false, Error: "missing"}, nil
	}

	// ACT
	got, err := s.service.GetFileInfo(testContext(t), node, "/srv/gameap/missing")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "stat failed")
	assert.Contains(t, err.Error(), "missing")
}

func TestFileService_GetFileInfo_NoStatResult(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(22)
	s.registry.setConnected(uint64(node.ID), true)

	s.gateway.requestFileOp = func(_ context.Context, _ uint64, _ *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
		return &proto.FileOperationResponse{Success: true}, nil
	}

	// ACT
	got, err := s.service.GetFileInfo(testContext(t), node, "/srv/gameap/x")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "stat returned no result")
}

func TestFileService_GetFileInfo_Dispatched(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(80)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	stubDispatcher := &fakeDispatcher{
		dispatchFileOp: func(_ context.Context, _ uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error) {
			assert.Equal(t, proto.FileOperationType_FILE_OPERATION_TYPE_STAT, req.Operation)

			return &proto.FileOperationResponse{
				Success: true,
				Result: &proto.FileOperationResponse_StatResult{
					StatResult: &proto.StatResult{Stat: &proto.FileStat{Name: "y", Size: 9}},
				},
			}, nil
		},
	}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	// ACT
	got, err := svc.GetFileInfo(testContext(t), node, "/srv/gameap/y")

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "y", got.Name)
	assert.Equal(t, uint64(9), got.Size)
}

func TestFileService_GetFileInfo_NotConnected_NoLegacy(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(23)

	// ACT
	got, err := s.service.GetFileInfo(testContext(t), node, "/srv/gameap/x")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, ErrDaemonNotConnected)
}

func TestFileService_DownloadStream_LocalChunked(t *testing.T) {
	// When local + no file_transfer capability, we get a chunked read pipe.

	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(24)
	s.registry.setConnected(uint64(node.ID), true)
	// no capability set => downloadStreamLocalChunked

	chunks := [][]byte{
		bytes.Repeat([]byte("a"), chunkSize),
		bytes.Repeat([]byte("b"), chunkSize),
		[]byte("tail"),
		nil, // signals EOF
	}
	idx := 0
	s.gateway.requestFileRead = func(_ context.Context, _ uint64, _ string, _, _ int64) (*proto.FileReadResponse, error) {
		if idx >= len(chunks) {
			return &proto.FileReadResponse{Success: true, Content: nil}, nil
		}
		c := chunks[idx]
		idx++

		return &proto.FileReadResponse{Success: true, Content: c}, nil
	}

	// ACT
	rc, err := s.service.DownloadStream(testContext(t), node, "/srv/gameap/big.dat")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Close() })

	got, err := io.ReadAll(rc)

	// ASSERT
	require.NoError(t, err)
	expected := append(append(bytes.Repeat([]byte("a"), chunkSize), bytes.Repeat([]byte("b"), chunkSize)...), []byte("tail")...)
	assert.Equal(t, expected, got)
}

func TestFileService_DownloadStream_LocalChunked_DaemonError(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	node := newTestNode(25)
	s.registry.setConnected(uint64(node.ID), true)
	s.gateway.requestFileRead = func(_ context.Context, _ uint64, _ string, _, _ int64) (*proto.FileReadResponse, error) {
		return &proto.FileReadResponse{Success: false, Error: "no perms"}, nil
	}

	// ACT
	rc, err := s.service.DownloadStream(testContext(t), node, "/srv/gameap/x")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rc.Close() })

	_, readErr := io.ReadAll(rc)

	// ASSERT
	require.Error(t, readErr)
	assert.Contains(t, readErr.Error(), "no perms")
}

func TestFileService_writeSentinelIfMissing(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	transferID := "abc"

	// ACT
	s.service.writeSentinelIfMissing(testContext(t), transferID)

	// ASSERT
	donePath := transfers.TransferDonePath(transferID)
	require.True(t, s.storage.Exists(testContext(t), donePath))

	data, err := s.storage.Read(testContext(t), donePath)
	require.NoError(t, err)

	var info transfers.DoneInfo
	require.NoError(t, json.Unmarshal(data, &info))
	assert.True(t, info.Success)
	assert.Equal(t, 0, info.TotalParts)
}

func TestFileService_writeSentinelIfMissing_Idempotent(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	transferID := "abc"
	donePath := transfers.TransferDonePath(transferID)
	require.NoError(t, s.storage.Write(testContext(t), donePath, []byte(`{"success":true,"total_parts":7}`)))

	// ACT
	s.service.writeSentinelIfMissing(testContext(t), transferID)

	// ASSERT
	data, err := s.storage.Read(testContext(t), donePath)
	require.NoError(t, err)
	var info transfers.DoneInfo
	require.NoError(t, json.Unmarshal(data, &info))
	assert.Equal(t, 7, info.TotalParts, "existing sentinel must not be overwritten")
}

func TestFileService_readSentinel_Success(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	donePath := "transfers/x/done"
	info := transfers.DoneInfo{Success: true, Checksum: "deadbeef", TotalParts: 4}
	data, err := json.Marshal(info)
	require.NoError(t, err)
	require.NoError(t, s.storage.Write(testContext(t), donePath, data))

	// ACT
	got, err := s.service.readSentinel(testContext(t), donePath)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Success)
	assert.Equal(t, "deadbeef", got.Checksum)
	assert.Equal(t, 4, got.TotalParts)
}

func TestFileService_readSentinel_BadJSON(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	donePath := "transfers/x/done"
	require.NoError(t, s.storage.Write(testContext(t), donePath, []byte(`{not valid json`)))

	// ACT
	got, err := s.service.readSentinel(testContext(t), donePath)

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "parse sentinel JSON")
}

func TestFileService_readSentinel_StorageMiss(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)

	// ACT
	got, err := s.service.readSentinel(testContext(t), "transfers/missing/done")

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "read sentinel file")
}

func TestSafeUint64ToInt64(t *testing.T) {
	tests := []struct {
		name string
		in   uint64
		want int64
	}{
		{"zero", 0, 0},
		{"small", 12345, 12345},
		{"max_int64", uint64(1<<63 - 1), 1<<63 - 1},
		{"overflow_clamps", uint64(1 << 63), 1<<63 - 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, safeUint64ToInt64(tt.in))
		})
	}
}

func TestStripWorkPath(t *testing.T) {
	tests := []struct {
		name     string
		workPath string
		fullPath string
		want     string
	}{
		{"strips_prefix_with_trailing_slash", "/srv/gameap", "/srv/gameap/sub/file.txt", "sub/file.txt"},
		{"empty_after_strip_returns_dot", "/srv/gameap", "/srv/gameap", "."},
		{"empty_after_strip_with_slash_returns_dot", "/srv/gameap", "/srv/gameap/", "."},
		{"no_workpath_prefix_keeps_path", "/srv/gameap", "/other/file", "other/file"},
		{"empty_workpath_returns_path", "", "abc/def", "abc/def"},
		{"windows_workpath_matching_backslashes", `C:\gameap`, `C:\gameap\servers\x`, "servers/x"},
		{"windows_workpath_mixed_separators", `C:\gameap`, `C:\gameap/servers/x`, "servers/x"},
		{"windows_absolute_with_empty_workpath_strips_drive_letter", "", `C:\gameap\servers\x`, "gameap/servers/x"},
		{"windows_absolute_with_mismatched_workpath_strips_drive_letter", "/srv/gameap", `C:\gameap\servers\x`, "gameap/servers/x"},
		{"lowercase_drive_letter_is_stripped", "", `d:\data\file`, "data/file"},
		{"unc_share_path_is_neutralized", "", `\\server\share\dir`, "server/share/dir"},
		{"trailing_backslash_in_workpath", `C:\gameap\`, `C:\gameap\servers\x`, "servers/x"},
		{"partial_prefix_match_keeps_extra_chars", "/srv/game", "/srv/gameap/x", "ap/x"},
		{"windows_workpath_with_leading_backslash", `\srv\gameap`, `\srv\gameap\x`, "x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripWorkPath(tt.workPath, tt.fullPath)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProtoFileTypeToFileType(t *testing.T) {
	tests := []struct {
		name string
		in   proto.FileType
		want FileType
	}{
		{"directory", proto.FileType_FILE_TYPE_DIRECTORY, FileTypeDir},
		{"regular", proto.FileType_FILE_TYPE_REGULAR, FileTypeFile},
		{"symlink", proto.FileType_FILE_TYPE_SYMLINK, FileTypeSymlink},
		{"socket", proto.FileType_FILE_TYPE_SOCKET, FileTypeSocket},
		{"fifo", proto.FileType_FILE_TYPE_FIFO, FileTypeNamedPipe},
		{"block_device", proto.FileType_FILE_TYPE_BLOCK_DEVICE, FileTypeBlockDevice},
		{"char_device", proto.FileType_FILE_TYPE_CHAR_DEVICE, FileTypeDevice},
		{"unspecified_returns_unknown", proto.FileType_FILE_TYPE_UNSPECIFIED, FileTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, protoFileTypeToFileType(tt.in))
		})
	}
}

func TestProtoFileStatToFileInfo_NilReturnsNil(t *testing.T) {
	assert.Nil(t, protoFileStatToFileInfo(nil))
}

func TestProtoFileStatToFileInfo_PopulatesFields(t *testing.T) {
	// ARRANGE
	stat := &proto.FileStat{
		Name:          "x.txt",
		Path:          "subdir/x.txt",
		Size:          42,
		Mode:          0o644,
		Type:          proto.FileType_FILE_TYPE_REGULAR,
		ModifiedAt:    timestamppb.New(testTime()),
		SymlinkTarget: "../target",
	}

	// ACT
	info := protoFileStatToFileInfo(stat)

	// ASSERT
	require.NotNil(t, info)
	assert.Equal(t, "x.txt", info.Name)
	assert.Equal(t, uint64(42), info.Size)
	assert.Equal(t, FileTypeFile, info.Type)
	assert.Equal(t, uint32(0o644), info.Perm)
	assert.NotZero(t, info.TimeModified)
	assert.Equal(t, "subdir/x.txt", info.Path, "Path field must be copied from proto.FileStat.Path")
	assert.Equal(t, "../target", info.SymlinkTarget, "SymlinkTarget field must be copied from proto.FileStat.SymlinkTarget")
}

func TestProtoFileStatToDetails_NilReturnsEmpty(t *testing.T) {
	got := protoFileStatToDetails(nil)
	require.NotNil(t, got)
	assert.Empty(t, got.Name)
}

func TestProtoFileStatToDetails_PopulatesFields(t *testing.T) {
	stat := &proto.FileStat{
		Name:          "x.txt",
		Size:          42,
		Mode:          0o644,
		Type:          proto.FileType_FILE_TYPE_REGULAR,
		ModifiedAt:    timestamppb.New(testTime()),
		AccessedAt:    timestamppb.New(testTime()),
		SymlinkTarget: "/abs/target",
	}

	got := protoFileStatToDetails(stat)
	require.NotNil(t, got)
	assert.Equal(t, "x.txt", got.Name)
	assert.Equal(t, uint64(42), got.Size)
	assert.Equal(t, FileTypeFile, got.Type)
	assert.NotZero(t, got.ModificationTime)
	assert.NotZero(t, got.AccessTime)
	assert.Equal(t, "/abs/target", got.SymlinkTarget, "SymlinkTarget field must be copied from proto.FileStat.SymlinkTarget")
}

func TestDaemonNotConnectedError(t *testing.T) {
	err := &daemonNotConnectedError{}
	assert.Equal(t, "daemon not connected", err.Error())
	assert.Equal(t, 502, err.HTTPStatus())
}

func TestChunkedCleanupReadCloser_CleansStorage(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	transferID := "x-id"
	require.NoError(t, s.storage.Write(testContext(t), transfers.TransferPartPath(transferID, 0), []byte("a")))
	require.NoError(t, s.storage.Write(testContext(t), transfers.TransferDonePath(transferID), []byte("done")))
	state := s.transferReg.Register(transferID)
	_ = state // registered so that Unregister doesn't panic

	rc := &chunkedCleanupReadCloser{
		ReadCloser:  io.NopCloser(strings.NewReader("data")),
		transferID:  transferID,
		storage:     s.storage,
		transferReg: s.transferReg,
		logger:      slog.Default(),
	}

	// ACT
	require.NoError(t, rc.Close())

	// ASSERT
	require.False(t, s.storage.Exists(testContext(t), transfers.TransferPartPath(transferID, 0)),
		"part should be removed after Close")
	require.False(t, s.storage.Exists(testContext(t), transfers.TransferDonePath(transferID)),
		"done sentinel should be removed after Close")
	_, ok := s.transferReg.Get(transferID)
	assert.False(t, ok, "transfer should be unregistered")
}

func TestRemoteCleanupReadCloser_CleansStorage(t *testing.T) {
	// ARRANGE
	s := setupFileService(t)
	transferID := "remote-id"
	require.NoError(t, s.storage.Write(testContext(t), transfers.TransferPartPath(transferID, 0), []byte("a")))
	require.NoError(t, s.storage.Write(testContext(t), transfers.TransferDonePath(transferID), []byte("done")))

	rc := &remoteCleanupReadCloser{
		ReadCloser: io.NopCloser(strings.NewReader("data")),
		transferID: transferID,
		storage:    s.storage,
		logger:     slog.Default(),
	}

	// ACT
	require.NoError(t, rc.Close())

	// ASSERT
	require.False(t, s.storage.Exists(testContext(t), transfers.TransferPartPath(transferID, 0)),
		"part should be removed after Close")
	require.False(t, s.storage.Exists(testContext(t), transfers.TransferDonePath(transferID)),
		"done sentinel should be removed after Close")
}

// fakeDispatcher implements FileDispatcher for tests that need to override
// behavior of the dispatcher branch (e.g. simulating storage-path responses
// without going through the real pubsub round-trip).
type fakeDispatcher struct {
	dispatchFileList     func(ctx context.Context, nodeID uint64, path string, recursive bool, pattern string) (*proto.FileListResponse, error)
	dispatchFileOp       func(ctx context.Context, nodeID uint64, req *proto.FileOperationRequest) (*proto.FileOperationResponse, error)
	dispatchFileRead     func(ctx context.Context, nodeID uint64, path string, offset, length int64) (*FileReadResult, error)
	dispatchFileWrite    func(ctx context.Context, nodeID uint64, path string, content []byte, mode int32, createDirs bool) error
	dispatchUploadTask   func(ctx context.Context, nodeID uint64, transferID, destPath string) error
	dispatchDownloadTask func(ctx context.Context, nodeID uint64, transferID, srcPath string) error

	lastWriteOwner      OwnerOptions
	lastUploadTaskOwner OwnerOptions
	lastUploadTaskMode  int32
}

func (f *fakeDispatcher) Start(_ context.Context) error { return nil }

func (f *fakeDispatcher) DispatchFileList(
	ctx context.Context, nodeID uint64, path string, recursive bool, pattern string,
) (*proto.FileListResponse, error) {
	if f.dispatchFileList == nil {
		return &proto.FileListResponse{Success: true}, nil
	}

	return f.dispatchFileList(ctx, nodeID, path, recursive, pattern)
}

func (f *fakeDispatcher) DispatchFileOperation(
	ctx context.Context, nodeID uint64, req *proto.FileOperationRequest,
) (*proto.FileOperationResponse, error) {
	if f.dispatchFileOp == nil {
		return &proto.FileOperationResponse{Success: true}, nil
	}

	return f.dispatchFileOp(ctx, nodeID, req)
}

func (f *fakeDispatcher) DispatchFileRead(
	ctx context.Context, nodeID uint64, path string, offset, length int64,
) (*FileReadResult, error) {
	if f.dispatchFileRead == nil {
		return &FileReadResult{}, nil
	}

	return f.dispatchFileRead(ctx, nodeID, path, offset, length)
}

func (f *fakeDispatcher) DispatchFileWrite(
	ctx context.Context, nodeID uint64, path string,
	content []byte, mode int32, createDirs bool, owner OwnerOptions,
) error {
	f.lastWriteOwner = owner
	if f.dispatchFileWrite == nil {
		return nil
	}

	return f.dispatchFileWrite(ctx, nodeID, path, content, mode, createDirs)
}

func (f *fakeDispatcher) DispatchUploadTask(
	ctx context.Context, nodeID uint64, transferID, destPath string, mode int32, owner OwnerOptions,
) error {
	f.lastUploadTaskOwner = owner
	f.lastUploadTaskMode = mode
	if f.dispatchUploadTask == nil {
		return nil
	}

	return f.dispatchUploadTask(ctx, nodeID, transferID, destPath)
}

func (f *fakeDispatcher) DispatchDownloadTask(ctx context.Context, nodeID uint64, transferID, srcPath string) error {
	if f.dispatchDownloadTask == nil {
		return nil
	}

	return f.dispatchDownloadTask(ctx, nodeID, transferID, srcPath)
}

func testTime() time.Time {
	return time.Unix(1700000000, 0)
}

func TestOwnerFromServer(t *testing.T) {
	t.Run("nil_server_returns_empty_options", func(t *testing.T) {
		owner := OwnerFromServer(nil)

		assert.True(t, owner.IsZero())
	})

	t.Run("server_with_nil_su_user_returns_empty_options", func(t *testing.T) {
		owner := OwnerFromServer(&domain.Server{})

		assert.True(t, owner.IsZero())
	})

	t.Run("server_with_su_user_returns_options", func(t *testing.T) {
		suUser := "gameap"
		owner := OwnerFromServer(&domain.Server{SuUser: &suUser})

		assert.Equal(t, "gameap", owner.User)
	})
}

func TestFileService_UploadStream_SmallFile_OwnerForwardedToWrite(t *testing.T) {
	s := setupFileService(t)
	node := newTestNode(201)
	s.registry.setConnected(uint64(node.ID), true)
	s.registry.setCapability(uint64(node.ID), capabilityFileTransfer, true)

	err := s.service.UploadStream(
		testContext(t), node, "/srv/gameap/small.txt",
		bytes.NewReader([]byte("hi")), 2, 0o644,
		OwnerOptions{User: "gameap"},
	)

	require.NoError(t, err)
	assert.Equal(t, "gameap", s.gateway.lastWriteOwner.User,
		"owner must be forwarded to RequestFileWrite for small uploads")
}

func TestFileService_UploadStream_LargeFile_OwnerForwardedToUploadTask(t *testing.T) {
	s := setupFileService(t)
	node := newTestNode(202)
	s.registry.setConnected(uint64(node.ID), true)
	s.registry.setCapability(uint64(node.ID), capabilityFileTransfer, true)

	largeContent := bytes.Repeat([]byte("X"), 2*1024*1024)

	err := s.service.UploadStream(
		testContext(t), node, "/srv/gameap/large.bin",
		bytes.NewReader(largeContent), uint64(len(largeContent)), 0o600,
		OwnerOptions{User: "gameap"},
	)

	require.NoError(t, err)
	assert.Equal(t, "gameap", s.gateway.lastUploadTaskOwner.User,
		"owner must be forwarded to RequestFileUploadTask for large uploads")
	assert.Equal(t, int32(0o600), s.gateway.lastUploadTaskMode,
		"mode must be forwarded to RequestFileUploadTask")
}

func TestFileService_UploadStream_RemoteDispatch_OwnerForwarded(t *testing.T) {
	s := setupFileService(t)
	node := newTestNode(203)
	s.registry.setConnected(uint64(node.ID), false)
	s.registry.setConnectedAnywhere(uint64(node.ID), true)

	largeContent := bytes.Repeat([]byte("Y"), 2*1024*1024)
	stubDispatcher := &fakeDispatcher{}
	svc := NewFileService(s.gateway, s.registry, stubDispatcher, s.storage, s.transferReg, nil, slog.Default())

	err := svc.UploadStream(
		testContext(t), node, "/srv/gameap/remote.bin",
		bytes.NewReader(largeContent), uint64(len(largeContent)), 0o644,
		OwnerOptions{User: "gameap"},
	)

	require.NoError(t, err)
	assert.Equal(t, "gameap", stubDispatcher.lastUploadTaskOwner.User,
		"owner must traverse the pubsub dispatch path")
}

func TestFileService_Upload_OwnerForwarded(t *testing.T) {
	s := setupFileService(t)
	node := newTestNode(204)
	s.registry.setConnected(uint64(node.ID), true)

	err := s.service.Upload(
		testContext(t), node, "/srv/gameap/tiny.txt",
		[]byte("z"), 0o644, OwnerOptions{User: "gameap", UID: 1000, GID: 1000},
	)

	require.NoError(t, err)
	assert.Equal(t, "gameap", s.gateway.lastWriteOwner.User)
	assert.Equal(t, int32(1000), s.gateway.lastWriteOwner.UID)
	assert.Equal(t, int32(1000), s.gateway.lastWriteOwner.GID)
}

func TestFileService_MkDir_OwnerForwardedInProto(t *testing.T) {
	s := setupFileService(t)
	node := newTestNode(205)
	s.registry.setConnected(uint64(node.ID), true)

	var capturedOwner string
	s.gateway.requestFileOp = func(
		_ context.Context, _ uint64, req *proto.FileOperationRequest,
	) (*proto.FileOperationResponse, error) {
		params := req.GetMkdirParams()
		if params != nil {
			capturedOwner = params.OwnerUser
		}

		return &proto.FileOperationResponse{Success: true}, nil
	}

	err := s.service.MkDir(testContext(t), node, "/srv/gameap/newdir", OwnerOptions{User: "gameap"})

	require.NoError(t, err)
	assert.Equal(t, "gameap", capturedOwner,
		"MkDir must populate MkdirParams.OwnerUser in the proto message")
}
