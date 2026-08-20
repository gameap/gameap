package base

import (
	"context"
	"fmt"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gamesWithoutRcon are the catalogue entries that genuinely have no RCON protocol the panel
// implements. Every other game must resolve, so a catalogue refresh that renames an engine
// cannot silently switch RCON off — which is exactly how the id Tech and ArmA games ended up
// unreachable before.
var gamesWithoutRcon = map[string]struct{}{
	"7d2d":       {},
	"fivem":      {},
	"hurtworld":  {},
	"hytale":     {},
	"justcause":  {},
	"mta":        {},
	"rok":        {},
	"rust":       {},
	"teamspeak3": {},
	"the-forest": {},
	"valheim":    {},
}

func embeddedCatalogue(t *testing.T) []domain.Game {
	t.Helper()

	response, err := services.NewFallbackGlobalAPIService().Games(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, response)

	games := make([]domain.Game, 0, len(response))
	for _, game := range response {
		games = append(games, *game.ToDomainGame())
	}

	return games
}

func TestDetermineProtocol_CoversEmbeddedCatalogue(t *testing.T) {
	t.Parallel()
	for _, game := range embeddedCatalogue(t) {
		t.Run(game.Code, func(t *testing.T) {
			t.Parallel()
			protocol, err := DetermineProtocol(game)

			if _, expected := gamesWithoutRcon[game.Code]; expected {
				assert.Error(t, err,
					"%s (engine %q) is on the no-rcon list but now resolves to %q — update the list",
					game.Code, game.Engine, protocol)

				return
			}

			require.NoError(t, err,
				"%s has engine %q, which no protocol table knows; a catalogue rename would silently disable rcon",
				game.Code, game.Engine)
			assert.NotEmpty(t, string(protocol))
		})
	}
}

func TestDeterminePlayerManager(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		game      domain.Game
		wantType  string
		wantError string
	}{
		{
			name:     "engine_family_resolves_a_custom_game_code",
			game:     domain.Game{Engine: "q3", EngineVersion: "3", Code: "my-quake-mod"},
			wantType: "*players.QuakePlayerManager",
		},
		{
			name:     "call_of_duty_engine",
			game:     domain.Game{Engine: "cod4", EngineVersion: "4", Code: "cod4"},
			wantType: "*players.QuakePlayerManager",
		},
		{
			name:     "arma_engine_covers_both_arma2_games",
			game:     domain.Game{Engine: "arma", EngineVersion: "2", Code: "arma2oa"},
			wantType: "*players.BattlEyePlayerManager",
		},
		{
			name:     "samp_engine",
			game:     domain.Game{Engine: "samp", EngineVersion: "1.0", Code: "samp"},
			wantType: "*players.SAMPPlayerManager",
		},
		{
			name:     "legacy_idtech_version_2",
			game:     domain.Game{Engine: "idtech", EngineVersion: "2", Code: "q2"},
			wantType: "*players.QuakePlayerManager",
		},
		{
			name:     "game_code_still_wins_for_valve_games",
			game:     domain.Game{Engine: "goldsource", EngineVersion: "1", Code: "cs"},
			wantType: "*players.ValvePlayerManager",
		},
		{
			name:     "minecraft_by_code",
			game:     domain.Game{Engine: "minecraft", EngineVersion: "1", Code: "minecraft"},
			wantType: "*players.MinecraftPlayerManager",
		},
		{
			// Source games have no players parser today and must not gain one by accident.
			name:      "source_game_without_a_parser",
			game:      domain.Game{Engine: "source", EngineVersion: "1", Code: "tf2"},
			wantError: players.ErrPlayersManagementNotSupported.Error(),
		},
		{
			name:      "game_without_player_management",
			game:      domain.Game{Engine: "rust", EngineVersion: "1", Code: "rust"},
			wantError: players.ErrPlayersManagementNotSupported.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			manager, err := DeterminePlayerManager(tt.game)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Nil(t, manager)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantType, fmt.Sprintf("%T", manager))
		})
	}
}
