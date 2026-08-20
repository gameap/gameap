package base

import (
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/quercon/rcon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineProtocolByEngine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		engine    string
		want      rcon.Protocol
		wantError string
	}{
		{
			name:   "source_engine",
			engine: "source",
			want:   rcon.ProtocolSource,
		},
		{
			name:   "goldsource_engine",
			engine: "goldsource",
			want:   rcon.ProtocolGoldSrc,
		},
		{
			name:   "goldsrc_alias",
			engine: "goldsrc",
			want:   rcon.ProtocolGoldSrc,
		},
		{
			name:   "minecraft_engine",
			engine: "minecraft",
			want:   rcon.ProtocolSource,
		},
		{
			name:   "uppercase_source_is_normalized",
			engine: "SOURCE",
			want:   rcon.ProtocolSource,
		},
		{
			name:   "mixed_case_goldsource_is_normalized",
			engine: "GoldSource",
			want:   rcon.ProtocolGoldSrc,
		},
		{
			name:   "quake3_engine",
			engine: "q3",
			want:   rcon.ProtocolQuake3,
		},
		{
			name:   "quake2_engine",
			engine: "q2",
			want:   rcon.ProtocolQuake2,
		},
		{
			name:   "call_of_duty_engine",
			engine: "cod4",
			want:   rcon.ProtocolQuake3,
		},
		{
			name:   "samp_engine",
			engine: "samp",
			want:   rcon.ProtocolSAMP,
		},
		{
			name:   "arma_engine",
			engine: "arma",
			want:   rcon.ProtocolBattlEye,
		},
		{
			name:   "arma3_engine",
			engine: "arma3",
			want:   rcon.ProtocolBattlEye,
		},
		{
			name:   "legacy_armedassault2oa_engine",
			engine: "armedassault2oa",
			want:   rcon.ProtocolBattlEye,
		},
		{
			name:   "legacy_armedassault3_engine",
			engine: "ArmedAssault3",
			want:   rcon.ProtocolBattlEye,
		},
		{
			// idtech alone is ambiguous between Quake 2 and Quake 3; only DetermineProtocol,
			// which sees the engine version, can resolve it.
			name:      "bare_idtech_engine_is_ambiguous",
			engine:    "idtech",
			wantError: "unable to determine RCON protocol for engine",
		},
		{
			name:      "unsupported_engine",
			engine:    "unreal",
			wantError: "unable to determine RCON protocol for engine",
		},
		{
			name:      "empty_engine",
			engine:    "",
			wantError: "unable to determine RCON protocol for engine",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			protocol, err := DetermineProtocolByEngine(tt.engine)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Empty(t, string(protocol), "protocol must be empty on error")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, protocol)
		})
	}
}

func TestDetermineProtocolByGameCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		gameCode  string
		want      rcon.Protocol
		wantError string
	}{
		{
			name:     "cs_goldsrc",
			gameCode: "cs",
			want:     rcon.ProtocolGoldSrc,
		},
		{
			name:     "cstrike_goldsrc",
			gameCode: "cstrike",
			want:     rcon.ProtocolGoldSrc,
		},
		{
			name:     "csgo_source",
			gameCode: "csgo",
			want:     rcon.ProtocolSource,
		},
		{
			name:     "cs2_source",
			gameCode: "cs2",
			want:     rcon.ProtocolSource,
		},
		{
			name:     "minecraft_source",
			gameCode: "minecraft",
			want:     rcon.ProtocolSource,
		},
		{
			name:     "hl_goldsrc",
			gameCode: "hl",
			want:     rcon.ProtocolGoldSrc,
		},
		{
			name:     "tf2_source",
			gameCode: "tf2",
			want:     rcon.ProtocolSource,
		},
		{
			name:     "valve_goldsrc",
			gameCode: "valve",
			want:     rcon.ProtocolGoldSrc,
		},
		{
			name:     "quake3_code",
			gameCode: "q3",
			want:     rcon.ProtocolQuake3,
		},
		{
			name:     "quake2_code",
			gameCode: "q2",
			want:     rcon.ProtocolQuake2,
		},
		{
			name:     "cod4_code",
			gameCode: "cod4",
			want:     rcon.ProtocolQuake3,
		},
		{
			name:     "samp_code",
			gameCode: "samp",
			want:     rcon.ProtocolSAMP,
		},
		{
			name:     "arma2oa_code",
			gameCode: "arma2oa",
			want:     rcon.ProtocolBattlEye,
		},
		{
			name:     "arma3_code",
			gameCode: "arma3",
			want:     rcon.ProtocolBattlEye,
		},
		{
			name:      "unknown_game_code",
			gameCode:  "doom",
			wantError: "unable to determine RCON protocol for game code",
		},
		{
			name:      "empty_game_code",
			gameCode:  "",
			wantError: "unable to determine RCON protocol for game code",
		},
		{
			// Locks down current behavior: DetermineProtocolByGameCode performs no case folding,
			// unlike DetermineProtocolByEngine which lowercases input.
			name:      "uppercase_not_mapped",
			gameCode:  "CS",
			wantError: "unable to determine RCON protocol for game code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			protocol, err := DetermineProtocolByGameCode(tt.gameCode)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Empty(t, string(protocol), "protocol must be empty on error")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, protocol)
		})
	}
}

func TestDetermineProtocol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		game      domain.Game
		want      rcon.Protocol
		wantError string
	}{
		{
			// Engine resolves first, so a valid engine wins even when the game code maps to a different protocol.
			name: "engine_takes_precedence",
			game: domain.Game{Engine: "source", Code: "cs"},
			want: rcon.ProtocolSource,
		},
		{
			name: "fallback_to_game_code",
			game: domain.Game{Engine: "unreal", Code: "cs"},
			want: rcon.ProtocolGoldSrc,
		},
		{
			name: "fallback_to_game_code_when_engine_empty",
			game: domain.Game{Engine: "", Code: "csgo"},
			want: rcon.ProtocolSource,
		},
		{
			name:      "both_invalid",
			game:      domain.Game{Engine: "unreal", Code: "doom"},
			wantError: "unable to determine RCON protocol for game code",
		},
		{
			name:      "both_empty",
			game:      domain.Game{Engine: "", Code: ""},
			wantError: "unable to determine RCON protocol for game code",
		},
		{
			name: "goldsource_engine_alias",
			game: domain.Game{Engine: "goldsource", Code: ""},
			want: rcon.ProtocolGoldSrc,
		},
		{
			name: "current_catalogue_quake3",
			game: domain.Game{Engine: "q3", EngineVersion: "3", Code: "q3"},
			want: rcon.ProtocolQuake3,
		},
		{
			name: "current_catalogue_arma2_shares_the_arma_engine",
			game: domain.Game{Engine: "arma", EngineVersion: "2", Code: "arma2oa"},
			want: rcon.ProtocolBattlEye,
		},
		{
			// Panels seeded from an older catalogue snapshot carry idtech plus a version.
			name: "legacy_idtech_version_2_is_quake2",
			game: domain.Game{Engine: "idtech", EngineVersion: "2", Code: "q2"},
			want: rcon.ProtocolQuake2,
		},
		{
			name: "legacy_idtech_version_3_is_quake3",
			game: domain.Game{Engine: "idtech", EngineVersion: "3", Code: "q3"},
			want: rcon.ProtocolQuake3,
		},
		{
			// An unusable version falls through to the game code rather than guessing.
			name: "legacy_idtech_without_version_falls_back_to_code",
			game: domain.Game{Engine: "idtech", Code: "q3"},
			want: rcon.ProtocolQuake3,
		},
		{
			name:      "legacy_idtech_with_unknown_version_and_code",
			game:      domain.Game{Engine: "idtech", EngineVersion: "1", Code: "quake1"},
			wantError: "unable to determine RCON protocol for game code",
		},
		{
			name:      "multi_theft_auto_has_no_rcon",
			game:      domain.Game{Engine: "mta", EngineVersion: "1.0", Code: "mta"},
			wantError: "unable to determine RCON protocol for game code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			protocol, err := DetermineProtocol(tt.game)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Empty(t, string(protocol), "protocol must be empty on error")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, protocol)
		})
	}
}
