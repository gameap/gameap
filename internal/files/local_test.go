package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLocalFileManager(t *testing.T) {
	t.Run("creates_with_valid_path", func(t *testing.T) {
		tempDir := t.TempDir()

		fm := NewLocalFileManager(tempDir)

		require.NotNil(t, fm)
		require.NotNil(t, fm.root)
	})

	t.Run("panics_with_invalid_path", func(t *testing.T) {
		assert.Panics(t, func() {
			NewLocalFileManager("/nonexistent/path/that/does/not/exist")
		})
	})
}

func TestLocalFileManager_Read(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, tempDir string)
		path        string
		wantData    []byte
		wantErr     bool
		errContains string
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
			wantErr:  false,
		},
		{
			name:        "read_non_existent_file",
			setup:       func(_ *testing.T, _ string) {},
			path:        "nonexistent.txt",
			wantData:    nil,
			wantErr:     true,
			errContains: "failed to read file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
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

func TestLocalFileManager_Write(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, tempDir string)
		path    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "write_new_file",
			setup:   func(_ *testing.T, _ string) {},
			path:    "new_file.txt",
			data:    []byte("new content"),
			wantErr: false,
		},
		{
			name: "write_creates_directories",
			setup: func(_ *testing.T, _ string) {
			},
			path:    "subdir/nested/file.txt",
			data:    []byte("nested content"),
			wantErr: false,
		},
		{
			name: "overwrite_existing_file",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "existing.txt"), []byte("old content"), 0644)
				require.NoError(t, err)
			},
			path:    "existing.txt",
			data:    []byte("new content"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
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

func TestLocalFileManager_Delete(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, tempDir string)
		path    string
		wantErr bool
	}{
		{
			name: "delete_existing_file",
			setup: func(t *testing.T, tempDir string) {
				t.Helper()
				err := os.WriteFile(filepath.Join(tempDir, "to_delete.txt"), []byte("content"), 0644)
				require.NoError(t, err)
			},
			path:    "to_delete.txt",
			wantErr: false,
		},
		{
			name:    "delete_non_existent_file_returns_error",
			setup:   func(_ *testing.T, _ string) {},
			path:    "nonexistent.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
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

func TestLocalFileManager_Exists(t *testing.T) {
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
	tests := []struct {
		name      string
		setup     func(t *testing.T, tempDir string)
		dir       string
		wantFiles []string
		wantErr   bool
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
			wantErr:   false,
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
			wantErr:   false,
		},
		{
			name:      "list_non_existent_directory_returns_error",
			setup:     func(_ *testing.T, _ string) {},
			dir:       "nonexistent",
			wantFiles: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tt.setup(t, tempDir)
			fm := NewLocalFileManager(tempDir)
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
