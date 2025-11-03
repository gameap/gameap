package rbac

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
)

const (
	cleanupInterval = 5 * time.Minute
)

type cacheKey struct {
	EntityType domain.EntityType
	EntityID   uint
}

// cacheEntry represents a cached permission entry with TTL.
type cacheEntry struct {
	Permissions map[domain.AbilityName][]domain.Permission
	ExpiresAt   time.Time
}

func (e *cacheEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// permissionCache is an in-memory cache for user permissions with TTL.
type permissionCache struct {
	cache map[cacheKey]*cacheEntry
	mutex sync.RWMutex
	ttl   time.Duration

	cancel chan struct{}
}

func newPermissionCache(ttl time.Duration) *permissionCache {
	cache := &permissionCache{
		cache:  make(map[cacheKey]*cacheEntry),
		ttl:    ttl,
		cancel: make(chan struct{}),
	}

	go cache.cleanup()

	return cache
}

// Get retrieves permissions from cache for a user.
func (c *permissionCache) Get(key cacheKey) (map[domain.AbilityName][]domain.Permission, bool) {
	if c.ttl == 0 {
		return nil, false
	}

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.cache[key]
	if !exists {
		return nil, false
	}

	if entry.IsExpired() {
		// Entry is expired, but we don't delete here to avoid write lock
		return nil, false
	}

	return entry.Permissions, true
}

func (c *permissionCache) Set(key cacheKey, permissions map[domain.AbilityName][]domain.Permission) {
	if c.ttl == 0 {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache[key] = &cacheEntry{
		Permissions: permissions,
		ExpiresAt:   time.Now().Add(c.ttl),
	}
}

func (c *permissionCache) Delete(key cacheKey) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.cache, key)
}

// Clear removes all cached permissions.
func (c *permissionCache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache = make(map[cacheKey]*cacheEntry)
}

func (c *permissionCache) Close() {
	close(c.cancel)
}

func (c *permissionCache) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return len(c.cache)
}

// cleanup runs periodically to remove expired entries.
func (c *permissionCache) cleanup() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.cancel:
			slog.Info("Permission cache cleanup stopped")

			return
		case <-ticker.C:
			c.mutex.Lock()
			for userID, entry := range c.cache {
				if entry.IsExpired() {
					delete(c.cache, userID)
				}
			}
			c.mutex.Unlock()
		}
	}
}
