package hostlibrary

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errPermissionDenied     = errors.New("permission denied")
	errDirectoryExists      = errors.New("directory already exists")
	errFileNotFoundInternal = errors.New("file not found")
	errNodeLookup           = errors.New("node lookup unavailable")
)

const testPluginID uint64 = 7

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
	hashFunc        func(
		ctx context.Context, node *domain.Node, paths []string, algorithm proto.HashAlgorithm,
	) (*proto.HashResult, error)
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

func (m *mockFileService) Hash(
	ctx context.Context, node *domain.Node, paths []string, algorithm proto.HashAlgorithm,
) (*proto.HashResult, error) {
	if m.hashFunc != nil {
		return m.hashFunc(ctx, node, paths, algorithm)
	}

	return &proto.HashResult{Algorithm: algorithm}, nil
}

type archiveStartCall struct {
	create  *daemon.CreateArchiveParams
	extract *daemon.ExtractArchiveParams
}

type mockArchiveService struct {
	mu            sync.Mutex
	startCalls    []archiveStartCall
	cancelCalls   []string
	startErr      error
	operationID   string
	snapshots     map[string]daemon.ArchiveOpSnapshot
	waitResult    *messages.ArchiveCompleteEventPayload
	waitErr       error
	waitBlocksCtx bool
}

func newMockArchiveService() *mockArchiveService {
	return &mockArchiveService{
		operationID: "op-test",
		snapshots:   make(map[string]daemon.ArchiveOpSnapshot),
	}
}

func (m *mockArchiveService) StartCreate(
	_ context.Context, _ *domain.Node, p daemon.CreateArchiveParams,
) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalls = append(m.startCalls, archiveStartCall{create: &p})

	if m.startErr != nil {
		return "", m.startErr
	}

	return m.operationID, nil
}

func (m *mockArchiveService) StartExtract(
	_ context.Context, _ *domain.Node, p daemon.ExtractArchiveParams,
) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalls = append(m.startCalls, archiveStartCall{extract: &p})

	if m.startErr != nil {
		return "", m.startErr
	}

	return m.operationID, nil
}

func (m *mockArchiveService) Cancel(_ context.Context, _ *domain.Node, operationID, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelCalls = append(m.cancelCalls, operationID)

	return nil
}

func (m *mockArchiveService) GetSnapshot(operationID string) (daemon.ArchiveOpSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot, ok := m.snapshots[operationID]

	return snapshot, ok
}

func (m *mockArchiveService) WaitCompletion(
	ctx context.Context, _ string,
) (*messages.ArchiveCompleteEventPayload, error) {
	if m.waitBlocksCtx {
		<-ctx.Done()

		return nil, ctx.Err()
	}

	if m.waitErr != nil {
		return nil, m.waitErr
	}

	return m.waitResult, nil
}

func (m *mockArchiveService) StartCalls() []archiveStartCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]archiveStartCall(nil), m.startCalls...)
}

func (m *mockArchiveService) CancelCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]string(nil), m.cancelCalls...)
}

type registrarCall struct {
	pluginID       uint64
	operationID    string
	nodeID         uint64
	reportProgress bool
}

type mockRegistrar struct {
	mu    sync.Mutex
	calls []registrarCall
}

func (m *mockRegistrar) Register(pluginID uint64, operationID string, nodeID uint64, reportProgress bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, registrarCall{
		pluginID:       pluginID,
		operationID:    operationID,
		nodeID:         nodeID,
		reportProgress: reportProgress,
	})
}

func (m *mockRegistrar) Calls() []registrarCall {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]registrarCall(nil), m.calls...)
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

// newNodeFSService builds the real NodeFSServiceImpl under test with the
// files permission granted. repoFails swaps in a repository whose node
// lookup errors.
func newNodeFSService(
	fs NodeFileService, repo *inmemory.NodeRepository, repoFails bool,
) *NodeFSServiceImpl {
	var nodeRepo repositories.NodeRepository = repo
	if repoFails {
		nodeRepo = &errNodeRepository{NodeRepository: repo}
	}

	return NewNodeFSService(
		testPluginID, fs, nodeRepo, newMockArchiveService(), &mockRegistrar{},
		stubPermissionChecker{allowed: true},
	)
}

func newAllowedNodeFSService(fs NodeFileService, repo *inmemory.NodeRepository) *NodeFSServiceImpl {
	return newNodeFSService(fs, repo, false)
}

// newArchiveNodeFSService wires explicit archive/registrar mocks for the
// archive-specific tests.
func newArchiveNodeFSService(
	archive *mockArchiveService, registrar *mockRegistrar, repo *inmemory.NodeRepository,
) *NodeFSServiceImpl {
	return NewNodeFSService(
		testPluginID, &mockFileService{}, repo, archive, registrar,
		stubPermissionChecker{allowed: true},
	)
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
	svc := newAllowedNodeFSService(fsSvc, repo)

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
	svc := newAllowedNodeFSService(fsSvc, repo)

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
	svc := newAllowedNodeFSService(fsSvc, repo)

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
	svc := newAllowedNodeFSService(fsSvc, repo)

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

func TestNodeFSHostLibraryFactory_Create(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	factory := NewNodeFSHostLibraryFactory(
		&mockFileService{}, repo, newMockArchiveService(), &mockRegistrar{},
		stubPermissionChecker{allowed: true},
	)

	lib := factory.Create(42)

	require.NotNil(t, lib)
	nodeFSLib, ok := lib.(*NodeFSHostLibrary)
	require.True(t, ok)
	require.NotNil(t, nodeFSLib.impl)
	assert.Equal(t, uint64(42), nodeFSLib.impl.pluginID, "factory must capture the plugin id")
}

func TestNodeFSService_FilesPermissionGatesEveryOperation(t *testing.T) {
	repo := setupNodeFSRepo(seedTestNode)
	svc := NewNodeFSService(
		testPluginID, &mockFileService{}, repo, newMockArchiveService(), &mockRegistrar{},
		&stubPermissionChecker{allowed: false},
	)
	ctx := context.Background()

	tests := []struct {
		name string
		call func() (success bool, errMsg *string)
	}{
		{"read_dir", func() (bool, *string) {
			resp, _ := svc.ReadDir(ctx, &nodefs.ReadDirRequest{NodeId: 1})

			return resp.Error == nil, resp.Error
		}},
		{"mk_dir", func() (bool, *string) {
			resp, _ := svc.MkDir(ctx, &nodefs.MkDirRequest{NodeId: 1, Path: "/x"})

			return resp.Success, resp.Error
		}},
		{"copy", func() (bool, *string) {
			resp, _ := svc.Copy(ctx, &nodefs.CopyRequest{NodeId: 1})

			return resp.Success, resp.Error
		}},
		{"move", func() (bool, *string) {
			resp, _ := svc.Move(ctx, &nodefs.MoveRequest{NodeId: 1})

			return resp.Success, resp.Error
		}},
		{"download", func() (bool, *string) {
			resp, _ := svc.Download(ctx, &nodefs.DownloadRequest{NodeId: 1})

			return resp.Error == nil, resp.Error
		}},
		{"upload", func() (bool, *string) {
			resp, _ := svc.Upload(ctx, &nodefs.UploadRequest{NodeId: 1})

			return resp.Success, resp.Error
		}},
		{"remove", func() (bool, *string) {
			resp, _ := svc.Remove(ctx, &nodefs.RemoveRequest{NodeId: 1})

			return resp.Success, resp.Error
		}},
		{"get_file_info", func() (bool, *string) {
			resp, _ := svc.GetFileInfo(ctx, &nodefs.GetFileInfoRequest{NodeId: 1})

			return resp.Found, resp.Error
		}},
		{"chmod", func() (bool, *string) {
			resp, _ := svc.Chmod(ctx, &nodefs.ChmodRequest{NodeId: 1})

			return resp.Success, resp.Error
		}},
		{"hash", func() (bool, *string) {
			resp, _ := svc.Hash(ctx, &nodefs.HashRequest{NodeId: 1, Paths: []string{"a"}})

			return resp.Success, resp.Error
		}},
		{"create_archive", func() (bool, *string) {
			resp, _ := svc.CreateArchive(ctx, &nodefs.CreateArchiveRequest{NodeId: 1})

			return resp.Success, resp.Error
		}},
		{"extract_archive", func() (bool, *string) {
			resp, _ := svc.ExtractArchive(ctx, &nodefs.ExtractArchiveRequest{NodeId: 1})

			return resp.Success, resp.Error
		}},
		{"start_create_archive", func() (bool, *string) {
			resp, _ := svc.StartCreateArchive(ctx, &nodefs.CreateArchiveRequest{NodeId: 1})

			return resp.Success, resp.Error
		}},
		{"start_extract_archive", func() (bool, *string) {
			resp, _ := svc.StartExtractArchive(ctx, &nodefs.ExtractArchiveRequest{NodeId: 1})

			return resp.Success, resp.Error
		}},
		{"cancel_archive", func() (bool, *string) {
			resp, _ := svc.CancelArchive(ctx, &nodefs.CancelArchiveRequest{OperationId: "op"})

			return resp.Success, resp.Error
		}},
		{"get_archive_operation", func() (bool, *string) {
			resp, _ := svc.GetArchiveOperation(ctx, &nodefs.GetArchiveOperationRequest{OperationId: "op"})

			return resp.Success, resp.Error
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			success, errMsg := tt.call()
			assert.False(t, success, "operation must be denied without the files grant")
			require.NotNil(t, errMsg)
			assert.Contains(t, *errMsg, "plugin permission files required")
		})
	}
}

func TestNodeFSService_Hash_MapsResults(t *testing.T) {
	// ARRANGE
	repo := setupNodeFSRepo(seedTestNode)
	fsSvc := &mockFileService{
		hashFunc: func(
			_ context.Context, _ *domain.Node, paths []string, algorithm proto.HashAlgorithm,
		) (*proto.HashResult, error) {
			assert.Equal(t, []string{"/srv/a.bin", "/srv/missing.bin"}, paths)
			assert.Equal(t, proto.HashAlgorithm_HASH_ALGORITHM_SHA256, algorithm,
				"unspecified algorithm must default to sha256")

			return &proto.HashResult{
				Algorithm: algorithm,
				Hashes: []*proto.FileHash{
					{Path: "a.bin", Hash: "aa11", Size: 5},
					{Path: "missing.bin", Error: "no such file"},
				},
			}, nil
		},
	}
	svc := newAllowedNodeFSService(fsSvc, repo)

	// ACT
	resp, err := svc.Hash(context.Background(), &nodefs.HashRequest{
		NodeId: 1,
		Paths:  []string{"/srv/a.bin", "/srv/missing.bin"},
	})

	// ASSERT
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, nodefs.HashAlgorithm_HASH_ALGORITHM_SHA256, resp.Algorithm)
	require.Len(t, resp.Results, 2)
	assert.Equal(t, "aa11", resp.Results[0].Hash)
	assert.Nil(t, resp.Results[0].Error)
	require.NotNil(t, resp.Results[1].Error)
	assert.Contains(t, *resp.Results[1].Error, "no such file")
}

func TestNodeFSService_StartCreateArchive_RegistersAndMapsParams(t *testing.T) {
	// ARRANGE
	repo := setupNodeFSRepo(seedTestNode)
	archive := newMockArchiveService()
	archive.operationID = "op-77"
	registrar := &mockRegistrar{}
	svc := newArchiveNodeFSService(archive, registrar, repo)

	// ACT
	resp, err := svc.StartCreateArchive(context.Background(), &nodefs.CreateArchiveRequest{
		NodeId:         1,
		ArchivePath:    "/srv/backup.tar.gz",
		Format:         nodefs.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ,
		BasePath:       "/srv",
		Sources:        []string{"/srv/maps"},
		Overwrite:      true,
		TimeoutSeconds: 120,
		ReportProgress: true,
	})

	// ASSERT
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "op-77", resp.OperationId)

	calls := archive.StartCalls()
	require.Len(t, calls, 1)
	require.NotNil(t, calls[0].create)
	assert.Equal(t, "/srv/backup.tar.gz", calls[0].create.ArchivePath)
	assert.Equal(t, proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ, calls[0].create.Format)
	assert.True(t, calls[0].create.Overwrite)
	assert.Equal(t, "plugin:7", calls[0].create.Options.Initiator)
	assert.Equal(t, uint(0), calls[0].create.Options.ServerID, "plugin operations are node-scoped")
	assert.Equal(t, 120*time.Second, calls[0].create.Options.Timeout)

	registrations := registrar.Calls()
	require.Len(t, registrations, 1)
	assert.Equal(t, testPluginID, registrations[0].pluginID)
	assert.Equal(t, "op-77", registrations[0].operationID)
	assert.Equal(t, uint64(1), registrations[0].nodeID)
	assert.True(t, registrations[0].reportProgress)
}

func TestNodeFSService_CreateArchive_SyncWaitsForCompletion(t *testing.T) {
	// ARRANGE
	repo := setupNodeFSRepo(seedTestNode)
	archive := newMockArchiveService()
	archive.operationID = "op-sync"
	archive.waitResult = &messages.ArchiveCompleteEventPayload{
		OperationID:    "op-sync",
		Success:        true,
		FilesProcessed: 4,
		BytesProcessed: 1024,
		ArchiveSize:    512,
		Format:         "zip",
	}
	registrar := &mockRegistrar{}
	svc := newArchiveNodeFSService(archive, registrar, repo)

	// ACT
	resp, err := svc.CreateArchive(context.Background(), &nodefs.CreateArchiveRequest{
		NodeId:         1,
		ArchivePath:    "/srv/a.zip",
		BasePath:       "/srv",
		Sources:        []string{"/srv/maps"},
		TimeoutSeconds: 30,
		ReportProgress: true,
	})

	// ASSERT
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.True(t, resp.Completed)
	assert.True(t, resp.OpSuccess)
	assert.Equal(t, "op-sync", resp.OperationId)
	assert.Equal(t, uint32(4), resp.FilesProcessed)
	assert.Equal(t, uint64(512), resp.ArchiveSize)
	assert.Equal(t, nodefs.ArchiveFormat_ARCHIVE_FORMAT_ZIP, resp.Format)

	registrations := registrar.Calls()
	require.Len(t, registrations, 1)
	assert.False(t, registrations[0].reportProgress,
		"blocking calls must never request progress callbacks")
}

func TestNodeFSService_CreateArchive_SyncWaitBudgetExhausted(t *testing.T) {
	// ARRANGE
	repo := setupNodeFSRepo(seedTestNode)
	archive := newMockArchiveService()
	archive.operationID = "op-slow"
	archive.waitBlocksCtx = true
	svc := newArchiveNodeFSService(archive, &mockRegistrar{}, repo)

	// ACT
	resp, err := svc.CreateArchive(context.Background(), &nodefs.CreateArchiveRequest{
		NodeId:         1,
		ArchivePath:    "/srv/a.zip",
		BasePath:       "/srv",
		Sources:        []string{"/srv/maps"},
		TimeoutSeconds: 1,
	})

	// ASSERT
	require.NoError(t, err)
	assert.True(t, resp.Success, "a started operation is not an error")
	assert.False(t, resp.Completed, "an exhausted wait budget answers completed=false")
	assert.Equal(t, "op-slow", resp.OperationId, "the plugin needs the id to keep polling")
}

func TestNodeFSService_CancelArchive_OwnershipEnforced(t *testing.T) {
	// ARRANGE
	repo := setupNodeFSRepo(seedTestNode)
	archive := newMockArchiveService()
	archive.snapshots["op-own"] = daemon.ArchiveOpSnapshot{
		OperationID: "op-own",
		NodeID:      1,
		Initiator:   "plugin:7",
		Status:      daemon.ArchiveOpRunning,
	}
	archive.snapshots["op-foreign"] = daemon.ArchiveOpSnapshot{
		OperationID: "op-foreign",
		NodeID:      1,
		Initiator:   "user:3",
		Status:      daemon.ArchiveOpRunning,
	}
	svc := newArchiveNodeFSService(archive, &mockRegistrar{}, repo)

	// ACT + ASSERT: own operation cancels.
	resp, err := svc.CancelArchive(context.Background(), &nodefs.CancelArchiveRequest{
		OperationId: "op-own",
		Reason:      "test",
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, []string{"op-own"}, archive.CancelCalls())

	// A foreign operation is indistinguishable from a missing one.
	resp, err = svc.CancelArchive(context.Background(), &nodefs.CancelArchiveRequest{
		OperationId: "op-foreign",
	})
	require.NoError(t, err)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "operation not found")
	assert.Equal(t, []string{"op-own"}, archive.CancelCalls(), "foreign operations must not be canceled")
}

func TestNodeFSService_GetArchiveOperation(t *testing.T) {
	// ARRANGE
	repo := setupNodeFSRepo(seedTestNode)
	archive := newMockArchiveService()
	archive.snapshots["op-run"] = daemon.ArchiveOpSnapshot{
		OperationID: "op-run",
		NodeID:      1,
		Initiator:   "plugin:7",
		Status:      daemon.ArchiveOpRunning,
		Progress: messages.ArchiveProgressEventPayload{
			FilesProcessed: 2,
			FilesTotal:     10,
			BytesProcessed: 100,
			BytesTotal:     1000,
			CurrentEntry:   "maps/x.bsp",
		},
	}
	archive.snapshots["op-done"] = daemon.ArchiveOpSnapshot{
		OperationID: "op-done",
		NodeID:      1,
		Initiator:   "plugin:7",
		Status:      daemon.ArchiveOpError,
		Result: &messages.ArchiveCompleteEventPayload{
			Success:        false,
			Error:          "canceled: by user",
			FilesProcessed: 3,
			Format:         "tar_gz",
		},
	}
	archive.snapshots["op-foreign"] = daemon.ArchiveOpSnapshot{
		OperationID: "op-foreign",
		Initiator:   "user:1",
	}
	svc := newArchiveNodeFSService(archive, &mockRegistrar{}, repo)
	ctx := context.Background()

	// ACT + ASSERT: running operation reports progress.
	resp, err := svc.GetArchiveOperation(ctx, &nodefs.GetArchiveOperationRequest{OperationId: "op-run"})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.True(t, resp.Found)
	assert.Equal(t, nodefs.ArchiveOperationStatus_ARCHIVE_OPERATION_STATUS_RUNNING, resp.Status)
	assert.Equal(t, uint32(2), resp.FilesProcessed)
	assert.Equal(t, "maps/x.bsp", resp.CurrentEntry)

	// Finished operation reports the final fields.
	resp, err = svc.GetArchiveOperation(ctx, &nodefs.GetArchiveOperationRequest{OperationId: "op-done"})
	require.NoError(t, err)
	assert.True(t, resp.Found)
	assert.Equal(t, nodefs.ArchiveOperationStatus_ARCHIVE_OPERATION_STATUS_ERROR, resp.Status)
	assert.False(t, resp.OpSuccess)
	require.NotNil(t, resp.OpError)
	assert.Contains(t, *resp.OpError, "canceled")
	assert.Equal(t, uint32(3), resp.FilesProcessed)
	assert.Equal(t, nodefs.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ, resp.Format)

	// Foreign and unknown operations are indistinguishable.
	resp, err = svc.GetArchiveOperation(ctx, &nodefs.GetArchiveOperationRequest{OperationId: "op-foreign"})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.False(t, resp.Found)

	resp, err = svc.GetArchiveOperation(ctx, &nodefs.GetArchiveOperationRequest{OperationId: "nope"})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.False(t, resp.Found)
}

func TestNodeFSService_StartCreateArchive_DaemonErrorSurfacesAsMessage(t *testing.T) {
	// ARRANGE
	repo := setupNodeFSRepo(seedTestNode)
	archive := newMockArchiveService()
	archive.startErr = daemon.ErrArchiveNotSupported
	registrar := &mockRegistrar{}
	svc := newArchiveNodeFSService(archive, registrar, repo)

	// ACT
	resp, err := svc.StartCreateArchive(context.Background(), &nodefs.CreateArchiveRequest{
		NodeId:      1,
		ArchivePath: "/srv/a.zip",
		BasePath:    "/srv",
		Sources:     []string{"/srv/x"},
	})

	// ASSERT: expected failures never become Go errors (wazero trap rule).
	require.NoError(t, err)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "node does not support archive operations")
	assert.Empty(t, registrar.Calls(), "failed starts must not register interest")
}
