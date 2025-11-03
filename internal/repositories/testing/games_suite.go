package testing

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type GameRepositorySuite struct {
	suite.Suite

	repo repositories.GameRepository
	fn   func(t *testing.T) repositories.GameRepository
}

func NewGameRepositorySuite(fn func(t *testing.T) repositories.GameRepository) *GameRepositorySuite {
	return &GameRepositorySuite{
		fn: fn,
	}
}

func (s *GameRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *GameRepositorySuite) TestGameRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_game", func(t *testing.T) {
		game := &domain.Game{
			Code:          "csgo",
			Name:          "Counter-Strike: Global Offensive",
			Engine:        "source",
			EngineVersion: "1",
			Enabled:       1,
		}

		err := s.repo.Save(ctx, game)
		require.NoError(t, err)
	})

	s.T().Run("update_existing_game", func(t *testing.T) {
		game := &domain.Game{
			Code:          "css",
			Name:          "Counter-Strike: Source",
			Engine:        "source",
			EngineVersion: "1",
			Enabled:       1,
		}

		err := s.repo.Save(ctx, game)
		require.NoError(t, err)

		game.Name = "CS: Source Updated"
		game.Enabled = 0
		err = s.repo.Save(ctx, game)
		require.NoError(t, err)

		games, err := s.repo.Find(ctx, &filters.FindGame{Codes: []string{"css"}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, games, 1)
		assert.Equal(t, "CS: Source Updated", games[0].Name)
		assert.Equal(t, 0, games[0].Enabled)
	})

	s.T().Run("save_with_steam_app_ids", func(t *testing.T) {
		game := &domain.Game{
			Code:              "tf2",
			Name:              "Team Fortress 2",
			Engine:            "source",
			EngineVersion:     "1",
			SteamAppIDLinux:   lo.ToPtr(uint(440)),
			SteamAppIDWindows: lo.ToPtr(uint(440)),
			SteamAppSetConfig: lo.ToPtr("90 mod tf"),
			Enabled:           1,
		}

		err := s.repo.Save(ctx, game)
		require.NoError(t, err)

		games, err := s.repo.Find(ctx, &filters.FindGame{Codes: []string{"tf2"}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, games, 1)
		assert.Equal(t, lo.ToPtr(uint(440)), games[0].SteamAppIDLinux)
		assert.Equal(t, lo.ToPtr(uint(440)), games[0].SteamAppIDWindows)
		assert.Equal(t, lo.ToPtr("90 mod tf"), games[0].SteamAppSetConfig)
	})
}

func (s *GameRepositorySuite) TestGameRepositoryFindAll() {
	ctx := context.Background()

	games := []*domain.Game{
		{Code: "csgo", Name: "Counter-Strike: Global Offensive", Engine: "source", EngineVersion: "1", Enabled: 1},
		{Code: "css", Name: "Counter-Strike: Source", Engine: "source", EngineVersion: "1", Enabled: 1},
		{Code: "tf2", Name: "Team Fortress 2", Engine: "source", EngineVersion: "1", Enabled: 1},
	}

	for _, game := range games {
		require.NoError(s.T(), s.repo.Save(ctx, game))
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

	s.T().Run("default_sorting_by_name", func(t *testing.T) {
		result, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(result), 3)

		foundCodes := make(map[string]bool)
		for _, game := range result {
			foundCodes[game.Code] = true
		}
		assert.True(t, foundCodes["csgo"])
		assert.True(t, foundCodes["css"])
		assert.True(t, foundCodes["tf2"])
	})

	s.T().Run("with_custom_sorting", func(t *testing.T) {
		result, err := s.repo.FindAll(ctx, []filters.Sorting{{Field: "code", Direction: filters.SortDirectionDesc}}, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(result), 2)

		for i := 0; i < len(result)-1; i++ {
			assert.GreaterOrEqual(t, result[i].Code, result[i+1].Code)
		}
	})
}

func (s *GameRepositorySuite) TestGameRepositoryFind() {
	ctx := context.Background()

	games := []*domain.Game{
		{Code: "csgo_find", Name: "Counter-Strike: Global Offensive", Engine: "source", EngineVersion: "1", Enabled: 1},
		{Code: "css_find", Name: "Counter-Strike: Source", Engine: "source", EngineVersion: "1", Enabled: 1},
		{Code: "minecraft_find", Name: "Minecraft", Engine: "minecraft", EngineVersion: "1", Enabled: 0},
	}

	for _, game := range games {
		require.NoError(s.T(), s.repo.Save(ctx, game))
	}

	s.T().Run("filter_by_codes", func(t *testing.T) {
		result, err := s.repo.Find(ctx, &filters.FindGame{Codes: []string{"csgo_find", "css_find"}}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	s.T().Run("filter_by_single_code", func(t *testing.T) {
		result, err := s.repo.Find(ctx, &filters.FindGame{Codes: []string{"minecraft_find"}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "Minecraft", result[0].Name)
		assert.Equal(t, "minecraft", result[0].Engine)
		assert.Equal(t, 0, result[0].Enabled)
	})

	s.T().Run("filter_no_results", func(t *testing.T) {
		result, err := s.repo.Find(ctx, &filters.FindGame{Codes: []string{"nonexistent"}}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, result, 0)
	})

	s.T().Run("nil_filter_returns_all", func(t *testing.T) {
		result, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(result), 3)
	})

	s.T().Run("with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{
			Limit:  2,
			Offset: 0,
		}

		result, err := s.repo.Find(ctx, &filters.FindGame{
			Codes: []string{"csgo_find", "css_find", "minecraft_find"}}, nil, pagination,
		)
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	s.T().Run("with_order", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "code", Direction: filters.SortDirectionDesc},
		}

		result, err := s.repo.Find(ctx, &filters.FindGame{
			Codes: []string{"csgo_find", "css_find", "minecraft_find"}}, order, nil,
		)
		require.NoError(t, err)
		require.Len(t, result, 3)

		for i := 0; i < len(result)-1; i++ {
			assert.GreaterOrEqual(t, result[i].Code, result[i+1].Code)
		}
	})
}

func (s *GameRepositorySuite) TestGameRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_existing_game", func(t *testing.T) {
		game := &domain.Game{
			Code:          "deleteme",
			Name:          "Delete Me Game",
			Engine:        "test",
			EngineVersion: "1",
			Enabled:       1,
		}

		require.NoError(t, s.repo.Save(ctx, game))
		err := s.repo.Delete(ctx, game.Code)
		require.NoError(t, err)

		result, err := s.repo.Find(ctx, &filters.FindGame{Codes: []string{game.Code}}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, result, 0)
	})

	s.T().Run("delete_non_existent_game", func(t *testing.T) {
		err := s.repo.Delete(ctx, "nonexistent")
		require.NoError(t, err)
	})

	s.T().Run("delete_already_deleted_game", func(t *testing.T) {
		game := &domain.Game{
			Code:          "double_delete",
			Name:          "Double Delete Game",
			Engine:        "test",
			EngineVersion: "1",
			Enabled:       1,
		}

		require.NoError(t, s.repo.Save(ctx, game))
		gameCode := game.Code

		err := s.repo.Delete(ctx, gameCode)
		require.NoError(t, err)

		err = s.repo.Delete(ctx, gameCode)
		require.NoError(t, err)
	})
}

func (s *GameRepositorySuite) TestGameRepositoryCompleteGameData() {
	ctx := context.Background()

	s.T().Run("save_and_retrieve_complete_game_data", func(t *testing.T) {
		game := &domain.Game{
			Code:                    "hl2dm",
			Name:                    "Half-Life 2: Deathmatch",
			Engine:                  "source",
			EngineVersion:           "1",
			SteamAppIDLinux:         lo.ToPtr(uint(320)),
			SteamAppIDWindows:       lo.ToPtr(uint(320)),
			SteamAppSetConfig:       lo.ToPtr("90 mod hl2mp"),
			RemoteRepositoryLinux:   lo.ToPtr("https://example.com/hl2dm-linux"),
			RemoteRepositoryWindows: lo.ToPtr("https://example.com/hl2dm-windows"),
			LocalRepositoryLinux:    lo.ToPtr("/srv/games/hl2dm-linux"),
			LocalRepositoryWindows:  lo.ToPtr("C:\\games\\hl2dm-windows"),
			Enabled:                 1,
		}

		err := s.repo.Save(ctx, game)
		require.NoError(t, err)

		games, err := s.repo.Find(ctx, &filters.FindGame{Codes: []string{"hl2dm"}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, games, 1)

		retrieved := games[0]
		assert.Equal(t, game.Code, retrieved.Code)
		assert.Equal(t, game.Name, retrieved.Name)
		assert.Equal(t, game.Engine, retrieved.Engine)
		assert.Equal(t, game.EngineVersion, retrieved.EngineVersion)
		assert.Equal(t, game.SteamAppIDLinux, retrieved.SteamAppIDLinux)
		assert.Equal(t, game.SteamAppIDWindows, retrieved.SteamAppIDWindows)
		assert.Equal(t, game.SteamAppSetConfig, retrieved.SteamAppSetConfig)
		assert.Equal(t, game.RemoteRepositoryLinux, retrieved.RemoteRepositoryLinux)
		assert.Equal(t, game.RemoteRepositoryWindows, retrieved.RemoteRepositoryWindows)
		assert.Equal(t, game.LocalRepositoryLinux, retrieved.LocalRepositoryLinux)
		assert.Equal(t, game.LocalRepositoryWindows, retrieved.LocalRepositoryWindows)
		assert.Equal(t, game.Enabled, retrieved.Enabled)
	})
}

func (s *GameRepositorySuite) TestGameRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		game := &domain.Game{
			Code:          "lifecycle_test",
			Name:          "Lifecycle Test Game",
			Engine:        "test_engine",
			EngineVersion: "1.0",
			Enabled:       1,
		}

		err := s.repo.Save(ctx, game)
		require.NoError(t, err)

		filter := &filters.FindGame{
			Codes: []string{game.Code},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, game.Name, results[0].Name)

		game.Name = "Updated Lifecycle Test Game"
		game.Enabled = 0
		err = s.repo.Save(ctx, game)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Updated Lifecycle Test Game", results[0].Name)
		assert.Equal(t, 0, results[0].Enabled)

		err = s.repo.Delete(ctx, game.Code)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("multiple_games_operations", func(t *testing.T) {
		var gameCodes []string
		for i := range 5 {
			game := &domain.Game{
				Code:          "multi_game_" + string(rune('A'+i)),
				Name:          "Multi Game " + string(rune('A'+i)),
				Engine:        "test",
				EngineVersion: "1",
				Enabled:       1,
			}
			require.NoError(t, s.repo.Save(ctx, game))
			gameCodes = append(gameCodes, game.Code)
		}

		filter := &filters.FindGame{
			Codes: gameCodes,
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 5)

		for i := range 3 {
			require.NoError(t, s.repo.Delete(ctx, gameCodes[i]))
		}

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})
}
