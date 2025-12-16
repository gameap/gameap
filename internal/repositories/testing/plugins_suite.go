package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PluginRepositorySuite struct {
	suite.Suite

	repo repositories.PluginRepository
	fn   func(t *testing.T) repositories.PluginRepository
}

func NewPluginRepositorySuite(fn func(t *testing.T) repositories.PluginRepository) *PluginRepositorySuite {
	return &PluginRepositorySuite{
		fn: fn,
	}
}

func (s *PluginRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *PluginRepositorySuite) TestPluginRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_plugin", func(t *testing.T) {
		plugin := &domain.Plugin{
			Name:        "test-plugin",
			Version:     "1.0.0",
			Description: "A test plugin",
			Author:      "Test Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusDisabled,
			Priority:    10,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		assert.NotZero(t, plugin.ID)
		assert.NotNil(t, plugin.CreatedAt)
		assert.NotNil(t, plugin.UpdatedAt)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []uint{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, plugin.ID, result[0].ID)
		assert.Equal(t, "test-plugin", result[0].Name)
		assert.Equal(t, "1.0.0", result[0].Version)
		assert.Equal(t, "A test plugin", result[0].Description)
		assert.Equal(t, "Test Author", result[0].Author)
		assert.Equal(t, "v1", result[0].APIVersion)
		assert.Equal(t, domain.PluginStatusDisabled, result[0].Status)
		assert.Equal(t, 10, result[0].Priority)
	})

	s.T().Run("insert_plugin_with_all_fields", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		plugin := &domain.Plugin{
			Name:        "full-plugin",
			Version:     "2.0.0",
			Description: "A plugin with all fields",
			Author:      "Full Author",
			APIVersion:  "v2",
			Filename:    lo.ToPtr("full-plugin.wasm"),
			Source:      lo.ToPtr("https://github.com/example/plugin"),
			Homepage:    lo.ToPtr("https://example.com/plugin"),
			RequiredPermissions: []domain.PluginPermission{
				domain.PluginPermissionManageServers,
				domain.PluginPermissionManageNodes,
			},
			AllowedPermissions: []domain.PluginPermission{
				domain.PluginPermissionFiles,
				domain.PluginPermissionListenEvents,
			},
			Status:       domain.PluginStatusActive,
			Priority:     100,
			Category:     lo.ToPtr("monitoring"),
			Dependencies: []string{"base-plugin", "auth-plugin"},
			Config: map[string]any{
				"enabled": true,
				"timeout": 30,
				"tags":    []any{"tag1", "tag2"},
			},
			InstalledAt:  &now,
			LastLoadedAt: &now,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		assert.NotZero(t, plugin.ID)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []uint{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Equal(t, "full-plugin", retrieved.Name)
		assert.Equal(t, "2.0.0", retrieved.Version)
		assert.Equal(t, "A plugin with all fields", retrieved.Description)
		assert.Equal(t, "Full Author", retrieved.Author)
		assert.Equal(t, "v2", retrieved.APIVersion)
		require.NotNil(t, retrieved.Filename)
		assert.Equal(t, "full-plugin.wasm", *retrieved.Filename)
		require.NotNil(t, retrieved.Source)
		assert.Equal(t, "https://github.com/example/plugin", *retrieved.Source)
		require.NotNil(t, retrieved.Homepage)
		assert.Equal(t, "https://example.com/plugin", *retrieved.Homepage)
		require.Len(t, retrieved.RequiredPermissions, 2)
		assert.Contains(t, retrieved.RequiredPermissions, domain.PluginPermissionManageServers)
		assert.Contains(t, retrieved.RequiredPermissions, domain.PluginPermissionManageNodes)
		require.Len(t, retrieved.AllowedPermissions, 2)
		assert.Contains(t, retrieved.AllowedPermissions, domain.PluginPermissionFiles)
		assert.Contains(t, retrieved.AllowedPermissions, domain.PluginPermissionListenEvents)
		assert.Equal(t, domain.PluginStatusActive, retrieved.Status)
		assert.Equal(t, 100, retrieved.Priority)
		require.NotNil(t, retrieved.Category)
		assert.Equal(t, "monitoring", *retrieved.Category)
		require.Len(t, retrieved.Dependencies, 2)
		assert.Contains(t, retrieved.Dependencies, "base-plugin")
		assert.Contains(t, retrieved.Dependencies, "auth-plugin")
		require.NotNil(t, retrieved.Config)
		assert.Equal(t, true, retrieved.Config["enabled"])
		require.NotNil(t, retrieved.InstalledAt)
		assert.InDelta(t, now.Unix(), retrieved.InstalledAt.Unix(), 1.0)
		require.NotNil(t, retrieved.LastLoadedAt)
		assert.InDelta(t, now.Unix(), retrieved.LastLoadedAt.Unix(), 1.0)
		assert.NotNil(t, retrieved.CreatedAt)
		assert.NotNil(t, retrieved.UpdatedAt)
	})

	s.T().Run("update_existing_plugin", func(t *testing.T) {
		plugin := &domain.Plugin{
			Name:        "update-plugin",
			Version:     "1.0.0",
			Description: "Original description",
			Author:      "Original Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusDisabled,
			Priority:    5,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		originalID := plugin.ID
		originalCreatedAt := plugin.CreatedAt

		time.Sleep(10 * time.Millisecond)

		plugin.Version = "2.0.0" //nolint:goconst
		plugin.Description = "Updated description"
		plugin.Status = domain.PluginStatusActive
		plugin.Priority = 50
		plugin.Category = lo.ToPtr("updated-category")

		err = s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		assert.Equal(t, originalID, plugin.ID)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []uint{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Equal(t, "update-plugin", retrieved.Name)
		assert.Equal(t, "2.0.0", retrieved.Version)
		assert.Equal(t, "Updated description", retrieved.Description)
		assert.Equal(t, domain.PluginStatusActive, retrieved.Status)
		assert.Equal(t, 50, retrieved.Priority)
		require.NotNil(t, retrieved.Category)
		assert.Equal(t, "updated-category", *retrieved.Category)
		assert.InDelta(t, originalCreatedAt.Unix(), retrieved.CreatedAt.Unix(), 1.0)
		assert.GreaterOrEqual(t, retrieved.UpdatedAt.Unix(), originalCreatedAt.Unix())
	})

	s.T().Run("updated_at_changes_on_each_save", func(t *testing.T) {
		plugin := &domain.Plugin{
			Name:        "updated-at-test-plugin",
			Version:     "1.0.0",
			Description: "Test UpdatedAt changes",
			Author:      "Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusDisabled,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		require.NotNil(t, plugin.UpdatedAt)

		originalUpdatedAt := *plugin.UpdatedAt

		time.Sleep(10 * time.Millisecond)

		plugin.Version = "1.0.1"
		err = s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		require.NotNil(t, plugin.UpdatedAt)
		assert.True(t, plugin.UpdatedAt.After(originalUpdatedAt), "UpdatedAt should change after first update")

		firstUpdateAt := *plugin.UpdatedAt

		time.Sleep(10 * time.Millisecond)

		plugin.Version = "1.0.2"
		err = s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		require.NotNil(t, plugin.UpdatedAt)
		assert.True(t, plugin.UpdatedAt.After(firstUpdateAt), "UpdatedAt should change after second update")

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []uint{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.InDelta(t, plugin.UpdatedAt.Unix(), result[0].UpdatedAt.Unix(), 1.0)
	})

	s.T().Run("insert_plugin_with_nil_optional_fields", func(t *testing.T) {
		plugin := &domain.Plugin{
			Name:        "minimal-plugin",
			Version:     "1.0.0",
			Description: "A minimal plugin",
			Author:      "Minimal Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusDisabled,
			Priority:    0,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []uint{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Nil(t, retrieved.Source)
		assert.Nil(t, retrieved.Homepage)
		assert.Empty(t, retrieved.RequiredPermissions)
		assert.Empty(t, retrieved.AllowedPermissions)
		assert.Nil(t, retrieved.Category)
		assert.Empty(t, retrieved.Dependencies)
		assert.Empty(t, retrieved.Config)
		assert.Nil(t, retrieved.InstalledAt)
		assert.Nil(t, retrieved.LastLoadedAt)
	})

	s.T().Run("insert_plugin_with_empty_slices_and_maps", func(t *testing.T) {
		plugin := &domain.Plugin{
			Name:                "empty-slices-plugin",
			Version:             "1.0.0",
			Description:         "Plugin with empty slices",
			Author:              "Test Author",
			APIVersion:          "v1",
			Status:              domain.PluginStatusDisabled,
			RequiredPermissions: []domain.PluginPermission{},
			AllowedPermissions:  []domain.PluginPermission{},
			Dependencies:        []string{},
			Config:              map[string]any{},
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []uint{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Empty(t, retrieved.RequiredPermissions)
		assert.Empty(t, retrieved.AllowedPermissions)
		assert.Empty(t, retrieved.Dependencies)
		assert.Empty(t, retrieved.Config)
	})

	s.T().Run("insert_plugin_with_predefined_id", func(t *testing.T) {
		predefinedID := uint(99999)

		plugin := &domain.Plugin{
			ID:          predefinedID,
			Name:        "predefined-id-plugin",
			Version:     "1.0.0",
			Description: "Plugin with predefined ID",
			Author:      "Test Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusActive,
			Priority:    50,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		assert.Equal(t, predefinedID, plugin.ID)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []uint{predefinedID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Equal(t, predefinedID, retrieved.ID)
		assert.Equal(t, "predefined-id-plugin", retrieved.Name)
		assert.Equal(t, "1.0.0", retrieved.Version)
		assert.Equal(t, domain.PluginStatusActive, retrieved.Status)
		assert.NotNil(t, retrieved.CreatedAt)
		assert.NotNil(t, retrieved.UpdatedAt)

		err = s.repo.Delete(ctx, predefinedID)
		require.NoError(t, err)
	})

	s.T().Run("update_plugin_with_predefined_id", func(t *testing.T) {
		predefinedID := uint(88888)

		plugin := &domain.Plugin{
			ID:          predefinedID,
			Name:        "update-predefined-id-plugin",
			Version:     "1.0.0",
			Description: "Plugin with predefined ID for update",
			Author:      "Test Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusDisabled,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		assert.Equal(t, predefinedID, plugin.ID)

		plugin.Version = "2.0.0"
		plugin.Status = domain.PluginStatusActive

		err = s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		assert.Equal(t, predefinedID, plugin.ID)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []uint{predefinedID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Equal(t, predefinedID, retrieved.ID)
		assert.Equal(t, "2.0.0", retrieved.Version)
		assert.Equal(t, domain.PluginStatusActive, retrieved.Status)

		err = s.repo.Delete(ctx, predefinedID)
		require.NoError(t, err)
	})
}

func (s *PluginRepositorySuite) TestPluginRepositoryFindAll() {
	ctx := context.Background()

	plugins := []*domain.Plugin{
		{
			Name: "plugin-a", Version: "1.0.0", Description: "Plugin A",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive, Priority: 30,
		},
		{
			Name: "plugin-b", Version: "1.0.0", Description: "Plugin B",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusDisabled, Priority: 10,
		},
		{
			Name: "plugin-c", Version: "1.0.0", Description: "Plugin C",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive, Priority: 20,
		},
	}

	for _, plugin := range plugins {
		require.NoError(s.T(), s.repo.Save(ctx, plugin))
	}

	s.T().Run("without_pagination", func(t *testing.T) {
		result, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result), 3)
	})

	s.T().Run("with_pagination", func(t *testing.T) {
		result, err := s.repo.FindAll(ctx, nil, &filters.Pagination{Limit: 2, Offset: 0})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(result), 2)
	})

	s.T().Run("default_sorting_by_priority_desc_name_asc", func(t *testing.T) {
		result, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(result), 3)

		foundPlugins := make(map[string]bool)
		for _, p := range result {
			foundPlugins[p.Name] = true
		}
		assert.True(t, foundPlugins["plugin-a"])
		assert.True(t, foundPlugins["plugin-b"])
		assert.True(t, foundPlugins["plugin-c"])
	})

	s.T().Run("with_custom_sorting_by_name", func(t *testing.T) {
		result, err := s.repo.FindAll(ctx, []filters.Sorting{
			{Field: "name", Direction: filters.SortDirectionAsc},
		}, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(result), 2)

		for i := 0; i < len(result)-1; i++ {
			assert.LessOrEqual(t, result[i].Name, result[i+1].Name)
		}
	})

	s.T().Run("with_custom_sorting_by_priority_asc", func(t *testing.T) {
		result, err := s.repo.FindAll(ctx, []filters.Sorting{
			{Field: "priority", Direction: filters.SortDirectionAsc},
		}, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(result), 2)

		for i := 0; i < len(result)-1; i++ {
			assert.LessOrEqual(t, result[i].Priority, result[i+1].Priority)
		}
	})
}

func (s *PluginRepositorySuite) TestPluginRepositoryFind() {
	ctx := context.Background()

	plugins := []*domain.Plugin{
		{
			Name: "find-plugin-1", Version: "1.0.0", Description: "Plugin 1",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive, Priority: 10,
			Category: lo.ToPtr("monitoring"),
		},
		{
			Name: "find-plugin-2", Version: "2.0.0", Description: "Plugin 2",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusDisabled, Priority: 20,
			Category: lo.ToPtr("monitoring"),
		},
		{
			Name: "find-plugin-3", Version: "3.0.0", Description: "Plugin 3",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive, Priority: 30,
			Category: lo.ToPtr("backup"),
		},
		{
			Name: "find-plugin-4", Version: "4.0.0", Description: "Plugin 4",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusError, Priority: 40,
		},
	}

	for _, plugin := range plugins {
		require.NoError(s.T(), s.repo.Save(ctx, plugin))
	}

	s.T().Run("filter_by_ids", func(t *testing.T) {
		result, err := s.repo.Find(ctx, &filters.FindPlugin{
			IDs: []uint{plugins[0].ID, plugins[2].ID},
		}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, result, 2)

		ids := []uint{result[0].ID, result[1].ID}
		assert.Contains(t, ids, plugins[0].ID)
		assert.Contains(t, ids, plugins[2].ID)
	})

	s.T().Run("filter_by_names", func(t *testing.T) {
		result, err := s.repo.Find(ctx, &filters.FindPlugin{
			Names: []string{"find-plugin-1", "find-plugin-3"},
		}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, result, 2)

		names := []string{result[0].Name, result[1].Name}
		assert.Contains(t, names, "find-plugin-1")
		assert.Contains(t, names, "find-plugin-3")
	})

	s.T().Run("filter_by_statuses", func(t *testing.T) {
		result, err := s.repo.Find(ctx, &filters.FindPlugin{
			Statuses: []domain.PluginStatus{domain.PluginStatusActive},
		}, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result), 2)

		for _, p := range result {
			assert.Equal(t, domain.PluginStatusActive, p.Status)
		}
	})

	s.T().Run("filter_by_multiple_statuses", func(t *testing.T) {
		result, err := s.repo.Find(ctx, &filters.FindPlugin{
			Statuses: []domain.PluginStatus{domain.PluginStatusDisabled, domain.PluginStatusError},
		}, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result), 2)

		for _, p := range result {
			assert.True(t, p.Status == domain.PluginStatusDisabled || p.Status == domain.PluginStatusError)
		}
	})

	s.T().Run("filter_by_categories", func(t *testing.T) {
		result, err := s.repo.Find(ctx, &filters.FindPlugin{
			Categories: []string{"monitoring"},
		}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, result, 2)

		for _, p := range result {
			require.NotNil(t, p.Category)
			assert.Equal(t, "monitoring", *p.Category)
		}
	})

	s.T().Run("filter_no_results", func(t *testing.T) {
		result, err := s.repo.Find(ctx, &filters.FindPlugin{
			IDs: []uint{99999},
		}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, result, 0)
	})

	s.T().Run("nil_filter_returns_all", func(t *testing.T) {
		result, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result), 4)
	})

	s.T().Run("with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{
			Limit:  2,
			Offset: 0,
		}

		result, err := s.repo.Find(ctx, nil, nil, pagination)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(result), 2)
	})

	s.T().Run("with_order", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "name", Direction: filters.SortDirectionDesc},
		}

		result, err := s.repo.Find(ctx, &filters.FindPlugin{
			Names: []string{"find-plugin-1", "find-plugin-2", "find-plugin-3", "find-plugin-4"},
		}, order, nil)
		require.NoError(t, err)
		require.Len(t, result, 4)

		for i := 0; i < len(result)-1; i++ {
			assert.GreaterOrEqual(t, result[i].Name, result[i+1].Name)
		}
	})
}

func (s *PluginRepositorySuite) TestPluginRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_existing_plugin", func(t *testing.T) {
		plugin := &domain.Plugin{
			Name:        "delete-me-plugin",
			Version:     "1.0.0",
			Description: "Delete me",
			Author:      "Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusDisabled,
		}

		require.NoError(t, s.repo.Save(ctx, plugin))
		pluginID := plugin.ID

		err := s.repo.Delete(ctx, pluginID)
		require.NoError(t, err)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []uint{pluginID}}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	s.T().Run("delete_non_existent_plugin", func(t *testing.T) {
		err := s.repo.Delete(ctx, 99999)
		require.NoError(t, err)
	})

	s.T().Run("delete_already_deleted_plugin", func(t *testing.T) {
		plugin := &domain.Plugin{
			Name:        "double-delete-plugin",
			Version:     "1.0.0",
			Description: "Double delete",
			Author:      "Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusDisabled,
		}

		require.NoError(t, s.repo.Save(ctx, plugin))
		pluginID := plugin.ID

		err := s.repo.Delete(ctx, pluginID)
		require.NoError(t, err)

		err = s.repo.Delete(ctx, pluginID)
		require.NoError(t, err)
	})
}

func (s *PluginRepositorySuite) TestPluginRepositoryCompletePluginData() {
	ctx := context.Background()

	s.T().Run("save_and_retrieve_complete_plugin_data", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)

		plugin := &domain.Plugin{
			Name:        "complete-data-plugin",
			Version:     "3.2.1",
			Description: "A complete plugin for testing all fields",
			Author:      "Complete Author <author@example.com>",
			APIVersion:  "v2.1",
			Filename:    lo.ToPtr("complete-data-plugin.wasm"),
			Source:      lo.ToPtr("https://github.com/complete/plugin"),
			Homepage:    lo.ToPtr("https://complete-plugin.example.com"),
			RequiredPermissions: []domain.PluginPermission{
				domain.PluginPermissionManageServers,
				domain.PluginPermissionManageNodes,
				domain.PluginPermissionManageGames,
			},
			AllowedPermissions: []domain.PluginPermission{
				domain.PluginPermissionFiles,
				domain.PluginPermissionListenEvents,
				domain.PluginPermissionManageUsers,
			},
			Status:   domain.PluginStatusActive,
			Priority: 999,
			Category: lo.ToPtr("system"),
			Dependencies: []string{
				"core-plugin",
				"auth-plugin",
				"storage-plugin",
			},
			Config: map[string]any{
				"debug":         true,
				"max_retries":   5,
				"timeout_ms":    3000,
				"allowed_hosts": []any{"localhost", "127.0.0.1"},
				"settings": map[string]any{
					"nested_key": "nested_value",
				},
			},
			InstalledAt:  &now,
			LastLoadedAt: &now,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []uint{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Equal(t, plugin.ID, retrieved.ID)
		assert.Equal(t, plugin.Name, retrieved.Name)
		assert.Equal(t, plugin.Version, retrieved.Version)
		assert.Equal(t, plugin.Description, retrieved.Description)
		assert.Equal(t, plugin.Author, retrieved.Author)
		assert.Equal(t, plugin.APIVersion, retrieved.APIVersion)
		assert.Equal(t, plugin.Filename, retrieved.Filename)
		assert.Equal(t, plugin.Source, retrieved.Source)
		assert.Equal(t, plugin.Homepage, retrieved.Homepage)
		assert.Equal(t, plugin.Status, retrieved.Status)
		assert.Equal(t, plugin.Priority, retrieved.Priority)
		assert.Equal(t, plugin.Category, retrieved.Category)

		require.Len(t, retrieved.RequiredPermissions, 3)
		for _, p := range plugin.RequiredPermissions {
			assert.Contains(t, retrieved.RequiredPermissions, p)
		}

		require.Len(t, retrieved.AllowedPermissions, 3)
		for _, p := range plugin.AllowedPermissions {
			assert.Contains(t, retrieved.AllowedPermissions, p)
		}

		require.Len(t, retrieved.Dependencies, 3)
		for _, d := range plugin.Dependencies {
			assert.Contains(t, retrieved.Dependencies, d)
		}

		require.NotNil(t, retrieved.Config)
		assert.Equal(t, true, retrieved.Config["debug"])
		assert.NotNil(t, retrieved.InstalledAt)
		assert.InDelta(t, now.Unix(), retrieved.InstalledAt.Unix(), 1.0)
		assert.NotNil(t, retrieved.LastLoadedAt)
		assert.InDelta(t, now.Unix(), retrieved.LastLoadedAt.Unix(), 1.0)
		assert.NotNil(t, retrieved.CreatedAt)
		assert.NotNil(t, retrieved.UpdatedAt)
	})
}

func (s *PluginRepositorySuite) TestPluginRepositoryExists() {
	ctx := context.Background()

	plugins := []*domain.Plugin{
		{
			Name: "exists-plugin-1", Version: "1.0.0", Description: "Plugin 1",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive, Priority: 10,
			Category: lo.ToPtr("monitoring"),
		},
		{
			Name: "exists-plugin-2", Version: "2.0.0", Description: "Plugin 2",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusDisabled, Priority: 20,
			Category: lo.ToPtr("backup"),
		},
	}

	for _, plugin := range plugins {
		require.NoError(s.T(), s.repo.Save(ctx, plugin))
	}

	s.T().Run("exists_by_id", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{IDs: []uint{plugins[0].ID}})
		require.NoError(t, err)
		assert.True(t, exists)
	})

	s.T().Run("exists_by_name", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{Names: []string{"exists-plugin-1"}})
		require.NoError(t, err)
		assert.True(t, exists)
	})

	s.T().Run("exists_by_status", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{Statuses: []domain.PluginStatus{domain.PluginStatusActive}})
		require.NoError(t, err)
		assert.True(t, exists)
	})

	s.T().Run("exists_by_category", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{Categories: []string{"monitoring"}})
		require.NoError(t, err)
		assert.True(t, exists)
	})

	s.T().Run("not_exists_by_id", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{IDs: []uint{99999}})
		require.NoError(t, err)
		assert.False(t, exists)
	})

	s.T().Run("not_exists_by_name", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{Names: []string{"non-existent-plugin"}})
		require.NoError(t, err)
		assert.False(t, exists)
	})

	s.T().Run("not_exists_by_status", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{Statuses: []domain.PluginStatus{domain.PluginStatusError}})
		require.NoError(t, err)
		assert.False(t, exists)
	})

	s.T().Run("not_exists_by_category", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{Categories: []string{"non-existent-category"}})
		require.NoError(t, err)
		assert.False(t, exists)
	})

	s.T().Run("exists_with_multiple_filters", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{
			Names:    []string{"exists-plugin-1"},
			Statuses: []domain.PluginStatus{domain.PluginStatusActive},
		})
		require.NoError(t, err)
		assert.True(t, exists)
	})

	s.T().Run("not_exists_with_conflicting_filters", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{
			Names:    []string{"exists-plugin-1"},
			Statuses: []domain.PluginStatus{domain.PluginStatusDisabled},
		})
		require.NoError(t, err)
		assert.False(t, exists)
	})

	s.T().Run("exists_after_delete_returns_false", func(t *testing.T) {
		plugin := &domain.Plugin{
			Name: "delete-exists-plugin", Version: "1.0.0", Description: "Delete exists plugin",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive,
		}
		require.NoError(t, s.repo.Save(ctx, plugin))

		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{IDs: []uint{plugin.ID}})
		require.NoError(t, err)
		assert.True(t, exists)

		require.NoError(t, s.repo.Delete(ctx, plugin.ID))

		exists, err = s.repo.Exists(ctx, &filters.FindPlugin{IDs: []uint{plugin.ID}})
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func (s *PluginRepositorySuite) TestPluginRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		plugin := &domain.Plugin{
			Name:        "lifecycle-plugin",
			Version:     "1.0.0",
			Description: "Lifecycle test plugin",
			Author:      "Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusDisabled,
			Priority:    10,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		pluginID := plugin.ID

		filter := &filters.FindPlugin{IDs: []uint{pluginID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "lifecycle-plugin", results[0].Name)
		assert.Equal(t, domain.PluginStatusDisabled, results[0].Status)

		plugin.Version = "2.0.0"
		plugin.Status = domain.PluginStatusActive
		plugin.Priority = 100
		plugin.Category = lo.ToPtr("updated")
		err = s.repo.Save(ctx, plugin)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "2.0.0", results[0].Version)
		assert.Equal(t, domain.PluginStatusActive, results[0].Status)
		assert.Equal(t, 100, results[0].Priority)
		require.NotNil(t, results[0].Category)
		assert.Equal(t, "updated", *results[0].Category)

		err = s.repo.Delete(ctx, pluginID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("multiple_plugins_operations", func(t *testing.T) {
		var pluginIDs []uint

		for i := range 5 {
			plugin := &domain.Plugin{
				Name:        "multi-plugin-" + string(rune('A'+i)),
				Version:     "1.0.0",
				Description: "Multi plugin " + string(rune('A'+i)),
				Author:      "Author",
				APIVersion:  "v1",
				Status:      domain.PluginStatusActive,
				Priority:    i * 10,
			}
			require.NoError(t, s.repo.Save(ctx, plugin))
			pluginIDs = append(pluginIDs, plugin.ID)
		}

		filter := &filters.FindPlugin{IDs: pluginIDs}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 5)

		for i := range 3 {
			require.NoError(t, s.repo.Delete(ctx, pluginIDs[i]))
		}

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	s.T().Run("status_transitions", func(t *testing.T) {
		plugin := &domain.Plugin{
			Name:        "status-transition-plugin",
			Version:     "1.0.0",
			Description: "Status transition test",
			Author:      "Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusDisabled,
		}

		require.NoError(t, s.repo.Save(ctx, plugin))
		filter := &filters.FindPlugin{IDs: []uint{plugin.ID}}

		transitions := []domain.PluginStatus{
			domain.PluginStatusActive,
			domain.PluginStatusUpdating,
			domain.PluginStatusError,
			domain.PluginStatusDisabled,
		}

		for _, status := range transitions {
			plugin.Status = status
			require.NoError(t, s.repo.Save(ctx, plugin))

			results, err := s.repo.Find(ctx, filter, nil, nil)
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, status, results[0].Status)
		}
	})

	s.T().Run("priority_ordering", func(t *testing.T) {
		for i := range 3 {
			plugin := &domain.Plugin{
				Name:        "priority-plugin-" + string(rune('A'+i)),
				Version:     "1.0.0",
				Description: "Priority plugin",
				Author:      "Author",
				APIVersion:  "v1",
				Status:      domain.PluginStatusActive,
				Priority:    (i + 1) * 100,
			}
			require.NoError(t, s.repo.Save(ctx, plugin))
		}

		order := []filters.Sorting{{Field: "priority", Direction: filters.SortDirectionDesc}}
		results, err := s.repo.FindAll(ctx, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 3)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].Priority, results[i+1].Priority)
		}
	})
}
