package players

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPlayerManagerByGameCode(t *testing.T) {
	tests := []struct {
		name      string
		gameCode  string
		wantType  string
		wantError string
	}{
		{name: "counter_strike_uses_valve_parser", gameCode: "cs", wantType: "*players.ValvePlayerManager"},
		{name: "minecraft_uses_its_own_parser", gameCode: "minecraft", wantType: "*players.MinecraftPlayerManager"},
		{name: "quake3_uses_quake_parser", gameCode: "q3", wantType: "*players.QuakePlayerManager"},
		{name: "quake2_uses_quake_parser", gameCode: "q2", wantType: "*players.QuakePlayerManager"},
		{name: "call_of_duty_uses_quake_parser", gameCode: "cod4", wantType: "*players.QuakePlayerManager"},
		{name: "samp_uses_samp_parser", gameCode: "samp", wantType: "*players.SAMPPlayerManager"},
		{name: "arma3_uses_battleye_parser", gameCode: "arma3", wantType: "*players.BattlEyePlayerManager"},
		{name: "arma2_operation_arrowhead", gameCode: "arma2oa", wantType: "*players.BattlEyePlayerManager"},
		{name: "unknown_game", gameCode: "rust", wantError: ErrPlayersManagementNotSupported.Error()},
		{name: "empty_game_code", gameCode: "", wantError: ErrPlayersManagementNotSupported.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewPlayerManagerByGameCode(tt.gameCode)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Nil(t, mgr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantType, typeName(mgr))
		})
	}
}

func TestNewPlayerManagerByEngine(t *testing.T) {
	tests := []struct {
		name      string
		engine    string
		wantType  string
		wantError string
	}{
		{name: "quake3_engine", engine: "q3", wantType: "*players.QuakePlayerManager"},
		{name: "quake2_engine", engine: "q2", wantType: "*players.QuakePlayerManager"},
		{name: "call_of_duty_engine", engine: "cod4", wantType: "*players.QuakePlayerManager"},
		{name: "samp_engine", engine: "samp", wantType: "*players.SAMPPlayerManager"},
		{name: "arma_engine", engine: "arma", wantType: "*players.BattlEyePlayerManager"},
		{name: "arma3_engine", engine: "arma3", wantType: "*players.BattlEyePlayerManager"},
		{name: "legacy_armedassault2_engine", engine: "armedassault2", wantType: "*players.BattlEyePlayerManager"},
		{name: "legacy_armedassault3_engine", engine: "armedassault3", wantType: "*players.BattlEyePlayerManager"},
		{
			name:      "source_is_deliberately_absent",
			engine:    "source",
			wantError: ErrPlayersManagementNotSupported.Error(),
		},
		{
			name:      "goldsource_is_deliberately_absent",
			engine:    "goldsource",
			wantError: ErrPlayersManagementNotSupported.Error(),
		},
		{name: "unknown_engine", engine: "unreal", wantError: ErrPlayersManagementNotSupported.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewPlayerManagerByEngine(tt.engine)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Nil(t, mgr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantType, typeName(mgr))
		})
	}
}

func typeName(mgr PlayerManager) string {
	return fmt.Sprintf("%T", mgr)
}
