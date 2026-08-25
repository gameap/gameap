package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFallbackCatalogIsValid guards the embedded catalog: it is regenerated from
// the gameap/games repository and must decode into definitions the panel accepts.
func TestFallbackCatalogIsValid(t *testing.T) {
	t.Parallel()

	// ARRANGE
	service := NewFallbackGlobalAPIService()

	// ACT
	games, err := service.Games(context.Background())

	// ASSERT
	require.NoError(t, err)
	assert.NotEmpty(t, games)

	for _, game := range games {
		for _, mod := range game.Mods {
			gameMod := mod.ToDomainGameMod()

			for i := range gameMod.Vars {
				gameMod.Vars[i].Normalize()
			}

			for i := range gameMod.FastRcon {
				gameMod.FastRcon[i].Normalize()
			}

			assert.NoErrorf(t, gameMod.Vars.Validate(), "%s / %s vars", game.Code, mod.Name)
			assert.NoErrorf(t, gameMod.FastRcon.Validate(), "%s / %s fast_rcon", game.Code, mod.Name)
		}
	}
}
