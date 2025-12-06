package files

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errMockRead   = errors.New("mock error")
	errMockWrite  = errors.New("mock write error")
	errMockDelete = errors.New("mock delete error")
	errMockList   = errors.New("mock list error")
)

func TestMockFileManager_Read(t *testing.T) {
	ctx := context.Background()

	t.Run("calls_custom_func_when_set", func(t *testing.T) {
		expectedData := []byte("mock data")
		mock := &MockFileManager{
			ReadFunc: func(_ context.Context, path string) ([]byte, error) {
				assert.Equal(t, "test.txt", path)

				return expectedData, errMockRead
			},
		}

		data, err := mock.Read(ctx, "test.txt")

		assert.Equal(t, expectedData, data)
		assert.Equal(t, errMockRead, err)
	})

	t.Run("returns_nil_when_func_not_set", func(t *testing.T) {
		mock := &MockFileManager{}

		data, err := mock.Read(ctx, "test.txt")

		assert.Nil(t, data)
		assert.NoError(t, err)
	})
}

func TestMockFileManager_Write(t *testing.T) {
	ctx := context.Background()

	t.Run("calls_custom_func_when_set", func(t *testing.T) {
		var capturedPath string
		var capturedData []byte
		mock := &MockFileManager{
			WriteFunc: func(_ context.Context, path string, data []byte) error {
				capturedPath = path
				capturedData = data

				return errMockWrite
			},
		}

		err := mock.Write(ctx, "output.txt", []byte("content"))

		assert.Equal(t, errMockWrite, err)
		assert.Equal(t, "output.txt", capturedPath)
		assert.Equal(t, []byte("content"), capturedData)
	})

	t.Run("returns_nil_when_func_not_set", func(t *testing.T) {
		mock := &MockFileManager{}

		err := mock.Write(ctx, "test.txt", []byte("data"))

		assert.NoError(t, err)
	})
}

func TestMockFileManager_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("calls_custom_func_when_set", func(t *testing.T) {
		var capturedPath string
		mock := &MockFileManager{
			DeleteFunc: func(_ context.Context, path string) error {
				capturedPath = path

				return errMockDelete
			},
		}

		err := mock.Delete(ctx, "to_delete.txt")

		assert.Equal(t, errMockDelete, err)
		assert.Equal(t, "to_delete.txt", capturedPath)
	})

	t.Run("returns_nil_when_func_not_set", func(t *testing.T) {
		mock := &MockFileManager{}

		err := mock.Delete(ctx, "test.txt")

		assert.NoError(t, err)
	})
}

func TestMockFileManager_Exists(t *testing.T) {
	ctx := context.Background()

	t.Run("calls_custom_func_when_set_returns_true", func(t *testing.T) {
		var capturedPath string
		mock := &MockFileManager{
			ExistsFunc: func(_ context.Context, path string) bool {
				capturedPath = path

				return true
			},
		}

		exists := mock.Exists(ctx, "exists.txt")

		assert.True(t, exists)
		assert.Equal(t, "exists.txt", capturedPath)
	})

	t.Run("calls_custom_func_when_set_returns_false", func(t *testing.T) {
		mock := &MockFileManager{
			ExistsFunc: func(_ context.Context, _ string) bool {
				return false
			},
		}

		exists := mock.Exists(ctx, "test.txt")

		assert.False(t, exists)
	})

	t.Run("returns_false_when_func_not_set", func(t *testing.T) {
		mock := &MockFileManager{}

		exists := mock.Exists(ctx, "test.txt")

		assert.False(t, exists)
	})
}

func TestMockFileManager_List(t *testing.T) {
	ctx := context.Background()

	t.Run("calls_custom_func_when_set", func(t *testing.T) {
		expectedFiles := []string{"file1.txt", "file2.txt"}
		var capturedDir string
		mock := &MockFileManager{
			ListFunc: func(_ context.Context, dir string) ([]string, error) {
				capturedDir = dir

				return expectedFiles, errMockList
			},
		}

		files, err := mock.List(ctx, "my_dir/")

		assert.Equal(t, expectedFiles, files)
		assert.Equal(t, errMockList, err)
		assert.Equal(t, "my_dir/", capturedDir)
	})

	t.Run("returns_nil_when_func_not_set", func(t *testing.T) {
		mock := &MockFileManager{}

		files, err := mock.List(ctx, "test/")

		assert.Nil(t, files)
		assert.NoError(t, err)
	})
}

func TestMockFileManager_ImplementsInterface(_ *testing.T) {
	var _ FileManager = (*MockFileManager)(nil)
}

func TestMockFileManager_MultipleMethodsConfigured(t *testing.T) {
	ctx := context.Background()

	mock := &MockFileManager{
		ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte("read data"), nil
		},
		WriteFunc: func(_ context.Context, _ string, _ []byte) error {
			return nil
		},
		ExistsFunc: func(_ context.Context, _ string) bool {
			return true
		},
	}

	data, err := mock.Read(ctx, "file.txt")
	require.NoError(t, err)
	assert.Equal(t, []byte("read data"), data)

	err = mock.Write(ctx, "file.txt", []byte("new data"))
	require.NoError(t, err)

	exists := mock.Exists(ctx, "file.txt")
	assert.True(t, exists)

	err = mock.Delete(ctx, "file.txt")
	require.NoError(t, err)

	files, err := mock.List(ctx, "dir/")
	require.NoError(t, err)
	assert.Nil(t, files)
}
