package files

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryFileManager_Read(t *testing.T) {
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
			fm := NewInMemoryFileManager()
			tt.setup(fm)
			ctx := context.Background()

			exists := fm.Exists(ctx, tt.path)

			assert.Equal(t, tt.exists, exists)
		})
	}
}

func TestInMemoryFileManager_List(t *testing.T) {
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

func TestInMemoryFileManager_Concurrency(_ *testing.T) {
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
