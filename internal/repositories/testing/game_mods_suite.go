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

type GameModRepositorySuite struct {
	suite.Suite

	repo repositories.GameModRepository
	fn   func(t *testing.T) repositories.GameModRepository
}

func NewGameModRepositorySuite(fn func(t *testing.T) repositories.GameModRepository) *GameModRepositorySuite {
	return &GameModRepositorySuite{
		fn: fn,
	}
}

func (s *GameModRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *GameModRepositorySuite) TestGameModRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_game_mod", func(t *testing.T) {
		gameMod := &domain.GameMod{
			GameCode: "cssource",
			Name:     "Counter-Strike: Source",
			FastRcon: domain.GameModFastRconList{
				{Info: "Status", Command: "status"},
				{Info: "Players", Command: "players"},
			},
			Vars: domain.GameModVarList{
				{Var: "hostname", Default: "My Server", Info: "Server name", AdminVar: false},
				{Var: "rcon_password", Default: "", Info: "RCON password", AdminVar: true},
			},
			RemoteRepositoryLinux:   lo.ToPtr("https://example.com/css_linux"),
			RemoteRepositoryWindows: lo.ToPtr("https://example.com/css_windows"),
			StartCmdLinux:           lo.ToPtr("./srcds_run -game cstrike"),
			StartCmdWindows:         lo.ToPtr("srcds.exe -game cstrike"),
			KickCmd:                 lo.ToPtr("kick {player}"),
			BanCmd:                  lo.ToPtr("ban {player}"),
			ChnameCmd:               lo.ToPtr("hostname {name}"),
			SrestartCmd:             lo.ToPtr("restart"),
			ChmapCmd:                lo.ToPtr("changelevel {map}"),
			SendmsgCmd:              lo.ToPtr("say {message}"),
			PasswdCmd:               lo.ToPtr("sv_password {password}"),
		}

		err := s.repo.Save(ctx, gameMod)
		require.NoError(t, err)
		assert.NotZero(t, gameMod.ID)
	})

	s.T().Run("update_existing_game_mod", func(t *testing.T) {
		gameMod := &domain.GameMod{
			GameCode: "csgo",
			Name:     "Counter-Strike: Global Offensive",
			FastRcon: domain.GameModFastRconList{
				{Info: "Status", Command: "status"},
			},
			Vars:            domain.GameModVarList{},
			StartCmdLinux:   lo.ToPtr("./csgo_linux"),
			StartCmdWindows: lo.ToPtr("csgo.exe"),
		}

		err := s.repo.Save(ctx, gameMod)
		require.NoError(t, err)
		originalID := gameMod.ID

		gameMod.Name = "CS:GO Updated"
		gameMod.FastRcon = append(gameMod.FastRcon, domain.GameModFastRcon{Info: "Version", Command: "version"})
		gameMod.Vars = append(gameMod.Vars, domain.GameModVar{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true})
		gameMod.KickCmd = lo.ToPtr("kickid {player}")

		err = s.repo.Save(ctx, gameMod)
		require.NoError(t, err)
		assert.Equal(t, originalID, gameMod.ID)

		filter := &filters.FindGameMod{
			IDs: []uint{gameMod.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "CS:GO Updated", results[0].Name)
		assert.Len(t, results[0].FastRcon, 2)
		assert.Len(t, results[0].Vars, 1)
		assert.Equal(t, "kickid {player}", *results[0].KickCmd)
	})

	s.T().Run("save_game_mod_with_nil_fields", func(t *testing.T) {
		gameMod := &domain.GameMod{
			GameCode: "minecraft",
			Name:     "Minecraft",
			FastRcon: nil,
			Vars:     nil,
		}

		err := s.repo.Save(ctx, gameMod)
		require.NoError(t, err)
		assert.NotZero(t, gameMod.ID)
	})

	s.T().Run("save_game_mod_with_empty_arrays", func(t *testing.T) {
		gameMod := &domain.GameMod{
			GameCode: "rust",
			Name:     "Rust",
			FastRcon: domain.GameModFastRconList{},
			Vars:     domain.GameModVarList{},
		}

		err := s.repo.Save(ctx, gameMod)
		require.NoError(t, err)
		assert.NotZero(t, gameMod.ID)

		filter := &filters.FindGameMod{
			IDs: []uint{gameMod.ID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.NotNil(t, results[0].FastRcon)
		assert.NotNil(t, results[0].Vars)
	})
}

func (s *GameModRepositorySuite) TestGameModRepositoryFindAll() {
	ctx := context.Background()

	gameMods := []*domain.GameMod{
		{
			GameCode: "tf2",
			Name:     "Team Fortress 2",
			FastRcon: domain.GameModFastRconList{{Info: "Status", Command: "status"}},
			Vars:     domain.GameModVarList{{Var: "hostname", Default: "TF2 Server", Info: "Server name", AdminVar: false}},
		},
		{
			GameCode: "l4d2",
			Name:     "Left 4 Dead 2",
			FastRcon: domain.GameModFastRconList{{Info: "Status", Command: "status"}},
			Vars:     domain.GameModVarList{},
		},
		{
			GameCode: "gmod",
			Name:     "Garry's Mod",
			FastRcon: domain.GameModFastRconList{},
			Vars:     domain.GameModVarList{},
		},
	}

	for _, gm := range gameMods {
		require.NoError(s.T(), s.repo.Save(ctx, gm))
	}

	s.T().Run("find_all_game_mods", func(t *testing.T) {
		results, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)

		names := make(map[string]bool)
		for _, result := range results {
			names[result.Name] = true
		}

		assert.True(t, names["Team Fortress 2"])
		assert.True(t, names["Left 4 Dead 2"])
		assert.True(t, names["Garry's Mod"])
	})

	s.T().Run("find_all_with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{
			Limit:  2,
			Offset: 0,
		}

		results, err := s.repo.FindAll(ctx, nil, pagination)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 2)
	})

	s.T().Run("find_all_with_order", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "name", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.FindAll(ctx, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 2)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].Name, results[i+1].Name)
		}
	})
}

func (s *GameModRepositorySuite) TestGameModRepositoryFind() {
	ctx := context.Background()

	gameMod1 := &domain.GameMod{
		GameCode: "ark",
		Name:     "ARK: Survival Evolved",
		FastRcon: domain.GameModFastRconList{{Info: "Status", Command: "status"}},
		Vars:     domain.GameModVarList{},
	}
	gameMod2 := &domain.GameMod{
		GameCode: "valheim",
		Name:     "Valheim",
		FastRcon: domain.GameModFastRconList{},
		Vars:     domain.GameModVarList{},
	}
	gameMod3 := &domain.GameMod{
		GameCode: "palworld",
		Name:     "Palworld",
		FastRcon: domain.GameModFastRconList{},
		Vars:     domain.GameModVarList{},
	}

	require.NoError(s.T(), s.repo.Save(ctx, gameMod1))
	require.NoError(s.T(), s.repo.Save(ctx, gameMod2))
	require.NoError(s.T(), s.repo.Save(ctx, gameMod3))

	s.T().Run("find_by_single_id", func(t *testing.T) {
		filter := &filters.FindGameMod{
			IDs: []uint{gameMod1.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, gameMod1.ID, results[0].ID)
		assert.Equal(t, "ARK: Survival Evolved", results[0].Name)
		require.Len(t, results[0].FastRcon, 1)
		assert.Equal(t, "Status", results[0].FastRcon[0].Info)
		assert.Equal(t, "status", results[0].FastRcon[0].Command)
	})

	s.T().Run("find_by_multiple_ids", func(t *testing.T) {
		filter := &filters.FindGameMod{
			IDs: []uint{gameMod1.ID, gameMod3.ID},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, gameMod1.ID)
		assert.Contains(t, ids, gameMod3.ID)
	})

	s.T().Run("find_by_game_code", func(t *testing.T) {
		filter := &filters.FindGameMod{
			GameCodes: []string{"valheim"},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "Valheim", results[0].Name)
	})

	s.T().Run("find_by_name", func(t *testing.T) {
		filter := &filters.FindGameMod{
			Names: []string{"Palworld"},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "palworld", results[0].GameCode)
	})

	s.T().Run("find_by_multiple_game_codes", func(t *testing.T) {
		filter := &filters.FindGameMod{
			GameCodes: []string{"ark", "palworld"},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		codes := []string{results[0].GameCode, results[1].GameCode}
		assert.Contains(t, codes, "ark")
		assert.Contains(t, codes, "palworld")
	})

	s.T().Run("find_with_nil_filter", func(t *testing.T) {
		results, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_with_empty_filter", func(t *testing.T) {
		filter := &filters.FindGameMod{}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_non_existent_id", func(t *testing.T) {
		filter := &filters.FindGameMod{
			IDs: []uint{99999},
		}

		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		filter := &filters.FindGameMod{
			IDs: []uint{gameMod1.ID, gameMod2.ID, gameMod3.ID},
		}
		pagination := &filters.Pagination{
			Limit:  2,
			Offset: 0,
		}

		results, err := s.repo.Find(ctx, filter, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	s.T().Run("find_with_order", func(t *testing.T) {
		filter := &filters.FindGameMod{
			IDs: []uint{gameMod1.ID, gameMod2.ID, gameMod3.ID},
		}
		order := []filters.Sorting{
			{Field: "name", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.Find(ctx, filter, order, nil)
		require.NoError(t, err)
		require.Len(t, results, 3)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].Name, results[i+1].Name)
		}
	})
}

func (s *GameModRepositorySuite) TestGameModRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_existing_game_mod", func(t *testing.T) {
		gameMod := &domain.GameMod{
			GameCode: "7dtd",
			Name:     "7 Days to Die",
			FastRcon: domain.GameModFastRconList{},
			Vars:     domain.GameModVarList{},
		}

		require.NoError(t, s.repo.Save(ctx, gameMod))
		gameModID := gameMod.ID

		err := s.repo.Delete(ctx, gameModID)
		require.NoError(t, err)

		filter := &filters.FindGameMod{
			IDs: []uint{gameModID},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("delete_non_existent_game_mod", func(t *testing.T) {
		err := s.repo.Delete(ctx, 99999)
		require.NoError(t, err)
	})

	s.T().Run("delete_already_deleted_game_mod", func(t *testing.T) {
		gameMod := &domain.GameMod{
			GameCode: "satisfactory",
			Name:     "Satisfactory",
			FastRcon: domain.GameModFastRconList{},
			Vars:     domain.GameModVarList{},
		}

		require.NoError(t, s.repo.Save(ctx, gameMod))
		gameModID := gameMod.ID

		err := s.repo.Delete(ctx, gameModID)
		require.NoError(t, err)

		err = s.repo.Delete(ctx, gameModID)
		require.NoError(t, err)
	})
}

func (s *GameModRepositorySuite) TestGameModRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		gameMod := &domain.GameMod{
			GameCode: "hlds",
			Name:     "Half-Life Dedicated Server",
			FastRcon: domain.GameModFastRconList{
				{Info: "Status", Command: "status"},
				{Info: "Users", Command: "users"},
			},
			Vars: domain.GameModVarList{
				{Var: "hostname", Default: "HL Server", Info: "Server name", AdminVar: false},
				{Var: "sv_password", Default: "", Info: "Server password", AdminVar: false},
			},
			StartCmdLinux:   lo.ToPtr("./hlds_run -game valve"),
			StartCmdWindows: lo.ToPtr("hlds.exe -game valve"),
			KickCmd:         lo.ToPtr("kick {player}"),
		}

		err := s.repo.Save(ctx, gameMod)
		require.NoError(t, err)
		assert.NotZero(t, gameMod.ID)

		filter := &filters.FindGameMod{
			GameCodes: []string{"hlds"},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Half-Life Dedicated Server", results[0].Name)
		assert.Len(t, results[0].FastRcon, 2)
		assert.Len(t, results[0].Vars, 2)

		gameMod.Name = "Half-Life Updated"
		gameMod.BanCmd = lo.ToPtr("banid {player}")
		err = s.repo.Save(ctx, gameMod)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Half-Life Updated", results[0].Name)
		assert.Equal(t, "banid {player}", *results[0].BanCmd)

		err = s.repo.Delete(ctx, gameMod.ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}
