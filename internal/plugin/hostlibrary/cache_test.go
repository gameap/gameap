package hostlibrary

import (
	"context"
	"testing"

	intcache "github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/pkg/plugin/sdk/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheService_Get(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		setupCache func(*intcache.InMemory, string)
		key        string
		keyPrefix  string
		wantFound  bool
		wantValue  []byte
	}{
		{
			name: "existing_key_returns_value",
			setupCache: func(c *intcache.InMemory, prefix string) {
				_ = c.Set(context.Background(), prefix+"testkey", []byte("testvalue"))
			},
			key:       "testkey",
			keyPrefix: "plugin1:",
			wantFound: true,
			wantValue: []byte("testvalue"),
		},
		{
			name:       "missing_key_returns_not_found",
			setupCache: func(_ *intcache.InMemory, _ string) {},
			key:        "nonexistent",
			keyPrefix:  "plugin1:",
			wantFound:  false,
			wantValue:  nil,
		},
		{
			name: "non_bytes_value_returns_not_found",
			setupCache: func(c *intcache.InMemory, prefix string) {
				_ = c.Set(context.Background(), prefix+"stringkey", "not bytes")
			},
			key:       "stringkey",
			keyPrefix: "plugin1:",
			wantFound: false,
			wantValue: nil,
		},
		{
			name: "key_prefix_applied_correctly",
			setupCache: func(c *intcache.InMemory, _ string) {
				_ = c.Set(context.Background(), "other:mykey", []byte("wrongvalue"))
				_ = c.Set(context.Background(), "plugin2:mykey", []byte("correctvalue"))
			},
			key:       "mykey",
			keyPrefix: "plugin2:",
			wantFound: true,
			wantValue: []byte("correctvalue"),
		},
		{
			name:       "empty_key",
			setupCache: func(_ *intcache.InMemory, _ string) {},
			key:        "",
			keyPrefix:  "plugin:",
			wantFound:  false,
			wantValue:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := intcache.NewInMemory()
			tt.setupCache(c, tt.keyPrefix)

			svc := NewCacheService(c, tt.keyPrefix)
			resp, err := svc.Get(context.Background(), &cache.CacheGetRequest{Key: tt.key})

			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, resp.Found)
			assert.Equal(t, tt.wantValue, resp.Value)
		})
	}
}

func TestCacheService_Set(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		key        string
		value      []byte
		ttlSeconds int64
		keyPrefix  string
		wantError  string
	}{
		{
			name:       "set_without_ttl_success",
			key:        "mykey",
			value:      []byte("myvalue"),
			ttlSeconds: 0,
			keyPrefix:  "plugin:",
		},
		{
			name:       "set_with_ttl_success",
			key:        "mykey",
			value:      []byte("myvalue"),
			ttlSeconds: 3600,
			keyPrefix:  "plugin:",
		},
		{
			name:       "set_empty_key",
			key:        "",
			value:      []byte("value"),
			ttlSeconds: 0,
			keyPrefix:  "plugin:",
		},
		{
			name:       "set_empty_value",
			key:        "key",
			value:      []byte{},
			ttlSeconds: 0,
			keyPrefix:  "plugin:",
		},
		{
			name:       "set_large_value",
			key:        "largekey",
			value:      make([]byte, 10000),
			ttlSeconds: 0,
			keyPrefix:  "plugin:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := intcache.NewInMemory()
			svc := NewCacheService(c, tt.keyPrefix)

			resp, err := svc.Set(context.Background(), &cache.CacheSetRequest{
				Key:        tt.key,
				Value:      tt.value,
				TtlSeconds: tt.ttlSeconds,
			})

			require.NoError(t, err)

			if tt.wantError != "" {
				assert.False(t, resp.Success)
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.True(t, resp.Success)
			assert.Nil(t, resp.Error)

			getResp, err := svc.Get(context.Background(), &cache.CacheGetRequest{Key: tt.key})
			require.NoError(t, err)
			assert.True(t, getResp.Found)
			assert.Equal(t, tt.value, getResp.Value)
		})
	}
}

func TestCacheService_Delete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		setupCache func(*intcache.InMemory, string)
		key        string
		keyPrefix  string
	}{
		{
			name: "delete_existing_key_success",
			setupCache: func(c *intcache.InMemory, prefix string) {
				_ = c.Set(context.Background(), prefix+"todelete", []byte("value"))
			},
			key:       "todelete",
			keyPrefix: "plugin:",
		},
		{
			name:       "delete_missing_key_still_success",
			setupCache: func(_ *intcache.InMemory, _ string) {},
			key:        "nonexistent",
			keyPrefix:  "plugin:",
		},
		{
			name:       "delete_empty_key",
			setupCache: func(_ *intcache.InMemory, _ string) {},
			key:        "",
			keyPrefix:  "plugin:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := intcache.NewInMemory()
			tt.setupCache(c, tt.keyPrefix)

			svc := NewCacheService(c, tt.keyPrefix)
			resp, err := svc.Delete(context.Background(), &cache.CacheDeleteRequest{Key: tt.key})

			require.NoError(t, err)
			assert.True(t, resp.Success)

			getResp, err := svc.Get(context.Background(), &cache.CacheGetRequest{Key: tt.key})
			require.NoError(t, err)
			assert.False(t, getResp.Found)
		})
	}
}

func TestCacheService_KeyPrefixIsolation(t *testing.T) {
	t.Parallel()
	c := intcache.NewInMemory()
	svc1 := NewCacheService(c, "plugin1:")
	svc2 := NewCacheService(c, "plugin2:")

	_, err := svc1.Set(context.Background(), &cache.CacheSetRequest{
		Key:   "sharedkey",
		Value: []byte("value1"),
	})
	require.NoError(t, err)

	_, err = svc2.Set(context.Background(), &cache.CacheSetRequest{
		Key:   "sharedkey",
		Value: []byte("value2"),
	})
	require.NoError(t, err)

	resp1, err := svc1.Get(context.Background(), &cache.CacheGetRequest{Key: "sharedkey"})
	require.NoError(t, err)
	assert.True(t, resp1.Found)
	assert.Equal(t, []byte("value1"), resp1.Value)

	resp2, err := svc2.Get(context.Background(), &cache.CacheGetRequest{Key: "sharedkey"})
	require.NoError(t, err)
	assert.True(t, resp2.Found)
	assert.Equal(t, []byte("value2"), resp2.Value)
}

func TestNewCacheHostLibrary(t *testing.T) {
	t.Parallel()
	c := intcache.NewInMemory()
	lib := NewCacheHostLibrary(c, "test:")

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
