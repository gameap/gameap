package hostlibrary

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"golang.org/x/sync/singleflight"
)

// CachedPermissionChecker keeps a plugin's grants in the instance's memory.
// Both hot paths — every privileged host call and every event delivery — ask
// the same question many times per second, and the answer only changes when
// an operator edits the grants.
//
// Freshness comes from the pub/sub announcement
// (gameap:plugin:subscriptions:refresh), which drops the entry on every
// instance; the TTL is the backstop for a broker that is down, not the
// primary mechanism.
//
// The whole grant set is cached rather than one answer per permission, so a
// single record read serves every question about that plugin.
type CachedPermissionChecker struct {
	source PluginGrantsReader
	ttl    time.Duration
	now    func() time.Time
	group  singleflight.Group

	mu      sync.RWMutex
	entries map[uint64]grantsEntry

	// generation is bumped by every invalidation. A load carries the
	// generation it started under, so grants read just before a revocation
	// cannot be stored just after it and outlive the announcement that was
	// meant to drop them.
	generation uint64
}

type grantsEntry struct {
	permissions []domain.PluginPermission
	expiresAt   time.Time
}

// NewCachedPermissionChecker caches grants for ttl. A ttl of zero or less
// disables caching: every question reads the record, as it did before the
// cache existed.
func NewCachedPermissionChecker(source PluginGrantsReader, ttl time.Duration) *CachedPermissionChecker {
	return &CachedPermissionChecker{
		source:  source,
		ttl:     ttl,
		now:     time.Now,
		entries: make(map[uint64]grantsEntry),
	}
}

func (c *CachedPermissionChecker) Has(
	ctx context.Context,
	pluginID uint64,
	permission domain.PluginPermission,
) (bool, error) {
	permissions, err := c.grants(ctx, pluginID)
	if err != nil {
		return false, err
	}

	return domain.PermissionSatisfied(permission, permissions), nil
}

// Grants answers from the cache when it can, otherwise reads the record once
// per plugin regardless of how many callers are waiting. The result is a copy:
// the cached slice is shared by every caller and must not be written to.
func (c *CachedPermissionChecker) Grants(
	ctx context.Context,
	pluginID uint64,
) ([]domain.PluginPermission, error) {
	permissions, err := c.grants(ctx, pluginID)
	if err != nil {
		return nil, err
	}

	return slices.Clone(permissions), nil
}

func (c *CachedPermissionChecker) grants(
	ctx context.Context,
	pluginID uint64,
) ([]domain.PluginPermission, error) {
	if pluginID == 0 {
		return nil, nil
	}

	if c.ttl <= 0 {
		return c.source.Grants(ctx, pluginID)
	}

	if permissions, ok := c.lookup(pluginID); ok {
		return permissions, nil
	}

	// Keyed by the plugin and the generation the read starts under: a burst
	// of deliveries costs one read instead of one per subscriber, while a
	// question asked after an invalidation starts a read of its own instead
	// of joining the one that predates it.
	generation := c.currentGeneration()

	permissions, err, _ := c.group.Do(cacheKey(pluginID, generation), func() (any, error) {
		return c.load(ctx, pluginID, generation)
	})
	if err != nil {
		return nil, err
	}

	loaded, _ := permissions.([]domain.PluginPermission)

	return loaded, nil
}

// Invalidate drops the plugin's cached grants; the next question reads the
// record.
func (c *CachedPermissionChecker) Invalidate(pluginID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, pluginID)
	c.generation++
}

// InvalidateAll drops every cached entry.
func (c *CachedPermissionChecker) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	clear(c.entries)
	c.generation++
}

func (c *CachedPermissionChecker) lookup(pluginID uint64) ([]domain.PluginPermission, bool) {
	c.mu.RLock()
	entry, ok := c.entries[pluginID]
	c.mu.RUnlock()

	if !ok || !c.now().Before(entry.expiresAt) {
		return nil, false
	}

	return entry.permissions, true
}

func (c *CachedPermissionChecker) currentGeneration() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.generation
}

func (c *CachedPermissionChecker) load(
	ctx context.Context,
	pluginID uint64,
	generation uint64,
) ([]domain.PluginPermission, error) {
	permissions, err := c.source.Grants(ctx, pluginID)
	if err != nil {
		return nil, err
	}

	// An empty set is never stored. It is what a plugin whose record exists
	// but carries no grants looks like — and that is exactly the state a
	// store install passes through between saving the record and recording
	// the manifest's permissions, and the state an uninstalled plugin ends
	// in. Caching it would deny a freshly installed plugin for a whole TTL,
	// while re-reading costs nothing: every check against an empty set is
	// refused anyway.
	if len(permissions) == 0 {
		return nil, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// An invalidation that landed while the record was being read wins: the
	// grants below were read before it and may already be revoked. The caller
	// still gets them — its question predates the change — but they are not
	// stored, so the next question reads the record again instead of holding
	// a revoked grant for a whole TTL.
	if c.generation != generation {
		return permissions, nil
	}

	c.entries[pluginID] = grantsEntry{
		permissions: permissions,
		expiresAt:   c.now().Add(c.ttl),
	}

	return permissions, nil
}

func cacheKey(pluginID, generation uint64) string {
	return strconv.FormatUint(pluginID, 10) + ":" + strconv.FormatUint(generation, 10)
}
