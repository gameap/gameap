// Quota tests for gameap-storage: OWASP API4:2023 Unrestricted Resource
// Consumption — one plugin cannot grow the panel database without bound.
package hostlibrary

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errQuotaTestConnection = errors.New("connection reset")

func newQuotaStorageService(cfg StorageConfig) *StorageServiceImpl {
	return NewStorageService(testPluginID, inmemory.NewPluginStorageRepository(), WithStorageQuotas(cfg))
}

func setEntry(t *testing.T, svc *StorageServiceImpl, key string, size int) *storage.StorageSetResponse {
	t.Helper()

	resp, err := svc.Set(context.Background(), &storage.StorageSetRequest{
		Key:     key,
		Payload: make([]byte, size),
	})
	require.NoError(t, err)

	return resp
}

func TestStorageService_Set_value_size_quota(t *testing.T) {
	t.Parallel()

	svc := newQuotaStorageService(StorageConfig{MaxValueBytes: 8})

	resp := setEntry(t, svc, "big", 9)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "payload exceeds 8 bytes", *resp.Error)

	resp = setEntry(t, svc, "fits", 8)
	assert.True(t, resp.Success)
}

func TestStorageService_Set_key_count_quota(t *testing.T) {
	t.Parallel()

	svc := newQuotaStorageService(StorageConfig{MaxKeysPerPlugin: 2})

	assert.True(t, setEntry(t, svc, "a", 1).Success)
	assert.True(t, setEntry(t, svc, "b", 1).Success)

	resp := setEntry(t, svc, "c", 1)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "at most 2 storage entries per plugin", *resp.Error)

	// Overwriting an existing key does not need a free slot.
	assert.True(t, setEntry(t, svc, "a", 3).Success)

	// Another plugin's entries do not count against this one.
	other := NewStorageService(testPluginID+1, svc.repo, WithStorageQuotas(StorageConfig{MaxKeysPerPlugin: 2}))
	assert.True(t, setEntry(t, other, "a", 1).Success)
}

func TestStorageService_Set_total_bytes_quota(t *testing.T) {
	t.Parallel()

	svc := newQuotaStorageService(StorageConfig{MaxTotalBytes: 10, MaxValueBytes: 10})

	assert.True(t, setEntry(t, svc, "a", 6).Success)

	resp := setEntry(t, svc, "b", 5)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "storage quota of 10 bytes exceeded", *resp.Error)

	// Replacing "a" releases its 6 bytes first: 10 fits exactly.
	assert.True(t, setEntry(t, svc, "a", 10).Success)
	assert.False(t, setEntry(t, svc, "b", 1).Success)

	// Freeing space makes room again.
	_, err := svc.Delete(context.Background(), &storage.StorageDeleteRequest{Key: "a"})
	require.NoError(t, err)
	assert.True(t, setEntry(t, svc, "b", 10).Success)
}

func TestStorageService_defaults_apply_without_config(t *testing.T) {
	t.Parallel()

	svc := NewStorageService(testPluginID, inmemory.NewPluginStorageRepository())

	assert.Equal(t, defaultStorageMaxKeysPerPlugin, svc.cfg.MaxKeysPerPlugin)
	assert.Equal(t, defaultStorageMaxValueBytes, svc.cfg.MaxValueBytes)
	assert.Equal(t, uint64(defaultStorageMaxTotalBytes), svc.cfg.MaxTotalBytes)

	partial := NewStorageService(testPluginID, inmemory.NewPluginStorageRepository(),
		WithStorageQuotas(StorageConfig{MaxKeysPerPlugin: 5}))
	assert.Equal(t, 5, partial.cfg.MaxKeysPerPlugin)
	assert.Equal(t, defaultStorageMaxValueBytes, partial.cfg.MaxValueBytes, "a partial config keeps the other quotas")
}

// failingUsageRepo makes the quota lookup fail.
type failingUsageRepo struct {
	repositories.PluginStorageRepository
}

func (failingUsageRepo) UsageByPlugin(context.Context, uint64) (domain.PluginStorageUsage, error) {
	return domain.PluginStorageUsage{}, errQuotaTestConnection
}

func TestStorageService_Set_hides_driver_errors(t *testing.T) {
	t.Parallel()

	svc := NewStorageService(testPluginID, failingUsageRepo{inmemory.NewPluginStorageRepository()})

	resp := setEntry(t, svc, "a", 1)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, storageReadFailureMessage, *resp.Error)
	assert.NotContains(t, *resp.Error, "connection reset")
}

func TestStorageService_List_pagination(t *testing.T) {
	t.Parallel()

	svc := newQuotaStorageService(StorageConfig{})
	ctx := context.Background()

	for i := range 5 {
		assert.True(t, setEntry(t, svc, "item:"+strconv.Itoa(i), 1).Success)
	}
	assert.True(t, setEntry(t, svc, "other", 1).Success)

	tests := []struct {
		name        string
		req         *storage.StorageListRequest
		wantKeys    []string
		wantHasMore bool
	}{
		{
			name:        "no_limit_returns_everything_as_before",
			req:         &storage.StorageListRequest{},
			wantKeys:    []string{"item:0", "item:1", "item:2", "item:3", "item:4", "other"},
			wantHasMore: false,
		},
		{
			name:        "first_page",
			req:         &storage.StorageListRequest{Limit: new(uint32(2))},
			wantKeys:    []string{"item:0", "item:1"},
			wantHasMore: true,
		},
		{
			name:        "middle_page",
			req:         &storage.StorageListRequest{Limit: new(uint32(2)), Offset: new(uint32(2))},
			wantKeys:    []string{"item:2", "item:3"},
			wantHasMore: true,
		},
		{
			name:        "last_page",
			req:         &storage.StorageListRequest{Limit: new(uint32(2)), Offset: new(uint32(4))},
			wantKeys:    []string{"item:4", "other"},
			wantHasMore: false,
		},
		{
			name:        "prefix_with_limit",
			req:         &storage.StorageListRequest{KeyPrefix: new("item:"), Limit: new(uint32(4))},
			wantKeys:    []string{"item:0", "item:1", "item:2", "item:3"},
			wantHasMore: true,
		},
		{
			name:        "prefix_without_limit",
			req:         &storage.StorageListRequest{KeyPrefix: new("item:")},
			wantKeys:    []string{"item:0", "item:1", "item:2", "item:3", "item:4"},
			wantHasMore: false,
		},
		{
			name:        "offset_past_the_end",
			req:         &storage.StorageListRequest{Limit: new(uint32(2)), Offset: new(uint32(10))},
			wantKeys:    []string{},
			wantHasMore: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := svc.List(ctx, tt.req)
			require.NoError(t, err)

			keys := make([]string, 0, len(resp.Entries))
			for _, entry := range resp.Entries {
				keys = append(keys, entry.Key)
			}

			assert.Equal(t, tt.wantKeys, keys)
			assert.Equal(t, tt.wantHasMore, resp.HasMore)
		})
	}
}

func TestStorageService_List_prefix_is_applied_by_the_repository(t *testing.T) {
	t.Parallel()

	// The prefix travels in the filter, so the repository never loads the
	// whole plugin storage to filter it in memory.
	repo := &capturingStorageRepo{PluginStorageRepository: inmemory.NewPluginStorageRepository()}
	svc := NewStorageService(testPluginID, repo)

	_, err := svc.List(context.Background(), &storage.StorageListRequest{KeyPrefix: new("backup:")})
	require.NoError(t, err)
	require.NotNil(t, repo.lastFilter)
	require.NotNil(t, repo.lastFilter.KeyPrefix)
	assert.Equal(t, "backup:", *repo.lastFilter.KeyPrefix)
}

type capturingStorageRepo struct {
	repositories.PluginStorageRepository

	lastFilter *filters.FindPluginStorage
}

func (r *capturingStorageRepo) Find(
	ctx context.Context,
	filter *filters.FindPluginStorage,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.PluginStorageEntry, error) {
	r.lastFilter = filter

	return r.PluginStorageRepository.Find(ctx, filter, order, pagination)
}
