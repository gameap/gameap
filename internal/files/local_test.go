package files

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/iotest"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBrokenReader = errors.New("boom")

func TestNewLocalFileManager(t *testing.T) {
	t.Parallel()
	t.Run("creates_with_valid_path", func(t *testing.T) {
		t.Parallel()
		tempDir := t.TempDir()

		fm := NewLocalFileManager(tempDir)

		require.NotNil(t, fm)
		require.NotNil(t, fm.root)
	})

	t.Run("panics_with_invalid_path", func(t *testing.T) {
		t.Parallel()
		assert.Panics(t, func() {
			NewLocalFileManager("/nonexistent/path/that/does/not/exist")
		})
	})
}

func TestLocalFileManager_Read(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(t *testing.T, tempDir string)
		path      string
		wantData  []byte
		wantError string
	}{
		{
			name: "read_existing_file",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("hello world"), 0644)
				require.NoError(t, err)
			},
			path:     "test.txt",
			wantData: []byte("hello world"),
		},
		{
			name:      "read_non_existent_file",
			setup:     func(_ *testing.T, _ string) {},
			path:      "nonexistent.txt",
			wantData:  nil,
			wantError: "failed to read file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
			ctx := context.Background()

			data, err := fm.Read(ctx, tt.path)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantData, data)
			}
		})
	}
}

func TestLocalFileManager_Write(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(t *testing.T, tempDir string)
		path      string
		data      []byte
		wantError string
	}{
		{
			name:  "write_new_file",
			setup: func(_ *testing.T, _ string) {},
			path:  "new_file.txt",
			data:  []byte("new content"),
		},
		{
			name: "write_creates_directories",
			setup: func(_ *testing.T, _ string) {
			},
			path: "subdir/nested/file.txt",
			data: []byte("nested content"),
		},
		{
			name: "overwrite_existing_file",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "existing.txt"), []byte("old content"), 0644)
				require.NoError(t, err)
			},
			path: "existing.txt",
			data: []byte("new content"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
			ctx := context.Background()

			err := fm.Write(ctx, tt.path, tt.data)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
				data, err := fm.Read(ctx, tt.path)
				require.NoError(t, err)
				assert.Equal(t, tt.data, data)
			}
		})
	}
}

func TestLocalFileManager_Delete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(t *testing.T, tempDir string)
		path      string
		wantError string
	}{
		{
			name: "delete_existing_file",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "to_delete.txt"), []byte("content"), 0644)
				require.NoError(t, err)
			},
			path: "to_delete.txt",
		},
		{
			name:      "delete_non_existent_file_returns_error",
			setup:     func(_ *testing.T, _ string) {},
			path:      "nonexistent.txt",
			wantError: "failed to delete file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
			ctx := context.Background()

			err := fm.Delete(ctx, tt.path)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
				assert.False(t, fm.Exists(ctx, tt.path))
			}
		})
	}
}

func TestLocalFileManager_Exists(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		setup  func(t *testing.T, tempDir string)
		path   string
		exists bool
	}{
		{
			name: "file_exists",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "exists.txt"), []byte("content"), 0644)
				require.NoError(t, err)
			},
			path:   "exists.txt",
			exists: true,
		},
		{
			name:   "file_does_not_exist",
			setup:  func(_ *testing.T, _ string) {},
			path:   "nonexistent.txt",
			exists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
			ctx := context.Background()

			exists := fm.Exists(ctx, tt.path)

			assert.Equal(t, tt.exists, exists)
		})
	}
}

func TestLocalFileManager_List(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(t *testing.T, tempDir string)
		dir       string
		wantFiles []string
		wantError string
	}{
		{
			name: "list_directory_files",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				subDir := filepath.Join(tempDir, "subdir")
				err := os.MkdirAll(subDir, 0755)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(subDir, "file1.txt"), []byte("content1"), 0644)
				require.NoError(t, err)
				err = os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("content2"), 0644)
				require.NoError(t, err)
			},
			dir:       "subdir",
			wantFiles: []string{"file1.txt", "file2.txt"},
		},
		{
			name: "list_empty_directory",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				emptyDir := filepath.Join(tempDir, "empty")
				err := os.MkdirAll(emptyDir, 0755)
				require.NoError(t, err)
			},
			dir:       "empty",
			wantFiles: []string{},
		},
		{
			name:      "list_non_existent_directory_returns_error",
			setup:     func(_ *testing.T, _ string) {},
			dir:       "nonexistent",
			wantFiles: nil,
			wantError: "failed to open directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
			ctx := context.Background()

			files, err := fm.List(ctx, tt.dir)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
				require.Len(t, files, len(tt.wantFiles))
				for _, wantFile := range tt.wantFiles {
					assert.Contains(t, files, wantFile)
				}
			}
		})
	}
}

func TestLocalFileManager_ReadStream(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(t *testing.T, tempDir string)
		path      string
		wantData  []byte
		wantError string
	}{
		{
			name: "reads_full_file_content",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "stream.txt"), []byte("hello stream world"), 0644)
				require.NoError(t, err)
			},
			path:     "stream.txt",
			wantData: []byte("hello stream world"),
		},
		{
			name: "empty_file_returns_empty_stream",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "empty.txt"), nil, 0644)
				require.NoError(t, err)
			},
			path:     "empty.txt",
			wantData: []byte{},
		},
		{
			name:      "non_existent_file_returns_error",
			setup:     func(_ *testing.T, _ string) {},
			path:      "nonexistent.txt",
			wantError: "failed to open file for reading",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
			ctx := context.Background()

			// ACT
			rc, err := fm.ReadStream(ctx, tt.path)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, rc, "reader must be nil on error")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, rc)
			defer func() { _ = rc.Close() }()

			data, readErr := io.ReadAll(rc)
			require.NoError(t, readErr)
			assert.Equal(t, tt.wantData, data, "stream content must match file content")
		})
	}
}

func TestLocalFileManager_ReadStreamAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(t *testing.T, tempDir string)
		path      string
		offset    int64
		wantData  []byte
		wantError string
	}{
		{
			name: "mid_file_offset_reads_remainder",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "data.txt"), []byte("hello world"), 0644)
				require.NoError(t, err)
			},
			path:     "data.txt",
			offset:   6,
			wantData: []byte("world"),
		},
		{
			name: "offset_zero_reads_full_content",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "data.txt"), []byte("abcdef"), 0644)
				require.NoError(t, err)
			},
			path:     "data.txt",
			offset:   0,
			wantData: []byte("abcdef"),
		},
		{
			name: "offset_equal_to_file_size_returns_empty",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "data.txt"), []byte("abcdef"), 0644)
				require.NoError(t, err)
			},
			path:     "data.txt",
			offset:   6,
			wantData: []byte{},
		},
		{
			name: "offset_beyond_file_size_returns_empty",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "data.txt"), []byte("abcdef"), 0644)
				require.NoError(t, err)
			},
			path:     "data.txt",
			offset:   100,
			wantData: []byte{},
		},
		{
			name: "negative_offset_returns_error",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "data.txt"), []byte("abcdef"), 0644)
				require.NoError(t, err)
			},
			path:      "data.txt",
			offset:    -1,
			wantError: "failed to seek to offset",
		},
		{
			name:      "non_existent_file_returns_error",
			setup:     func(_ *testing.T, _ string) {},
			path:      "nonexistent.txt",
			offset:    0,
			wantError: "failed to open file for reading at offset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
			ctx := context.Background()

			// ACT
			rc, err := fm.ReadStreamAt(ctx, tt.path, tt.offset)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, rc, "reader must be nil on error")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, rc)
			defer func() { _ = rc.Close() }()

			data, readErr := io.ReadAll(rc)
			require.NoError(t, readErr)
			assert.Equal(t, tt.wantData, data, "stream content from offset must match expected slice")
		})
	}
}

func TestLocalFileManager_WriteStream(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(t *testing.T, tempDir string)
		path      string
		reader    io.Reader
		wantData  []byte
		wantError string
	}{
		{
			name:     "writes_new_file_from_reader",
			setup:    func(_ *testing.T, _ string) {},
			path:     "stream_new.txt",
			reader:   bytes.NewReader([]byte("stream content")),
			wantData: []byte("stream content"),
		},
		{
			name: "overwrites_existing_file_truncating",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(
					filepath.Join(tempDir, "existing.txt"),
					[]byte("a much longer original content"),
					0644,
				)
				require.NoError(t, err)
			},
			path:     "existing.txt",
			reader:   bytes.NewReader([]byte("short")),
			wantData: []byte("short"),
		},
		{
			name:     "creates_nested_directories",
			setup:    func(_ *testing.T, _ string) {},
			path:     "deep/nested/dirs/file.txt",
			reader:   bytes.NewReader([]byte("nested")),
			wantData: []byte("nested"),
		},
		{
			name:     "writes_empty_file_from_empty_reader",
			setup:    func(_ *testing.T, _ string) {},
			path:     "empty_stream.txt",
			reader:   bytes.NewReader(nil),
			wantData: []byte{},
		},
		{
			name:      "broken_reader_returns_error",
			setup:     func(_ *testing.T, _ string) {},
			path:      "broken.txt",
			reader:    iotest.ErrReader(errBrokenReader),
			wantError: "failed to write stream data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
			ctx := context.Background()

			// ACT
			err := fm.WriteStream(ctx, tt.path, tt.reader)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				return
			}

			require.NoError(t, err)

			got, readErr := fm.Read(ctx, tt.path)
			require.NoError(t, readErr, "file must be readable after WriteStream")
			assert.Equal(t, tt.wantData, got, "persisted content must match streamed content")
		})
	}
}

func TestLocalFileManager_DeleteByPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		setup          func(t *testing.T, fm *LocalFileManager)
		prefix         string
		wantSurviving  []string
		wantRemoved    []string
		wantDirRemoved []string
		wantError      string
	}{
		{
			name: "removes_files_matching_prefix",
			setup: func(t *testing.T, fm *LocalFileManager) {
				t.Helper()
				ctx := context.Background()
				require.NoError(t, fm.Write(ctx, "logs/job_1.log", []byte("a")))
				require.NoError(t, fm.Write(ctx, "logs/job_2.log", []byte("b")))
				require.NoError(t, fm.Write(ctx, "logs/other.log", []byte("c")))
			},
			prefix:        "logs/job_",
			wantSurviving: []string{"logs/other.log"},
			wantRemoved:   []string{"logs/job_1.log", "logs/job_2.log"},
		},
		{
			name: "removes_directory_recursively_when_prefix_matches_dir_name",
			setup: func(t *testing.T, fm *LocalFileManager) {
				t.Helper()
				ctx := context.Background()
				require.NoError(t, fm.Write(ctx, "data/cache/a.bin", []byte("a")))
				require.NoError(t, fm.Write(ctx, "data/cache/inner/b.bin", []byte("b")))
				require.NoError(t, fm.Write(ctx, "data/keep.bin", []byte("k")))
			},
			prefix:         "data/cache",
			wantSurviving:  []string{"data/keep.bin"},
			wantRemoved:    []string{"data/cache/a.bin", "data/cache/inner/b.bin"},
			wantDirRemoved: []string{"data/cache/inner", "data/cache"},
		},
		{
			name: "no_matching_entries_is_noop",
			setup: func(t *testing.T, fm *LocalFileManager) {
				t.Helper()
				ctx := context.Background()
				require.NoError(t, fm.Write(ctx, "logs/keep.log", []byte("k")))
			},
			prefix:        "logs/missing_",
			wantSurviving: []string{"logs/keep.log"},
		},
		{
			name:          "missing_parent_directory_is_noop",
			setup:         func(_ *testing.T, _ *LocalFileManager) {},
			prefix:        "no/such/dir/anything",
			wantSurviving: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			tempDir := t.TempDir()
			fm := NewLocalFileManager(tempDir)
			tt.setup(t, fm)
			ctx := context.Background()

			// ACT
			err := fm.DeleteByPrefix(ctx, tt.prefix)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")

				return
			}

			require.NoError(t, err)

			for _, p := range tt.wantSurviving {
				assert.True(t, fm.Exists(ctx, p), "expected surviving file to remain: %s", p)
			}
			for _, p := range tt.wantRemoved {
				assert.False(t, fm.Exists(ctx, p), "expected removed file to be gone: %s", p)
			}
			for _, p := range tt.wantDirRemoved {
				assert.False(t, fm.Exists(ctx, p), "expected removed directory to be gone: %s", p)
			}
		})
	}
}

func TestLocalFileManager_PathTraversal_Rejected(t *testing.T) {
	t.Parallel()
	// ARRANGE
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "root")
	require.NoError(t, os.Mkdir(rootDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "escape.txt"), []byte("OUTSIDE"), 0644))

	fm := NewLocalFileManager(rootDir)
	ctx := context.Background()

	// ACT
	data, err := fm.Read(ctx, "../escape.txt")

	// ASSERT
	require.Error(t, err, "os.Root must reject path traversal via ..")
	assert.Nil(t, data, "no data should be returned on rejected traversal")
	assert.Contains(t, err.Error(), "failed to read file", "error must be wrapped by Read")
}
