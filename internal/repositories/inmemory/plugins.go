package inmemory

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

type PluginRepository struct {
	mu      sync.RWMutex
	plugins map[domain.Uint64ID]*domain.Plugin
}

func NewPluginRepository() *PluginRepository {
	return &PluginRepository{
		plugins: make(map[domain.Uint64ID]*domain.Plugin),
	}
}

func (r *PluginRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]domain.Plugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		plugins = append(plugins, r.copyPlugin(plugin))
	}

	r.sortPlugins(plugins, order)

	return r.applyPagination(plugins, pagination), nil
}

func (r *PluginRepository) Find(
	_ context.Context,
	filter *filters.FindPlugin,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var plugins []domain.Plugin

	for _, plugin := range r.plugins {
		if r.matchesFilter(plugin, filter) {
			plugins = append(plugins, r.copyPlugin(plugin))
		}
	}

	r.sortPlugins(plugins, order)

	return r.applyPagination(plugins, pagination), nil
}

func (r *PluginRepository) Save(_ context.Context, plugin *domain.Plugin) error {
	if plugin.ID == 0 {
		return errors.New("plugin ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	plugin.UpdatedAt = new(now)

	_, exists := r.plugins[plugin.ID]
	if !exists {
		if plugin.CreatedAt == nil || plugin.CreatedAt.IsZero() {
			plugin.CreatedAt = new(now)
		}
	}

	r.plugins[plugin.ID] = &domain.Plugin{
		ID:                  plugin.ID,
		Name:                plugin.Name,
		Version:             plugin.Version,
		Description:         plugin.Description,
		Author:              plugin.Author,
		APIVersion:          plugin.APIVersion,
		Filename:            plugin.Filename,
		Source:              plugin.Source,
		Homepage:            plugin.Homepage,
		Checksum:            copyStringPtr(plugin.Checksum),
		RequiredPermissions: copyPermissions(plugin.RequiredPermissions),
		AllowedPermissions:  copyPermissions(plugin.AllowedPermissions),
		Status:              plugin.Status,
		Priority:            plugin.Priority,
		Generation:          plugin.Generation,
		Category:            plugin.Category,
		Dependencies:        copyStrings(plugin.Dependencies),
		Config:              copyConfig(plugin.Config),
		ConfigSchema:        copyStringPtr(plugin.ConfigSchema),
		InstalledAt:         plugin.InstalledAt,
		LastLoadedAt:        plugin.LastLoadedAt,
		LastError:           copyStringPtr(plugin.LastError),
		LastErrorAt:         copyTimePtr(plugin.LastErrorAt),
		CreatedAt:           plugin.CreatedAt,
		UpdatedAt:           plugin.UpdatedAt,
	}

	return nil
}

func (r *PluginRepository) UpdateLoadState(
	_ context.Context,
	id domain.Uint64ID,
	state domain.PluginLoadState,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, exists := r.plugins[id]
	if !exists {
		return repositories.ErrPluginNotFound
	}

	plugin.Status = state.Status
	plugin.LastError = copyStringPtr(state.LastError)
	plugin.LastErrorAt = copyTimePtr(state.LastErrorAt)
	plugin.LastLoadedAt = copyTimePtr(state.LastLoadedAt)
	plugin.Generation = state.Generation
	plugin.ConfigSchema = copyStringPtr(state.ConfigSchema)
	plugin.UpdatedAt = new(time.Now())

	return nil
}

func (r *PluginRepository) Delete(_ context.Context, id domain.Uint64ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.plugins, id)

	return nil
}

func (r *PluginRepository) Exists(_ context.Context, filter *filters.FindPlugin) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, plugin := range r.plugins {
		if r.matchesFilter(plugin, filter) {
			return true, nil
		}
	}

	return false, nil
}

func (r *PluginRepository) matchesFilter(plugin *domain.Plugin, filter *filters.FindPlugin) bool {
	if filter == nil {
		return true
	}

	if len(filter.IDs) > 0 && !slices.Contains(filter.IDs, plugin.ID) {
		return false
	}

	if len(filter.Names) > 0 && !slices.Contains(filter.Names, plugin.Name) {
		return false
	}

	if len(filter.Statuses) > 0 && !slices.Contains(filter.Statuses, plugin.Status) {
		return false
	}

	if len(filter.Categories) > 0 {
		if plugin.Category == nil || !slices.Contains(filter.Categories, *plugin.Category) {
			return false
		}
	}

	return true
}

func (r *PluginRepository) sortPlugins(plugins []domain.Plugin, order []filters.Sorting) {
	if len(order) == 0 {
		sort.Slice(plugins, func(i, j int) bool {
			if plugins[i].Priority != plugins[j].Priority {
				return plugins[i].Priority > plugins[j].Priority
			}

			return plugins[i].Name < plugins[j].Name
		})

		return
	}

	sort.Slice(plugins, func(i, j int) bool {
		for _, sorting := range order {
			var result int
			switch sorting.Field {
			case "id":
				result = cmp.Compare(plugins[i].ID, plugins[j].ID)
			case "name":
				result = strings.Compare(plugins[i].Name, plugins[j].Name)
			case "priority":
				result = plugins[i].Priority - plugins[j].Priority
			case "status":
				result = strings.Compare(string(plugins[i].Status), string(plugins[j].Status))
			default:
				continue
			}

			if result != 0 {
				if sorting.Direction == filters.SortDirectionDesc {
					return result > 0
				}

				return result < 0
			}
		}

		return false
	})
}

func (r *PluginRepository) applyPagination(plugins []domain.Plugin, pagination *filters.Pagination) []domain.Plugin {
	if pagination == nil {
		return plugins
	}

	length := uint64(len(plugins))
	if pagination.Offset >= length {
		return []domain.Plugin{}
	}

	start := pagination.Offset
	limit := pagination.Limit
	if limit == 0 {
		limit = filters.DefaultLimit
	}
	end := min(start+limit, length)

	return plugins[start:end]
}

func (r *PluginRepository) copyPlugin(plugin *domain.Plugin) domain.Plugin {
	return domain.Plugin{
		ID:                  plugin.ID,
		Name:                plugin.Name,
		Version:             plugin.Version,
		Description:         plugin.Description,
		Author:              plugin.Author,
		APIVersion:          plugin.APIVersion,
		Filename:            plugin.Filename,
		Source:              plugin.Source,
		Homepage:            plugin.Homepage,
		Checksum:            copyStringPtr(plugin.Checksum),
		RequiredPermissions: copyPermissions(plugin.RequiredPermissions),
		AllowedPermissions:  copyPermissions(plugin.AllowedPermissions),
		Status:              plugin.Status,
		Priority:            plugin.Priority,
		Generation:          plugin.Generation,
		Category:            plugin.Category,
		Dependencies:        copyStrings(plugin.Dependencies),
		Config:              copyConfig(plugin.Config),
		ConfigSchema:        copyStringPtr(plugin.ConfigSchema),
		InstalledAt:         plugin.InstalledAt,
		LastLoadedAt:        plugin.LastLoadedAt,
		LastError:           copyStringPtr(plugin.LastError),
		LastErrorAt:         copyTimePtr(plugin.LastErrorAt),
		CreatedAt:           plugin.CreatedAt,
		UpdatedAt:           plugin.UpdatedAt,
	}
}

func copyStringPtr(s *string) *string {
	if s == nil {
		return nil
	}

	return new(*s)
}

func copyTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}

	return new(*t)
}

func copyPermissions(permissions []domain.PluginPermission) []domain.PluginPermission {
	if permissions == nil {
		return nil
	}

	result := make([]domain.PluginPermission, len(permissions))
	copy(result, permissions)

	return result
}

func copyStrings(strs []string) []string {
	if strs == nil {
		return nil
	}

	result := make([]string, len(strs))
	copy(result, strs)

	return result
}

func copyConfig(config map[string]any) map[string]any {
	if config == nil {
		return nil
	}

	return maps.Clone(config)
}
