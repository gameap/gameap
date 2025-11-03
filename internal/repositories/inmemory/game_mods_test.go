package inmemory_test

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestGameModRepository(t *testing.T) {
	suite.Run(t, repotesting.NewGameModRepositorySuite(
		func(_ *testing.T) repositories.GameModRepository {
			return inmemory.NewGameModRepository()
		},
	))
}

func TestGameModRepository_FindByNames(t *testing.T) {
	repo := inmemory.NewGameModRepository()

	gameMod1 := &domain.GameMod{
		GameCode: "valve",
		Name:     "Half-Life Deathmatch",
	}
	gameMod2 := &domain.GameMod{
		GameCode: "cstrike",
		Name:     "Counter-Strike",
	}
	gameMod3 := &domain.GameMod{
		GameCode: "dod",
		Name:     "Day of Defeat",
	}

	require.NoError(t, repo.Save(context.Background(), gameMod1))
	require.NoError(t, repo.Save(context.Background(), gameMod2))
	require.NoError(t, repo.Save(context.Background(), gameMod3))

	t.Run("Find_by_single_name_returns_one_game_mod", func(t *testing.T) {
		filter := &filters.FindGameMod{
			Names: []string{"Counter-Strike"},
		}
		gameMods, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, gameMods, 1)
		assert.Equal(t, "Counter-Strike", gameMods[0].Name)
		assert.Equal(t, "cstrike", gameMods[0].GameCode)
	})

	t.Run("Find_by_multiple_names_returns_multiple_game_mods", func(t *testing.T) {
		filter := &filters.FindGameMod{
			Names: []string{"Counter-Strike", "Day of Defeat"},
		}
		gameMods, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, gameMods, 2)

		names := make([]string, len(gameMods))
		for i, gm := range gameMods {
			names[i] = gm.Name
		}
		assert.Contains(t, names, "Counter-Strike")
		assert.Contains(t, names, "Day of Defeat")
	})

	t.Run("Find_by_nonexistent_name_returns_empty", func(t *testing.T) {
		filter := &filters.FindGameMod{
			Names: []string{"Nonexistent Game"},
		}
		gameMods, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, gameMods, 0)
	})

	t.Run("Find_by_empty_names_returns_all_game_mods", func(t *testing.T) {
		filter := &filters.FindGameMod{
			Names: []string{},
		}
		gameMods, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, gameMods, 3)
	})

	t.Run("Find_combines_name_and_game_code_filters", func(t *testing.T) {
		filter := &filters.FindGameMod{
			Names:     []string{"Counter-Strike", "Day of Defeat"},
			GameCodes: []string{"cstrike"},
		}
		gameMods, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, gameMods, 1)
		assert.Equal(t, "Counter-Strike", gameMods[0].Name)
		assert.Equal(t, "cstrike", gameMods[0].GameCode)
	})
}
