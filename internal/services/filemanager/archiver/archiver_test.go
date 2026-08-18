package archiver_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/services/filemanager/archiver"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contentLister struct {
	files map[string]string
	stat  *daemon.FileDetails
}

func (l *contentLister) ReadDirRecursive(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
	out := make([]*daemon.FileInfo, 0, len(l.files))
	for path, content := range l.files {
		out = append(out, &daemon.FileInfo{
			Path: path,
			Size: uint64(len(content)),
			Type: daemon.FileTypeFile,
			Perm: 0o644,
		})
	}

	return out, nil
}

func (l *contentLister) GetFileInfo(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
	return l.stat, nil
}

type contentStreamer struct {
	files map[string]string
}

func (s *contentStreamer) DownloadStream(_ context.Context, _ *domain.Node, filePath string) (io.ReadCloser, error) {
	rel := strings.TrimPrefix(filePath, "/work/")
	body, ok := s.files[rel]
	if !ok {
		return io.NopCloser(strings.NewReader("")), nil
	}

	return io.NopCloser(strings.NewReader(body)), nil
}

func TestArchiver_WriteArchive(t *testing.T) {
	t.Parallel()

	node := &domain.Node{ID: 1, WorkPath: "/work"}

	t.Run("writes_zip_with_correct_paths_and_content", func(t *testing.T) {
		t.Parallel()

		files := map[string]string{
			"server-1/data/a.txt":     "alpha",
			"server-1/data/sub/b.txt": "bravo",
		}
		lister := &contentLister{files: files, stat: &daemon.FileDetails{Type: daemon.FileTypeDir}}
		streamer := &contentStreamer{files: files}

		a := archiver.NewArchiver(lister, streamer, nil)
		manifest, err := a.BuildManifest(context.Background(), node, "/work/server-1/data", archiver.Limits{})
		require.NoError(t, err)

		var buf bytes.Buffer
		result, err := a.WriteArchive(context.Background(), &buf, node, manifest, archiver.Options{})
		require.NoError(t, err)
		assert.Greater(t, result.BytesWritten, uint64(0))

		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)

		got := make(map[string]string)
		for _, f := range zr.File {
			rc, openErr := f.Open()
			require.NoError(t, openErr)
			body, readErr := io.ReadAll(rc)
			_ = rc.Close()
			require.NoError(t, readErr)
			got[f.Name] = string(body)
		}

		assert.Equal(t, "alpha", got["data/a.txt"])
		assert.Equal(t, "bravo", got["data/sub/b.txt"])
	})

	t.Run("writes_skipped_report_when_special_files_present", func(t *testing.T) {
		t.Parallel()

		lister := &mixedLister{
			stat: &daemon.FileDetails{Type: daemon.FileTypeDir},
			items: []*daemon.FileInfo{
				{Path: "server-1/data/regular.txt", Size: 5, Type: daemon.FileTypeFile, Perm: 0o644},
				{Path: "server-1/data/socket", Type: daemon.FileTypeSocket},
			},
		}
		streamer := &contentStreamer{files: map[string]string{"server-1/data/regular.txt": "hello"}}

		a := archiver.NewArchiver(lister, streamer, nil)
		manifest, err := a.BuildManifest(context.Background(), node, "/work/server-1/data", archiver.Limits{})
		require.NoError(t, err)

		var buf bytes.Buffer
		_, err = a.WriteArchive(context.Background(), &buf, node, manifest, archiver.Options{})
		require.NoError(t, err)

		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)

		var foundSkipped bool
		for _, f := range zr.File {
			if f.Name == "_SKIPPED.txt" {
				foundSkipped = true
				rc, _ := f.Open()
				body, _ := io.ReadAll(rc)
				_ = rc.Close()
				assert.Contains(t, string(body), "data/socket")
			}
		}
		assert.True(t, foundSkipped, "_SKIPPED.txt entry expected")
	})

	t.Run("preserves_symlinks_as_zip_symlink_entries", func(t *testing.T) {
		t.Parallel()

		lister := &mixedLister{
			stat: &daemon.FileDetails{Type: daemon.FileTypeDir},
			items: []*daemon.FileInfo{
				{Path: "server-1/data/link", Type: daemon.FileTypeSymlink, SymlinkTarget: "../target"},
			},
		}
		streamer := &contentStreamer{files: map[string]string{}}

		a := archiver.NewArchiver(lister, streamer, nil)
		manifest, err := a.BuildManifest(context.Background(), node, "/work/server-1/data", archiver.Limits{})
		require.NoError(t, err)

		var buf bytes.Buffer
		_, err = a.WriteArchive(context.Background(), &buf, node, manifest, archiver.Options{})
		require.NoError(t, err)

		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)

		require.Len(t, zr.File, 1)
		entry := zr.File[0]
		assert.Equal(t, "data/link", entry.Name)
		assert.NotZero(t, entry.Mode()&os.ModeSymlink, "symlink mode bit must be set")

		rc, _ := entry.Open()
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		assert.Equal(t, "../target", string(body))
	})

	t.Run("error_open_stream_propagates_wrapped", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		lister := &mixedLister{
			stat: &daemon.FileDetails{Type: daemon.FileTypeDir},
			items: []*daemon.FileInfo{
				{Path: "server-1/data/regular.txt", Size: 5, Type: daemon.FileTypeFile, Perm: 0o644},
			},
		}
		streamer := &errStreamer{err: errors.New("daemon down")}
		a := archiver.NewArchiver(lister, streamer, nil)
		manifest, err := a.BuildManifest(context.Background(), node, "/work/server-1/data", archiver.Limits{})
		require.NoError(t, err)

		// ACT
		var buf bytes.Buffer
		result, err := a.WriteArchive(context.Background(), &buf, node, manifest, archiver.Options{})

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "open file stream", "wrap layer must surface")
		assert.Contains(t, err.Error(), "daemon down", "underlying error message must propagate")
		_, zipErr := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.Error(t, zipErr, "partial output must not be finalized into a valid archive")
	})

	t.Run("close_stream_error_logged_archive_succeeds", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		lister := &mixedLister{
			stat: &daemon.FileDetails{Type: daemon.FileTypeDir},
			items: []*daemon.FileInfo{
				{Path: "server-1/data/regular.txt", Size: 5, Type: daemon.FileTypeFile, Perm: 0o644},
			},
		}
		streamer := &errCloseStreamer{
			body:     "hello",
			closeErr: errors.New("close kaboom"),
		}
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
		a := archiver.NewArchiver(lister, streamer, logger)
		manifest, err := a.BuildManifest(context.Background(), node, "/work/server-1/data", archiver.Limits{})
		require.NoError(t, err)

		// ACT
		var buf bytes.Buffer
		result, err := a.WriteArchive(context.Background(), &buf, node, manifest, archiver.Options{})

		// ASSERT
		require.NoError(t, err, "close error must not abort the archive")
		require.NotNil(t, result)
		assert.Contains(t, logBuf.String(), "failed to close file stream", "close error must be logged")
	})

	t.Run("symlink_with_empty_target_falls_back_to_GetFileInfo", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		lister := &symlinkLookupLister{
			items: []*daemon.FileInfo{
				{Path: "server-1/data/link", Type: daemon.FileTypeSymlink, SymlinkTarget: ""},
			},
			rootDetails: &daemon.FileDetails{Type: daemon.FileTypeDir},
			lookupResult: &daemon.FileDetails{
				Type:          daemon.FileTypeSymlink,
				SymlinkTarget: "/abs/target",
			},
		}
		streamer := &contentStreamer{files: map[string]string{}}
		a := archiver.NewArchiver(lister, streamer, nil)
		manifest, err := a.BuildManifest(context.Background(), node, "/work/server-1/data", archiver.Limits{})
		require.NoError(t, err)

		// ACT
		var buf bytes.Buffer
		_, err = a.WriteArchive(context.Background(), &buf, node, manifest, archiver.Options{})
		require.NoError(t, err)

		// ASSERT
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)
		require.Len(t, zr.File, 1)
		rc, _ := zr.File[0].Open()
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		assert.Equal(t, "/abs/target", string(body), "missing symlink target must be filled from GetFileInfo")
	})

	t.Run("symlink_with_empty_target_and_lookup_error_writes_empty", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		lister := &symlinkLookupLister{
			items: []*daemon.FileInfo{
				{Path: "server-1/data/link", Type: daemon.FileTypeSymlink, SymlinkTarget: ""},
			},
			rootDetails: &daemon.FileDetails{Type: daemon.FileTypeDir},
			lookupErr:   errors.New("stat failed"),
		}
		streamer := &contentStreamer{files: map[string]string{}}
		a := archiver.NewArchiver(lister, streamer, nil)
		manifest, err := a.BuildManifest(context.Background(), node, "/work/server-1/data", archiver.Limits{})
		require.NoError(t, err)

		// ACT
		var buf bytes.Buffer
		_, err = a.WriteArchive(context.Background(), &buf, node, manifest, archiver.Options{})

		// ASSERT
		require.NoError(t, err, "lookup error must be swallowed and the entry still emitted")
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)
		require.Len(t, zr.File, 1)
		rc, _ := zr.File[0].Open()
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		assert.Empty(t, string(body), "no target known means empty entry body")
	})

	t.Run("compression_level_marks_method_deflate", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		files := map[string]string{
			"server-1/data/x.txt": strings.Repeat("ABCDEF ", 200),
		}
		lister := &contentLister{files: files, stat: &daemon.FileDetails{Type: daemon.FileTypeDir}}
		streamer := &contentStreamer{files: files}
		a := archiver.NewArchiver(lister, streamer, nil)
		manifest, err := a.BuildManifest(context.Background(), node, "/work/server-1/data", archiver.Limits{})
		require.NoError(t, err)

		// ACT
		var buf bytes.Buffer
		_, err = a.WriteArchive(context.Background(), &buf, node, manifest, archiver.Options{CompressLevel: 6})
		require.NoError(t, err)

		// ASSERT
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)
		require.Len(t, zr.File, 1)
		f := zr.File[0]
		assert.Equal(t, zip.Deflate, f.Method, "file entry must use Deflate method")
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		assert.Equal(t, files["server-1/data/x.txt"], string(body), "round-trip body must match")
	})

	t.Run("directory_entry_uses_store_method_and_trailing_slash", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		lister := &mixedLister{
			stat: &daemon.FileDetails{Type: daemon.FileTypeDir},
			items: []*daemon.FileInfo{
				{Path: "server-1/data/sub", Type: daemon.FileTypeDir, Perm: 0o755},
				{Path: "server-1/data/sub/keep.txt", Size: 1, Type: daemon.FileTypeFile, Perm: 0o644},
			},
		}
		streamer := &contentStreamer{files: map[string]string{"server-1/data/sub/keep.txt": "k"}}
		a := archiver.NewArchiver(lister, streamer, nil)
		manifest, err := a.BuildManifest(context.Background(), node, "/work/server-1/data", archiver.Limits{})
		require.NoError(t, err)

		// ACT
		var buf bytes.Buffer
		_, err = a.WriteArchive(context.Background(), &buf, node, manifest, archiver.Options{})
		require.NoError(t, err)

		// ASSERT
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.NoError(t, err)

		var dirEntry *zip.File
		for _, f := range zr.File {
			if f.Name == "data/sub/" {
				dirEntry = f

				break
			}
		}
		require.NotNil(t, dirEntry, "directory entry with trailing slash must be present")
		assert.Equal(t, zip.Store, dirEntry.Method, "directory entry must use Store method")
		rc, _ := dirEntry.Open()
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		assert.Empty(t, string(body), "directory entry body must be empty")
	})

	t.Run("context_cancelled_between_entries_returns_ctx_err", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		lister := &mixedLister{
			stat: &daemon.FileDetails{Type: daemon.FileTypeDir},
			items: []*daemon.FileInfo{
				{Path: "server-1/data/a.txt", Size: 1, Type: daemon.FileTypeFile, Perm: 0o644},
				{Path: "server-1/data/b.txt", Size: 1, Type: daemon.FileTypeFile, Perm: 0o644},
			},
		}
		ctx, cancel := context.WithCancel(context.Background())
		// Larger than zip.Writer's internal bufio buffer so part of the entry reaches the output.
		streamer := &cancellingStreamer{cancel: cancel, body: strings.Repeat("x", 16*1024)}
		a := archiver.NewArchiver(lister, streamer, nil)
		manifest, err := a.BuildManifest(ctx, node, "/work/server-1/data", archiver.Limits{})
		require.NoError(t, err)

		// ACT
		var buf bytes.Buffer
		result, err := a.WriteArchive(ctx, &buf, node, manifest, archiver.Options{})

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, result)
		assert.True(
			t,
			errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled"),
			"context cancellation must surface as ctx.Err: %v", err,
		)
		assert.Positive(t, buf.Len(), "the first entry was streamed before the cancellation")
		_, zipErr := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		require.Error(t, zipErr, "cancelled output must not be finalized into a valid archive")
	})
}

type errStreamer struct {
	err error
}

func (s *errStreamer) DownloadStream(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
	return nil, s.err
}

type errCloseStreamer struct {
	body     string
	closeErr error
}

type readWithErrCloser struct {
	io.Reader

	err error
}

func (r *readWithErrCloser) Close() error { return r.err }

func (s *errCloseStreamer) DownloadStream(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
	return &readWithErrCloser{Reader: strings.NewReader(s.body), err: s.closeErr}, nil
}

type symlinkLookupLister struct {
	items        []*daemon.FileInfo
	rootDetails  *daemon.FileDetails
	lookupResult *daemon.FileDetails
	lookupErr    error
	rootCalled   atomic.Bool
}

func (m *symlinkLookupLister) ReadDirRecursive(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
	return m.items, nil
}

func (m *symlinkLookupLister) GetFileInfo(_ context.Context, _ *domain.Node, p string) (*daemon.FileDetails, error) {
	if !m.rootCalled.Load() {
		m.rootCalled.Store(true)

		return m.rootDetails, nil
	}
	if m.lookupErr != nil {
		return nil, errors.WithMessage(m.lookupErr, "lookup "+p)
	}

	return m.lookupResult, nil
}

type cancellingStreamer struct {
	cancel context.CancelFunc
	body   string
	called atomic.Int32
}

func (s *cancellingStreamer) DownloadStream(_ context.Context, _ *domain.Node, _ string) (io.ReadCloser, error) {
	if s.called.Add(1) == 1 {
		s.cancel()
	}

	return io.NopCloser(strings.NewReader(s.body)), nil
}

type mixedLister struct {
	stat  *daemon.FileDetails
	items []*daemon.FileInfo
}

func (m *mixedLister) ReadDirRecursive(_ context.Context, _ *domain.Node, _ string) ([]*daemon.FileInfo, error) {
	return m.items, nil
}

func (m *mixedLister) GetFileInfo(_ context.Context, _ *domain.Node, _ string) (*daemon.FileDetails, error) {
	return m.stat, nil
}
