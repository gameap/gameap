package rbac

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheEntry_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "not_expired_future_time",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "expired_past_time",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
		{
			name:      "expired_just_now",
			expiresAt: time.Now().Add(-1 * time.Millisecond),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &cacheEntry{
				Permissions: make(map[domain.AbilityName][]domain.Permission),
				ExpiresAt:   tt.expiresAt,
			}

			got := entry.IsExpired()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewPermissionCache(t *testing.T) {
	ttl := 5 * time.Minute
	cache := newPermissionCache(ttl)

	require.NotNil(t, cache)
	assert.NotNil(t, cache.cache)
	assert.Equal(t, ttl, cache.ttl)
}

func TestPermissionCache_SetAndGet(t *testing.T) {
	cache := newPermissionCache(1 * time.Hour)
	key := cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   1,
	}

	permissions := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameView: {
			{ID: 1, AbilityID: 1},
		},
	}

	cache.Set(key, permissions)

	got, exists := cache.Get(key)
	require.True(t, exists)
	assert.Equal(t, permissions, got)
}

func TestPermissionCache_Get_NonExistent(t *testing.T) {
	cache := newPermissionCache(1 * time.Hour)
	key := cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   999,
	}

	got, exists := cache.Get(key)
	assert.False(t, exists)
	assert.Nil(t, got)
}

func TestPermissionCache_Get_Expired(t *testing.T) {
	cache := newPermissionCache(1 * time.Millisecond)
	key := cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   1,
	}

	permissions := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameView: {
			{ID: 1, AbilityID: 1},
		},
	}

	cache.Set(key, permissions)

	time.Sleep(10 * time.Millisecond)

	got, exists := cache.Get(key)
	assert.False(t, exists)
	assert.Nil(t, got)
}

func TestPermissionCache_Get_ZeroTTL(t *testing.T) {
	cache := newPermissionCache(0)
	key := cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   1,
	}

	permissions := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameView: {
			{ID: 1, AbilityID: 1},
		},
	}

	cache.Set(key, permissions)

	got, exists := cache.Get(key)
	assert.False(t, exists)
	assert.Nil(t, got)
}

func TestPermissionCache_Set_ZeroTTL(t *testing.T) {
	cache := newPermissionCache(0)
	key := cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   1,
	}

	permissions := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameView: {
			{ID: 1, AbilityID: 1},
		},
	}

	cache.Set(key, permissions)

	assert.Equal(t, 0, cache.Len())
}

func TestPermissionCache_Delete(t *testing.T) {
	cache := newPermissionCache(1 * time.Hour)
	key := cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   1,
	}

	permissions := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameView: {
			{ID: 1, AbilityID: 1},
		},
	}

	cache.Set(key, permissions)

	_, exists := cache.Get(key)
	require.True(t, exists)

	cache.Delete(key)

	_, exists = cache.Get(key)
	assert.False(t, exists)
}

func TestPermissionCache_Delete_NonExistent(t *testing.T) {
	cache := newPermissionCache(1 * time.Hour)
	key := cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   999,
	}

	cache.Delete(key)

	assert.Equal(t, 0, cache.Len())
}

func TestPermissionCache_Clear(t *testing.T) {
	cache := newPermissionCache(1 * time.Hour)

	key1 := cacheKey{EntityType: domain.EntityTypeUser, EntityID: 1}
	key2 := cacheKey{EntityType: domain.EntityTypeUser, EntityID: 2}
	key3 := cacheKey{EntityType: domain.EntityTypeRole, EntityID: 1}

	permissions := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameView: {{ID: 1, AbilityID: 1}},
	}

	cache.Set(key1, permissions)
	cache.Set(key2, permissions)
	cache.Set(key3, permissions)

	require.Equal(t, 3, cache.Len())

	cache.Clear()

	assert.Equal(t, 0, cache.Len())

	_, exists := cache.Get(key1)
	assert.False(t, exists)
	_, exists = cache.Get(key2)
	assert.False(t, exists)
	_, exists = cache.Get(key3)
	assert.False(t, exists)
}

func TestPermissionCache_MultipleKeys(t *testing.T) {
	cache := newPermissionCache(1 * time.Hour)

	key1 := cacheKey{EntityType: domain.EntityTypeUser, EntityID: 1}
	key2 := cacheKey{EntityType: domain.EntityTypeUser, EntityID: 2}
	key3 := cacheKey{EntityType: domain.EntityTypeRole, EntityID: 1}

	permissions1 := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameView: {{ID: 1, AbilityID: 1}},
	}
	permissions2 := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameEdit: {{ID: 2, AbilityID: 2}},
	}
	permissions3 := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameCreate: {{ID: 3, AbilityID: 3}},
	}

	cache.Set(key1, permissions1)
	cache.Set(key2, permissions2)
	cache.Set(key3, permissions3)

	got1, exists1 := cache.Get(key1)
	require.True(t, exists1)
	assert.Equal(t, permissions1, got1)

	got2, exists2 := cache.Get(key2)
	require.True(t, exists2)
	assert.Equal(t, permissions2, got2)

	got3, exists3 := cache.Get(key3)
	require.True(t, exists3)
	assert.Equal(t, permissions3, got3)
}

func TestPermissionCache_OverwriteExisting(t *testing.T) {
	cache := newPermissionCache(1 * time.Hour)
	key := cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   1,
	}

	permissions1 := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameView: {{ID: 1, AbilityID: 1}},
	}
	permissions2 := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameEdit: {{ID: 2, AbilityID: 2}},
	}

	cache.Set(key, permissions1)
	cache.Set(key, permissions2)

	got, exists := cache.Get(key)
	require.True(t, exists)
	assert.Equal(t, permissions2, got)
	assert.NotEqual(t, permissions1, got)
}

func TestPermissionCache_ConcurrentAccess(t *testing.T) {
	cache := newPermissionCache(1 * time.Hour)
	key := cacheKey{
		EntityType: domain.EntityTypeUser,
		EntityID:   1,
	}

	permissions := map[domain.AbilityName][]domain.Permission{
		domain.AbilityNameView: {{ID: 1, AbilityID: 1}},
	}

	done := make(chan bool)

	for range 10 {
		go func() {
			cache.Set(key, permissions)
			_, _ = cache.Get(key)
			done <- true
		}()
	}

	for range 10 {
		<-done
	}

	got, exists := cache.Get(key)
	require.True(t, exists)
	assert.Equal(t, permissions, got)
}

func TestPermissionCache_Cleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cache := newPermissionCache(10 * time.Millisecond)

		key1 := cacheKey{EntityType: domain.EntityTypeUser, EntityID: 1}
		key2 := cacheKey{EntityType: domain.EntityTypeUser, EntityID: 2}

		permissions := map[domain.AbilityName][]domain.Permission{
			domain.AbilityNameView: {{ID: 1, AbilityID: 1}},
		}

		cache.Set(key1, permissions)
		cache.Set(key2, permissions)

		time.Sleep(1 * time.Minute)
		require.Equal(t, 2, cache.Len())

		time.Sleep(10 * time.Minute)
		require.Equal(t, 0, cache.Len())

		cache.Close()
		synctest.Wait()
	})
}
