package hostlibrary

import (
	"context"
	"os"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCappedNodeFSService(fs NodeFileService, maxInline uint64) *NodeFSServiceImpl {
	repo := setupNodeFSRepo(seedTestNode)

	return NewNodeFSService(
		testPluginID, fs, repo, newMockArchiveService(), &mockRegistrar{},
		stubPermissionChecker{allowed: true},
		WithNodeFSMaxInlineBytes(maxInline),
	)
}

func TestNodeFSService_Download_inline_limit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		maxInline    uint64
		fileSize     uint64
		statErr      error
		wantError    string
		wantDownload bool
		wantStat     bool
	}{
		{
			name:         "no_cap_skips_stat",
			maxInline:    0,
			fileSize:     1 << 30,
			wantDownload: true,
			wantStat:     false,
		},
		{
			name:         "under_cap_downloads",
			maxInline:    1024,
			fileSize:     1024,
			wantDownload: true,
			wantStat:     true,
		},
		{
			name:      "over_cap_is_refused_before_reading",
			maxInline: 1024,
			fileSize:  1025,
			wantError: "file too large: 1025 bytes exceeds the inline download limit of 1024 bytes",
			wantStat:  true,
		},
		{
			name:      "stat_error_with_cap_is_reported",
			maxInline: 1024,
			statErr:   errors.New("daemon unavailable"),
			wantError: "stat failed: daemon unavailable",
			wantStat:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var statCalls, downloadCalls atomic.Int32
			fs := &mockFileService{
				getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
					statCalls.Add(1)

					if tt.statErr != nil {
						return nil, tt.statErr
					}

					return &daemon.FileDetails{Name: "file.bin", Size: tt.fileSize}, nil
				},
				downloadFunc: func(_ context.Context, _ *domain.Node, _ string) ([]byte, error) {
					downloadCalls.Add(1)

					return []byte("content"), nil
				},
			}
			svc := newCappedNodeFSService(fs, tt.maxInline)

			resp, err := svc.Download(context.Background(), &nodefs.DownloadRequest{NodeId: 1, Path: "/home/file.bin"})
			require.NoError(t, err)

			assert.Equal(t, tt.wantStat, statCalls.Load() == 1)
			assert.Equal(t, tt.wantDownload, downloadCalls.Load() == 1)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
				assert.Nil(t, resp.Content)

				return
			}

			assert.Nil(t, resp.Error)
			assert.Equal(t, []byte("content"), resp.Content)
		})
	}
}

func TestNodeFSService_Upload_inline_limit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		maxInline   uint64
		contentSize int
		wantError   string
		wantUpload  bool
	}{
		{name: "no_cap_uploads", maxInline: 0, contentSize: 4096, wantUpload: true},
		{name: "under_cap_uploads", maxInline: 4096, contentSize: 4096, wantUpload: true},
		{
			name:        "over_cap_is_refused",
			maxInline:   4096,
			contentSize: 4097,
			wantError:   "content too large: 4097 bytes exceeds the inline upload limit of 4096 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var uploadCalls atomic.Int32
			fs := &mockFileService{
				uploadFunc: func(_ context.Context, _ *domain.Node, _ string, _ []byte, _ os.FileMode) error {
					uploadCalls.Add(1)

					return nil
				},
			}
			svc := newCappedNodeFSService(fs, tt.maxInline)

			resp, err := svc.Upload(context.Background(), &nodefs.UploadRequest{
				NodeId:  1,
				Path:    "/home/file.bin",
				Content: make([]byte, tt.contentSize),
			})
			require.NoError(t, err)

			assert.Equal(t, tt.wantUpload, uploadCalls.Load() == 1)

			if tt.wantError != "" {
				assert.False(t, resp.Success)
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.True(t, resp.Success)
			assert.Nil(t, resp.Error)
		})
	}
}

func TestNodeFSHostLibraryFactory_passes_options(t *testing.T) {
	t.Parallel()
	factory := NewNodeFSHostLibraryFactory(
		&mockFileService{}, setupNodeFSRepo(nil), newMockArchiveService(), &mockRegistrar{},
		stubPermissionChecker{allowed: true},
		WithNodeFSMaxInlineBytes(123),
	)

	lib, ok := factory.Create(42).(*NodeFSHostLibrary)
	require.True(t, ok)
	assert.Equal(t, uint64(123), lib.impl.maxInlineBytes)
	assert.Equal(t, uint64(42), lib.impl.pluginID)
}
