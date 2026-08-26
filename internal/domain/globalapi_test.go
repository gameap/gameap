package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalAPIGame_ToDomainGame(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    *GlobalAPIGame
		expected *Game
	}{
		{
			name: "all_fields_populated",
			input: &GlobalAPIGame{
				Code:                    "cs2",
				StartCode:               "cs2ds",
				Name:                    "Counter-Strike 2",
				Engine:                  "Source 2",
				EngineVersion:           "1.0",
				SteamAppIDLinux:         730,
				SteamAppIDWindows:       740,
				SteamAppSetConfig:       "cs2_config",
				RemoteRepositoryLinux:   "https://repo.example.com/linux",
				RemoteRepositoryWindows: "https://repo.example.com/windows",
			},
			expected: &Game{
				Code:                    "cs2",
				Name:                    "Counter-Strike 2",
				Engine:                  "Source 2",
				EngineVersion:           "1.0",
				SteamAppIDLinux:         new(uint(730)),
				SteamAppIDWindows:       new(uint(740)),
				SteamAppSetConfig:       new("cs2_config"),
				RemoteRepositoryLinux:   new("https://repo.example.com/linux"),
				RemoteRepositoryWindows: new("https://repo.example.com/windows"),
				Enabled:                 1,
			},
		},
		{
			name: "minimal_fields",
			input: &GlobalAPIGame{
				Code:          "minecraft",
				StartCode:     "mc",
				Name:          "Minecraft",
				Engine:        "Java",
				EngineVersion: "1.20",
			},
			expected: &Game{
				Code:          "minecraft",
				Name:          "Minecraft",
				Engine:        "Java",
				EngineVersion: "1.20",
				Enabled:       1,
			},
		},
		{
			name: "zero_steam_app_ids_not_set",
			input: &GlobalAPIGame{
				Code:              "game1",
				Name:              "Game One",
				Engine:            "Unity",
				EngineVersion:     "2023.1",
				SteamAppIDLinux:   0,
				SteamAppIDWindows: 0,
			},
			expected: &Game{
				Code:          "game1",
				Name:          "Game One",
				Engine:        "Unity",
				EngineVersion: "2023.1",
				Enabled:       1,
			},
		},
		{
			name: "only_linux_steam_app_id",
			input: &GlobalAPIGame{
				Code:              "valheim",
				Name:              "Valheim",
				Engine:            "Unity",
				EngineVersion:     "2020.3",
				SteamAppIDLinux:   896660,
				SteamAppIDWindows: 0,
			},
			expected: &Game{
				Code:              "valheim",
				Name:              "Valheim",
				Engine:            "Unity",
				EngineVersion:     "2020.3",
				SteamAppIDLinux:   new(uint(896660)),
				SteamAppIDWindows: nil,
				Enabled:           1,
			},
		},
		{
			name: "only_windows_steam_app_id",
			input: &GlobalAPIGame{
				Code:              "rust",
				Name:              "Rust",
				Engine:            "Unity",
				EngineVersion:     "2019.4",
				SteamAppIDLinux:   0,
				SteamAppIDWindows: 258550,
			},
			expected: &Game{
				Code:              "rust",
				Name:              "Rust",
				Engine:            "Unity",
				EngineVersion:     "2019.4",
				SteamAppIDLinux:   nil,
				SteamAppIDWindows: new(uint(258550)),
				Enabled:           1,
			},
		},
		{
			name: "empty_strings_not_set",
			input: &GlobalAPIGame{
				Code:                    "ark",
				Name:                    "ARK",
				Engine:                  "Unreal",
				EngineVersion:           "4.5",
				SteamAppSetConfig:       "",
				RemoteRepositoryLinux:   "",
				RemoteRepositoryWindows: "",
			},
			expected: &Game{
				Code:          "ark",
				Name:          "ARK",
				Engine:        "Unreal",
				EngineVersion: "4.5",
				Enabled:       1,
			},
		},
		{
			name: "only_linux_repository",
			input: &GlobalAPIGame{
				Code:                    "tf2",
				Name:                    "Team Fortress 2",
				Engine:                  "Source",
				EngineVersion:           "1.0",
				RemoteRepositoryLinux:   "https://repo.example.com/tf2/linux",
				RemoteRepositoryWindows: "",
			},
			expected: &Game{
				Code:                  "tf2",
				Name:                  "Team Fortress 2",
				Engine:                "Source",
				EngineVersion:         "1.0",
				RemoteRepositoryLinux: new("https://repo.example.com/tf2/linux"),
				Enabled:               1,
			},
		},
		{
			name: "with_steam_config_only",
			input: &GlobalAPIGame{
				Code:              "csgo",
				Name:              "CS:GO",
				Engine:            "Source",
				EngineVersion:     "1.0",
				SteamAppSetConfig: "csgo_ds.txt",
			},
			expected: &Game{
				Code:              "csgo",
				Name:              "CS:GO",
				Engine:            "Source",
				EngineVersion:     "1.0",
				SteamAppSetConfig: new("csgo_ds.txt"),
				Enabled:           1,
			},
		},
		{
			name: "metadata_is_copied",
			input: &GlobalAPIGame{
				Code:          "csgo",
				Name:          "CS:GO",
				Engine:        "Source",
				EngineVersion: "1.0",
				Metadata: Metadata{
					"docker_image": "ghcr.io/gameap/csgo:latest",
					"default_port": float64(27015),
				},
			},
			expected: &Game{
				Code:          "csgo",
				Name:          "CS:GO",
				Engine:        "Source",
				EngineVersion: "1.0",
				Enabled:       1,
				Metadata: Metadata{
					"docker_image": "ghcr.io/gameap/csgo:latest",
					"default_port": float64(27015),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE / ACT
			result := test.input.ToDomainGame()

			// ASSERT
			assert.Equal(t, test.expected, result, "domain game mismatch")
		})
	}
}

func TestGlobalAPIGameMod_ToDomainGameMod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    *GlobalAPIGameMod
		expected *GameMod
	}{
		{
			name: "all_fields_populated",
			input: &GlobalAPIGameMod{
				ID:       1,
				GameCode: "csgo",
				Name:     "Classic Competitive",
				FastRcon: GameModFastRconList{
					{Info: "Status", Command: "status"},
					{Info: "Players", Command: "players"},
				},
				Vars: GameModVarList{
					{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
				},
				RemoteRepositoryLinux:   "https://repo.example.com/csgo/linux",
				RemoteRepositoryWindows: "https://repo.example.com/csgo/windows",
				StartCmdLinux:           "./srcds_run -game csgo",
				StartCmdWindows:         "srcds.exe -game csgo",
				KickCmd:                 "kick {player}",
				BanCmd:                  "banid {player}",
				ChnameCmd:               "name {name}",
				SrestartCmd:             "restart",
				ChmapCmd:                "changelevel {map}",
				SendmsgCmd:              "say {message}",
				PasswdCmd:               "sv_password {password}",
			},
			expected: &GameMod{
				GameCode: "csgo",
				Name:     "Classic Competitive",
				FastRcon: GameModFastRconList{
					{Info: "Status", Command: "status"},
					{Info: "Players", Command: "players"},
				},
				Vars: GameModVarList{
					{Var: "sv_cheats", Default: "0", Info: "Enable cheats", AdminVar: true},
				},
				RemoteRepositoryLinux:   new("https://repo.example.com/csgo/linux"),
				RemoteRepositoryWindows: new("https://repo.example.com/csgo/windows"),
				StartCmdLinux:           new("./srcds_run -game csgo"),
				StartCmdWindows:         new("srcds.exe -game csgo"),
				KickCmd:                 new("kick {player}"),
				BanCmd:                  new("banid {player}"),
				ChnameCmd:               new("name {name}"),
				SrestartCmd:             new("restart"),
				ChmapCmd:                new("changelevel {map}"),
				SendmsgCmd:              new("say {message}"),
				PasswdCmd:               new("sv_password {password}"),
			},
		},
		{
			name: "minimal_fields",
			input: &GlobalAPIGameMod{
				ID:       2,
				GameCode: "minecraft",
				Name:     "Vanilla",
				FastRcon: nil,
				Vars:     nil,
			},
			expected: &GameMod{
				GameCode: "minecraft",
				Name:     "Vanilla",
				FastRcon: nil,
				Vars:     nil,
			},
		},
		{
			name: "empty_strings_not_set",
			input: &GlobalAPIGameMod{
				ID:                      3,
				GameCode:                "rust",
				Name:                    "Modded",
				RemoteRepositoryLinux:   "",
				RemoteRepositoryWindows: "",
				StartCmdLinux:           "",
				StartCmdWindows:         "",
				KickCmd:                 "",
				BanCmd:                  "",
				ChnameCmd:               "",
				SrestartCmd:             "",
				ChmapCmd:                "",
				SendmsgCmd:              "",
				PasswdCmd:               "",
			},
			expected: &GameMod{
				GameCode: "rust",
				Name:     "Modded",
			},
		},
		{
			name: "partial_commands_set",
			input: &GlobalAPIGameMod{
				ID:              4,
				GameCode:        "tf2",
				Name:            "Default",
				StartCmdLinux:   "./srcds_run -game tf",
				StartCmdWindows: "",
				KickCmd:         "sm_kick {player}",
				BanCmd:          "",
			},
			expected: &GameMod{
				GameCode:      "tf2",
				Name:          "Default",
				StartCmdLinux: new("./srcds_run -game tf"),
				KickCmd:       new("sm_kick {player}"),
			},
		},
		{
			name: "only_linux_fields",
			input: &GlobalAPIGameMod{
				ID:                    5,
				GameCode:              "valheim",
				Name:                  "Plus",
				RemoteRepositoryLinux: "https://repo.example.com/valheim",
				StartCmdLinux:         "./valheim_server.x86_64",
			},
			expected: &GameMod{
				GameCode:              "valheim",
				Name:                  "Plus",
				RemoteRepositoryLinux: new("https://repo.example.com/valheim"),
				StartCmdLinux:         new("./valheim_server.x86_64"),
			},
		},
		{
			name: "only_windows_fields",
			input: &GlobalAPIGameMod{
				ID:                      6,
				GameCode:                "ark",
				Name:                    "Survival Evolved",
				RemoteRepositoryWindows: "https://repo.example.com/ark",
				StartCmdWindows:         "ShooterGameServer.exe",
			},
			expected: &GameMod{
				GameCode:                "ark",
				Name:                    "Survival Evolved",
				RemoteRepositoryWindows: new("https://repo.example.com/ark"),
				StartCmdWindows:         new("ShooterGameServer.exe"),
			},
		},
		{
			name: "only_admin_commands",
			input: &GlobalAPIGameMod{
				ID:          7,
				GameCode:    "gmod",
				Name:        "DarkRP",
				KickCmd:     "ulx kick {player}",
				BanCmd:      "ulx ban {player}",
				ChnameCmd:   "ulx name {name}",
				SrestartCmd: "ulx restart",
			},
			expected: &GameMod{
				GameCode:    "gmod",
				Name:        "DarkRP",
				KickCmd:     new("ulx kick {player}"),
				BanCmd:      new("ulx ban {player}"),
				ChnameCmd:   new("ulx name {name}"),
				SrestartCmd: new("ulx restart"),
			},
		},
		{
			name: "only_game_commands",
			input: &GlobalAPIGameMod{
				ID:         8,
				GameCode:   "cs2",
				Name:       "Competitive",
				ChmapCmd:   "map {map}",
				SendmsgCmd: "say {message}",
				PasswdCmd:  "sv_password {password}",
			},
			expected: &GameMod{
				GameCode:   "cs2",
				Name:       "Competitive",
				ChmapCmd:   new("map {map}"),
				SendmsgCmd: new("say {message}"),
				PasswdCmd:  new("sv_password {password}"),
			},
		},
		{
			name: "with_fast_rcon_only",
			input: &GlobalAPIGameMod{
				ID:       9,
				GameCode: "squad",
				Name:     "Infantry Only",
				FastRcon: GameModFastRconList{
					{Info: "AdminListPlayers", Command: "AdminListPlayers"},
					{Info: "ShowNextMap", Command: "ShowNextMap"},
				},
			},
			expected: &GameMod{
				GameCode: "squad",
				Name:     "Infantry Only",
				FastRcon: GameModFastRconList{
					{Info: "AdminListPlayers", Command: "AdminListPlayers"},
					{Info: "ShowNextMap", Command: "ShowNextMap"},
				},
			},
		},
		{
			name: "with_vars_only",
			input: &GlobalAPIGameMod{
				ID:       10,
				GameCode: "minecraft",
				Name:     "Survival",
				Vars: GameModVarList{
					{Var: "difficulty", Default: "normal", Info: "Game difficulty", AdminVar: false},
					{Var: "max-players", Default: "20", Info: "Maximum players", AdminVar: true},
				},
			},
			expected: &GameMod{
				GameCode: "minecraft",
				Name:     "Survival",
				Vars: GameModVarList{
					{Var: "difficulty", Default: "normal", Info: "Game difficulty", AdminVar: false},
					{Var: "max-players", Default: "20", Info: "Maximum players", AdminVar: true},
				},
			},
		},
		{
			name: "metadata_is_copied",
			input: &GlobalAPIGameMod{
				ID:       11,
				GameCode: "csgo",
				Name:     "WithMeta",
				Metadata: Metadata{
					"key":    "value",
					"nested": map[string]any{"x": float64(1)},
				},
			},
			expected: &GameMod{
				GameCode: "csgo",
				Name:     "WithMeta",
				Metadata: Metadata{
					"key":    "value",
					"nested": map[string]any{"x": float64(1)},
				},
			},
		},
		{
			name: "metadata_nil_stays_nil",
			input: &GlobalAPIGameMod{
				ID:       12,
				GameCode: "csgo",
				Name:     "NoMeta",
				Metadata: nil,
			},
			expected: &GameMod{
				GameCode: "csgo",
				Name:     "NoMeta",
				Metadata: nil,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE / ACT
			result := test.input.ToDomainGameMod()

			// ASSERT
			assert.Equal(t, test.expected, result, "domain game mod mismatch")
		})
	}
}

func TestGlobalAPIResponse_Structure(t *testing.T) {
	t.Parallel()

	t.Run("response_with_string_data", func(t *testing.T) {
		t.Parallel()

		response := GlobalAPIResponse[string]{
			Data:    "test data",
			Message: "Operation successful",
			Success: true,
		}

		assert.Equal(t, "test data", response.Data)
		assert.Equal(t, "Operation successful", response.Message)
		assert.True(t, response.Success)
	})

	t.Run("response_with_game_data", func(t *testing.T) {
		t.Parallel()

		game := GlobalAPIGame{
			Code:   "test",
			Name:   "Test Game",
			Engine: "Test Engine",
		}

		response := GlobalAPIResponse[GlobalAPIGame]{
			Data:    game,
			Message: "Game retrieved",
			Success: true,
		}

		assert.Equal(t, game, response.Data)
		assert.Equal(t, "Game retrieved", response.Message)
		assert.True(t, response.Success)
	})

	t.Run("response_with_game_slice", func(t *testing.T) {
		t.Parallel()

		games := []GlobalAPIGame{
			{Code: "game1", Name: "Game One", Engine: "Engine1"},
			{Code: "game2", Name: "Game Two", Engine: "Engine2"},
		}

		response := GlobalAPIResponse[[]GlobalAPIGame]{
			Data:    games,
			Message: "Games retrieved",
			Success: true,
		}

		require.Len(t, response.Data, 2)
		assert.Equal(t, "game1", response.Data[0].Code)
		assert.Equal(t, "game2", response.Data[1].Code)
		assert.True(t, response.Success)
	})

	t.Run("response_with_error", func(t *testing.T) {
		t.Parallel()

		response := GlobalAPIResponse[any]{
			Data:    nil,
			Message: "Not found",
			Success: false,
		}

		assert.Nil(t, response.Data)
		assert.Equal(t, "Not found", response.Message)
		assert.False(t, response.Success)
	})
}

func TestGlobalAPIGame_WithMods(t *testing.T) {
	t.Parallel()

	game := &GlobalAPIGame{
		Code:   "csgo",
		Name:   "Counter-Strike: Global Offensive",
		Engine: "Source",
		Mods: []GlobalAPIGameMod{
			{
				ID:       1,
				GameCode: "csgo",
				Name:     "Classic",
			},
			{
				ID:       2,
				GameCode: "csgo",
				Name:     "Competitive",
			},
		},
	}

	require.Len(t, game.Mods, 2)
	assert.Equal(t, "Classic", game.Mods[0].Name)
	assert.Equal(t, "Competitive", game.Mods[1].Name)
}

func TestGlobalAPIGameMod_ToDomainGameMod_NilLists(t *testing.T) {
	t.Parallel()

	mod := &GlobalAPIGameMod{
		ID:       1,
		GameCode: "test",
		Name:     "Test Mod",
		FastRcon: nil,
		Vars:     nil,
	}

	result := mod.ToDomainGameMod()

	assert.Nil(t, result.FastRcon)
	assert.Nil(t, result.Vars)
}

func TestGlobalAPIGameMod_ToDomainGameMod_EmptyLists(t *testing.T) {
	t.Parallel()

	mod := &GlobalAPIGameMod{
		ID:       1,
		GameCode: "test",
		Name:     "Test Mod",
		FastRcon: GameModFastRconList{},
		Vars:     GameModVarList{},
	}

	result := mod.ToDomainGameMod()

	assert.NotNil(t, result.FastRcon)
	assert.Empty(t, result.FastRcon)
	assert.NotNil(t, result.Vars)
	assert.Empty(t, result.Vars)
}
