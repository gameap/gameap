package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackGlobalAPIService_Games(t *testing.T) {
	t.Parallel()

	t.Run("returns_games_from_embedded_json", func(t *testing.T) {
		t.Parallel()

		service := NewFallbackGlobalAPIService()

		games, err := service.Games(context.Background())

		require.NoError(t, err)
		require.NotEmpty(t, games)

		hasCounterStrike := false
		for _, game := range games {
			if game.Code == "cstrike" {
				hasCounterStrike = true
				assert.Equal(t, "Counter-Strike 1.6", game.Name)
				assert.Equal(t, "GoldSource", game.Engine)
				assert.NotEmpty(t, game.Mods)

				break
			}
		}
		assert.True(t, hasCounterStrike, "Counter-Strike 1.6 should be in the games list")
	})

	t.Run("games_have_valid_structure", func(t *testing.T) {
		t.Parallel()

		service := NewFallbackGlobalAPIService()

		games, err := service.Games(context.Background())

		require.NoError(t, err)

		for _, game := range games {
			assert.NotEmpty(t, game.Code, "game code should not be empty")
			assert.NotEmpty(t, game.Name, "game name should not be empty")
			assert.NotEmpty(t, game.Engine, "game engine should not be empty")
		}
	})
}
