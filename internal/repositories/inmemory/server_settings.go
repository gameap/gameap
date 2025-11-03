package inmemory

import (
	"cmp"
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
)

type ServerSettingRepository struct {
	mu       sync.RWMutex
	settings map[uint]*domain.ServerSetting
	nextID   uint32

	// Hash indexes for efficient filtering
	serverIDIndex map[uint]map[uint]struct{}   // serverID -> settingIDs
	nameIndex     map[string]map[uint]struct{} // name -> settingIDs
}

func NewServerSettingRepository() *ServerSettingRepository {
	return &ServerSettingRepository{
		settings:      make(map[uint]*domain.ServerSetting),
		serverIDIndex: make(map[uint]map[uint]struct{}),
		nameIndex:     make(map[string]map[uint]struct{}),
	}
}

func (r *ServerSettingRepository) Find(
	_ context.Context,
	filter *filters.FindServerSetting,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerSetting, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == nil {
		filter = &filters.FindServerSetting{}
	}

	// Use hash indexes for efficient filtering
	candidateIDs := r.getFilteredSettingIDs(filter)

	settings := make([]domain.ServerSetting, 0, len(candidateIDs))
	for settingID := range candidateIDs {
		if setting, exists := r.settings[settingID]; exists {
			settings = append(settings, *setting)
		}
	}

	r.sortSettings(settings, order)

	return r.applyPagination(settings, pagination), nil
}

func (r *ServerSettingRepository) Save(_ context.Context, setting *domain.ServerSetting) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove old indexes if updating existing setting
	if setting.ID != 0 {
		if oldSetting, exists := r.settings[setting.ID]; exists {
			r.removeFromIndexes(oldSetting)
		}
	} else {
		setting.ID = uint(atomic.AddUint32(&r.nextID, 1))
	}

	// Save setting
	r.settings[setting.ID] = &domain.ServerSetting{
		ID:       setting.ID,
		Name:     setting.Name,
		ServerID: setting.ServerID,
		Value:    setting.Value,
	}

	// Add to indexes
	r.addToIndexes(r.settings[setting.ID])

	return nil
}

func (r *ServerSettingRepository) Delete(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if setting, exists := r.settings[id]; exists {
		// Remove from indexes
		r.removeFromIndexes(setting)
	}

	delete(r.settings, id)

	return nil
}

func (r *ServerSettingRepository) addToIndexes(setting *domain.ServerSetting) {
	// ServerID index
	if r.serverIDIndex[setting.ServerID] == nil {
		r.serverIDIndex[setting.ServerID] = make(map[uint]struct{})
	}
	r.serverIDIndex[setting.ServerID][setting.ID] = struct{}{}

	// Name index
	if r.nameIndex[setting.Name] == nil {
		r.nameIndex[setting.Name] = make(map[uint]struct{})
	}
	r.nameIndex[setting.Name][setting.ID] = struct{}{}
}

func (r *ServerSettingRepository) removeFromIndexes(setting *domain.ServerSetting) {
	// ServerID index
	if settingSet, exists := r.serverIDIndex[setting.ServerID]; exists {
		delete(settingSet, setting.ID)
		if len(settingSet) == 0 {
			delete(r.serverIDIndex, setting.ServerID)
		}
	}

	// Name index
	if settingSet, exists := r.nameIndex[setting.Name]; exists {
		delete(settingSet, setting.ID)
		if len(settingSet) == 0 {
			delete(r.nameIndex, setting.Name)
		}
	}
}

//nolint:gocognit
func (r *ServerSettingRepository) getFilteredSettingIDs(filter *filters.FindServerSetting) map[uint]struct{} {
	resultIDs := make(map[uint]struct{}, len(r.settings))

	if filter == nil {
		// No filter, return all setting IDs
		for settingID := range r.settings {
			resultIDs[settingID] = struct{}{}
		}

		return resultIDs
	}

	// Start with the first available filter result
	switch {
	case len(filter.IDs) > 0:
		for _, id := range filter.IDs {
			if _, exists := r.settings[id]; exists {
				resultIDs[id] = struct{}{}
			}
		}
	case len(filter.ServerIDs) > 0:
		for _, serverID := range filter.ServerIDs {
			if settingSet, exists := r.serverIDIndex[serverID]; exists {
				for settingID := range settingSet {
					resultIDs[settingID] = struct{}{}
				}
			}
		}
	case len(filter.Names) > 0:
		for _, name := range filter.Names {
			if settingSet, exists := r.nameIndex[name]; exists {
				for settingID := range settingSet {
					resultIDs[settingID] = struct{}{}
				}
			}
		}
	default:
		// No filters, return all settings
		for settingID := range r.settings {
			resultIDs[settingID] = struct{}{}
		}
	}

	// Apply intersection for additional filters
	if len(filter.ServerIDs) > 0 && len(filter.IDs) > 0 {
		r.intersectWithServerIDs(resultIDs, filter.ServerIDs)
	}
	if len(filter.Names) > 0 && (len(filter.IDs) > 0 || len(filter.ServerIDs) > 0) {
		r.intersectWithNames(resultIDs, filter.Names)
	}

	return resultIDs
}

func (r *ServerSettingRepository) intersectWithServerIDs(resultIDs map[uint]struct{}, serverIDs []uint) {
	validIDs := make(map[uint]struct{})
	for _, serverID := range serverIDs {
		if settingSet, exists := r.serverIDIndex[serverID]; exists {
			for settingID := range settingSet {
				if _, exists := resultIDs[settingID]; exists {
					validIDs[settingID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *ServerSettingRepository) intersectWithNames(resultIDs map[uint]struct{}, names []string) {
	validIDs := make(map[uint]struct{})
	for _, name := range names {
		if settingSet, exists := r.nameIndex[name]; exists {
			for settingID := range settingSet {
				if _, exists := resultIDs[settingID]; exists {
					validIDs[settingID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *ServerSettingRepository) sortSettings(settings []domain.ServerSetting, order []filters.Sorting) {
	if len(order) == 0 {
		sort.Slice(settings, func(i, j int) bool {
			return settings[i].ID < settings[j].ID
		})

		return
	}

	sort.Slice(settings, func(i, j int) bool {
		for _, o := range order {
			cmpRes := r.compareSettings(&settings[i], &settings[j], o.Field)
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

func (r *ServerSettingRepository) compareSettings(a, b *domain.ServerSetting, field string) int {
	switch field {
	case "id":
		return cmp.Compare(a.ID, b.ID)
	case "name":
		return strings.Compare(a.Name, b.Name)
	case "server_id":
		return cmp.Compare(a.ServerID, b.ServerID)
	default:
		return 0
	}
}

func (r *ServerSettingRepository) applyPagination(
	settings []domain.ServerSetting,
	pagination *filters.Pagination,
) []domain.ServerSetting {
	if pagination == nil {
		return settings
	}

	limit := pagination.Limit
	if limit <= 0 {
		limit = filters.DefaultLimit
	}

	offset := max(pagination.Offset, 0)

	if offset >= len(settings) {
		return []domain.ServerSetting{}
	}

	end := min(offset+limit, len(settings))

	return settings[offset:end]
}
