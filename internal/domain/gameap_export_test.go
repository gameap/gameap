package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGameExport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantExport *GameExport
		wantError  string
	}{
		{
			name: "valid_minimal_export",
			input: `
schema_version: "1.0"
game:
  code: "cstrike"
  name: "Counter-Strike 1.6"
  engine: "GoldSource"
`,
			wantExport: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "cstrike",
					Name:   "Counter-Strike 1.6",
					Engine: "GoldSource",
				},
			},
		},
		{
			name: "valid_full_export",
			input: `
schema_version: "1.0"
exported_at: "2024-01-15T10:30:00Z"
exported_by: "GameAP v3.0.0"
game:
  code: "cstrike"
  name: "Counter-Strike 1.6"
  engine: "GoldSource"
  engine_version: "1.0"
  steam_app_id_linux: 90
  steam_app_id_windows: 90
  steam_app_set_config: "mod cstrike"
  remote_repository_linux: "http://example.com/linux"
  remote_repository_windows: "http://example.com/windows"
  metadata:
    custom_key: "custom_value"
mods:
  - name: "Classic"
    start_cmd_linux: "./hlds_run -game cstrike +port {port}"
    start_cmd_windows: "hlds.exe -game cstrike +port {port}"
    kick_cmd: "kick {name}"
    ban_cmd: "banid 0 {name} kick"
    srestart_cmd: "restart"
    chmap_cmd: "changelevel {map}"
    sendmsg_cmd: "say {msg}"
    passwd_cmd: "sv_password {password}"
    fast_rcon:
      - info: "Restart Map"
        command: "changelevel {map}"
    vars:
      - var: "maxplayers"
        default: "32"
        info: "Maximum players"
        admin_var: false
`,
			wantExport: &GameExport{
				SchemaVersion: "1.0",
				ExportedAt:    "2024-01-15T10:30:00Z",
				ExportedBy:    "GameAP v3.0.0",
				Game: GameExportGame{
					Code:                    "cstrike",
					Name:                    "Counter-Strike 1.6",
					Engine:                  "GoldSource",
					EngineVersion:           "1.0",
					SteamAppIDLinux:         new(uint(90)),
					SteamAppIDWindows:       new(uint(90)),
					SteamAppSetConfig:       new("mod cstrike"),
					RemoteRepositoryLinux:   new("http://example.com/linux"),
					RemoteRepositoryWindows: new("http://example.com/windows"),
					Metadata: Metadata{
						"custom_key": "custom_value",
					},
				},
				Mods: []GameExportMod{
					{
						Name:            "Classic",
						StartCmdLinux:   new("./hlds_run -game cstrike +port {port}"),
						StartCmdWindows: new("hlds.exe -game cstrike +port {port}"),
						KickCmd:         new("kick {name}"),
						BanCmd:          new("banid 0 {name} kick"),
						SrestartCmd:     new("restart"),
						ChmapCmd:        new("changelevel {map}"),
						SendmsgCmd:      new("say {msg}"),
						PasswdCmd:       new("sv_password {password}"),
						FastRcon: []GameExportModFastRcon{
							{Info: "Restart Map", Command: "changelevel {map}"},
						},
						Vars: []GameExportModVar{
							{Var: "maxplayers", Default: "32", Info: "Maximum players", AdminVar: false},
						},
					},
				},
			},
		},
		{
			name: "valid_export_without_mods",
			input: `
schema_version: "1.0"
game:
  code: "test"
  name: "Test Game"
  engine: "Test Engine"
mods: []
`,
			wantExport: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
				Mods: []GameExportMod{},
			},
		},
		{
			name: "invalid_yaml",
			input: `
schema_version: "1.0"
game:
  code: "test
  name: "Test Game"
`,
			wantError: "failed to parse GameAP YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			export, err := ParseGameExport([]byte(tt.input))

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantExport.SchemaVersion, export.SchemaVersion)
			assert.Equal(t, tt.wantExport.Game.Code, export.Game.Code)
			assert.Equal(t, tt.wantExport.Game.Name, export.Game.Name)
			assert.Equal(t, tt.wantExport.Game.Engine, export.Game.Engine)
			require.Len(t, export.Mods, len(tt.wantExport.Mods))
		})
	}
}

func TestGameExport_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		export    *GameExport
		wantError string
	}{
		{
			name: "valid_minimal",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
		},
		{
			name: "valid_with_mods",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
				Mods: []GameExportMod{
					{Name: "Mod1"},
					{Name: "Mod2"},
				},
			},
		},
		{
			name: "missing_schema_version",
			export: &GameExport{
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
			wantError: "schema_version is required",
		},
		{
			name: "unsupported_schema_version",
			export: &GameExport{
				SchemaVersion: "2.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
			wantError: "unsupported schema version",
		},
		{
			name: "missing_game_code",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
			wantError: "game.code is required",
		},
		{
			name: "game_code_too_short",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "t",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
			wantError: "game.code must be between 2 and 16 characters",
		},
		{
			name: "game_code_too_long",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "this_code_is_way_too_long",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
			wantError: "game.code must be between 2 and 16 characters",
		},
		{
			name: "game_code_invalid_format",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "Test Game",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
			wantError: "game.code must match pattern",
		},
		{
			name: "game_code_with_uppercase",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "TestGame",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
			wantError: "game.code must match pattern",
		},
		{
			name: "missing_game_name",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Engine: "Test Engine",
				},
			},
			wantError: "game.name is required",
		},
		{
			name: "game_name_too_short",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "T",
					Engine: "Test Engine",
				},
			},
			wantError: "game.name must be between 2 and 128 characters",
		},
		{
			name: "missing_game_engine",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code: "test",
					Name: "Test Game",
				},
			},
			wantError: "game.engine is required",
		},
		{
			name: "missing_mod_name",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
				Mods: []GameExportMod{
					{Name: ""},
				},
			},
			wantError: "mods[0].name is required",
		},
		{
			name: "duplicate_mod_names",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
				Mods: []GameExportMod{
					{Name: "Classic"},
					{Name: "Classic"},
				},
			},
			wantError: "duplicate mod name: Classic",
		},
		{
			name: "start_cmd_linux_too_long",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
				Mods: []GameExportMod{
					{
						Name:          "Mod1",
						StartCmdLinux: new(string(make([]byte, 1001))),
					},
				},
			},
			wantError: "mods[0].start_cmd_linux must be at most 1000 characters",
		},
		{
			name: "kick_cmd_too_long",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
				Mods: []GameExportMod{
					{
						Name:    "Mod1",
						KickCmd: new(string(make([]byte, 201))),
					},
				},
			},
			wantError: "mods[0].kick_cmd must be at most 200 characters",
		},
		{
			name: "code_at_min_2",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "ab",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
		},
		{
			name: "code_at_max_16",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   strings.Repeat("a", 16),
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
		},
		{
			name: "code_too_long_17",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   strings.Repeat("a", 17),
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
			wantError: "game.code must be between 2 and 16 characters",
		},
		{
			name: "name_at_min_2",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "ab",
					Engine: "Test Engine",
				},
			},
		},
		{
			name: "name_at_max_128",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   strings.Repeat("a", 128),
					Engine: "Test Engine",
				},
			},
		},
		{
			name: "name_too_long_129",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   strings.Repeat("a", 129),
					Engine: "Test Engine",
				},
			},
			wantError: "game.name must be between 2 and 128 characters",
		},
		{
			name: "engine_at_max_length_128",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: strings.Repeat("a", 128),
				},
			},
		},
		{
			name: "engine_too_long_129",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: strings.Repeat("a", 129),
				},
			},
			wantError: "game.engine must be at most 128 characters",
		},
		{
			name: "engine_version_at_max_128",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:          "test",
					Name:          "Test Game",
					Engine:        "Test Engine",
					EngineVersion: strings.Repeat("a", 128),
				},
			},
		},
		{
			name: "engine_version_too_long_129",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:          "test",
					Name:          "Test Game",
					Engine:        "Test Engine",
					EngineVersion: strings.Repeat("a", 129),
				},
			},
			wantError: "game.engine_version must be at most 128 characters",
		},
		{
			name: "remote_repo_linux_at_max_128",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:                  "test",
					Name:                  "Test Game",
					Engine:                "Test Engine",
					RemoteRepositoryLinux: new(strings.Repeat("a", 128)),
				},
			},
		},
		{
			name: "remote_repo_linux_too_long_129",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:                  "test",
					Name:                  "Test Game",
					Engine:                "Test Engine",
					RemoteRepositoryLinux: new(strings.Repeat("a", 129)),
				},
			},
			wantError: "game.remote_repository_linux must be at most 128 characters",
		},
		{
			name: "remote_repo_windows_at_max_128",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:                    "test",
					Name:                    "Test Game",
					Engine:                  "Test Engine",
					RemoteRepositoryWindows: new(strings.Repeat("a", 128)),
				},
			},
		},
		{
			name: "remote_repo_windows_too_long_129",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:                    "test",
					Name:                    "Test Game",
					Engine:                  "Test Engine",
					RemoteRepositoryWindows: new(strings.Repeat("a", 129)),
				},
			},
			wantError: "game.remote_repository_windows must be at most 128 characters",
		},
		{
			name: "local_repo_linux_at_max_128",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:                 "test",
					Name:                 "Test Game",
					Engine:               "Test Engine",
					LocalRepositoryLinux: new(strings.Repeat("a", 128)),
				},
			},
		},
		{
			name: "local_repo_linux_too_long_129",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:                 "test",
					Name:                 "Test Game",
					Engine:               "Test Engine",
					LocalRepositoryLinux: new(strings.Repeat("a", 129)),
				},
			},
			wantError: "game.local_repository_linux must be at most 128 characters",
		},
		{
			name: "local_repo_windows_at_max_128",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:                   "test",
					Name:                   "Test Game",
					Engine:                 "Test Engine",
					LocalRepositoryWindows: new(strings.Repeat("a", 128)),
				},
			},
		},
		{
			name: "local_repo_windows_too_long_129",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:                   "test",
					Name:                   "Test Game",
					Engine:                 "Test Engine",
					LocalRepositoryWindows: new(strings.Repeat("a", 129)),
				},
			},
			wantError: "game.local_repository_windows must be at most 128 characters",
		},
		{
			name: "steam_app_set_config_at_max_128",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:              "test",
					Name:              "Test Game",
					Engine:            "Test Engine",
					SteamAppSetConfig: new(strings.Repeat("a", 128)),
				},
			},
		},
		{
			name: "steam_app_set_config_too_long_129",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:              "test",
					Name:              "Test Game",
					Engine:            "Test Engine",
					SteamAppSetConfig: new(strings.Repeat("a", 129)),
				},
			},
			wantError: "game.steam_app_set_config must be at most 128 characters",
		},
		{
			name: "start_cmd_linux_at_max_1000",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
				Mods: []GameExportMod{
					{
						Name:          "Mod1",
						StartCmdLinux: new(strings.Repeat("a", 1000)),
					},
				},
			},
		},
		{
			name: "kick_cmd_at_max_200",
			export: &GameExport{
				SchemaVersion: "1.0",
				Game: GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
				Mods: []GameExportMod{
					{
						Name:    "Mod1",
						KickCmd: new(strings.Repeat("a", 200)),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.export.Validate()

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message should contain expected substring")

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestValidateModCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mod       GameExportMod
		index     int
		wantError string
	}{
		{
			name:  "all_commands_nil_passes",
			mod:   GameExportMod{Name: "Mod1"},
			index: 0,
		},
		{
			name: "start_cmd_linux_at_max_1000",
			mod: GameExportMod{
				Name:          "Mod1",
				StartCmdLinux: new(strings.Repeat("a", 1000)),
			},
			index: 0,
		},
		{
			name: "start_cmd_linux_too_long_1001",
			mod: GameExportMod{
				Name:          "Mod1",
				StartCmdLinux: new(strings.Repeat("a", 1001)),
			},
			index:     0,
			wantError: "mods[0].start_cmd_linux must be at most 1000 characters",
		},
		{
			name: "start_cmd_windows_at_max_1000",
			mod: GameExportMod{
				Name:            "Mod1",
				StartCmdWindows: new(strings.Repeat("a", 1000)),
			},
			index: 0,
		},
		{
			name: "start_cmd_windows_too_long_1001",
			mod: GameExportMod{
				Name:            "Mod1",
				StartCmdWindows: new(strings.Repeat("a", 1001)),
			},
			index:     0,
			wantError: "mods[0].start_cmd_windows must be at most 1000 characters",
		},
		{
			name: "kick_cmd_at_max_200",
			mod: GameExportMod{
				Name:    "Mod1",
				KickCmd: new(strings.Repeat("a", 200)),
			},
			index: 0,
		},
		{
			name: "kick_cmd_too_long_201",
			mod: GameExportMod{
				Name:    "Mod1",
				KickCmd: new(strings.Repeat("a", 201)),
			},
			index:     0,
			wantError: "mods[0].kick_cmd must be at most 200 characters",
		},
		{
			name: "ban_cmd_at_max_200",
			mod: GameExportMod{
				Name:   "Mod1",
				BanCmd: new(strings.Repeat("a", 200)),
			},
			index: 0,
		},
		{
			name: "ban_cmd_too_long_201",
			mod: GameExportMod{
				Name:   "Mod1",
				BanCmd: new(strings.Repeat("a", 201)),
			},
			index:     0,
			wantError: "mods[0].ban_cmd must be at most 200 characters",
		},
		{
			name: "chname_cmd_at_max_200",
			mod: GameExportMod{
				Name:      "Mod1",
				ChnameCmd: new(strings.Repeat("a", 200)),
			},
			index: 0,
		},
		{
			name: "chname_cmd_too_long_201",
			mod: GameExportMod{
				Name:      "Mod1",
				ChnameCmd: new(strings.Repeat("a", 201)),
			},
			index:     0,
			wantError: "mods[0].chname_cmd must be at most 200 characters",
		},
		{
			name: "srestart_cmd_at_max_200",
			mod: GameExportMod{
				Name:        "Mod1",
				SrestartCmd: new(strings.Repeat("a", 200)),
			},
			index: 0,
		},
		{
			name: "srestart_cmd_too_long_201",
			mod: GameExportMod{
				Name:        "Mod1",
				SrestartCmd: new(strings.Repeat("a", 201)),
			},
			index:     0,
			wantError: "mods[0].srestart_cmd must be at most 200 characters",
		},
		{
			name: "chmap_cmd_at_max_200",
			mod: GameExportMod{
				Name:     "Mod1",
				ChmapCmd: new(strings.Repeat("a", 200)),
			},
			index: 0,
		},
		{
			name: "chmap_cmd_too_long_201",
			mod: GameExportMod{
				Name:     "Mod1",
				ChmapCmd: new(strings.Repeat("a", 201)),
			},
			index:     0,
			wantError: "mods[0].chmap_cmd must be at most 200 characters",
		},
		{
			name: "sendmsg_cmd_at_max_200",
			mod: GameExportMod{
				Name:       "Mod1",
				SendmsgCmd: new(strings.Repeat("a", 200)),
			},
			index: 0,
		},
		{
			name: "sendmsg_cmd_too_long_201",
			mod: GameExportMod{
				Name:       "Mod1",
				SendmsgCmd: new(strings.Repeat("a", 201)),
			},
			index:     0,
			wantError: "mods[0].sendmsg_cmd must be at most 200 characters",
		},
		{
			name: "passwd_cmd_at_max_200",
			mod: GameExportMod{
				Name:      "Mod1",
				PasswdCmd: new(strings.Repeat("a", 200)),
			},
			index: 0,
		},
		{
			name: "passwd_cmd_too_long_201",
			mod: GameExportMod{
				Name:      "Mod1",
				PasswdCmd: new(strings.Repeat("a", 201)),
			},
			index:     0,
			wantError: "mods[0].passwd_cmd must be at most 200 characters",
		},
		{
			name: "mod_remote_repo_linux_at_max_128",
			mod: GameExportMod{
				Name:                  "Mod1",
				RemoteRepositoryLinux: new(strings.Repeat("a", 128)),
			},
			index: 0,
		},
		{
			name: "mod_remote_repo_linux_too_long_129",
			mod: GameExportMod{
				Name:                  "Mod1",
				RemoteRepositoryLinux: new(strings.Repeat("a", 129)),
			},
			index:     0,
			wantError: "mods[0].remote_repository_linux must be at most 128 characters",
		},
		{
			name: "mod_remote_repo_windows_at_max_128",
			mod: GameExportMod{
				Name:                    "Mod1",
				RemoteRepositoryWindows: new(strings.Repeat("a", 128)),
			},
			index: 0,
		},
		{
			name: "mod_remote_repo_windows_too_long_129",
			mod: GameExportMod{
				Name:                    "Mod1",
				RemoteRepositoryWindows: new(strings.Repeat("a", 129)),
			},
			index:     0,
			wantError: "mods[0].remote_repository_windows must be at most 128 characters",
		},
		{
			name: "mod_local_repo_linux_at_max_128",
			mod: GameExportMod{
				Name:                 "Mod1",
				LocalRepositoryLinux: new(strings.Repeat("a", 128)),
			},
			index: 0,
		},
		{
			name: "mod_local_repo_linux_too_long_129",
			mod: GameExportMod{
				Name:                 "Mod1",
				LocalRepositoryLinux: new(strings.Repeat("a", 129)),
			},
			index:     0,
			wantError: "mods[0].local_repository_linux must be at most 128 characters",
		},
		{
			name: "mod_local_repo_windows_at_max_128",
			mod: GameExportMod{
				Name:                   "Mod1",
				LocalRepositoryWindows: new(strings.Repeat("a", 128)),
			},
			index: 0,
		},
		{
			name: "mod_local_repo_windows_too_long_129",
			mod: GameExportMod{
				Name:                   "Mod1",
				LocalRepositoryWindows: new(strings.Repeat("a", 129)),
			},
			index:     0,
			wantError: "mods[0].local_repository_windows must be at most 128 characters",
		},
		{
			name: "index_in_error_message",
			mod: GameExportMod{
				Name:          "Mod1",
				StartCmdLinux: new(strings.Repeat("a", 1001)),
			},
			index:     5,
			wantError: "mods[5].start_cmd_linux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			mod := tt.mod

			// ACT
			err := validateModCommands(&mod, tt.index)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message should contain expected substring")

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGameExportGame_ToDomainGame(t *testing.T) {
	t.Parallel()

	exportGame := &GameExportGame{
		Code:                    "cstrike",
		Name:                    "Counter-Strike 1.6",
		Engine:                  "GoldSource",
		EngineVersion:           "1.0",
		SteamAppIDLinux:         new(uint(90)),
		SteamAppIDWindows:       new(uint(90)),
		SteamAppSetConfig:       new("mod cstrike"),
		RemoteRepositoryLinux:   new("http://example.com/linux"),
		RemoteRepositoryWindows: new("http://example.com/windows"),
		LocalRepositoryLinux:    new("/local/linux"),
		LocalRepositoryWindows:  new("C:\\local\\windows"),
		Metadata: Metadata{
			"custom": "value",
		},
	}

	game := exportGame.ToDomainGame()

	assert.Equal(t, "cstrike", game.Code)
	assert.Equal(t, "Counter-Strike 1.6", game.Name)
	assert.Equal(t, "GoldSource", game.Engine)
	assert.Equal(t, "1.0", game.EngineVersion)
	assert.Equal(t, uint(90), *game.SteamAppIDLinux)
	assert.Equal(t, uint(90), *game.SteamAppIDWindows)
	assert.Equal(t, "mod cstrike", *game.SteamAppSetConfig)
	assert.Equal(t, "http://example.com/linux", *game.RemoteRepositoryLinux)
	assert.Equal(t, "http://example.com/windows", *game.RemoteRepositoryWindows)
	assert.Equal(t, "/local/linux", *game.LocalRepositoryLinux)
	assert.Equal(t, "C:\\local\\windows", *game.LocalRepositoryWindows)
	assert.Equal(t, 1, game.Enabled)
	assert.Equal(t, "value", game.Metadata["custom"])
}

func TestGameExportMod_ToDomainGameMod(t *testing.T) {
	t.Parallel()

	exportMod := &GameExportMod{
		Name: "Classic",
		FastRcon: []GameExportModFastRcon{
			{Info: "Restart", Command: "restart"},
		},
		Vars: []GameExportModVar{
			{Var: "maxplayers", Default: "32", Info: "Max players", AdminVar: false},
		},
		RemoteRepositoryLinux:   new("http://mod.com/linux"),
		RemoteRepositoryWindows: new("http://mod.com/windows"),
		LocalRepositoryLinux:    new("/mod/linux"),
		LocalRepositoryWindows:  new("C:\\mod\\windows"),
		StartCmdLinux:           new("./start.sh"),
		StartCmdWindows:         new("start.exe"),
		KickCmd:                 new("kick {name}"),
		BanCmd:                  new("ban {name}"),
		ChnameCmd:               new("rename {name}"),
		SrestartCmd:             new("restart"),
		ChmapCmd:                new("changelevel {map}"),
		SendmsgCmd:              new("say {msg}"),
		PasswdCmd:               new("password {pass}"),
		Metadata: Metadata{
			"mod_key": "mod_value",
		},
	}

	gameMod := exportMod.ToDomainGameMod("cstrike")

	assert.Equal(t, "cstrike", gameMod.GameCode)
	assert.Equal(t, "Classic", gameMod.Name)
	require.Len(t, gameMod.FastRcon, 1)
	assert.Equal(t, "Restart", gameMod.FastRcon[0].Info)
	assert.Equal(t, "restart", gameMod.FastRcon[0].Command)
	require.Len(t, gameMod.Vars, 1)
	assert.Equal(t, "maxplayers", gameMod.Vars[0].Var)
	assert.Equal(t, GameModVarDefault("32"), gameMod.Vars[0].Default)
	assert.Equal(t, "http://mod.com/linux", *gameMod.RemoteRepositoryLinux)
	assert.Equal(t, "./start.sh", *gameMod.StartCmdLinux)
	assert.Equal(t, "kick {name}", *gameMod.KickCmd)
	assert.Equal(t, "mod_value", gameMod.Metadata["mod_key"])
}

func TestNewGameExportFromDomain(t *testing.T) {
	t.Parallel()

	game := &Game{
		Code:                    "cstrike",
		Name:                    "Counter-Strike 1.6",
		Engine:                  "GoldSource",
		EngineVersion:           "1.0",
		SteamAppIDLinux:         new(uint(90)),
		SteamAppIDWindows:       new(uint(90)),
		SteamAppSetConfig:       new("mod cstrike"),
		RemoteRepositoryLinux:   new("http://example.com/linux"),
		RemoteRepositoryWindows: new("http://example.com/windows"),
		Enabled:                 1,
		Metadata: Metadata{
			"custom":      "value",
			"pelican_egg": map[string]any{"should": "be excluded"},
		},
	}

	mods := []GameMod{
		{
			ID:       1,
			GameCode: "cstrike",
			Name:     "Classic",
			FastRcon: GameModFastRconList{
				{Info: "Restart", Command: "restart"},
			},
			Vars: GameModVarList{
				{Var: "maxplayers", Default: "32", Info: "Max players"},
			},
			StartCmdLinux: new("./hlds_run"),
			KickCmd:       new("kick {name}"),
			Metadata: Metadata{
				"pelican_egg": map[string]any{"should": "be excluded"},
			},
		},
	}

	export := NewGameExportFromDomain(game, mods, "v3.0.0")

	assert.Equal(t, CurrentSchemaVersion, export.SchemaVersion)
	assert.NotEmpty(t, export.ExportedAt)
	assert.Equal(t, "GameAP v3.0.0", export.ExportedBy)

	assert.Equal(t, "cstrike", export.Game.Code)
	assert.Equal(t, "Counter-Strike 1.6", export.Game.Name)
	assert.Equal(t, "GoldSource", export.Game.Engine)
	assert.Equal(t, uint(90), *export.Game.SteamAppIDLinux)
	assert.Equal(t, "value", export.Game.Metadata["custom"])
	assert.NotNil(t, export.Game.Metadata["pelican_egg"])
	require.Len(t, export.Mods, 1)
	assert.Equal(t, "Classic", export.Mods[0].Name)
	assert.Equal(t, "./hlds_run", *export.Mods[0].StartCmdLinux)
	require.Len(t, export.Mods[0].FastRcon, 1)
	require.Len(t, export.Mods[0].Vars, 1)
	assert.NotNil(t, export.Mods[0].Metadata["pelican_egg"])
}

func TestNewGameExportFromDomain_FastRconAndVarsNilWhenEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		fastRcon        GameModFastRconList
		vars            GameModVarList
		wantFastRconNil bool
		wantVarsNil     bool
		wantFastRconLen int
		wantVarsLen     int
	}{
		{
			name:            "nil_fast_rcon_and_vars_yield_nil_slices",
			fastRcon:        nil,
			vars:            nil,
			wantFastRconNil: true,
			wantVarsNil:     true,
		},
		{
			name:            "empty_fast_rcon_and_vars_yield_nil_slices",
			fastRcon:        GameModFastRconList{},
			vars:            GameModVarList{},
			wantFastRconNil: true,
			wantVarsNil:     true,
		},
		{
			name: "non_empty_fast_rcon_and_vars_yield_non_nil_slices",
			fastRcon: GameModFastRconList{
				{Info: "Status", Command: "status"},
			},
			vars: GameModVarList{
				{Var: "maxplayers", Default: "32", Info: "Max players"},
			},
			wantFastRconNil: false,
			wantVarsNil:     false,
			wantFastRconLen: 1,
			wantVarsLen:     1,
		},
	}

	game := &Game{
		Code:    "cstrike",
		Name:    "Counter-Strike 1.6",
		Engine:  "GoldSource",
		Enabled: 1,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			mods := []GameMod{
				{
					ID:       1,
					GameCode: "cstrike",
					Name:     "Classic",
					FastRcon: tt.fastRcon,
					Vars:     tt.vars,
				},
			}

			// ACT
			export := NewGameExportFromDomain(game, mods, "")

			// ASSERT
			require.Len(t, export.Mods, 1)

			if tt.wantFastRconNil {
				assert.Nil(t, export.Mods[0].FastRcon, "fast rcon should be nil when source is empty")
			} else {
				require.Len(t, export.Mods[0].FastRcon, tt.wantFastRconLen)
			}

			if tt.wantVarsNil {
				assert.Nil(t, export.Mods[0].Vars, "vars should be nil when source is empty")
			} else {
				require.Len(t, export.Mods[0].Vars, tt.wantVarsLen)
			}
		})
	}
}

func TestNewGameExportFromDomain_ExportedByWithoutVersion(t *testing.T) {
	t.Parallel()

	// ARRANGE
	game := &Game{
		Code:    "cstrike",
		Name:    "Counter-Strike 1.6",
		Engine:  "GoldSource",
		Enabled: 1,
	}

	// ACT
	export := NewGameExportFromDomain(game, nil, "")

	// ASSERT
	assert.Equal(t, "GameAP", export.ExportedBy, "without version, ExportedBy should be exact 'GameAP'")
}

func TestGameExport_ToYAML(t *testing.T) {
	t.Parallel()

	export := &GameExport{
		SchemaVersion: "1.0",
		ExportedAt:    "2024-01-15T10:30:00Z",
		ExportedBy:    "GameAP v3.0.0",
		Game: GameExportGame{
			Code:   "test",
			Name:   "Test Game",
			Engine: "Test Engine",
		},
		Mods: []GameExportMod{
			{
				Name:          "Default",
				StartCmdLinux: new("./start.sh"),
			},
		},
	}

	yamlData, err := export.ToYAML()
	require.NoError(t, err)

	parsed, err := ParseGameExport(yamlData)
	require.NoError(t, err)

	assert.Equal(t, export.SchemaVersion, parsed.SchemaVersion)
	assert.Equal(t, export.Game.Code, parsed.Game.Code)
	assert.Equal(t, export.Game.Name, parsed.Game.Name)
	require.Len(t, parsed.Mods, 1)
	assert.Equal(t, "Default", parsed.Mods[0].Name)
	assert.Equal(t, "./start.sh", *parsed.Mods[0].StartCmdLinux)
}

func TestGameExport_TypedVarsRoundTrip(t *testing.T) {
	t.Parallel()

	// ARRANGE
	source := `
schema_version: "1.0"
game:
  code: minecraft
  name: Minecraft
  engine: Java
mods:
  - name: Default
    fast_rcon:
      - info: Status
        command: status
        i18n:
          ru:
            info: Статус
    vars:
      - var: version
        default: '1.20.4'
        info: Minecraft version
        admin_var: true
        type: select
        description: The version the server runs
        allow_custom: true
        options:
          - '1.21'
          - {value: '1.20.4', label: 1.20.4 (LTS), i18n: {ru: {label: 1.20.4 (LTS)}}}
        rules:
          min_length: 1
          max_length: 16
          pattern: '[0-9.]+'
        i18n:
          ru:
            info: Версия Minecraft
      - var: pvp
        default: 'on'
        info: PvP
        type: bool
        true_value: 'on'
        false_value: 'off'
      - var: maxplayers
        default: 20
        info: Max players
        type: int
        rules:
          required: true
          min: 1
          max: 64
`

	// ACT
	export, err := ParseGameExport([]byte(source))
	require.NoError(t, err)

	validateErr := export.Validate()

	// ASSERT
	require.NoError(t, validateErr)
	require.Len(t, export.Mods, 1)
	require.Len(t, export.Mods[0].Vars, 3)

	version := export.Mods[0].Vars[0]
	assert.Equal(t, GameModVarTypeSelect, version.Type)
	assert.True(t, version.AllowCustom)
	assert.Equal(t, "The version the server runs", version.Description)
	assert.Equal(t, GameModVarOptions{
		{Value: "1.21"},
		{Value: "1.20.4", Label: "1.20.4 (LTS)", I18n: GameModVarOptionI18n{"ru": {Label: "1.20.4 (LTS)"}}},
	}, version.Options)
	require.NotNil(t, version.Rules)
	assert.Equal(t, 16, *version.Rules.MaxLength)
	assert.Equal(t, GameModVarI18n{"ru": {Info: "Версия Minecraft"}}, version.I18n)

	pvp := export.Mods[0].Vars[1]
	require.NotNil(t, pvp.TrueValue)
	require.NotNil(t, pvp.FalseValue)
	assert.Equal(t, "on", *pvp.TrueValue)
	assert.Equal(t, "off", *pvp.FalseValue)

	// A bare YAML number reaches the panel as its literal text.
	assert.Equal(t, GameModVarDefault("20"), export.Mods[0].Vars[2].Default)

	assert.Equal(t,
		GameModFastRconI18n{"ru": {Info: "Статус"}},
		export.Mods[0].FastRcon[0].I18n,
	)

	// ACT: the round trip through the domain model and back must not lose a field.
	// The comparison is made against a second parse: a conversion that mutated
	// its source would otherwise be compared against its own output.
	original, originalErr := ParseGameExport([]byte(source))
	require.NoError(t, originalErr)

	gameMod := export.Mods[0].ToDomainGameMod(export.Game.Code)
	reexported := NewGameExportFromDomain(export.Game.ToDomainGame(), []GameMod{*gameMod}, "")

	encoded, encodeErr := reexported.ToYAML()
	require.NoError(t, encodeErr)

	reparsed, parseErr := ParseGameExport(encoded)

	// ASSERT
	require.NoError(t, parseErr)
	require.Len(t, reparsed.Mods, 1)
	assert.Equal(t, original.Mods[0].Vars, reparsed.Mods[0].Vars)
	assert.Equal(t, original.Mods[0].FastRcon, reparsed.Mods[0].FastRcon)
}

// Normalize rewrites an option in place, so both conversions have to work on a
// copy of the option list: a shared backing array would let a conversion edit
// the definition the caller still holds.
func TestGameExportMod_ConversionsDoNotRewriteTheirSource(t *testing.T) {
	t.Parallel()

	// ARRANGE: a label repeating its value and an en translation are exactly
	// what Normalize drops.
	sourceOptions := func() GameModVarOptions {
		return GameModVarOptions{
			{Value: "vanilla", Label: "vanilla"},
			{Value: "paper", Label: "Paper", I18n: GameModVarOptionI18n{"en": {Label: "Paper"}}},
		}
	}

	exportMod := GameExportMod{
		Name: "Vanilla",
		Vars: []GameExportModVar{
			{Var: "mod", Default: "vanilla", Info: "Mod", Type: GameModVarTypeSelect, Options: sourceOptions()},
		},
	}

	gameMod := GameMod{
		GameCode: "minecraft",
		Name:     "Vanilla",
		Vars: GameModVarList{
			{Var: "mod", Default: "vanilla", Info: "Mod", Type: GameModVarTypeSelect, Options: sourceOptions()},
		},
	}

	// ACT
	converted := exportMod.ToDomainGameMod("minecraft")
	exported := NewGameExportFromDomain(&Game{Code: "minecraft"}, []GameMod{gameMod}, "")

	// ASSERT
	normalized := GameModVarOptions{{Value: "vanilla"}, {Value: "paper", Label: "Paper"}}

	require.Len(t, converted.Vars, 1)
	assert.Equal(t, normalized, converted.Vars[0].Options)
	require.Len(t, exportMod.Vars, 1)
	assert.Equal(t, sourceOptions(), exportMod.Vars[0].Options)

	require.Len(t, exported.Mods, 1)
	require.Len(t, exported.Mods[0].Vars, 1)
	assert.Equal(t, normalized, exported.Mods[0].Vars[0].Options)
	require.Len(t, gameMod.Vars, 1)
	assert.Equal(t, sourceOptions(), gameMod.Vars[0].Options)
}

func TestGameExport_Validate_RejectsInvalidVars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		vars      string
		wantError string
	}{
		{
			name:      "select_without_options",
			vars:      "      - var: mod\n        info: Mod\n        type: select\n",
			wantError: "type select requires a non-empty options list",
		},
		{
			name:      "unknown_type",
			vars:      "      - var: mod\n        info: Mod\n        type: date\n",
			wantError: "unknown variable type: date",
		},
		{
			name:      "duplicate_names",
			vars:      "      - var: mod\n        info: Mod\n      - var: mod\n        info: Mod again\n",
			wantError: "duplicate variable name: mod",
		},
		{
			name:      "name_too_long",
			vars:      "      - var: a_very_long_environment_variable_name\n        info: Long\n",
			wantError: "variable name must be at most 32 characters",
		},
		{
			name:      "missing_info",
			vars:      "      - var: mod\n",
			wantError: "variable info is required",
		},
		{
			name:      "pattern_with_lookahead",
			vars:      "      - var: mod\n        info: Mod\n        rules:\n          pattern: '(?=a).*'\n",
			wantError: "invalid regular expression",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			source := "schema_version: \"1.0\"\ngame:\n  code: minecraft\n  name: Minecraft\n  engine: Java\n" +
				"mods:\n  - name: Default\n    vars:\n" + test.vars

			export, err := ParseGameExport([]byte(source))
			require.NoError(t, err)

			// ACT
			validateErr := export.Validate()

			// ASSERT
			require.Error(t, validateErr)
			assert.Contains(t, validateErr.Error(), test.wantError)
		})
	}
}
