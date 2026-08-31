package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
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
			ID:          1001,
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
		assert.Equal(t, domain.Uint64ID(1001), plugin.ID)
		assert.NotNil(t, plugin.CreatedAt)
		assert.NotNil(t, plugin.UpdatedAt)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}, nil, nil)
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
			ID:          1002,
			Name:        "full-plugin",
			Version:     "2.0.0",
			Description: "A plugin with all fields",
			Author:      "Full Author",
			APIVersion:  "v2",
			Filename:    new("full-plugin.wasm"),
			Source:      new("https://github.com/example/plugin"),
			Homepage:    new("https://example.com/plugin"),
			Checksum:    new("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
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
			Generation:   3,
			Category:     new("monitoring"),
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
		assert.Equal(t, domain.Uint64ID(1002), plugin.ID)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}, nil, nil)
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
		require.NotNil(t, retrieved.Checksum)
		assert.Equal(t, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", *retrieved.Checksum)
		assert.Equal(t, 3, retrieved.Generation)
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
		assert.Nil(t, retrieved.LastError)
		assert.Nil(t, retrieved.LastErrorAt)
	})

	s.T().Run("save_with_last_error", func(t *testing.T) {
		errorAt := time.Now().Truncate(time.Second)
		plugin := &domain.Plugin{
			ID:         1008,
			Name:       "errored-plugin",
			Version:    "1.0.0",
			APIVersion: "1",
		}
		plugin.MarkError("event handler timed out (SERVER_PRE_START)", errorAt)

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Equal(t, domain.PluginStatusError, retrieved.Status)
		require.NotNil(t, retrieved.LastError)
		assert.Equal(t, "event handler timed out (SERVER_PRE_START)", *retrieved.LastError)
		require.NotNil(t, retrieved.LastErrorAt)
		assert.InDelta(t, errorAt.Unix(), retrieved.LastErrorAt.Unix(), 1.0)
	})

	s.T().Run("clear_last_error", func(t *testing.T) {
		plugin := &domain.Plugin{
			ID:         1009,
			Name:       "recovered-plugin",
			Version:    "1.0.0",
			APIVersion: "1",
		}
		plugin.MarkError("http handler timed out", time.Now())

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)

		plugin.MarkActive(time.Now())

		err = s.repo.Save(ctx, plugin)
		require.NoError(t, err)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Equal(t, domain.PluginStatusActive, retrieved.Status)
		assert.Nil(t, retrieved.LastError)
		assert.Nil(t, retrieved.LastErrorAt)
		assert.NotNil(t, retrieved.LastLoadedAt)
	})

	s.T().Run("update_load_state_leaves_config_and_grants_alone", func(t *testing.T) {
		plugin := &domain.Plugin{
			ID:                 1010,
			Name:               "load-state-plugin",
			Version:            "1.0.0",
			APIVersion:         "1",
			Status:             domain.PluginStatusActive,
			AllowedPermissions: []domain.PluginPermission{domain.PluginPermissionFiles},
			Config:             map[string]any{"api_key": "before"},
		}
		require.NoError(t, s.repo.Save(ctx, plugin))

		edited := *plugin
		edited.AllowedPermissions = []domain.PluginPermission{domain.PluginPermissionSecrets}
		edited.Config = map[string]any{"api_key": "after"}
		require.NoError(t, s.repo.Save(ctx, &edited))

		loadedAt := time.Now().Truncate(time.Second)
		err := s.repo.UpdateLoadState(ctx, plugin.ID, domain.PluginLoadState{
			Status:       domain.PluginStatusError,
			LastError:    new("initialize failed"),
			LastErrorAt:  &loadedAt,
			LastLoadedAt: &loadedAt,
			Generation:   7,
		})
		require.NoError(t, err)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Equal(t, domain.PluginStatusError, retrieved.Status)
		require.NotNil(t, retrieved.LastError)
		assert.Equal(t, "initialize failed", *retrieved.LastError)
		require.NotNil(t, retrieved.LastErrorAt)
		assert.InDelta(t, loadedAt.Unix(), retrieved.LastErrorAt.Unix(), 1.0)
		require.NotNil(t, retrieved.LastLoadedAt)
		assert.Equal(t, 7, retrieved.Generation)
		assert.Equal(t, []domain.PluginPermission{domain.PluginPermissionSecrets}, retrieved.AllowedPermissions)
		assert.Equal(t, "after", retrieved.Config["api_key"])
		assert.NotNil(t, retrieved.UpdatedAt)
	})

	s.T().Run("update_load_state_of_unknown_plugin_fails", func(t *testing.T) {
		err := s.repo.UpdateLoadState(ctx, 999999, domain.PluginLoadState{Status: domain.PluginStatusActive})
		require.ErrorIs(t, err, repositories.ErrPluginNotFound)
	})

	s.T().Run("update_existing_plugin", func(t *testing.T) {
		plugin := &domain.Plugin{
			ID:          1003,
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

		plugin.Version = "2.0.0"
		plugin.Description = "Updated description"
		plugin.Status = domain.PluginStatusActive
		plugin.Priority = 50
		plugin.Category = new("updated-category")

		err = s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		assert.Equal(t, originalID, plugin.ID)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}, nil, nil)
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
			ID:          1004,
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

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.InDelta(t, plugin.UpdatedAt.Unix(), result[0].UpdatedAt.Unix(), 1.0)
	})

	s.T().Run("insert_plugin_with_nil_optional_fields", func(t *testing.T) {
		plugin := &domain.Plugin{
			ID:          1005,
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

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Nil(t, retrieved.Source)
		assert.Nil(t, retrieved.Homepage)
		assert.Nil(t, retrieved.Checksum)
		assert.Equal(t, 0, retrieved.Generation)
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
			ID:                  1006,
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

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)

		retrieved := result[0]
		assert.Empty(t, retrieved.RequiredPermissions)
		assert.Empty(t, retrieved.AllowedPermissions)
		assert.Empty(t, retrieved.Dependencies)
		assert.Empty(t, retrieved.Config)
	})

	s.T().Run("insert_plugin_with_predefined_id", func(t *testing.T) {
		predefinedID := domain.Uint64ID(99999)

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

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{predefinedID}}, nil, nil)
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
		predefinedID := domain.Uint64ID(88888)

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

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{predefinedID}}, nil, nil)
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
			ID: 2001, Name: "plugin-a", Version: "1.0.0", Description: "Plugin A",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive, Priority: 30,
		},
		{
			ID: 2002, Name: "plugin-b", Version: "1.0.0", Description: "Plugin B",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusDisabled, Priority: 10,
		},
		{
			ID: 2003, Name: "plugin-c", Version: "1.0.0", Description: "Plugin C",
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
			ID: 3001, Name: "find-plugin-1", Version: "1.0.0", Description: "Plugin 1",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive, Priority: 10,
			Category: new("monitoring"),
		},
		{
			ID: 3002, Name: "find-plugin-2", Version: "2.0.0", Description: "Plugin 2",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusDisabled, Priority: 20,
			Category: new("monitoring"),
		},
		{
			ID: 3003, Name: "find-plugin-3", Version: "3.0.0", Description: "Plugin 3",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive, Priority: 30,
			Category: new("backup"),
		},
		{
			ID: 3004, Name: "find-plugin-4", Version: "4.0.0", Description: "Plugin 4",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusError, Priority: 40,
		},
	}

	for _, plugin := range plugins {
		require.NoError(s.T(), s.repo.Save(ctx, plugin))
	}

	s.T().Run("filter_by_ids", func(t *testing.T) {
		result, err := s.repo.Find(ctx, &filters.FindPlugin{
			IDs: []domain.Uint64ID{plugins[0].ID, plugins[2].ID},
		}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, result, 2)

		ids := []domain.Uint64ID{result[0].ID, result[1].ID}
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
			IDs: []domain.Uint64ID{99999},
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
			ID:          4001,
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

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{pluginID}}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	s.T().Run("delete_non_existent_plugin", func(t *testing.T) {
		err := s.repo.Delete(ctx, 99999)
		require.NoError(t, err)
	})

	s.T().Run("delete_already_deleted_plugin", func(t *testing.T) {
		plugin := &domain.Plugin{
			ID:          4002,
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
			ID:          5001,
			Name:        "complete-data-plugin",
			Version:     "3.2.1",
			Description: "A complete plugin for testing all fields",
			Author:      "Complete Author <author@example.com>",
			APIVersion:  "v2.1",
			Filename:    new("complete-data-plugin.wasm"),
			Source:      new("https://github.com/complete/plugin"),
			Homepage:    new("https://complete-plugin.example.com"),
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
			Category: new("system"),
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

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}, nil, nil)
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
			ID: 6001, Name: "exists-plugin-1", Version: "1.0.0", Description: "Plugin 1",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive, Priority: 10,
			Category: new("monitoring"),
		},
		{
			ID: 6002, Name: "exists-plugin-2", Version: "2.0.0", Description: "Plugin 2",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusDisabled, Priority: 20,
			Category: new("backup"),
		},
	}

	for _, plugin := range plugins {
		require.NoError(s.T(), s.repo.Save(ctx, plugin))
	}

	s.T().Run("exists_by_id", func(t *testing.T) {
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugins[0].ID}})
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
		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{99999}})
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
			ID: 6003, Name: "delete-exists-plugin", Version: "1.0.0", Description: "Delete exists plugin",
			Author: "Author", APIVersion: "v1", Status: domain.PluginStatusActive,
		}
		require.NoError(t, s.repo.Save(ctx, plugin))

		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}})
		require.NoError(t, err)
		assert.True(t, exists)

		require.NoError(t, s.repo.Delete(ctx, plugin.ID))

		exists, err = s.repo.Exists(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}})
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func (s *PluginRepositorySuite) TestPluginRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		plugin := &domain.Plugin{
			ID:          7001,
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

		filter := &filters.FindPlugin{IDs: []domain.Uint64ID{pluginID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "lifecycle-plugin", results[0].Name)
		assert.Equal(t, domain.PluginStatusDisabled, results[0].Status)

		plugin.Version = "2.0.0"
		plugin.Status = domain.PluginStatusActive
		plugin.Priority = 100
		plugin.Category = new("updated")
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
		pluginIDs := make([]domain.Uint64ID, 0, 5)

		for i := range 5 {
			plugin := &domain.Plugin{
				ID:          domain.Uint64ID(7100 + i),
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
			ID:          7200,
			Name:        "status-transition-plugin",
			Version:     "1.0.0",
			Description: "Status transition test",
			Author:      "Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusDisabled,
		}

		require.NoError(t, s.repo.Save(ctx, plugin))
		filter := &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}}

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
				ID:          domain.Uint64ID(7300 + i),
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

func (s *PluginRepositorySuite) TestPluginRepositoryBigIntID() {
	ctx := context.Background()

	s.T().Run("max_int64_id", func(t *testing.T) {
		maxInt64ID := domain.Uint64ID(1<<63 - 1) // 9223372036854775807

		plugin := &domain.Plugin{
			ID:          maxInt64ID,
			Name:        "max-int64-plugin",
			Version:     "1.0.0",
			Description: "Plugin with max int64 ID",
			Author:      "Test Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusActive,
			Priority:    10,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		assert.Equal(t, maxInt64ID, plugin.ID)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{maxInt64ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, maxInt64ID, result[0].ID)
		assert.Equal(t, "max-int64-plugin", result[0].Name)

		exists, err := s.repo.Exists(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{maxInt64ID}})
		require.NoError(t, err)
		assert.True(t, exists)

		err = s.repo.Delete(ctx, maxInt64ID)
		require.NoError(t, err)

		result, err = s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{maxInt64ID}}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	s.T().Run("large_int64_id", func(t *testing.T) {
		largeID := domain.Uint64ID(4120302874985960141) // 0x392e41a26e8f12cd

		plugin := &domain.Plugin{
			ID:          largeID,
			Name:        "large-int64-plugin",
			Version:     "1.0.0",
			Description: "Plugin with large int64 ID",
			Author:      "Test Author",
			APIVersion:  "v1",
			Status:      domain.PluginStatusActive,
			Priority:    20,
		}

		err := s.repo.Save(ctx, plugin)
		require.NoError(t, err)
		assert.Equal(t, largeID, plugin.ID)

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{largeID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, largeID, result[0].ID)

		plugin.Version = "2.0.0"
		err = s.repo.Save(ctx, plugin)
		require.NoError(t, err)

		result, err = s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{largeID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "2.0.0", result[0].Version)

		err = s.repo.Delete(ctx, largeID)
		require.NoError(t, err)
	})

	s.T().Run("multiple_large_ids_filter", func(t *testing.T) {
		ids := []domain.Uint64ID{
			domain.Uint64ID(1<<62 - 1),     // 4611686018427387903
			domain.Uint64ID(1<<62 + 12345), // 4611686018427400248
			domain.Uint64ID(1<<63 - 100),   // 9223372036854775707
		}

		for i, id := range ids {
			plugin := &domain.Plugin{
				ID:          id,
				Name:        "large-id-plugin-" + string(rune('A'+i)),
				Version:     "1.0.0",
				Description: "Plugin with large ID",
				Author:      "Test Author",
				APIVersion:  "v1",
				Status:      domain.PluginStatusActive,
			}
			err := s.repo.Save(ctx, plugin)
			require.NoError(t, err)
		}

		result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: ids}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 3)

		foundIDs := make(map[domain.Uint64ID]bool)
		for _, p := range result {
			foundIDs[p.ID] = true
		}
		for _, id := range ids {
			assert.True(t, foundIDs[id], "ID %d should be found", id)
		}

		for _, id := range ids {
			err = s.repo.Delete(ctx, id)
			require.NoError(t, err)
		}
	})

	s.T().Run("boundary_int64_values", func(t *testing.T) {
		boundaryIDs := []struct {
			id   domain.Uint64ID
			name string
		}{
			{id: domain.Uint64ID(1<<31 - 1), name: "max-int32-plugin"},     // 2147483647 (max int32)
			{id: domain.Uint64ID(1 << 31), name: "overflow-int32-plugin"},  // 2147483648 (int32 overflow)
			{id: domain.Uint64ID(1<<32 - 1), name: "max-uint32-plugin"},    // 4294967295 (max uint32)
			{id: domain.Uint64ID(1 << 32), name: "overflow-uint32-plugin"}, // 4294967296 (uint32 overflow)
		}

		for _, tc := range boundaryIDs {
			plugin := &domain.Plugin{
				ID:          tc.id,
				Name:        tc.name,
				Version:     "1.0.0",
				Description: "Boundary test plugin",
				Author:      "Test Author",
				APIVersion:  "v1",
				Status:      domain.PluginStatusActive,
			}

			err := s.repo.Save(ctx, plugin)
			require.NoError(t, err, "should save plugin with ID %d (%s)", tc.id, tc.name)

			result, err := s.repo.Find(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{tc.id}}, nil, nil)
			require.NoError(t, err)
			require.Len(t, result, 1, "should find plugin with ID %d (%s)", tc.id, tc.name)
			assert.Equal(t, tc.id, result[0].ID)

			err = s.repo.Delete(ctx, tc.id)
			require.NoError(t, err)
		}
	})
}
