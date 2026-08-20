package files

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryFileManager_Read(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setup       func(fm *InMemoryFileManager)
		path        string
		wantData    []byte
		wantErr     bool
		errContains string
	}{
		{
			name: "read_existing_file",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "test.txt", []byte("hello world"))
			},
			path:     "test.txt",
			wantData: []byte("hello world"),
			wantErr:  false,
		},
		{
			name:        "read_non_existent_file",
			setup:       func(_ *InMemoryFileManager) {},
			path:        "nonexistent.txt",
			wantData:    nil,
			wantErr:     true,
			errContains: "file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := NewInMemoryFileManager()
			tt.setup(fm)
			ctx := context.Background()

			data, err := fm.Read(ctx, tt.path)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantData, data)
			}
		})
	}
}

func TestInMemoryFileManager_Write(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		setup   func(fm *InMemoryFileManager)
		path    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "write_new_file",
			setup:   func(_ *InMemoryFileManager) {},
			path:    "new_file.txt",
			data:    []byte("new content"),
			wantErr: false,
		},
		{
			name: "overwrite_existing_file",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "existing.txt", []byte("old content"))
			},
			path:    "existing.txt",
			data:    []byte("new content"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := NewInMemoryFileManager()
			tt.setup(fm)
			ctx := context.Background()

			err := fm.Write(ctx, tt.path, tt.data)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				data, err := fm.Read(ctx, tt.path)
				require.NoError(t, err)
				assert.Equal(t, tt.data, data)
			}
		})
	}
}

func TestInMemoryFileManager_Delete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		setup   func(fm *InMemoryFileManager)
		path    string
		wantErr bool
	}{
		{
			name: "delete_existing_file",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "to_delete.txt", []byte("content"))
			},
			path:    "to_delete.txt",
			wantErr: false,
		},
		{
			name:    "delete_non_existent_file",
			setup:   func(_ *InMemoryFileManager) {},
			path:    "nonexistent.txt",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := NewInMemoryFileManager()
			tt.setup(fm)
			ctx := context.Background()

			err := fm.Delete(ctx, tt.path)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.False(t, fm.Exists(ctx, tt.path))
			}
		})
	}
}

func TestInMemoryFileManager_Exists(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		setup  func(fm *InMemoryFileManager)
		path   string
		exists bool
	}{
		{
			name: "file_exists",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "exists.txt", []byte("content"))
			},
			path:   "exists.txt",
			exists: true,
		},
		{
			name:   "file_does_not_exist",
			setup:  func(_ *InMemoryFileManager) {},
			path:   "nonexistent.txt",
			exists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := NewInMemoryFileManager()
			tt.setup(fm)
			ctx := context.Background()

			exists := fm.Exists(ctx, tt.path)

			assert.Equal(t, tt.exists, exists)
		})
	}
}

func TestInMemoryFileManager_List(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		setup     func(fm *InMemoryFileManager)
		dir       string
		wantFiles []string
		wantErr   bool
	}{
		{
			name: "list_files_with_prefix",
			setup: func(fm *InMemoryFileManager) {
				ctx := context.Background()
				_ = fm.Write(ctx, "dir/file1.txt", []byte("content1"))
				_ = fm.Write(ctx, "dir/file2.txt", []byte("content2"))
				_ = fm.Write(ctx, "other/file3.txt", []byte("content3"))
			},
			dir:       "dir/",
			wantFiles: []string{"dir/file1.txt", "dir/file2.txt"},
			wantErr:   false,
		},
		{
			name:      "list_empty_directory",
			setup:     func(_ *InMemoryFileManager) {},
			dir:       "empty/",
			wantFiles: nil,
			wantErr:   false,
		},
		{
			name: "list_multiple_files",
			setup: func(fm *InMemoryFileManager) {
				ctx := context.Background()
				_ = fm.Write(ctx, "a.txt", []byte("a"))
				_ = fm.Write(ctx, "b.txt", []byte("b"))
				_ = fm.Write(ctx, "c.txt", []byte("c"))
			},
			dir:       "",
			wantFiles: []string{"a.txt", "b.txt", "c.txt"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := NewInMemoryFileManager()
			tt.setup(fm)
			ctx := context.Background()

			files, err := fm.List(ctx, tt.dir)

			if tt.wantErr {
				require.Error(t, err)
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

func TestInMemoryFileManager_Concurrency(t *testing.T) {
	t.Parallel()
	fm := NewInMemoryFileManager()
	ctx := context.Background()

	const goroutines = 100
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				path := "concurrent_file.txt"
				_ = fm.Write(ctx, path, []byte("data"))
			}
		}()

		go func() {
			defer wg.Done()
			for range iterations {
				path := "concurrent_file.txt"
				_, _ = fm.Read(ctx, path)
				_ = fm.Exists(ctx, path)
			}
		}()
	}

	wg.Wait()
}

func TestInMemoryFileManager_ReadStream(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setup       func(fm *InMemoryFileManager)
		path        string
		wantData    []byte
		wantErr     bool
		errContains string
	}{
		{
			name: "read_existing_file",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "stream.txt", []byte("stream content"))
			},
			path:     "stream.txt",
			wantData: []byte("stream content"),
			wantErr:  false,
		},
		{
			name:        "read_non_existent_file",
			setup:       func(_ *InMemoryFileManager) {},
			path:        "nonexistent.txt",
			wantErr:     true,
			errContains: "file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := NewInMemoryFileManager()
			tt.setup(fm)
			ctx := context.Background()

			reader, err := fm.ReadStream(ctx, tt.path)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}

				return
			}

			require.NoError(t, err)
			data, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			require.NoError(t, reader.Close())
			assert.Equal(t, tt.wantData, data)
		})
	}
}

func TestInMemoryFileManager_ReadStream_ReturnsCopy(t *testing.T) {
	t.Parallel()
	fm := NewInMemoryFileManager()
	ctx := context.Background()

	require.NoError(t, fm.Write(ctx, "copy.txt", []byte("original")))

	reader, err := fm.ReadStream(ctx, "copy.txt")
	require.NoError(t, err)

	// Overwrite the file after opening the stream: the stream must keep the old content.
	require.NoError(t, fm.Write(ctx, "copy.txt", []byte("overwritten")))

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	assert.Equal(t, []byte("original"), data)
}

func TestInMemoryFileManager_ReadStreamAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setup       func(fm *InMemoryFileManager)
		path        string
		offset      int64
		wantData    []byte
		wantErr     bool
		errContains string
	}{
		{
			name: "zero_offset_reads_whole_file",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "file.txt", []byte("hello world"))
			},
			path:     "file.txt",
			offset:   0,
			wantData: []byte("hello world"),
			wantErr:  false,
		},
		{
			name: "offset_reads_tail",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "file.txt", []byte("hello world"))
			},
			path:     "file.txt",
			offset:   6,
			wantData: []byte("world"),
			wantErr:  false,
		},
		{
			name: "offset_at_end_returns_empty",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "file.txt", []byte("hello"))
			},
			path:     "file.txt",
			offset:   5,
			wantData: []byte{},
			wantErr:  false,
		},
		{
			name: "offset_beyond_end_returns_empty",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "file.txt", []byte("hello"))
			},
			path:     "file.txt",
			offset:   100,
			wantData: []byte{},
			wantErr:  false,
		},
		{
			name:        "read_non_existent_file",
			setup:       func(_ *InMemoryFileManager) {},
			path:        "nonexistent.txt",
			offset:      0,
			wantErr:     true,
			errContains: "file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := NewInMemoryFileManager()
			tt.setup(fm)
			ctx := context.Background()

			reader, err := fm.ReadStreamAt(ctx, tt.path, tt.offset)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}

				return
			}

			require.NoError(t, err)
			data, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			require.NoError(t, reader.Close())
			assert.Equal(t, tt.wantData, data)
		})
	}
}

func TestInMemoryFileManager_WriteStream(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setup       func(fm *InMemoryFileManager)
		path        string
		reader      io.Reader
		wantData    []byte
		wantErr     bool
		errContains string
	}{
		{
			name:     "write_new_file",
			setup:    func(_ *InMemoryFileManager) {},
			path:     "streamed.txt",
			reader:   strings.NewReader("streamed content"),
			wantData: []byte("streamed content"),
			wantErr:  false,
		},
		{
			name: "overwrite_existing_file",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "existing.txt", []byte("old content"))
			},
			path:     "existing.txt",
			reader:   strings.NewReader("new content"),
			wantData: []byte("new content"),
			wantErr:  false,
		},
		{
			name:        "broken_reader_returns_error",
			setup:       func(_ *InMemoryFileManager) {},
			path:        "broken.txt",
			reader:      iotest.ErrReader(errBrokenReader),
			wantErr:     true,
			errContains: "failed to read stream data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := NewInMemoryFileManager()
			tt.setup(fm)
			ctx := context.Background()

			err := fm.WriteStream(ctx, tt.path, tt.reader)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.False(t, fm.Exists(ctx, tt.path), "a broken stream must not create the file")

				return
			}

			require.NoError(t, err)
			data, readErr := fm.Read(ctx, tt.path)
			require.NoError(t, readErr)
			assert.Equal(t, tt.wantData, data)
		})
	}
}

func TestInMemoryFileManager_DeleteByPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		setup        func(fm *InMemoryFileManager)
		prefix       string
		wantExisting []string
		wantDeleted  []string
	}{
		{
			name: "deletes_only_matching_prefix",
			setup: func(fm *InMemoryFileManager) {
				ctx := context.Background()
				_ = fm.Write(ctx, "dir/file1.txt", []byte("1"))
				_ = fm.Write(ctx, "dir/file2.txt", []byte("2"))
				_ = fm.Write(ctx, "other/file3.txt", []byte("3"))
			},
			prefix:       "dir/",
			wantExisting: []string{"other/file3.txt"},
			wantDeleted:  []string{"dir/file1.txt", "dir/file2.txt"},
		},
		{
			name: "non_matching_prefix_deletes_nothing",
			setup: func(fm *InMemoryFileManager) {
				_ = fm.Write(context.Background(), "file.txt", []byte("content"))
			},
			prefix:       "missing/",
			wantExisting: []string{"file.txt"},
			wantDeleted:  nil,
		},
		{
			name: "empty_prefix_deletes_everything",
			setup: func(fm *InMemoryFileManager) {
				ctx := context.Background()
				_ = fm.Write(ctx, "a.txt", []byte("a"))
				_ = fm.Write(ctx, "b.txt", []byte("b"))
			},
			prefix:       "",
			wantExisting: nil,
			wantDeleted:  []string{"a.txt", "b.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fm := NewInMemoryFileManager()
			tt.setup(fm)
			ctx := context.Background()

			err := fm.DeleteByPrefix(ctx, tt.prefix)

			require.NoError(t, err)
			for _, path := range tt.wantExisting {
				assert.True(t, fm.Exists(ctx, path), "file %s must survive DeleteByPrefix(%q)", path, tt.prefix)
			}
			for _, path := range tt.wantDeleted {
				assert.False(t, fm.Exists(ctx, path), "file %s must be deleted by DeleteByPrefix(%q)", path, tt.prefix)
			}
		})
	}
}
