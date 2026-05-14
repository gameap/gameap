package hostlibrary

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errPermissionDenied     = errors.New("permission denied")
	errDirectoryExists      = errors.New("directory already exists")
	errFileNotFoundInternal = errors.New("file not found")
)

type mockFileService struct {
	readDirFunc     func(ctx context.Context, node *domain.Node, path string) ([]*daemon.FileInfo, error)
	mkDirFunc       func(ctx context.Context, node *domain.Node, path string) error
	copyFunc        func(ctx context.Context, node *domain.Node, src, dst string) error
	moveFunc        func(ctx context.Context, node *domain.Node, src, dst string) error
	downloadFunc    func(ctx context.Context, node *domain.Node, path string) ([]byte, error)
	uploadFunc      func(ctx context.Context, node *domain.Node, path string, content []byte, perm os.FileMode) error
	removeFunc      func(ctx context.Context, node *domain.Node, path string, recursive bool) error
	getFileInfoFunc func(ctx context.Context, node *domain.Node, path string) (*daemon.FileDetails, error)
	chmodFunc       func(ctx context.Context, node *domain.Node, path string, permissions int32) error
}

func (m *mockFileService) ReadDir(ctx context.Context, node *domain.Node, path string) ([]*daemon.FileInfo, error) {
	if m.readDirFunc != nil {
		return m.readDirFunc(ctx, node, path)
	}

	return nil, nil
}

func (m *mockFileService) MkDir(
	ctx context.Context, node *domain.Node, path string, _ daemon.OwnerOptions,
) error {
	if m.mkDirFunc != nil {
		return m.mkDirFunc(ctx, node, path)
	}

	return nil
}

func (m *mockFileService) Copy(ctx context.Context, node *domain.Node, src, dst string) error {
	if m.copyFunc != nil {
		return m.copyFunc(ctx, node, src, dst)
	}

	return nil
}

func (m *mockFileService) Move(ctx context.Context, node *domain.Node, src, dst string) error {
	if m.moveFunc != nil {
		return m.moveFunc(ctx, node, src, dst)
	}

	return nil
}

func (m *mockFileService) Download(ctx context.Context, node *domain.Node, path string) ([]byte, error) {
	if m.downloadFunc != nil {
		return m.downloadFunc(ctx, node, path)
	}

	return nil, nil
}

func (m *mockFileService) Upload(
	ctx context.Context,
	node *domain.Node,
	path string,
	content []byte,
	perm os.FileMode,
	_ daemon.OwnerOptions,
) error {
	if m.uploadFunc != nil {
		return m.uploadFunc(ctx, node, path, content, perm)
	}

	return nil
}

func (m *mockFileService) Remove(ctx context.Context, node *domain.Node, path string, recursive bool) error {
	if m.removeFunc != nil {
		return m.removeFunc(ctx, node, path, recursive)
	}

	return nil
}

func (m *mockFileService) GetFileInfo(ctx context.Context, node *domain.Node, path string) (*daemon.FileDetails, error) {
	if m.getFileInfoFunc != nil {
		return m.getFileInfoFunc(ctx, node, path)
	}

	return nil, nil
}

func (m *mockFileService) Chmod(ctx context.Context, node *domain.Node, path string, permissions int32) error {
	if m.chmodFunc != nil {
		return m.chmodFunc(ctx, node, path, permissions)
	}

	return nil
}

type nodeFSServiceImplForTest struct {
	fileService *mockFileService
	nodeRepo    *inmemory.NodeRepository
}

func newNodeFSServiceForTest(
	fileService *mockFileService,
	nodeRepo *inmemory.NodeRepository,
) *nodeFSServiceImplForTest {
	return &nodeFSServiceImplForTest{
		fileService: fileService,
		nodeRepo:    nodeRepo,
	}
}

func (s *nodeFSServiceImplForTest) getNode(ctx context.Context, nodeID uint64) (*domain.Node, error) {
	nodes, err := s.nodeRepo.Find(ctx, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	for i := range nodes {
		if nodes[i].ID == uint(nodeID) {
			return &nodes[i], nil
		}
	}

	return nil, nil
}

func (s *nodeFSServiceImplForTest) ReadDir(
	ctx context.Context,
	req *nodefs.ReadDirRequest,
) (*nodefs.ReadDirResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.ReadDirResponse{Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.ReadDirResponse{Error: new("node not found")}, nil
	}

	files, err := s.fileService.ReadDir(ctx, node, req.Path)
	if err != nil {
		return &nodefs.ReadDirResponse{Error: new(err.Error())}, nil
	}

	return &nodefs.ReadDirResponse{
		Files: convertFileInfosToProto(files),
	}, nil
}

func (s *nodeFSServiceImplForTest) MkDir(
	ctx context.Context,
	req *nodefs.MkDirRequest,
) (*nodefs.MkDirResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.MkDirResponse{Success: false, Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.MkDirResponse{Success: false, Error: new("node not found")}, nil
	}

	err = s.fileService.MkDir(ctx, node, req.Path, daemon.OwnerOptions{})
	if err != nil {
		return &nodefs.MkDirResponse{Success: false, Error: new(err.Error())}, nil
	}

	return &nodefs.MkDirResponse{Success: true}, nil
}

func (s *nodeFSServiceImplForTest) Download(
	ctx context.Context,
	req *nodefs.DownloadRequest,
) (*nodefs.DownloadResponse, error) {
	node, err := s.getNode(ctx, req.NodeId)
	if err != nil {
		return &nodefs.DownloadResponse{Error: new(err.Error())}, nil
	}

	if node == nil {
		return &nodefs.DownloadResponse{Error: new("node not found")}, nil
	}

	content, err := s.fileService.Download(ctx, node, req.Path)
	if err != nil {
		return &nodefs.DownloadResponse{Error: new(err.Error())}, nil
	}

	return &nodefs.DownloadResponse{Content: content}, nil
}

func TestNodeFSService_ReadDir(t *testing.T) {
	tests := []struct {
		name      string
		setupRepo func(*inmemory.NodeRepository)
		setupFS   func() *mockFileService
		request   *nodefs.ReadDirRequest
		wantError string
		wantCount int
	}{
		{
			name:      "node_not_found_returns_error",
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupFS: func() *mockFileService {
				return &mockFileService{}
			},
			request: &nodefs.ReadDirRequest{
				NodeId: 999,
				Path:   "/home",
			},
			wantError: "node not found",
		},
		{
			name: "success_returns_file_list",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{Name: "TestNode", OS: domain.NodeOSLinux})
			},
			setupFS: func() *mockFileService {
				return &mockFileService{
					readDirFunc: func(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
						return []*daemon.FileInfo{
							{Name: "file1.txt", Type: daemon.FileTypeFile, Size: 100},
							{Name: "dir1", Type: daemon.FileTypeDir, Size: 0},
						}, nil
					},
				}
			},
			request: &nodefs.ReadDirRequest{
				NodeId: 1,
				Path:   "/home",
			},
			wantCount: 2,
		},
		{
			name: "service_error_returns_error",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{Name: "TestNode", OS: domain.NodeOSLinux})
			},
			setupFS: func() *mockFileService {
				return &mockFileService{
					readDirFunc: func(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
						return nil, errPermissionDenied
					},
				}
			},
			request: &nodefs.ReadDirRequest{
				NodeId: 1,
				Path:   "/root",
			},
			wantError: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewNodeRepository()
			tt.setupRepo(repo)
			fsSvc := tt.setupFS()

			svc := newNodeFSServiceForTest(fsSvc, repo)
			resp, err := svc.ReadDir(context.Background(), tt.request)

			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.Nil(t, resp.Error)
			assert.Len(t, resp.Files, tt.wantCount)
		})
	}
}

func TestNodeFSService_MkDir(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.NodeRepository)
		setupFS     func() *mockFileService
		request     *nodefs.MkDirRequest
		wantError   string
		wantSuccess bool
	}{
		{
			name:      "node_not_found_returns_error",
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupFS: func() *mockFileService {
				return &mockFileService{}
			},
			request: &nodefs.MkDirRequest{
				NodeId: 999,
				Path:   "/home/newdir",
			},
			wantError:   "node not found",
			wantSuccess: false,
		},
		{
			name: "success_creates_directory",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{Name: "TestNode", OS: domain.NodeOSLinux})
			},
			setupFS: func() *mockFileService {
				return &mockFileService{
					mkDirFunc: func(_ context.Context, _ *domain.Node, _ string) error {
						return nil
					},
				}
			},
			request: &nodefs.MkDirRequest{
				NodeId: 1,
				Path:   "/home/newdir",
			},
			wantSuccess: true,
		},
		{
			name: "service_error_returns_error",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{Name: "TestNode", OS: domain.NodeOSLinux})
			},
			setupFS: func() *mockFileService {
				return &mockFileService{
					mkDirFunc: func(_ context.Context, _ *domain.Node, _ string) error {
						return errDirectoryExists
					},
				}
			},
			request: &nodefs.MkDirRequest{
				NodeId: 1,
				Path:   "/home/existing",
			},
			wantError:   "directory already exists",
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewNodeRepository()
			tt.setupRepo(repo)
			fsSvc := tt.setupFS()

			svc := newNodeFSServiceForTest(fsSvc, repo)
			resp, err := svc.MkDir(context.Background(), tt.request)

			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
			}
		})
	}
}

func TestNodeFSService_Download(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.NodeRepository)
		setupFS     func() *mockFileService
		request     *nodefs.DownloadRequest
		wantError   string
		wantContent []byte
	}{
		{
			name:      "node_not_found_returns_error",
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupFS: func() *mockFileService {
				return &mockFileService{}
			},
			request: &nodefs.DownloadRequest{
				NodeId: 999,
				Path:   "/home/file.txt",
			},
			wantError: "node not found",
		},
		{
			name: "success_returns_content",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{Name: "TestNode", OS: domain.NodeOSLinux})
			},
			setupFS: func() *mockFileService {
				return &mockFileService{
					downloadFunc: func(_ context.Context, _ *domain.Node, _ string) ([]byte, error) {
						return []byte("file content"), nil
					},
				}
			},
			request: &nodefs.DownloadRequest{
				NodeId: 1,
				Path:   "/home/file.txt",
			},
			wantContent: []byte("file content"),
		},
		{
			name: "service_error_returns_error",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{Name: "TestNode", OS: domain.NodeOSLinux})
			},
			setupFS: func() *mockFileService {
				return &mockFileService{
					downloadFunc: func(_ context.Context, _ *domain.Node, _ string) ([]byte, error) {
						return nil, errFileNotFoundInternal
					},
				}
			},
			request: &nodefs.DownloadRequest{
				NodeId: 1,
				Path:   "/home/missing.txt",
			},
			wantError: "file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewNodeRepository()
			tt.setupRepo(repo)
			fsSvc := tt.setupFS()

			svc := newNodeFSServiceForTest(fsSvc, repo)
			resp, err := svc.Download(context.Background(), tt.request)

			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.Nil(t, resp.Error)
			assert.Equal(t, tt.wantContent, resp.Content)
		})
	}
}

func TestConvertFileTypeToProto(t *testing.T) {
	tests := []struct {
		name     string
		input    daemon.FileType
		expected nodefs.FileType
	}{
		{
			name:     "dir_type",
			input:    daemon.FileTypeDir,
			expected: nodefs.FileType_FILE_TYPE_DIR,
		},
		{
			name:     "file_type",
			input:    daemon.FileTypeFile,
			expected: nodefs.FileType_FILE_TYPE_FILE,
		},
		{
			name:     "device_type",
			input:    daemon.FileTypeDevice,
			expected: nodefs.FileType_FILE_TYPE_DEVICE,
		},
		{
			name:     "block_device_type",
			input:    daemon.FileTypeBlockDevice,
			expected: nodefs.FileType_FILE_TYPE_BLOCK_DEVICE,
		},
		{
			name:     "named_pipe_type",
			input:    daemon.FileTypeNamedPipe,
			expected: nodefs.FileType_FILE_TYPE_NAMED_PIPE,
		},
		{
			name:     "symlink_type",
			input:    daemon.FileTypeSymlink,
			expected: nodefs.FileType_FILE_TYPE_SYMLINK,
		},
		{
			name:     "socket_type",
			input:    daemon.FileTypeSocket,
			expected: nodefs.FileType_FILE_TYPE_SOCKET,
		},
		{
			name:     "unknown_type",
			input:    daemon.FileType(100),
			expected: nodefs.FileType_FILE_TYPE_UNKNOWN,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertFileTypeToProto(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewNodeFSHostLibrary(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	lib := NewNodeFSHostLibrary(nil, repo)

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
