package pelicaneggimporter

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/domain/gamesimport"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImporter_Import(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		egg            *gamesimport.PelicanEgg
		setupGame      func(*inmemory.GameRepository)
		setupGameMod   func(*inmemory.GameModRepository)
		wantErr        bool
		wantErrContain string
		validate       func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult)
	}{
		{
			name: "successful_import_new_game",
			egg: &gamesimport.PelicanEgg{
				UUID:        "82c049db-06e3-416a-8ed3-805cc53105a9",
				Author:      "test@example.com",
				Name:        "Rage.MP",
				Description: "Rage Multiplayer Server",
				DockerImages: map[string]string{
					"ghcr.io/parkervcp/yolks:debian": "ghcr.io/parkervcp/yolks:debian",
				},
				Startup: "./ragemp-server",
				Config: gamesimport.PelicanEggConfig{
					Startup: gamesimport.PelicanEggConfigStartup{
						Done: "The server is ready to accept connections",
					},
					Stop: "^X",
				},
				Scripts: gamesimport.PelicanEggScripts{
					Installation: gamesimport.PelicanEggInstallationScript{
						Script:     "#!/bin/bash\nmkdir -p /mnt/server",
						Container:  "ghcr.io/parkervcp/installers:debian",
						Entrypoint: "bash",
					},
				},
				Variables: []gamesimport.PelicanEggVariable{
					{
						Name:         "Server Name",
						Description:  "Name of your server",
						EnvVariable:  "SERVER_NAME",
						DefaultValue: "My Rage.MP Server",
						UserViewable: true,
						UserEditable: true,
						Rules:        gamesimport.FlexibleRules{"required|string|max:64"},
						FieldType:    "text",
					},
				},
				Raw: map[string]any{
					"uuid":        "82c049db-06e3-416a-8ed3-805cc53105a9",
					"author":      "test@example.com",
					"name":        "Rage.MP",
					"description": "Rage Multiplayer Server",
					"_comment":    "DO NOT EDIT",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				require.NotNil(t, result.Game)
				require.NotNil(t, result.GameMod)

				assert.Equal(t, "rage_mp", result.Game.Code)
				assert.Equal(t, "Rage.MP", result.Game.Name)
				assert.Equal(t, "pelican", result.Game.Engine)
				assert.Equal(t, 1, result.Game.Enabled)

				pelicanEgg, ok := result.Game.Metadata["pelican_egg"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "82c049db-06e3-416a-8ed3-805cc53105a9", pelicanEgg["uuid"])
				assert.Equal(t, "DO NOT EDIT", pelicanEgg["_comment"])

				assert.Equal(t, "Default", result.GameMod.Name)
				assert.Equal(t, "rage_mp", result.GameMod.GameCode)
				assert.Equal(t, "./ragemp-server", lo.FromPtr(result.GameMod.StartCmdLinux))

				require.Len(t, result.GameMod.Vars, 1)
				assert.Equal(t, "SERVER_NAME", result.GameMod.Vars[0].Var)
				assert.Equal(t, "My Rage.MP Server", string(result.GameMod.Vars[0].Default))
				assert.Equal(t, "Name of your server", result.GameMod.Vars[0].Info)
				assert.False(t, result.GameMod.Vars[0].AdminVar)

				dockerImage, ok := result.GameMod.Metadata["docker_image"].(string)
				require.True(t, ok)
				assert.Equal(t, "ghcr.io/parkervcp/yolks:debian", dockerImage)

				startupDone, ok := result.GameMod.Metadata["docker_startup_done"].(string)
				require.True(t, ok)
				assert.Equal(t, "The server is ready to accept connections", startupDone)

				gameModPelicanEgg, ok := result.GameMod.Metadata["pelican_egg"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "82c049db-06e3-416a-8ed3-805cc53105a9", gameModPelicanEgg["uuid"])

				dockerEntrypoint, ok := result.GameMod.Metadata["docker_installation_entrypoint"].(string)
				require.True(t, ok)
				assert.Equal(t, "bash", dockerEntrypoint)

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)

				gameMods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, gameMods, 1)
			},
		},
		{
			name: "successful_import_with_startup_variable_transformation",
			egg: &gamesimport.PelicanEgg{
				Name:    "Test Game",
				Startup: "./server -port {{server.build.default.port}} -ip {{server.build.env.SERVER_IP}} -name {{SERVER_NAME}}",
				Variables: []gamesimport.PelicanEggVariable{
					{
						Name:         "Server Name",
						EnvVariable:  "SERVER_NAME",
						DefaultValue: "Test Server",
						UserEditable: false,
					},
				},
				Raw: map[string]any{
					"name":    "Test Game",
					"startup": "./server -port {{server.build.default.port}} -ip {{server.build.env.SERVER_IP}} -name {{SERVER_NAME}}",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, _ *inmemory.GameRepository, _ *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				expectedCmd := "./server -port {port} -ip {ip} -name {SERVER_NAME}"
				assert.Equal(t, expectedCmd, lo.FromPtr(result.GameMod.StartCmdLinux))
				assert.True(t, result.GameMod.Vars[0].AdminVar)
			},
		},
		{
			name: "successful_import_PLCN_v3_format_with_startup_commands",
			egg: &gamesimport.PelicanEgg{
				UUID:   "plcn-v3-uuid",
				Author: "test@example.com",
				Name:   "PLCN v3 Game",
				Meta: gamesimport.PelicanEggMeta{
					Version: "PLCN_v3",
				},
				StartupCommands: map[string]string{
					"Default": "./server -port {{server.build.default.port}} -name {{SERVER_NAME}}",
				},
				Variables: []gamesimport.PelicanEggVariable{
					{
						Name:         "Server Name",
						EnvVariable:  "SERVER_NAME",
						DefaultValue: "My Server",
						UserEditable: true,
						Rules:        gamesimport.FlexibleRules{"required", "string", "max:64"},
					},
				},
				Raw: map[string]any{
					"uuid": "plcn-v3-uuid",
					"name": "PLCN v3 Game",
					"startup_commands": map[string]any{
						"Default": "./server -port {{server.build.default.port}} -name {{SERVER_NAME}}",
					},
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, _ *inmemory.GameRepository, _ *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				expectedCmd := "./server -port {port} -name {SERVER_NAME}"
				assert.Equal(t, expectedCmd, lo.FromPtr(result.GameMod.StartCmdLinux))
				assert.Equal(t, "plcn_v3_game", result.Game.Code)
			},
		},
		{
			name: "PLCN_v3_format_with_legacy_startup_fallback",
			egg: &gamesimport.PelicanEgg{
				Name:    "Legacy Fallback Game",
				Startup: "./legacy_server",
				StartupCommands: map[string]string{
					"Other": "./other_server",
				},
				Raw: map[string]any{
					"name":    "Legacy Fallback Game",
					"startup": "./legacy_server",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, _ *inmemory.GameRepository, _ *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				assert.Equal(t, "./legacy_server", lo.FromPtr(result.GameMod.StartCmdLinux))
			},
		},
		{
			name: "successful_import_updates_existing_game",
			egg: &gamesimport.PelicanEgg{
				UUID: "new-uuid",
				Name: "Existing Game",
				Raw: map[string]any{
					"uuid": "new-uuid",
					"name": "Existing Game",
				},
			},
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "existing_game",
					Name:    "Existing Game",
					Engine:  "old_engine",
					Enabled: 1,
					Metadata: domain.Metadata{
						"custom_field": "preserve_me",
					},
				})
			},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, _ *inmemory.GameModRepository, _ *ImportResult) {
				t.Helper()

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)

				game := games[0]
				assert.Equal(t, "pelican", game.Engine)
				assert.Equal(t, "preserve_me", game.Metadata["custom_field"])
				pelicanEgg, ok := game.Metadata["pelican_egg"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "new-uuid", pelicanEgg["uuid"])
			},
		},
		{
			name: "successful_import_updates_existing_game_mod",
			egg: &gamesimport.PelicanEgg{
				Name:    "Test Game",
				Startup: "./new_server",
				Raw: map[string]any{
					"name":    "Test Game",
					"startup": "./new_server",
				},
			},
			setupGame: func(_ *inmemory.GameRepository) {},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "test_game",
					Name:          "Default",
					StartCmdLinux: new("./old_server"),
					Metadata: domain.Metadata{
						"existing_field": "keep_me",
					},
				})
			},
			wantErr: false,
			validate: func(t *testing.T, _ *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, _ *ImportResult) {
				t.Helper()

				gameMods, err := gameModRepo.Find(context.Background(), &filters.FindGameMod{
					Names:     []string{"Default"},
					GameCodes: []string{"test_game"},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, gameMods, 1)

				mod := gameMods[0]
				assert.Equal(t, "./new_server", lo.FromPtr(mod.StartCmdLinux))
				assert.Equal(t, "keep_me", mod.Metadata["existing_field"])
			},
		},
		{
			name:           "nil_egg_returns_error",
			egg:            nil,
			setupGame:      func(_ *inmemory.GameRepository) {},
			setupGameMod:   func(_ *inmemory.GameModRepository) {},
			wantErr:        true,
			wantErrContain: "egg cannot be nil",
		},
		{
			name: "empty_name_returns_error",
			egg: &gamesimport.PelicanEgg{
				Name: "",
			},
			setupGame:      func(_ *inmemory.GameRepository) {},
			setupGameMod:   func(_ *inmemory.GameModRepository) {},
			wantErr:        true,
			wantErrContain: "egg name is required",
		},
		{
			name: "short_name_gets_code_padding",
			egg: &gamesimport.PelicanEgg{
				Name: "X",
				Raw: map[string]any{
					"name": "X",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, _ *inmemory.GameRepository, _ *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				require.Len(t, result.Game.Code, 2)
			},
		},
		{
			name: "long_name_gets_truncated_code",
			egg: &gamesimport.PelicanEgg{
				Name: "This Is A Very Long Game Name That Should Be Truncated",
				Raw: map[string]any{
					"name": "This Is A Very Long Game Name That Should Be Truncated",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, _ *inmemory.GameRepository, _ *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				assert.LessOrEqual(t, len(result.Game.Code), 16)
			},
		},
		{
			name: "multiple_variables_transformation",
			egg: &gamesimport.PelicanEgg{
				Name:    "Multi Var Game",
				Startup: "./server",
				Variables: []gamesimport.PelicanEggVariable{
					{
						Name:         "Var One",
						EnvVariable:  "VAR_ONE",
						DefaultValue: "value1",
						UserEditable: true,
					},
					{
						Name:         "Var Two",
						Description:  "Second variable",
						EnvVariable:  "VAR_TWO",
						DefaultValue: "value2",
						UserEditable: false,
					},
					{
						Name:         "Var Three",
						EnvVariable:  "VAR_THREE",
						DefaultValue: "123",
						UserEditable: true,
					},
				},
				Raw: map[string]any{
					"name":    "Multi Var Game",
					"startup": "./server",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, _ *inmemory.GameRepository, _ *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				require.Len(t, result.GameMod.Vars, 3)

				assert.Equal(t, "VAR_ONE", result.GameMod.Vars[0].Var)
				assert.Equal(t, "Var One", result.GameMod.Vars[0].Info)
				assert.False(t, result.GameMod.Vars[0].AdminVar)

				assert.Equal(t, "VAR_TWO", result.GameMod.Vars[1].Var)
				assert.Equal(t, "Second variable", result.GameMod.Vars[1].Info)
				assert.True(t, result.GameMod.Vars[1].AdminVar)

				assert.Equal(t, "VAR_THREE", result.GameMod.Vars[2].Var)
				assert.Equal(t, "123", string(result.GameMod.Vars[2].Default))
				assert.False(t, result.GameMod.Vars[2].AdminVar)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()

			tt.setupGame(gameRepo)
			tt.setupGameMod(gameModRepo)

			importer := NewImporter(
				gameRepo,
				gameModRepo,
				services.NewNilTransactionManager(),
			)

			result, err := importer.Import(context.Background(), tt.egg, nil)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, gameRepo, gameModRepo, result)
				}
			}
		})
	}
}

func TestImporter_Import_WithOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		egg            *gamesimport.PelicanEgg
		opts           *gamesimport.Options
		wantErr        bool
		wantErrContain string
		validate       func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult)
	}{
		{
			name: "override_name_only",
			egg: &gamesimport.PelicanEgg{
				Name:    "Original Name",
				Startup: "./server",
				Raw: map[string]any{
					"name": "Original Name",
				},
			},
			opts: &gamesimport.Options{
				Name: new("Overridden Name"),
			},
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, _ *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				assert.Equal(t, "original_name", result.Game.Code)
				assert.Equal(t, "Overridden Name", result.Game.Name)

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "Overridden Name", games[0].Name)
			},
		},
		{
			name: "override_code_only",
			egg: &gamesimport.PelicanEgg{
				Name:    "Original Name",
				Startup: "./server",
				Raw: map[string]any{
					"name": "Original Name",
				},
			},
			opts: &gamesimport.Options{
				Code: new("custom_code"),
			},
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				assert.Equal(t, "custom_code", result.Game.Code)
				assert.Equal(t, "Original Name", result.Game.Name)

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "custom_code", games[0].Code)

				mods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, mods, 1)
				assert.Equal(t, "custom_code", mods[0].GameCode)
			},
		},
		{
			name: "override_both_code_and_name",
			egg: &gamesimport.PelicanEgg{
				Name:    "Original Name",
				Startup: "./server",
				Raw: map[string]any{
					"name": "Original Name",
				},
			},
			opts: &gamesimport.Options{
				Code: new("my_game"),
				Name: new("My Custom Game"),
			},
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				assert.Equal(t, "my_game", result.Game.Code)
				assert.Equal(t, "My Custom Game", result.Game.Name)

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "my_game", games[0].Code)
				assert.Equal(t, "My Custom Game", games[0].Name)

				mods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, mods, 1)
				assert.Equal(t, "my_game", mods[0].GameCode)
			},
		},
		{
			name: "invalid_code_in_options",
			egg: &gamesimport.PelicanEgg{
				Name:    "Test Game",
				Startup: "./server",
				Raw: map[string]any{
					"name": "Test Game",
				},
			},
			opts: &gamesimport.Options{
				Code: new("INVALID"),
			},
			wantErr:        true,
			wantErrContain: "code must match pattern",
		},
		{
			name: "code_too_short_in_options",
			egg: &gamesimport.PelicanEgg{
				Name:    "Test Game",
				Startup: "./server",
				Raw: map[string]any{
					"name": "Test Game",
				},
			},
			opts: &gamesimport.Options{
				Code: new("a"),
			},
			wantErr:        true,
			wantErrContain: "code must be between 2 and 16 characters",
		},
		{
			name: "name_too_short_in_options",
			egg: &gamesimport.PelicanEgg{
				Name:    "Test Game",
				Startup: "./server",
				Raw: map[string]any{
					"name": "Test Game",
				},
			},
			opts: &gamesimport.Options{
				Name: new("A"),
			},
			wantErr:        true,
			wantErrContain: "name must be between 2 and 128 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()

			importer := NewImporter(
				gameRepo,
				gameModRepo,
				services.NewNilTransactionManager(),
			)

			result, err := importer.Import(context.Background(), tt.egg, tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, gameRepo, gameModRepo, result)
				}
			}
		})
	}
}

func TestGenerateGameCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple_name",
			input:    "Minecraft",
			expected: "minecraft",
		},
		{
			name:     "name_with_dots",
			input:    "Rage.MP",
			expected: "rage_mp",
		},
		{
			name:     "name_with_spaces",
			input:    "Counter Strike",
			expected: "counter_strike",
		},
		{
			name:     "name_with_special_chars",
			input:    "Half-Life 2: Episode One",
			expected: "half_life_2_epis",
		},
		{
			name:     "very_short_name",
			input:    "X",
			expected: "gm",
		},
		{
			name:     "name_with_numbers",
			input:    "Rust 2024",
			expected: "rust_2024",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := generateGameCode(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.LessOrEqual(t, len(result), 16)
			assert.GreaterOrEqual(t, len(result), 2)
		})
	}
}

func TestTransformStartupCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		startup  string
		expected string
	}{
		{
			name:     "port_variable",
			startup:  "./server -port {{server.build.default.port}}",
			expected: "./server -port {port}",
		},
		{
			name:     "ip_variable",
			startup:  "./server -ip {{server.build.default.ip}}",
			expected: "./server -ip {ip}",
		},
		{
			name:     "env_port",
			startup:  "./server -port {{server.build.env.PORT}}",
			expected: "./server -port {port}",
		},
		{
			name:     "env_ip",
			startup:  "./server -ip {{server.build.env.SERVER_IP}}",
			expected: "./server -ip {ip}",
		},
		{
			name:     "custom_env_var",
			startup:  "./server -name {{SERVER_NAME}}",
			expected: "./server -name {SERVER_NAME}",
		},
		{
			name:     "server_build_env_var",
			startup:  "./server -name {{server.build.env.SERVER_NAME}}",
			expected: "./server -name {SERVER_NAME}",
		},
		{
			name:     "multiple_variables",
			startup:  "./server -port {{server.build.default.port}} -name {{SERVER_NAME}} -slots {{MAX_PLAYERS}}",
			expected: "./server -port {port} -name {SERVER_NAME} -slots {MAX_PLAYERS}",
		},
		{
			name:     "no_variables",
			startup:  "./server",
			expected: "./server",
		},
		{
			name:     "shell_and_operator",
			startup:  "cd /app && ./server",
			expected: `/bin/sh -c "cd /app && ./server"`,
		},
		{
			name:     "shell_semicolon",
			startup:  "echo starting; ./server",
			expected: `/bin/sh -c "echo starting; ./server"`,
		},
		{
			name:     "shell_pipe",
			startup:  "cat config.txt | ./server",
			expected: `/bin/sh -c "cat config.txt | ./server"`,
		},
		{
			name:     "shell_or_operator",
			startup:  "./server || exit 1",
			expected: `/bin/sh -c "./server || exit 1"`,
		},
		{
			name:     "shell_redirect_output",
			startup:  "./server > log.txt",
			expected: `/bin/sh -c "./server > log.txt"`,
		},
		{
			name:     "shell_redirect_input",
			startup:  "./server < input.txt",
			expected: `/bin/sh -c "./server < input.txt"`,
		},
		{
			name:     "shell_subshell",
			startup:  "./server -dir=$(pwd)",
			expected: `/bin/sh -c "./server -dir=$(pwd)"`,
		},
		{
			name:     "shell_background",
			startup:  "./watcher & ./server",
			expected: `/bin/sh -c "./watcher & ./server"`,
		},
		{
			name:     "shell_with_quotes_escaped",
			startup:  `echo "hello" && ./server`,
			expected: `/bin/sh -c "echo \"hello\" && ./server"`,
		},
		{
			name:     "shell_complex_with_variables",
			startup:  "cd /app && ./server -port {{server.build.default.port}} -name {{SERVER_NAME}}",
			expected: `/bin/sh -c "cd /app && ./server -port {port} -name {SERVER_NAME}"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := transformStartupCommand(tt.startup)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTransformVariables(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		variables []gamesimport.PelicanEggVariable
		expected  domain.GameModVarList
	}{
		{
			name:      "empty_variables",
			variables: nil,
			expected:  domain.GameModVarList{},
		},
		{
			name: "single_variable_user_editable",
			variables: []gamesimport.PelicanEggVariable{
				{
					Name:         "Server Name",
					Description:  "Name of the server",
					EnvVariable:  "SERVER_NAME",
					DefaultValue: "My Server",
					UserEditable: true,
				},
			},
			expected: domain.GameModVarList{
				{
					Var:      "SERVER_NAME",
					Default:  "My Server",
					Info:     "Name of the server",
					AdminVar: false,
				},
			},
		},
		{
			name: "single_variable_admin_only",
			variables: []gamesimport.PelicanEggVariable{
				{
					Name:         "Secret Key",
					EnvVariable:  "SECRET_KEY",
					DefaultValue: "secret123",
					UserEditable: false,
				},
			},
			expected: domain.GameModVarList{
				{
					Var:      "SECRET_KEY",
					Default:  "secret123",
					Info:     "Secret Key",
					AdminVar: true,
				},
			},
		},
		{
			name: "variable_with_empty_description_uses_name",
			variables: []gamesimport.PelicanEggVariable{
				{
					Name:         "Max Players",
					Description:  "",
					EnvVariable:  "MAX_PLAYERS",
					DefaultValue: "32",
					UserEditable: true,
				},
			},
			expected: domain.GameModVarList{
				{
					Var:      "MAX_PLAYERS",
					Default:  "32",
					Info:     "Max Players",
					AdminVar: false,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := transformVariables(tt.variables)
			require.Len(t, result, len(tt.expected))

			for i, v := range result {
				assert.Equal(t, tt.expected[i].Var, v.Var)
				assert.Equal(t, tt.expected[i].Default, v.Default)
				assert.Equal(t, tt.expected[i].Info, v.Info)
				assert.Equal(t, tt.expected[i].AdminVar, v.AdminVar)
			}
		})
	}
}

func TestParsePelicanEgg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		jsonData       string
		wantErr        bool
		wantErrContain string
		validate       func(t *testing.T, egg *gamesimport.PelicanEgg)
	}{
		{
			name: "valid_egg_json",
			jsonData: `{
				"uuid": "test-uuid",
				"name": "Test Game",
				"author": "test@example.com",
				"description": "A test game",
				"startup": "./server -port {{server.build.default.port}}",
				"docker_images": {
					"image1": "image1:latest"
				},
				"variables": [
					{
						"name": "Server Name",
						"env_variable": "SERVER_NAME",
						"default_value": "Test",
						"user_editable": true
					}
				]
			}`,
			wantErr: false,
			validate: func(t *testing.T, egg *gamesimport.PelicanEgg) {
				t.Helper()

				assert.Equal(t, "test-uuid", egg.UUID)
				assert.Equal(t, "Test Game", egg.Name)
				assert.Equal(t, "test@example.com", egg.Author)
				assert.Equal(t, "A test game", egg.Description)
				assert.Equal(t, "./server -port {{server.build.default.port}}", egg.Startup)
				require.Len(t, egg.DockerImages, 1)
				assert.Equal(t, "image1:latest", egg.DockerImages["image1"])
				require.Len(t, egg.Variables, 1)
				assert.Equal(t, "SERVER_NAME", egg.Variables[0].EnvVariable)

				require.NotNil(t, egg.Raw)
				assert.Equal(t, "test-uuid", egg.Raw["uuid"])
				assert.Equal(t, "Test Game", egg.Raw["name"])
			},
		},
		{
			name:           "invalid_json",
			jsonData:       `{invalid json}`,
			wantErr:        true,
			wantErrContain: "failed to parse pelican egg JSON",
		},
		{
			name:     "empty_json",
			jsonData: `{}`,
			wantErr:  false,
			validate: func(t *testing.T, egg *gamesimport.PelicanEgg) {
				t.Helper()

				assert.Empty(t, egg.UUID)
				assert.Empty(t, egg.Name)
				require.NotNil(t, egg.Raw)
				assert.Empty(t, egg.Raw)
			},
		},
		{
			name: "PLCN_v3_format_with_startup_commands_and_array_rules",
			jsonData: `{
				"meta": {"version": "PLCN_v3"},
				"uuid": "plcn-v3-test",
				"name": "PLCN v3 Test",
				"startup_commands": {
					"Default": "./server -port {{server.build.default.port}}"
				},
				"variables": [
					{
						"name": "Test Var",
						"env_variable": "TEST_VAR",
						"default_value": "test",
						"rules": ["required", "string"]
					}
				]
			}`,
			wantErr: false,
			validate: func(t *testing.T, egg *gamesimport.PelicanEgg) {
				t.Helper()

				assert.Equal(t, "plcn-v3-test", egg.UUID)
				assert.Equal(t, "PLCN_v3", egg.Meta.Version)
				assert.Equal(t, "", egg.Startup)
				assert.Equal(t, "./server -port {{server.build.default.port}}", egg.StartupCommands["Default"])
				assert.Equal(t, "./server -port {{server.build.default.port}}", egg.GetStartupCommand())
				require.Len(t, egg.Variables, 1)
				assert.Equal(t, gamesimport.FlexibleRules{"required", "string"}, egg.Variables[0].Rules)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			egg, err := gamesimport.ParsePelicanEgg([]byte(tt.jsonData))

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, egg)
				}
			}
		})
	}
}
