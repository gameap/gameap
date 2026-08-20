package files

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCachingFileManager(t *testing.T) {
	t.Parallel()
	t.Run("creates_with_inner_and_cache", func(t *testing.T) {
		t.Parallel()
		inner := &MockFileManager{}
		cache := &MockFileManager{}

		cfm := NewCachingFileManager(inner, cache)

		require.NotNil(t, cfm)
		assert.Equal(t, inner, cfm.inner)
		assert.Equal(t, cache, cfm.cache)
	})
}

func TestCachingFileManager_Read(t *testing.T) {
	t.Parallel()
	t.Run("cache_miss_reads_from_inner_and_caches", func(t *testing.T) {
		t.Parallel()
		expectedData := []byte("data from inner")
		innerReadCalls := 0
		cacheWriteCalls := 0

		inner := &MockFileManager{
			ReadFunc: func(_ context.Context, path string) ([]byte, error) {
				innerReadCalls++
				assert.Equal(t, "test/file.wasm", path)

				return expectedData, nil
			},
		}
		cache := &MockFileManager{
			ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
				return nil, errors.New("not found")
			},
			WriteFunc: func(_ context.Context, _ string, data []byte) error {
				cacheWriteCalls++
				assert.Equal(t, expectedData, data)

				return nil
			},
		}
		cfm := NewCachingFileManager(inner, cache)

		data, err := cfm.Read(context.Background(), "test/file.wasm")

		require.NoError(t, err)
		assert.Equal(t, expectedData, data)
		assert.Equal(t, 1, innerReadCalls)
		assert.Equal(t, 1, cacheWriteCalls)
	})

	t.Run("cache_hit_returns_cached_data", func(t *testing.T) {
		t.Parallel()
		cachedData := []byte("cached data")
		innerReadCalls := 0

		inner := &MockFileManager{
			ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
				innerReadCalls++

				return []byte("inner data"), nil
			},
		}
		cache := &MockFileManager{
			ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
				return cachedData, nil
			},
		}
		cfm := NewCachingFileManager(inner, cache)

		data, err := cfm.Read(context.Background(), "test/file.wasm")

		require.NoError(t, err)
		assert.Equal(t, cachedData, data)
		assert.Equal(t, 0, innerReadCalls)
	})

	t.Run("inner_error_is_propagated", func(t *testing.T) {
		t.Parallel()
		innerErr := errors.New("inner read error")
		inner := &MockFileManager{
			ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
				return nil, innerErr
			},
		}
		cache := &MockFileManager{
			ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
				return nil, errors.New("not found")
			},
		}
		cfm := NewCachingFileManager(inner, cache)

		data, err := cfm.Read(context.Background(), "test/file.wasm")

		require.Error(t, err)
		assert.Equal(t, innerErr, err)
		assert.Nil(t, data)
	})

	t.Run("cache_write_error_does_not_affect_read", func(t *testing.T) {
		t.Parallel()
		expectedData := []byte("data from inner")
		inner := &MockFileManager{
			ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
				return expectedData, nil
			},
		}
		cache := &MockFileManager{
			ReadFunc: func(_ context.Context, _ string) ([]byte, error) {
				return nil, errors.New("not found")
			},
			WriteFunc: func(_ context.Context, _ string, _ []byte) error {
				return errors.New("cache write error")
			},
		}
		cfm := NewCachingFileManager(inner, cache)

		data, err := cfm.Read(context.Background(), "test/file.wasm")

		require.NoError(t, err)
		assert.Equal(t, expectedData, data)
	})
}

func TestCachingFileManager_Write(t *testing.T) {
	t.Parallel()
	t.Run("writes_to_inner_and_updates_cache", func(t *testing.T) {
		t.Parallel()
		writtenData := []byte("new data")
		innerWriteCalls := 0
		cacheDeleteCalls := 0
		cacheWriteCalls := 0

		inner := &MockFileManager{
			WriteFunc: func(_ context.Context, path string, data []byte) error {
				innerWriteCalls++
				assert.Equal(t, "test/file.wasm", path)
				assert.Equal(t, writtenData, data)

				return nil
			},
		}
		cache := &MockFileManager{
			DeleteFunc: func(_ context.Context, _ string) error {
				cacheDeleteCalls++

				return nil
			},
			WriteFunc: func(_ context.Context, _ string, data []byte) error {
				cacheWriteCalls++
				assert.Equal(t, writtenData, data)

				return nil
			},
		}
		cfm := NewCachingFileManager(inner, cache)

		err := cfm.Write(context.Background(), "test/file.wasm", writtenData)

		require.NoError(t, err)
		assert.Equal(t, 1, innerWriteCalls)
		assert.Equal(t, 1, cacheDeleteCalls)
		assert.Equal(t, 1, cacheWriteCalls)
	})

	t.Run("inner_error_prevents_cache_update", func(t *testing.T) {
		t.Parallel()
		innerErr := errors.New("inner write error")
		cacheWriteCalls := 0

		inner := &MockFileManager{
			WriteFunc: func(_ context.Context, _ string, _ []byte) error {
				return innerErr
			},
		}
		cache := &MockFileManager{
			WriteFunc: func(_ context.Context, _ string, _ []byte) error {
				cacheWriteCalls++

				return nil
			},
		}
		cfm := NewCachingFileManager(inner, cache)

		err := cfm.Write(context.Background(), "test/file.wasm", []byte("data"))

		require.Error(t, err)
		assert.Equal(t, innerErr, err)
		assert.Equal(t, 0, cacheWriteCalls)
	})
}

func TestCachingFileManager_Delete(t *testing.T) {
	t.Parallel()
	t.Run("deletes_from_inner_and_cache", func(t *testing.T) {
		t.Parallel()
		innerDeleteCalls := 0
		cacheDeleteCalls := 0

		inner := &MockFileManager{
			DeleteFunc: func(_ context.Context, path string) error {
				innerDeleteCalls++
				assert.Equal(t, "test/file.wasm", path)

				return nil
			},
		}
		cache := &MockFileManager{
			DeleteFunc: func(_ context.Context, _ string) error {
				cacheDeleteCalls++

				return nil
			},
		}
		cfm := NewCachingFileManager(inner, cache)

		err := cfm.Delete(context.Background(), "test/file.wasm")

		require.NoError(t, err)
		assert.Equal(t, 1, innerDeleteCalls)
		assert.Equal(t, 1, cacheDeleteCalls)
	})

	t.Run("inner_error_prevents_cache_deletion", func(t *testing.T) {
		t.Parallel()
		innerErr := errors.New("inner delete error")
		cacheDeleteCalls := 0

		inner := &MockFileManager{
			DeleteFunc: func(_ context.Context, _ string) error {
				return innerErr
			},
		}
		cache := &MockFileManager{
			DeleteFunc: func(_ context.Context, _ string) error {
				cacheDeleteCalls++

				return nil
			},
		}
		cfm := NewCachingFileManager(inner, cache)

		err := cfm.Delete(context.Background(), "test/file.wasm")

		require.Error(t, err)
		assert.Equal(t, innerErr, err)
		assert.Equal(t, 0, cacheDeleteCalls)
	})

	t.Run("cache_delete_error_is_ignored", func(t *testing.T) {
		t.Parallel()
		innerDeleteCalls := 0

		inner := &MockFileManager{
			DeleteFunc: func(_ context.Context, _ string) error {
				innerDeleteCalls++

				return nil
			},
		}
		cache := &MockFileManager{
			DeleteFunc: func(_ context.Context, _ string) error {
				return errors.New("cache delete error")
			},
		}
		cfm := NewCachingFileManager(inner, cache)

		err := cfm.Delete(context.Background(), "test/file.wasm")

		require.NoError(t, err)
		assert.Equal(t, 1, innerDeleteCalls)
	})
}

func TestCachingFileManager_Exists(t *testing.T) {
	t.Parallel()
	t.Run("delegates_to_inner", func(t *testing.T) {
		t.Parallel()
		existsCalls := 0
		inner := &MockFileManager{
			ExistsFunc: func(_ context.Context, path string) bool {
				existsCalls++
				assert.Equal(t, "test/file.wasm", path)

				return true
			},
		}
		cache := &MockFileManager{}
		cfm := NewCachingFileManager(inner, cache)

		exists := cfm.Exists(context.Background(), "test/file.wasm")

		assert.True(t, exists)
		assert.Equal(t, 1, existsCalls)
	})
}

func TestCachingFileManager_List(t *testing.T) {
	t.Parallel()
	t.Run("delegates_to_inner", func(t *testing.T) {
		t.Parallel()
		expectedFiles := []string{"file1.wasm", "file2.wasm"}
		listCalls := 0
		inner := &MockFileManager{
			ListFunc: func(_ context.Context, dir string) ([]string, error) {
				listCalls++
				assert.Equal(t, "plugins/", dir)

				return expectedFiles, nil
			},
		}
		cache := &MockFileManager{}
		cfm := NewCachingFileManager(inner, cache)

		files, err := cfm.List(context.Background(), "plugins/")

		require.NoError(t, err)
		assert.Equal(t, expectedFiles, files)
		assert.Equal(t, 1, listCalls)
	})
}

func TestCachingFileManager_cachePath(t *testing.T) {
	t.Parallel()
	t.Run("same_path_produces_same_hash", func(t *testing.T) {
		t.Parallel()
		inner := &MockFileManager{}
		cache := &MockFileManager{}
		cfm := NewCachingFileManager(inner, cache)

		path1 := cfm.cachePath("test/file.wasm")
		path2 := cfm.cachePath("test/file.wasm")

		assert.Equal(t, path1, path2)
	})

	t.Run("different_paths_produce_different_hashes", func(t *testing.T) {
		t.Parallel()
		inner := &MockFileManager{}
		cache := &MockFileManager{}
		cfm := NewCachingFileManager(inner, cache)

		path1 := cfm.cachePath("test/file1.wasm")
		path2 := cfm.cachePath("test/file2.wasm")

		assert.NotEqual(t, path1, path2)
	})
}

func TestCachingFileManager_ImplementsInterface(t *testing.T) {
	t.Parallel()
	var _ FileManager = (*CachingFileManager)(nil)
}
