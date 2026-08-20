package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/storage"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageService_Get(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		pluginID    uint64
		setupRepo   func(*inmemory.PluginStorageRepository)
		request     *storage.StorageGetRequest
		wantFound   bool
		wantPayload []byte
	}{
		{
			name:     "existing_entry_returns_found",
			pluginID: 1,
			setupRepo: func(r *inmemory.PluginStorageRepository) {
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{
					PluginID: 1,
					Key:      "testkey",
					Payload:  []byte("testpayload"),
				})
			},
			request: &storage.StorageGetRequest{
				Key: "testkey",
			},
			wantFound:   true,
			wantPayload: []byte("testpayload"),
		},
		{
			name:      "missing_entry_returns_not_found",
			pluginID:  1,
			setupRepo: func(_ *inmemory.PluginStorageRepository) {},
			request: &storage.StorageGetRequest{
				Key: "nonexistent",
			},
			wantFound: false,
		},
		{
			name:     "different_plugin_returns_not_found",
			pluginID: 1,
			setupRepo: func(r *inmemory.PluginStorageRepository) {
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{
					PluginID: 2,
					Key:      "testkey",
					Payload:  []byte("otherplugin"),
				})
			},
			request: &storage.StorageGetRequest{
				Key: "testkey",
			},
			wantFound: false,
		},
		{
			name:     "entity_type_filter_applied",
			pluginID: 1,
			setupRepo: func(r *inmemory.PluginStorageRepository) {
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{
					PluginID:   1,
					Key:        "config",
					EntityType: lo.ToPtr(string(domain.EntityTypeUser)),
					EntityID:   new(uint(42)),
					Payload:    []byte("userconfig"),
				})
			},
			request: &storage.StorageGetRequest{
				Key:        "config",
				EntityType: lo.ToPtr(proto.EntityType_ENTITY_TYPE_USER),
				EntityId:   new(uint64(42)),
			},
			wantFound:   true,
			wantPayload: []byte("userconfig"),
		},
		{
			name:     "entity_type_mismatch_returns_not_found",
			pluginID: 1,
			setupRepo: func(r *inmemory.PluginStorageRepository) {
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{
					PluginID:   1,
					Key:        "config",
					EntityType: lo.ToPtr(string(domain.EntityTypeUser)),
					EntityID:   new(uint(42)),
					Payload:    []byte("userconfig"),
				})
			},
			request: &storage.StorageGetRequest{
				Key:        "config",
				EntityType: lo.ToPtr(proto.EntityType_ENTITY_TYPE_SERVER),
				EntityId:   new(uint64(42)),
			},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewPluginStorageRepository()
			tt.setupRepo(repo)

			svc := NewStorageService(tt.pluginID, repo)
			resp, err := svc.Get(context.Background(), tt.request)

			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, resp.Found)

			if tt.wantFound {
				assert.Equal(t, tt.wantPayload, resp.Payload)
			}
		})
	}
}

func TestStorageService_Set(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		pluginID  uint64
		request   *storage.StorageSetRequest
		wantError string
	}{
		{
			name:     "new_entry_created",
			pluginID: 1,
			request: &storage.StorageSetRequest{
				Key:     "newkey",
				Payload: []byte("newpayload"),
			},
		},
		{
			name:     "entry_with_entity_type",
			pluginID: 1,
			request: &storage.StorageSetRequest{
				Key:        "entitykey",
				EntityType: lo.ToPtr(proto.EntityType_ENTITY_TYPE_SERVER),
				EntityId:   new(uint64(100)),
				Payload:    []byte("serverpayload"),
			},
		},
		{
			name:     "empty_payload",
			pluginID: 1,
			request: &storage.StorageSetRequest{
				Key:     "emptykey",
				Payload: []byte{},
			},
		},
		{
			name:     "large_payload",
			pluginID: 1,
			request: &storage.StorageSetRequest{
				Key:     "largekey",
				Payload: make([]byte, 10000),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewPluginStorageRepository()
			svc := NewStorageService(tt.pluginID, repo)

			resp, err := svc.Set(context.Background(), tt.request)

			require.NoError(t, err)

			if tt.wantError != "" {
				assert.False(t, resp.Success)
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)

				return
			}

			assert.True(t, resp.Success)
			assert.Nil(t, resp.Error)

			getResp, err := svc.Get(context.Background(), &storage.StorageGetRequest{
				Key:        tt.request.Key,
				EntityType: tt.request.EntityType,
				EntityId:   tt.request.EntityId,
			})
			require.NoError(t, err)
			assert.True(t, getResp.Found)
			assert.Equal(t, tt.request.Payload, getResp.Payload)
		})
	}
}

func TestStorageService_Set_ExistingEntryUpdated(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewPluginStorageRepository()
	svc := NewStorageService(1, repo)

	_, err := svc.Set(context.Background(), &storage.StorageSetRequest{
		Key:     "updatekey",
		Payload: []byte("original"),
	})
	require.NoError(t, err)

	_, err = svc.Set(context.Background(), &storage.StorageSetRequest{
		Key:     "updatekey",
		Payload: []byte("updated"),
	})
	require.NoError(t, err)

	getResp, err := svc.Get(context.Background(), &storage.StorageGetRequest{Key: "updatekey"})
	require.NoError(t, err)
	assert.True(t, getResp.Found)
	assert.Equal(t, []byte("updated"), getResp.Payload)
}

func TestStorageService_Delete(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		pluginID  uint64
		setupRepo func(*inmemory.PluginStorageRepository)
		request   *storage.StorageDeleteRequest
	}{
		{
			name:     "entry_deleted",
			pluginID: 1,
			setupRepo: func(r *inmemory.PluginStorageRepository) {
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{
					PluginID: 1,
					Key:      "todelete",
					Payload:  []byte("data"),
				})
			},
			request: &storage.StorageDeleteRequest{
				Key: "todelete",
			},
		},
		{
			name:      "nonexistent_key_no_error",
			pluginID:  1,
			setupRepo: func(_ *inmemory.PluginStorageRepository) {},
			request: &storage.StorageDeleteRequest{
				Key: "nonexistent",
			},
		},
		{
			name:     "delete_with_entity_filter",
			pluginID: 1,
			setupRepo: func(r *inmemory.PluginStorageRepository) {
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{
					PluginID:   1,
					Key:        "entitykey",
					EntityType: lo.ToPtr(string(domain.EntityTypeServer)),
					EntityID:   new(uint(50)),
					Payload:    []byte("data"),
				})
			},
			request: &storage.StorageDeleteRequest{
				Key:        "entitykey",
				EntityType: lo.ToPtr(proto.EntityType_ENTITY_TYPE_SERVER),
				EntityId:   new(uint64(50)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewPluginStorageRepository()
			tt.setupRepo(repo)

			svc := NewStorageService(tt.pluginID, repo)
			resp, err := svc.Delete(context.Background(), tt.request)

			require.NoError(t, err)
			assert.True(t, resp.Success)

			getResp, err := svc.Get(context.Background(), &storage.StorageGetRequest{
				Key:        tt.request.Key,
				EntityType: tt.request.EntityType,
				EntityId:   tt.request.EntityId,
			})
			require.NoError(t, err)
			assert.False(t, getResp.Found)
		})
	}
}

func TestStorageService_List(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		pluginID  uint64
		setupRepo func(*inmemory.PluginStorageRepository)
		request   *storage.StorageListRequest
		wantCount int
		wantKeys  []string
	}{
		{
			name:     "returns_all_plugin_entries",
			pluginID: 1,
			setupRepo: func(r *inmemory.PluginStorageRepository) {
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{PluginID: 1, Key: "key1", Payload: []byte("1")})
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{PluginID: 1, Key: "key2", Payload: []byte("2")})
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{PluginID: 2, Key: "other", Payload: []byte("3")})
			},
			request:   &storage.StorageListRequest{},
			wantCount: 2,
			wantKeys:  []string{"key1", "key2"},
		},
		{
			name:     "filter_by_entity_type",
			pluginID: 1,
			setupRepo: func(r *inmemory.PluginStorageRepository) {
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{
					PluginID:   1,
					Key:        "userkey",
					EntityType: lo.ToPtr(string(domain.EntityTypeUser)),
					EntityID:   new(uint(1)),
					Payload:    []byte("1"),
				})
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{
					PluginID:   1,
					Key:        "serverkey",
					EntityType: lo.ToPtr(string(domain.EntityTypeServer)),
					EntityID:   new(uint(2)),
					Payload:    []byte("2"),
				})
			},
			request: &storage.StorageListRequest{
				EntityType: lo.ToPtr(proto.EntityType_ENTITY_TYPE_USER),
				EntityId:   new(uint64(1)),
			},
			wantCount: 1,
			wantKeys:  []string{"userkey"},
		},
		{
			name:     "filter_by_key_prefix",
			pluginID: 1,
			setupRepo: func(r *inmemory.PluginStorageRepository) {
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{PluginID: 1, Key: "config:app", Payload: []byte("1")})
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{PluginID: 1, Key: "config:user", Payload: []byte("2")})
				_ = r.Save(context.Background(), &domain.PluginStorageEntry{PluginID: 1, Key: "data:other", Payload: []byte("3")})
			},
			request: &storage.StorageListRequest{
				KeyPrefix: new("config:"),
			},
			wantCount: 2,
			wantKeys:  []string{"config:app", "config:user"},
		},
		{
			name:      "empty_plugin_returns_empty",
			pluginID:  99,
			setupRepo: func(_ *inmemory.PluginStorageRepository) {},
			request:   &storage.StorageListRequest{},
			wantCount: 0,
			wantKeys:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewPluginStorageRepository()
			tt.setupRepo(repo)

			svc := NewStorageService(tt.pluginID, repo)
			resp, err := svc.List(context.Background(), tt.request)

			require.NoError(t, err)
			require.Len(t, resp.Entries, tt.wantCount)

			actualKeys := make([]string, len(resp.Entries))
			for i, entry := range resp.Entries {
				actualKeys[i] = entry.Key
			}

			for _, wantKey := range tt.wantKeys {
				assert.Contains(t, actualKeys, wantKey)
			}
		})
	}
}

func TestStorageHostLibraryFactory_Create(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewPluginStorageRepository()
	factory := NewStorageHostLibraryFactory(repo)

	lib := factory.Create(42)

	assert.NotNil(t, lib)
	storageLib, ok := lib.(*StorageHostLibrary)
	require.True(t, ok)
	assert.NotNil(t, storageLib.impl)
}

func TestNewStorageHostLibrary(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewPluginStorageRepository()
	lib := NewStorageHostLibrary(1, repo)

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
