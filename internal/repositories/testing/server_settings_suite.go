package testing

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ServerSettingRepositorySuite struct {
	suite.Suite

	repo repositories.ServerSettingRepository
	fn   func(t *testing.T) repositories.ServerSettingRepository
}

func NewServerSettingRepositorySuite(fn func(t *testing.T) repositories.ServerSettingRepository) *ServerSettingRepositorySuite {
	return &ServerSettingRepositorySuite{
		fn: fn,
	}
}

func (s *ServerSettingRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *ServerSettingRepositorySuite) TestServerSettingRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_setting_with_string_value", func(t *testing.T) {
		setting := &domain.ServerSetting{
			Name:     "hostname",
			ServerID: 1,
			Value:    domain.NewServerSettingValue("Test Server"),
		}

		err := s.repo.Save(ctx, setting)
		require.NoError(t, err)
		assert.NotZero(t, setting.ID)
	})

	s.T().Run("insert_new_setting_with_bool_value", func(t *testing.T) {
		setting := &domain.ServerSetting{
			Name:     "auto_start",
			ServerID: 1,
			Value:    domain.NewServerSettingValue(true),
		}

		err := s.repo.Save(ctx, setting)
		require.NoError(t, err)
		assert.NotZero(t, setting.ID)
	})

	s.T().Run("update_existing_setting", func(t *testing.T) {
		setting := &domain.ServerSetting{
			Name:     "max_players",
			ServerID: 2,
			Value:    domain.NewServerSettingValue("32"),
		}

		err := s.repo.Save(ctx, setting)
		require.NoError(t, err)
		originalID := setting.ID

		setting.Value = domain.NewServerSettingValue("64")

		err = s.repo.Save(ctx, setting)
		require.NoError(t, err)
		assert.Equal(t, originalID, setting.ID)

		filter := &filters.FindServerSetting{IDs: []uint{setting.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		val, ok := results[0].Value.String()
		require.True(t, ok)
		assert.Equal(t, "64", val)
	})

	s.T().Run("save_multiple_settings_for_same_server", func(t *testing.T) {
		serverID := uint(10)

		setting1 := &domain.ServerSetting{
			Name:     "map",
			ServerID: serverID,
			Value:    domain.NewServerSettingValue("de_dust2"),
		}
		setting2 := &domain.ServerSetting{
			Name:     "maxplayers",
			ServerID: serverID,
			Value:    domain.NewServerSettingValue("16"),
		}

		require.NoError(t, s.repo.Save(ctx, setting1))
		require.NoError(t, s.repo.Save(ctx, setting2))
		assert.NotEqual(t, setting1.ID, setting2.ID)

		filter := &filters.FindServerSetting{ServerIDs: []uint{serverID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})
}

func (s *ServerSettingRepositorySuite) TestServerSettingRepositoryFind() {
	ctx := context.Background()

	setting1 := &domain.ServerSetting{
		Name:     "hostname",
		ServerID: 100,
		Value:    domain.NewServerSettingValue("Server 1"),
	}
	setting2 := &domain.ServerSetting{
		Name:     "map",
		ServerID: 100,
		Value:    domain.NewServerSettingValue("de_dust2"),
	}
	setting3 := &domain.ServerSetting{
		Name:     "hostname",
		ServerID: 200,
		Value:    domain.NewServerSettingValue("Server 2"),
	}

	require.NoError(s.T(), s.repo.Save(ctx, setting1))
	require.NoError(s.T(), s.repo.Save(ctx, setting2))
	require.NoError(s.T(), s.repo.Save(ctx, setting3))

	s.T().Run("find_by_single_id", func(t *testing.T) {
		filter := &filters.FindServerSetting{
			IDs: []uint{setting1.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, setting1.ID, results[0].ID)
		assert.Equal(t, "hostname", results[0].Name)
	})

	s.T().Run("find_by_multiple_ids", func(t *testing.T) {
		filter := &filters.FindServerSetting{
			IDs: []uint{setting1.ID, setting3.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, setting1.ID)
		assert.Contains(t, ids, setting3.ID)
	})

	s.T().Run("find_by_server_id", func(t *testing.T) {
		filter := &filters.FindServerSetting{
			ServerIDs: []uint{100},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		for _, result := range results {
			assert.Equal(t, uint(100), result.ServerID)
		}
	})

	s.T().Run("find_by_name", func(t *testing.T) {
		filter := &filters.FindServerSetting{
			Names: []string{"hostname"},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		for _, result := range results {
			assert.Equal(t, "hostname", result.Name)
		}
	})

	s.T().Run("find_by_server_id_and_name", func(t *testing.T) {
		filter := &filters.FindServerSetting{
			ServerIDs: []uint{100},
			Names:     []string{"map"},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "map", results[0].Name)
		assert.Equal(t, uint(100), results[0].ServerID)
	})

	s.T().Run("find_with_nil_filter", func(t *testing.T) {
		results, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_non_existent", func(t *testing.T) {
		filter := &filters.FindServerSetting{
			IDs: []uint{99999},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		filter := &filters.FindServerSetting{
			ServerIDs: []uint{100},
		}
		pagination := &filters.Pagination{
			Limit:  1,
			Offset: 0,
		}

		results, err := s.repo.Find(ctx, filter, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	s.T().Run("find_with_order", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.Find(ctx, nil, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 2)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})
}

func (s *ServerSettingRepositorySuite) TestServerSettingRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_existing_setting", func(t *testing.T) {
		setting := &domain.ServerSetting{
			Name:     "delete_me",
			ServerID: 1000,
			Value:    domain.NewServerSettingValue("value"),
		}

		require.NoError(t, s.repo.Save(ctx, setting))
		settingID := setting.ID

		err := s.repo.Delete(ctx, settingID)
		require.NoError(t, err)

		filter := &filters.FindServerSetting{
			IDs: []uint{settingID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("delete_non_existent_setting", func(t *testing.T) {
		err := s.repo.Delete(ctx, 99999)
		require.NoError(t, err)
	})

	s.T().Run("delete_already_deleted_setting", func(t *testing.T) {
		setting := &domain.ServerSetting{
			Name:     "double_delete",
			ServerID: 1001,
			Value:    domain.NewServerSettingValue("value"),
		}

		require.NoError(t, s.repo.Save(ctx, setting))
		settingID := setting.ID

		err := s.repo.Delete(ctx, settingID)
		require.NoError(t, err)

		err = s.repo.Delete(ctx, settingID)
		require.NoError(t, err)
	})
}

func (s *ServerSettingRepositorySuite) TestServerSettingRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		serverID := uint(5000)

		setting := &domain.ServerSetting{
			Name:     "lifecycle_setting",
			ServerID: serverID,
			Value:    domain.NewServerSettingValue("initial_value"),
		}

		err := s.repo.Save(ctx, setting)
		require.NoError(t, err)
		assert.NotZero(t, setting.ID)

		filter := &filters.FindServerSetting{
			IDs: []uint{setting.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "lifecycle_setting", results[0].Name)
		val, ok := results[0].Value.String()
		require.True(t, ok)
		assert.Equal(t, "initial_value", val)

		setting.Value = domain.NewServerSettingValue("updated_value")
		err = s.repo.Save(ctx, setting)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		val2, ok2 := results[0].Value.String()
		require.True(t, ok2)
		assert.Equal(t, "updated_value", val2)

		err = s.repo.Delete(ctx, setting.ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("multiple_settings_per_server", func(t *testing.T) {
		serverID := uint(6000)

		settings := []*domain.ServerSetting{
			{Name: "hostname", ServerID: serverID, Value: domain.NewServerSettingValue("My Server")},
			{Name: "map", ServerID: serverID, Value: domain.NewServerSettingValue("de_dust2")},
			{Name: "maxplayers", ServerID: serverID, Value: domain.NewServerSettingValue("32")},
			{Name: "password", ServerID: serverID, Value: domain.NewServerSettingValue("secret")},
		}

		for _, setting := range settings {
			require.NoError(t, s.repo.Save(ctx, setting))
		}

		filter := &filters.FindServerSetting{
			ServerIDs: []uint{serverID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 4)

		err = s.repo.Delete(ctx, settings[1].ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		names := make(map[string]bool)
		for _, result := range results {
			names[result.Name] = true
		}
		assert.True(t, names["hostname"])
		assert.False(t, names["map"])
		assert.True(t, names["maxplayers"])
		assert.True(t, names["password"])
	})

	s.T().Run("settings_across_multiple_servers", func(t *testing.T) {
		server1ID := uint(7000)
		server2ID := uint(7001)
		server3ID := uint(7002)

		settings := []*domain.ServerSetting{
			{Name: "hostname", ServerID: server1ID, Value: domain.NewServerSettingValue("Server 1")},
			{Name: "hostname", ServerID: server2ID, Value: domain.NewServerSettingValue("Server 2")},
			{Name: "hostname", ServerID: server3ID, Value: domain.NewServerSettingValue("Server 3")},
			{Name: "map", ServerID: server1ID, Value: domain.NewServerSettingValue("de_dust2")},
			{Name: "map", ServerID: server2ID, Value: domain.NewServerSettingValue("de_inferno")},
		}

		for _, setting := range settings {
			require.NoError(t, s.repo.Save(ctx, setting))
		}

		filter := &filters.FindServerSetting{
			ServerIDs: []uint{server1ID, server2ID, server3ID},
			Names:     []string{"hostname"},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		filter = &filters.FindServerSetting{
			ServerIDs: []uint{server1ID, server2ID},
		}
		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 4)

		filter = &filters.FindServerSetting{
			ServerIDs: []uint{server1ID},
			Names:     []string{"map"},
		}
		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		val, ok := results[0].Value.String()
		require.True(t, ok)
		assert.Equal(t, "de_dust2", val)
	})
}
