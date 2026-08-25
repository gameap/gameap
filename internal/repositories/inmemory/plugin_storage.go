package inmemory

import (
	"cmp"
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
)

type PluginStorageRepository struct {
	mu      sync.RWMutex
	entries map[uint64]*domain.PluginStorageEntry
	nextID  atomic.Uint64

	pluginIDIndex map[uint64]map[uint64]struct{}
	keyIndex      map[string]map[uint64]struct{}
	entityIndex   map[entityKey]map[uint64]struct{}
}

type entityKey struct {
	entityType string
	entityID   uint
}

func NewPluginStorageRepository() *PluginStorageRepository {
	return &PluginStorageRepository{
		entries:       make(map[uint64]*domain.PluginStorageEntry),
		pluginIDIndex: make(map[uint64]map[uint64]struct{}),
		keyIndex:      make(map[string]map[uint64]struct{}),
		entityIndex:   make(map[entityKey]map[uint64]struct{}),
	}
}

func (r *PluginStorageRepository) Find(
	_ context.Context,
	filter *filters.FindPluginStorage,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.PluginStorageEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == nil {
		filter = &filters.FindPluginStorage{}
	}

	candidateIDs := r.getFilteredEntryIDs(filter)

	entries := make([]domain.PluginStorageEntry, 0, len(candidateIDs))
	for entryID := range candidateIDs {
		if entry, exists := r.entries[entryID]; exists {
			entries = append(entries, *entry)
		}
	}

	r.sortEntries(entries, order)

	return r.applyPagination(entries, pagination), nil
}

func (r *PluginStorageRepository) Save(_ context.Context, entry *domain.PluginStorageEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	r.resolveEntryID(entry, &now)
	entry.UpdatedAt = &now

	savedEntry := &domain.PluginStorageEntry{
		ID:         entry.ID,
		PluginID:   entry.PluginID,
		Key:        entry.Key,
		EntityType: entry.EntityType,
		EntityID:   entry.EntityID,
		Payload:    make([]byte, len(entry.Payload)),
		CreatedAt:  entry.CreatedAt,
		UpdatedAt:  entry.UpdatedAt,
	}
	copy(savedEntry.Payload, entry.Payload)

	r.entries[entry.ID] = savedEntry

	r.addToIndexes(savedEntry)

	return nil
}

func (r *PluginStorageRepository) resolveEntryID(entry *domain.PluginStorageEntry, now *time.Time) {
	if entry.ID != 0 {
		if oldEntry, exists := r.entries[entry.ID]; exists {
			r.removeFromIndexes(oldEntry)
		}

		return
	}

	existingID := r.findExistingEntry(entry.PluginID, entry.Key, entry.EntityType, entry.EntityID)
	if existingID != 0 {
		entry.ID = existingID
		if oldEntry, exists := r.entries[entry.ID]; exists {
			r.removeFromIndexes(oldEntry)
			entry.CreatedAt = oldEntry.CreatedAt
		}

		return
	}

	entry.ID = r.nextID.Add(1)
	entry.CreatedAt = now
}

func (r *PluginStorageRepository) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry, exists := r.entries[id]; exists {
		r.removeFromIndexes(entry)
	}

	delete(r.entries, id)

	return nil
}

func (r *PluginStorageRepository) DeleteByPlugin(_ context.Context, pluginID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entrySet, exists := r.pluginIDIndex[pluginID]; exists {
		for entryID := range entrySet {
			if entry, exists := r.entries[entryID]; exists {
				r.removeFromIndexes(entry)
				delete(r.entries, entryID)
			}
		}
	}

	return nil
}

func (r *PluginStorageRepository) DeleteByFilter(_ context.Context, filter *filters.FindPluginStorage) error {
	// Reject a nil OR an all-empty filter to match the SQL repositories: an
	// empty filter would otherwise delete every entry.
	if filter == nil ||
		(len(filter.IDs) == 0 && len(filter.PluginIDs) == 0 &&
			len(filter.Keys) == 0 && len(filter.EntityPairs) == 0) {
		return errors.New("a non-empty filter is required for DeleteByFilter")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	candidateIDs := r.getFilteredEntryIDs(filter)
	for entryID := range candidateIDs {
		if entry, exists := r.entries[entryID]; exists {
			r.removeFromIndexes(entry)
			delete(r.entries, entryID)
		}
	}

	return nil
}

func (r *PluginStorageRepository) findExistingEntry(
	pluginID uint64,
	key string,
	entityType *string,
	entityID *uint,
) uint64 {
	pluginEntries, exists := r.pluginIDIndex[pluginID]
	if !exists {
		return 0
	}

	for entryID := range pluginEntries {
		entry, exists := r.entries[entryID]
		if !exists {
			continue
		}
		if entry.Key != key {
			continue
		}
		if !ptrEqual(entry.EntityType, entityType) {
			continue
		}
		if !ptrEqualUint(entry.EntityID, entityID) {
			continue
		}

		return entryID
	}

	return 0
}

func ptrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return *a == *b
}

func ptrEqualUint(a, b *uint) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return *a == *b
}

func (r *PluginStorageRepository) addToIndexes(entry *domain.PluginStorageEntry) {
	if r.pluginIDIndex[entry.PluginID] == nil {
		r.pluginIDIndex[entry.PluginID] = make(map[uint64]struct{})
	}
	r.pluginIDIndex[entry.PluginID][entry.ID] = struct{}{}

	if r.keyIndex[entry.Key] == nil {
		r.keyIndex[entry.Key] = make(map[uint64]struct{})
	}
	r.keyIndex[entry.Key][entry.ID] = struct{}{}

	eKey := makeEntityKey(entry.EntityType, entry.EntityID)
	if r.entityIndex[eKey] == nil {
		r.entityIndex[eKey] = make(map[uint64]struct{})
	}
	r.entityIndex[eKey][entry.ID] = struct{}{}
}

func (r *PluginStorageRepository) removeFromIndexes(entry *domain.PluginStorageEntry) {
	if entrySet, exists := r.pluginIDIndex[entry.PluginID]; exists {
		delete(entrySet, entry.ID)
		if len(entrySet) == 0 {
			delete(r.pluginIDIndex, entry.PluginID)
		}
	}

	if entrySet, exists := r.keyIndex[entry.Key]; exists {
		delete(entrySet, entry.ID)
		if len(entrySet) == 0 {
			delete(r.keyIndex, entry.Key)
		}
	}

	eKey := makeEntityKey(entry.EntityType, entry.EntityID)
	if entrySet, exists := r.entityIndex[eKey]; exists {
		delete(entrySet, entry.ID)
		if len(entrySet) == 0 {
			delete(r.entityIndex, eKey)
		}
	}
}

func makeEntityKey(entityType *string, entityID *uint) entityKey {
	key := entityKey{}
	if entityType != nil {
		key.entityType = *entityType
	}
	if entityID != nil {
		key.entityID = *entityID
	}

	return key
}

//nolint:gocognit
func (r *PluginStorageRepository) getFilteredEntryIDs(filter *filters.FindPluginStorage) map[uint64]struct{} {
	resultIDs := make(map[uint64]struct{}, len(r.entries))

	if filter == nil {
		for entryID := range r.entries {
			resultIDs[entryID] = struct{}{}
		}

		return resultIDs
	}

	switch {
	case len(filter.IDs) > 0:
		for _, id := range filter.IDs {
			if _, exists := r.entries[id]; exists {
				resultIDs[id] = struct{}{}
			}
		}
	case len(filter.PluginIDs) > 0:
		for _, pluginID := range filter.PluginIDs {
			if entrySet, exists := r.pluginIDIndex[pluginID]; exists {
				for entryID := range entrySet {
					resultIDs[entryID] = struct{}{}
				}
			}
		}
	case len(filter.Keys) > 0:
		for _, key := range filter.Keys {
			if entrySet, exists := r.keyIndex[key]; exists {
				for entryID := range entrySet {
					resultIDs[entryID] = struct{}{}
				}
			}
		}
	case len(filter.EntityPairs) > 0:
		for _, pair := range filter.EntityPairs {
			eKey := makeEntityKey(pair.EntityType, pair.EntityID)
			if entrySet, exists := r.entityIndex[eKey]; exists {
				for entryID := range entrySet {
					resultIDs[entryID] = struct{}{}
				}
			}
		}
	default:
		for entryID := range r.entries {
			resultIDs[entryID] = struct{}{}
		}
	}

	if len(filter.PluginIDs) > 0 && len(filter.IDs) > 0 {
		r.intersectWithPluginIDs(resultIDs, filter.PluginIDs)
	}
	if len(filter.Keys) > 0 && (len(filter.IDs) > 0 || len(filter.PluginIDs) > 0) {
		r.intersectWithKeys(resultIDs, filter.Keys)
	}
	if len(filter.EntityPairs) > 0 && (len(filter.IDs) > 0 || len(filter.PluginIDs) > 0 || len(filter.Keys) > 0) {
		r.intersectWithEntityPairs(resultIDs, filter.EntityPairs)
	}

	if filter.KeyPrefix != nil && *filter.KeyPrefix != "" {
		r.intersectWithKeyPrefix(resultIDs, *filter.KeyPrefix)
	}

	return resultIDs
}

func (r *PluginStorageRepository) intersectWithKeyPrefix(resultIDs map[uint64]struct{}, prefix string) {
	for entryID := range resultIDs {
		entry, ok := r.entries[entryID]
		if !ok || !strings.HasPrefix(entry.Key, prefix) {
			delete(resultIDs, entryID)
		}
	}
}

func (r *PluginStorageRepository) UsageByPlugin(_ context.Context, pluginID uint64) (domain.PluginStorageUsage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var usage domain.PluginStorageUsage

	for entryID := range r.pluginIDIndex[pluginID] {
		entry, ok := r.entries[entryID]
		if !ok {
			continue
		}

		usage.Keys++
		usage.Bytes += uint64(len(entry.Payload))
	}

	return usage, nil
}

func (r *PluginStorageRepository) intersectWithPluginIDs(resultIDs map[uint64]struct{}, pluginIDs []uint64) {
	validIDs := make(map[uint64]struct{})
	for _, pluginID := range pluginIDs {
		if entrySet, exists := r.pluginIDIndex[pluginID]; exists {
			for entryID := range entrySet {
				if _, exists := resultIDs[entryID]; exists {
					validIDs[entryID] = struct{}{}
				}
			}
		}
	}
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *PluginStorageRepository) intersectWithKeys(resultIDs map[uint64]struct{}, keys []string) {
	validIDs := make(map[uint64]struct{})
	for _, key := range keys {
		if entrySet, exists := r.keyIndex[key]; exists {
			for entryID := range entrySet {
				if _, exists := resultIDs[entryID]; exists {
					validIDs[entryID] = struct{}{}
				}
			}
		}
	}
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *PluginStorageRepository) intersectWithEntityPairs(
	resultIDs map[uint64]struct{},
	pairs []domain.PluginStorageEntityPair,
) {
	validIDs := make(map[uint64]struct{})
	for _, pair := range pairs {
		eKey := makeEntityKey(pair.EntityType, pair.EntityID)
		if entrySet, exists := r.entityIndex[eKey]; exists {
			for entryID := range entrySet {
				if _, exists := resultIDs[entryID]; exists {
					validIDs[entryID] = struct{}{}
				}
			}
		}
	}
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *PluginStorageRepository) sortEntries(entries []domain.PluginStorageEntry, order []filters.Sorting) {
	if len(order) == 0 {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].ID < entries[j].ID
		})

		return
	}

	sort.Slice(entries, func(i, j int) bool {
		for _, o := range order {
			cmpRes := r.compareEntries(&entries[i], &entries[j], o.Field)
			if cmpRes != 0 {
				if o.Direction == filters.SortDirectionDesc {
					return cmpRes > 0
				}

				return cmpRes < 0
			}
		}

		return false
	})
}

func (r *PluginStorageRepository) compareEntries(a, b *domain.PluginStorageEntry, field string) int {
	switch field {
	case "id":
		return cmp.Compare(a.ID, b.ID)
	case "plugin_id":
		return cmp.Compare(a.PluginID, b.PluginID)
	case "key":
		return strings.Compare(a.Key, b.Key)
	default:
		return 0
	}
}

func (r *PluginStorageRepository) applyPagination(
	entries []domain.PluginStorageEntry,
	pagination *filters.Pagination,
) []domain.PluginStorageEntry {
	if pagination == nil {
		return entries
	}

	limit := pagination.Limit
	if limit == 0 {
		limit = filters.DefaultLimit
	}

	offset := pagination.Offset
	length := uint64(len(entries))

	if offset >= length {
		return []domain.PluginStorageEntry{}
	}

	end := min(offset+limit, length)

	return entries[offset:end]
}
