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
		allowAllGuard(testPluginID),
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
		wantLimit    uint64
		wantStat     bool
	}{
		{
			name:         "no_cap_skips_stat_and_reads_everything",
			maxInline:    0,
			fileSize:     1 << 30,
			wantDownload: true,
			wantLimit:    0,
			wantStat:     false,
		},
		{
			name:         "under_cap_reads_one_byte_past_the_cap",
			maxInline:    1024,
			fileSize:     1024,
			wantDownload: true,
			wantLimit:    1025,
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
			var gotLimit atomic.Uint64
			fs := &mockFileService{
				getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
					statCalls.Add(1)

					if tt.statErr != nil {
						return nil, tt.statErr
					}

					return &daemon.FileDetails{Name: "file.bin", Size: tt.fileSize}, nil
				},
				downloadFunc: func(_ context.Context, _ *domain.Node, _ string, limit uint64) ([]byte, error) {
					downloadCalls.Add(1)
					gotLimit.Store(limit)

					return []byte("content"), nil
				},
			}
			svc := newCappedNodeFSService(fs, tt.maxInline)

			resp, err := svc.Download(context.Background(), &nodefs.DownloadRequest{NodeId: 1, Path: "/home/file.bin"})
			require.NoError(t, err)

			assert.Equal(t, tt.wantStat, statCalls.Load() == 1)
			assert.Equal(t, tt.wantDownload, downloadCalls.Load() == 1)
			if tt.wantDownload {
				assert.Equal(t, tt.wantLimit, gotLimit.Load(), "the read must be bounded by the cap")
			}

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

func TestNodeFSService_Download_enforces_limit_after_read(t *testing.T) {
	t.Parallel()
	// The file grew between stat and read: the stat passed, the content is
	// over the cap and must not reach the guest. The node is asked for one
	// byte past the cap only, so the rest of the file is never buffered.
	const grownSize = 1 << 20

	var gotLimit atomic.Uint64
	fs := &mockFileService{
		getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
			return &daemon.FileDetails{Name: "log.txt", Size: 10}, nil
		},
		downloadFunc: func(_ context.Context, _ *domain.Node, _ string, limit uint64) ([]byte, error) {
			gotLimit.Store(limit)

			return make([]byte, min(limit, grownSize)), nil
		},
	}
	svc := newCappedNodeFSService(fs, 1024)

	resp, err := svc.Download(context.Background(), &nodefs.DownloadRequest{NodeId: 1, Path: "/home/log.txt"})
	require.NoError(t, err)

	assert.Equal(t, uint64(1025), gotLimit.Load(), "the read stops one byte past the cap")
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, "file too large: content exceeds the inline download limit of 1024 bytes")
	assert.Nil(t, resp.Content)
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
		NewGuard(stubPermissionChecker{allowed: true}),
		WithNodeFSMaxInlineBytes(123),
	)

	lib, ok := factory.Create(42).(*NodeFSHostLibrary)
	require.True(t, ok)
	assert.Equal(t, uint64(123), lib.impl.maxInlineBytes)
	assert.Equal(t, uint64(42), lib.impl.pluginID)
}

// A file past the inline limit used to be unreadable altogether. Naming a
// window makes it readable a piece at a time: only the window has to fit.
func TestNodeFSService_Download_windowed(t *testing.T) {
	t.Parallel()

	const (
		inlineCap = 1024
		fileSize  = 10_000
	)

	tests := []struct {
		name       string
		offset     uint64
		length     uint64
		wantOffset uint64
		wantRead   uint64 // the limit handed to the node; 0 = no read expected
		wantServed uint64 // the bytes that reach the guest
		wantError  string
	}{
		{
			name:   "a_whole_file_past_the_cap_is_still_refused",
			offset: 0, length: 0,
			wantError: "file too large",
		},
		{
			name:   "a_window_inside_the_cap_is_served",
			offset: 4096, length: 512,
			wantOffset: 4096, wantRead: 513, wantServed: 512,
		},
		{
			name:   "a_window_larger_than_the_cap_is_refused",
			offset: 0, length: inlineCap + 1,
			wantError: "window too large",
		},
		{
			name:   "an_open_ended_window_is_cut_to_the_cap",
			offset: 4096, length: 0,
			wantOffset: 4096, wantRead: inlineCap + 1, wantServed: inlineCap,
		},
		{
			name:   "the_last_window_is_served_short",
			offset: fileSize - 256, length: 512,
			wantOffset: fileSize - 256, wantRead: 513, wantServed: 256,
		},
		{
			name:   "a_window_past_the_end_answers_empty",
			offset: fileSize, length: 512,
			wantOffset: fileSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var reads atomic.Int32
			var gotOffset, gotLimit atomic.Uint64
			fs := &mockFileService{
				getFileInfoFunc: func(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
					return &daemon.FileDetails{Name: "big.bin", Size: fileSize}, nil
				},
				downloadRangeFunc: func(
					_ context.Context, _ *domain.Node, _ string, offset, limit uint64,
				) ([]byte, error) {
					reads.Add(1)
					gotOffset.Store(offset)
					gotLimit.Store(limit)

					// What the node does, and what the trimming depends on: at
					// most limit bytes from offset, limit 0 reading to the end.
					available := fileSize - min(fileSize, offset)
					if limit == 0 {
						limit = available
					}

					return make([]byte, min(limit, available)), nil
				},
			}
			svc := newCappedNodeFSService(fs, inlineCap)

			resp, err := svc.Download(context.Background(), &nodefs.DownloadRequest{
				NodeId: 1, Path: "/home/big.bin", Offset: tt.offset, Length: tt.length,
			})
			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
				assert.Zero(t, reads.Load(), "a refused window never reaches the node")

				return
			}

			require.Nil(t, resp.Error)
			assert.Equal(t, tt.wantOffset, resp.Offset, "the offset is echoed back")
			assert.Equal(t, uint64(fileSize), resp.TotalSize, "so the caller knows when to stop paging")
			assert.Equal(t, tt.wantServed, uint64(len(resp.Content)),
				"the guest is served the window it asked for, never the extra byte the read took")

			if tt.wantRead == 0 {
				assert.Zero(t, reads.Load(), "reading past the end costs no round trip")
				assert.Empty(t, resp.Content)

				return
			}

			require.Equal(t, int32(1), reads.Load())
			assert.Equal(t, tt.wantOffset, gotOffset.Load())
			assert.Equal(t, tt.wantRead, gotLimit.Load(),
				"the node is asked for one byte past the window, to tell full from cut")
		})
	}
}
