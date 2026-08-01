package hostlibrary

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errPermissionDenied     = errors.New("permission denied")
	errDirectoryExists      = errors.New("directory already exists")
	errFileNotFoundInternal = errors.New("file not found")
	errNodeLookup           = errors.New("node lookup unavailable")
)

var _ NodeFileService = (*mockFileService)(nil)

type mockFileService struct {
	readDirFunc     func(ctx context.Context, node *domain.Node, path string) ([]*daemon.FileInfo, error)
	mkDirFunc       func(ctx context.Context, node *domain.Node, path string) error
	copyFunc        func(ctx context.Context, node *domain.Node, src, dst string) error
	moveFunc        func(ctx context.Context, node *domain.Node, src, dst string) error
	downloadFunc    func(ctx context.Context, node *domain.Node, path string) ([]byte, error)
	uploadFunc      func(ctx context.Context, node *domain.Node, path string, content []byte, perm os.FileMode) error
	removeFunc      func(ctx context.Context, node *domain.Node, path string, recursive bool) error
	getFileInfoFunc func(ctx context.Context, node *domain.Node, path string) (*daemon.FileDetails, error)
	chmodFunc       func(ctx context.Context, node *domain.Node, path string, permissions uint32) error
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

func (m *mockFileService) Chmod(ctx context.Context, node *domain.Node, path string, permissions uint32) error {
	if m.chmodFunc != nil {
		return m.chmodFunc(ctx, node, path, permissions)
	}

	return nil
}

// errNodeRepository makes the node lookup itself fail, which is otherwise
// unreachable through the in-memory repository.
type errNodeRepository struct {
	*inmemory.NodeRepository
}

func (r *errNodeRepository) Find(
	_ context.Context,
	_ *filters.FindNode,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.Node, error) {
	return nil, errNodeLookup
}

func setupNodeFSRepo(seed func(*inmemory.NodeRepository)) *inmemory.NodeRepository {
	repo := inmemory.NewNodeRepository()
	if seed != nil {
		seed(repo)
	}

	return repo
}

// newNodeFSService builds the real NodeFSServiceImpl under test. repoFails
// swaps in a repository whose node lookup errors.
func newNodeFSService(
	fs NodeFileService, repo *inmemory.NodeRepository, repoFails bool,
) *NodeFSServiceImpl {
	if repoFails {
		return NewNodeFSService(fs, &errNodeRepository{NodeRepository: repo})
	}

	return NewNodeFSService(fs, repo)
}

func seedTestNode(r *inmemory.NodeRepository) {
	_ = r.Save(context.Background(), &domain.Node{Name: "TestNode", OS: domain.NodeOSLinux})
}

func TestNodeFSService_ReadDir(t *testing.T) {
	tests := []struct {
		name      string
		setupRepo func(*inmemory.NodeRepository)
		repoFails bool
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
			name:      "node_lookup_error_is_reported",
			setupRepo: seedTestNode,
			repoFails: true,
			setupFS: func() *mockFileService {
				return &mockFileService{}
			},
			request: &nodefs.ReadDirRequest{
				NodeId: 1,
				Path:   "/home",
			},
			wantError: "node lookup unavailable",
		},
		{
			name:      "success_returns_file_list",
			setupRepo: seedTestNode,
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
			name:      "service_error_returns_error",
			setupRepo: seedTestNode,
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
			// ARRANGE
			repo := setupNodeFSRepo(tt.setupRepo)
			svc := newNodeFSService(tt.setupFS(), repo, tt.repoFails)

			// ACT
			resp, err := svc.ReadDir(context.Background(), tt.request)

			// ASSERT
			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")

				return
			}

			assert.Nil(t, resp.Error)
			require.Len(t, resp.Files, tt.wantCount)
		})
	}
}

func TestNodeFSService_ReadDir_MapsFileFields(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewNodeRepository()
	seedTestNode(repo)
	fsSvc := &mockFileService{
		readDirFunc: func(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
			return []*daemon.FileInfo{
				{Name: "app.log", Type: daemon.FileTypeFile, Size: 42, TimeModified: 1700000000, Perm: 0o644},
			}, nil
		},
	}
	svc := NewNodeFSService(fsSvc, repo)

	// ACT
	resp, err := svc.ReadDir(context.Background(), &nodefs.ReadDirRequest{NodeId: 1, Path: "/var/log"})

	// ASSERT
	require.NoError(t, err)
	require.Len(t, resp.Files, 1)
	assert.Equal(t, "app.log", resp.Files[0].Name)
	assert.Equal(t, uint64(42), resp.Files[0].Size)
	assert.Equal(t, uint64(1700000000), resp.Files[0].ModifiedTime)
	assert.Equal(t, uint32(0o644), resp.Files[0].Permissions)
	assert.Equal(t, nodefs.FileType_FILE_TYPE_FILE, resp.Files[0].Type)
}

func TestNodeFSService_MkDir(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.NodeRepository)
		repoFails   bool
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
			name:      "node_lookup_error_is_reported",
			setupRepo: seedTestNode,
			repoFails: true,
			setupFS: func() *mockFileService {
				return &mockFileService{}
			},
			request: &nodefs.MkDirRequest{
				NodeId: 1,
				Path:   "/home/newdir",
			},
			wantError:   "node lookup unavailable",
			wantSuccess: false,
		},
		{
			name:      "success_creates_directory",
			setupRepo: seedTestNode,
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
			name:      "service_error_returns_error",
			setupRepo: seedTestNode,
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
			// ARRANGE
			repo := setupNodeFSRepo(tt.setupRepo)
			svc := newNodeFSService(tt.setupFS(), repo, tt.repoFails)

			// ACT
			resp, err := svc.MkDir(context.Background(), tt.request)

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")
			}
		})
	}
}

func TestNodeFSService_Download(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.NodeRepository)
		repoFails   bool
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
			name:      "node_lookup_error_is_reported",
			setupRepo: seedTestNode,
			repoFails: true,
			setupFS: func() *mockFileService {
				return &mockFileService{}
			},
			request: &nodefs.DownloadRequest{
				NodeId: 1,
				Path:   "/home/file.txt",
			},
			wantError: "node lookup unavailable",
		},
		{
			name:      "success_returns_content",
			setupRepo: seedTestNode,
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
			name:      "service_error_returns_error",
			setupRepo: seedTestNode,
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
			// ARRANGE
			repo := setupNodeFSRepo(tt.setupRepo)
			svc := newNodeFSService(tt.setupFS(), repo, tt.repoFails)

			// ACT
			resp, err := svc.Download(context.Background(), tt.request)

			// ASSERT
			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")

				return
			}

			assert.Nil(t, resp.Error)
			assert.Equal(t, tt.wantContent, resp.Content)
		})
	}
}

func TestNodeFSService_Copy(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.NodeRepository)
		repoFails   bool
		setupFS     func() *mockFileService
		nodeID      uint64
		wantError   string
		wantSuccess bool
	}{
		{
			name:        "node_not_found_returns_error",
			setupRepo:   func(_ *inmemory.NodeRepository) {},
			setupFS:     func() *mockFileService { return &mockFileService{} },
			nodeID:      999,
			wantError:   "node not found",
			wantSuccess: false,
		},
		{
			name:        "node_lookup_error_is_reported",
			setupRepo:   seedTestNode,
			repoFails:   true,
			setupFS:     func() *mockFileService { return &mockFileService{} },
			nodeID:      1,
			wantError:   "node lookup unavailable",
			wantSuccess: false,
		},
		{
			name:      "success_copies_file",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, _, _ string) error {
						return nil
					},
				}
			},
			nodeID:      1,
			wantSuccess: true,
		},
		{
			name:      "service_error_returns_error",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					copyFunc: func(_ context.Context, _ *domain.Node, _, _ string) error {
						return errPermissionDenied
					},
				}
			},
			nodeID:      1,
			wantError:   "permission denied",
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := setupNodeFSRepo(tt.setupRepo)
			svc := newNodeFSService(tt.setupFS(), repo, tt.repoFails)

			// ACT
			resp, err := svc.Copy(context.Background(), &nodefs.CopyRequest{
				NodeId:      tt.nodeID,
				Source:      "/home/a.txt",
				Destination: "/home/b.txt",
			})

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")

				return
			}

			assert.Nil(t, resp.Error)
		})
	}
}

func TestNodeFSService_Copy_PassesSourceAndDestination(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewNodeRepository()
	seedTestNode(repo)

	var gotSrc, gotDst string
	fsSvc := &mockFileService{
		copyFunc: func(_ context.Context, _ *domain.Node, src, dst string) error {
			gotSrc, gotDst = src, dst

			return nil
		},
	}
	svc := NewNodeFSService(fsSvc, repo)

	// ACT
	resp, err := svc.Copy(context.Background(), &nodefs.CopyRequest{
		NodeId:      1,
		Source:      "/src/file.txt",
		Destination: "/dst/file.txt",
	})

	// ASSERT
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "/src/file.txt", gotSrc, "source must be forwarded unchanged")
	assert.Equal(t, "/dst/file.txt", gotDst, "destination must be forwarded unchanged")
}

func TestNodeFSService_Move(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.NodeRepository)
		repoFails   bool
		setupFS     func() *mockFileService
		nodeID      uint64
		wantError   string
		wantSuccess bool
	}{
		{
			name:        "node_not_found_returns_error",
			setupRepo:   func(_ *inmemory.NodeRepository) {},
			setupFS:     func() *mockFileService { return &mockFileService{} },
			nodeID:      999,
			wantError:   "node not found",
			wantSuccess: false,
		},
		{
			name:        "node_lookup_error_is_reported",
			setupRepo:   seedTestNode,
			repoFails:   true,
			setupFS:     func() *mockFileService { return &mockFileService{} },
			nodeID:      1,
			wantError:   "node lookup unavailable",
			wantSuccess: false,
		},
		{
			name:      "success_moves_file",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					moveFunc: func(_ context.Context, _ *domain.Node, _, _ string) error {
						return nil
					},
				}
			},
			nodeID:      1,
			wantSuccess: true,
		},
		{
			name:      "service_error_returns_error",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					moveFunc: func(_ context.Context, _ *domain.Node, _, _ string) error {
						return errFileNotFoundInternal
					},
				}
			},
			nodeID:      1,
			wantError:   "file not found",
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := setupNodeFSRepo(tt.setupRepo)
			svc := newNodeFSService(tt.setupFS(), repo, tt.repoFails)

			// ACT
			resp, err := svc.Move(context.Background(), &nodefs.MoveRequest{
				NodeId:      tt.nodeID,
				Source:      "/home/a.txt",
				Destination: "/home/b.txt",
			})

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")

				return
			}

			assert.Nil(t, resp.Error)
		})
	}
}

func TestNodeFSService_Upload(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.NodeRepository)
		repoFails   bool
		setupFS     func() *mockFileService
		nodeID      uint64
		wantError   string
		wantSuccess bool
	}{
		{
			name:        "node_not_found_returns_error",
			setupRepo:   func(_ *inmemory.NodeRepository) {},
			setupFS:     func() *mockFileService { return &mockFileService{} },
			nodeID:      999,
			wantError:   "node not found",
			wantSuccess: false,
		},
		{
			name:        "node_lookup_error_is_reported",
			setupRepo:   seedTestNode,
			repoFails:   true,
			setupFS:     func() *mockFileService { return &mockFileService{} },
			nodeID:      1,
			wantError:   "node lookup unavailable",
			wantSuccess: false,
		},
		{
			name:      "success_uploads_content",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					uploadFunc: func(_ context.Context, _ *domain.Node, _ string, _ []byte, _ os.FileMode) error {
						return nil
					},
				}
			},
			nodeID:      1,
			wantSuccess: true,
		},
		{
			name:      "service_error_returns_error",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					uploadFunc: func(_ context.Context, _ *domain.Node, _ string, _ []byte, _ os.FileMode) error {
						return errPermissionDenied
					},
				}
			},
			nodeID:      1,
			wantError:   "permission denied",
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := setupNodeFSRepo(tt.setupRepo)
			svc := newNodeFSService(tt.setupFS(), repo, tt.repoFails)

			// ACT
			resp, err := svc.Upload(context.Background(), &nodefs.UploadRequest{
				NodeId:      tt.nodeID,
				Path:        "/home/file.txt",
				Content:     []byte("payload"),
				Permissions: 0o644,
			})

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")

				return
			}

			assert.Nil(t, resp.Error)
		})
	}
}

func TestNodeFSService_Upload_ForwardsContentAndPermissions(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewNodeRepository()
	seedTestNode(repo)

	var gotContent []byte
	var gotPerm os.FileMode
	fsSvc := &mockFileService{
		uploadFunc: func(_ context.Context, _ *domain.Node, _ string, content []byte, perm os.FileMode) error {
			gotContent, gotPerm = content, perm

			return nil
		},
	}
	svc := NewNodeFSService(fsSvc, repo)

	// ACT
	resp, err := svc.Upload(context.Background(), &nodefs.UploadRequest{
		NodeId:      1,
		Path:        "/home/file.txt",
		Content:     []byte("hello"),
		Permissions: 0o755,
	})

	// ASSERT
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, []byte("hello"), gotContent, "content must be forwarded unchanged")
	assert.Equal(t, os.FileMode(0o755), gotPerm, "permissions must be converted to os.FileMode")
}

func TestNodeFSService_Remove(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.NodeRepository)
		repoFails   bool
		setupFS     func() *mockFileService
		nodeID      uint64
		recursive   bool
		wantError   string
		wantSuccess bool
	}{
		{
			name:        "node_not_found_returns_error",
			setupRepo:   func(_ *inmemory.NodeRepository) {},
			setupFS:     func() *mockFileService { return &mockFileService{} },
			nodeID:      999,
			wantError:   "node not found",
			wantSuccess: false,
		},
		{
			name:        "node_lookup_error_is_reported",
			setupRepo:   seedTestNode,
			repoFails:   true,
			setupFS:     func() *mockFileService { return &mockFileService{} },
			nodeID:      1,
			wantError:   "node lookup unavailable",
			wantSuccess: false,
		},
		{
			name:      "success_removes_recursively",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					removeFunc: func(_ context.Context, _ *domain.Node, _ string, recursive bool) error {
						if !recursive {
							return errPermissionDenied
						}

						return nil
					},
				}
			},
			nodeID:      1,
			recursive:   true,
			wantSuccess: true,
		},
		{
			name:      "service_error_returns_error",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					removeFunc: func(_ context.Context, _ *domain.Node, _ string, _ bool) error {
						return errFileNotFoundInternal
					},
				}
			},
			nodeID:      1,
			wantError:   "file not found",
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := setupNodeFSRepo(tt.setupRepo)
			svc := newNodeFSService(tt.setupFS(), repo, tt.repoFails)

			// ACT
			resp, err := svc.Remove(context.Background(), &nodefs.RemoveRequest{
				NodeId:    tt.nodeID,
				Path:      "/home/dir",
				Recursive: tt.recursive,
			})

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")

				return
			}

			assert.Nil(t, resp.Error)
		})
	}
}

func TestNodeFSService_GetFileInfo(t *testing.T) {
	tests := []struct {
		name      string
		setupRepo func(*inmemory.NodeRepository)
		repoFails bool
		setupFS   func() *mockFileService
		nodeID    uint64
		wantError string
		wantFound bool
	}{
		{
			name:      "node_not_found_returns_error",
			setupRepo: func(_ *inmemory.NodeRepository) {},
			setupFS:   func() *mockFileService { return &mockFileService{} },
			nodeID:    999,
			wantError: "node not found",
			wantFound: false,
		},
		{
			name:      "node_lookup_error_is_reported",
			setupRepo: seedTestNode,
			repoFails: true,
			setupFS:   func() *mockFileService { return &mockFileService{} },
			nodeID:    1,
			wantError: "node lookup unavailable",
			wantFound: false,
		},
		{
			name:      "success_returns_details",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return &daemon.FileDetails{
							Name: "file.txt",
							Mime: "text/plain",
							Size: 128,
							Type: daemon.FileTypeFile,
							Perm: 0o600,
						}, nil
					},
				}
			},
			nodeID:    1,
			wantFound: true,
		},
		{
			name:      "service_error_returns_error",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
						return nil, errFileNotFoundInternal
					},
				}
			},
			nodeID:    1,
			wantError: "file not found",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := setupNodeFSRepo(tt.setupRepo)
			svc := newNodeFSService(tt.setupFS(), repo, tt.repoFails)

			// ACT
			resp, err := svc.GetFileInfo(context.Background(), &nodefs.GetFileInfoRequest{
				NodeId: tt.nodeID,
				Path:   "/home/file.txt",
			})

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, resp.Found)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")

				return
			}

			require.NotNil(t, resp.File)
			assert.Equal(t, "file.txt", resp.File.Name)
			assert.Equal(t, "text/plain", resp.File.Mime)
			assert.Equal(t, uint64(128), resp.File.Size)
			assert.Equal(t, uint32(0o600), resp.File.Permissions)
			assert.Equal(t, nodefs.FileType_FILE_TYPE_FILE, resp.File.Type)
		})
	}
}

func TestNodeFSService_Chmod(t *testing.T) {
	tests := []struct {
		name        string
		setupRepo   func(*inmemory.NodeRepository)
		repoFails   bool
		setupFS     func() *mockFileService
		nodeID      uint64
		wantError   string
		wantSuccess bool
	}{
		{
			name:        "node_not_found_returns_error",
			setupRepo:   func(_ *inmemory.NodeRepository) {},
			setupFS:     func() *mockFileService { return &mockFileService{} },
			nodeID:      999,
			wantError:   "node not found",
			wantSuccess: false,
		},
		{
			name:        "node_lookup_error_is_reported",
			setupRepo:   seedTestNode,
			repoFails:   true,
			setupFS:     func() *mockFileService { return &mockFileService{} },
			nodeID:      1,
			wantError:   "node lookup unavailable",
			wantSuccess: false,
		},
		{
			name:      "success_changes_permissions",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					chmodFunc: func(_ context.Context, _ *domain.Node, _ string, _ uint32) error {
						return nil
					},
				}
			},
			nodeID:      1,
			wantSuccess: true,
		},
		{
			name:      "service_error_returns_error",
			setupRepo: seedTestNode,
			setupFS: func() *mockFileService {
				return &mockFileService{
					chmodFunc: func(_ context.Context, _ *domain.Node, _ string, _ uint32) error {
						return errPermissionDenied
					},
				}
			},
			nodeID:      1,
			wantError:   "permission denied",
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := setupNodeFSRepo(tt.setupRepo)
			svc := newNodeFSService(tt.setupFS(), repo, tt.repoFails)

			// ACT
			resp, err := svc.Chmod(context.Background(), &nodefs.ChmodRequest{
				NodeId:      tt.nodeID,
				Path:        "/home/file.txt",
				Permissions: 0o750,
			})

			// ASSERT
			require.NoError(t, err)
			assert.Equal(t, tt.wantSuccess, resp.Success)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError, "error message mismatch")

				return
			}

			assert.Nil(t, resp.Error)
		})
	}
}

func TestNodeFSService_Chmod_ForwardsPermissionsUnchanged(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewNodeRepository()
	seedTestNode(repo)

	var gotPerm uint32
	fsSvc := &mockFileService{
		chmodFunc: func(_ context.Context, _ *domain.Node, _ string, permissions uint32) error {
			gotPerm = permissions

			return nil
		},
	}
	svc := NewNodeFSService(fsSvc, repo)

	// ACT
	resp, err := svc.Chmod(context.Background(), &nodefs.ChmodRequest{
		NodeId:      1,
		Path:        "/home/file.txt",
		Permissions: 0o600,
	})

	// ASSERT
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, uint32(0o600), gotPerm, "permissions must reach the file service unchanged")
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
