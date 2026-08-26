package gameapimporter

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/domain/gamesimport"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImporter_Import(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		export       *domain.GameExport
		setupGame    func(*inmemory.GameRepository)
		setupGameMod func(*inmemory.GameModRepository)
		wantError    string
		validate     func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult)
	}{
		{
			name: "successful_import_new_game_and_mods",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:              "cstrike",
					Name:              "Counter-Strike 1.6",
					Engine:            "GoldSource",
					EngineVersion:     "1.0",
					SteamAppIDLinux:   new(uint(90)),
					SteamAppIDWindows: new(uint(90)),
				},
				Mods: []domain.GameExportMod{
					{
						Name:          "Classic",
						StartCmdLinux: new("./hlds_run -game cstrike +port {port}"),
						KickCmd:       new("kick {name}"),
						FastRcon: []domain.GameExportModFastRcon{
							{Info: "Restart", Command: "changelevel {map}"},
						},
						Vars: []domain.GameExportModVar{
							{Var: "maxplayers", Default: "32", Info: "Max players"},
						},
					},
					{
						Name:          "Deathmatch",
						StartCmdLinux: new("./hlds_run -game cstrike_dm +port {port}"),
					},
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				require.NotNil(t, result.Game)

				assert.Equal(t, "cstrike", result.Game.Code)
				assert.Equal(t, "Counter-Strike 1.6", result.Game.Name)
				assert.Equal(t, "GoldSource", result.Game.Engine)
				assert.Equal(t, 1, result.Game.Enabled)

				require.Len(t, result.ModsCreated, 2)
				assert.Contains(t, result.ModsCreated, "Classic")
				assert.Contains(t, result.ModsCreated, "Deathmatch")
				require.Len(t, result.ModsUpdated, 0)

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)

				gameMods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, gameMods, 2)

				classicMods, err := gameModRepo.Find(context.Background(), &filters.FindGameMod{
					Names:     []string{"Classic"},
					GameCodes: []string{"cstrike"},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, classicMods, 1)

				classic := classicMods[0]
				assert.Equal(t, "./hlds_run -game cstrike +port {port}", *classic.StartCmdLinux)
				assert.Equal(t, "kick {name}", *classic.KickCmd)
				require.Len(t, classic.FastRcon, 1)
				require.Len(t, classic.Vars, 1)
			},
		},
		{
			name: "successful_import_game_only_without_mods",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "test",
					Name:   "Test Game",
					Engine: "Test Engine",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				assert.Equal(t, "test", result.Game.Code)
				require.Len(t, result.ModsCreated, 0)
				require.Len(t, result.ModsUpdated, 0)

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)

				gameMods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, gameMods, 0)
			},
		},
		{
			name: "successful_update_existing_game",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:          "existing",
					Name:          "Updated Name",
					Engine:        "New Engine",
					EngineVersion: "2.0",
					Metadata: domain.Metadata{
						"new_key": "new_value",
					},
				},
			},
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:          "existing",
					Name:          "Old Name",
					Engine:        "Old Engine",
					EngineVersion: "1.0",
					Enabled:       1,
					Metadata: domain.Metadata{
						"old_key": "old_value",
					},
				})
			},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, _ *inmemory.GameModRepository, _ *ImportResult) {
				t.Helper()

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)

				game := games[0]
				assert.Equal(t, "Updated Name", game.Name)
				assert.Equal(t, "New Engine", game.Engine)
				assert.Equal(t, "2.0", game.EngineVersion)
				assert.Equal(t, "old_value", game.Metadata["old_key"])
				assert.Equal(t, "new_value", game.Metadata["new_key"])
			},
		},
		{
			name: "successful_update_existing_mod",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "existing",
					Name:   "Existing Game",
					Engine: "Test",
				},
				Mods: []domain.GameExportMod{
					{
						Name:          "Default",
						StartCmdLinux: new("./new_command"),
					},
				},
			},
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "existing",
					Name:    "Existing Game",
					Engine:  "Test",
					Enabled: 1,
				})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "existing",
					Name:          "Default",
					StartCmdLinux: new("./old_command"),
					Metadata: domain.Metadata{
						"existing_key": "keep_me",
					},
				})
			},
			validate: func(t *testing.T, _ *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				require.Len(t, result.ModsCreated, 0)
				require.Len(t, result.ModsUpdated, 1)
				assert.Equal(t, "Default", result.ModsUpdated[0])

				mods, err := gameModRepo.Find(context.Background(), &filters.FindGameMod{
					Names:     []string{"Default"},
					GameCodes: []string{"existing"},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, mods, 1)

				mod := mods[0]
				assert.Equal(t, "./new_command", *mod.StartCmdLinux)
				assert.Equal(t, "keep_me", mod.Metadata["existing_key"])
			},
		},
		{
			name: "mixed_create_and_update_mods",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "mixed",
					Name:   "Mixed Game",
					Engine: "Test",
				},
				Mods: []domain.GameExportMod{
					{Name: "Existing", StartCmdLinux: new("./updated")},
					{Name: "NewMod", StartCmdLinux: new("./new")},
				},
			},
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "mixed",
					Name:    "Mixed Game",
					Engine:  "Test",
					Enabled: 1,
				})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "mixed",
					Name:          "Existing",
					StartCmdLinux: new("./old"),
				})
			},
			validate: func(t *testing.T, _ *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				require.Len(t, result.ModsCreated, 1)
				require.Len(t, result.ModsUpdated, 1)
				assert.Equal(t, "NewMod", result.ModsCreated[0])
				assert.Equal(t, "Existing", result.ModsUpdated[0])

				mods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, mods, 2)
			},
		},
		{
			name:         "nil_export_returns_error",
			export:       nil,
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantError:    "export cannot be nil",
		},
		{
			name: "missing_schema_version_returns_validation_error",
			export: &domain.GameExport{
				Game: domain.GameExportGame{
					Code:   "test",
					Name:   "Test",
					Engine: "Test",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantError:    "schema_version is required",
		},
		{
			name: "invalid_game_code_returns_validation_error",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "Invalid Code",
					Name:   "Test",
					Engine: "Test",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantError:    "game.code must match pattern",
		},
		{
			name: "duplicate_mod_names_returns_validation_error",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "test",
					Name:   "Test",
					Engine: "Test",
				},
				Mods: []domain.GameExportMod{
					{Name: "Same"},
					{Name: "Same"},
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantError:    "duplicate mod name: Same",
		},
		{
			name: "game_code_too_long_returns_validation_error",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "this_code_is_way_too_long_for_the_limit",
					Name:   "Test",
					Engine: "Test",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantError:    "game.code must be between 2 and 16 characters",
		},
		{
			name: "unsupported_schema_version_returns_error",
			export: &domain.GameExport{
				SchemaVersion: "2.0",
				Game: domain.GameExportGame{
					Code:   "test",
					Name:   "Test",
					Engine: "Test",
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantError:    "unsupported schema version",
		},
		{
			name: "import_with_all_mod_fields",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "full",
					Name:   "Full Game",
					Engine: "Full",
				},
				Mods: []domain.GameExportMod{
					{
						Name:                    "Complete",
						RemoteRepositoryLinux:   new("http://linux"),
						RemoteRepositoryWindows: new("http://windows"),
						LocalRepositoryLinux:    new("/linux"),
						LocalRepositoryWindows:  new("C:\\windows"),
						StartCmdLinux:           new("./linux"),
						StartCmdWindows:         new("start.exe"),
						KickCmd:                 new("kick"),
						BanCmd:                  new("ban"),
						ChnameCmd:               new("name"),
						SrestartCmd:             new("restart"),
						ChmapCmd:                new("map"),
						SendmsgCmd:              new("say"),
						PasswdCmd:               new("pass"),
						FastRcon: []domain.GameExportModFastRcon{
							{Info: "Info1", Command: "cmd1"},
							{Info: "Info2", Command: "cmd2"},
						},
						Vars: []domain.GameExportModVar{
							{Var: "var1", Default: "val1", Info: "Info1", AdminVar: false},
							{Var: "var2", Default: "val2", Info: "Info2", AdminVar: true},
						},
						Metadata: domain.Metadata{
							"custom": "value",
						},
					},
				},
			},
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			validate: func(t *testing.T, _ *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)

				mods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, mods, 1)

				mod := mods[0]
				assert.Equal(t, "Complete", mod.Name)
				assert.Equal(t, "full", mod.GameCode)
				assert.Equal(t, "http://linux", *mod.RemoteRepositoryLinux)
				assert.Equal(t, "http://windows", *mod.RemoteRepositoryWindows)
				assert.Equal(t, "/linux", *mod.LocalRepositoryLinux)
				assert.Equal(t, "C:\\windows", *mod.LocalRepositoryWindows)
				assert.Equal(t, "./linux", *mod.StartCmdLinux)
				assert.Equal(t, "start.exe", *mod.StartCmdWindows)
				assert.Equal(t, "kick", *mod.KickCmd)
				assert.Equal(t, "ban", *mod.BanCmd)
				assert.Equal(t, "name", *mod.ChnameCmd)
				assert.Equal(t, "restart", *mod.SrestartCmd)
				assert.Equal(t, "map", *mod.ChmapCmd)
				assert.Equal(t, "say", *mod.SendmsgCmd)
				assert.Equal(t, "pass", *mod.PasswdCmd)
				require.Len(t, mod.FastRcon, 2)
				require.Len(t, mod.Vars, 2)
				assert.True(t, mod.Vars[1].AdminVar)
				assert.Equal(t, "value", mod.Metadata["custom"])
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

			result, err := importer.Import(context.Background(), tt.export, nil)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, gameRepo, gameModRepo, result)
			}
		})
	}
}

func TestImporter_Import_WithOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		export    *domain.GameExport
		opts      *gamesimport.Options
		wantError string
		validate  func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult)
	}{
		{
			name: "override_name_only",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "cstrike",
					Name:   "Original Name",
					Engine: "GoldSource",
				},
			},
			opts: &gamesimport.Options{
				Name: new("Overridden Name"),
			},
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, _ *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				assert.Equal(t, "cstrike", result.Game.Code)
				assert.Equal(t, "Overridden Name", result.Game.Name)

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "Overridden Name", games[0].Name)
			},
		},
		{
			name: "override_code_only",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "original",
					Name:   "Original Name",
					Engine: "GoldSource",
				},
				Mods: []domain.GameExportMod{
					{
						Name:          "Default",
						StartCmdLinux: new("./server"),
					},
				},
			},
			opts: &gamesimport.Options{
				Code: new("overridden"),
			},
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				assert.Equal(t, "overridden", result.Game.Code)
				assert.Equal(t, "Original Name", result.Game.Name)

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "overridden", games[0].Code)

				mods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, mods, 1)
				assert.Equal(t, "overridden", mods[0].GameCode)
			},
		},
		{
			name: "override_both_code_and_name",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "original",
					Name:   "Original Name",
					Engine: "GoldSource",
				},
			},
			opts: &gamesimport.Options{
				Code: new("custom"),
				Name: new("Custom Name"),
			},
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, _ *inmemory.GameModRepository, result *ImportResult) {
				t.Helper()

				require.NotNil(t, result)
				assert.Equal(t, "custom", result.Game.Code)
				assert.Equal(t, "Custom Name", result.Game.Name)

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "custom", games[0].Code)
				assert.Equal(t, "Custom Name", games[0].Name)
			},
		},
		{
			name: "invalid_code_in_options",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "original",
					Name:   "Original Name",
					Engine: "GoldSource",
				},
			},
			opts: &gamesimport.Options{
				Code: new("INVALID"),
			},
			wantError: "code must match pattern",
		},
		{
			name: "code_too_short_in_options",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "original",
					Name:   "Original Name",
					Engine: "GoldSource",
				},
			},
			opts: &gamesimport.Options{
				Code: new("a"),
			},
			wantError: "code must be between 2 and 16 characters",
		},
		{
			name: "name_too_short_in_options",
			export: &domain.GameExport{
				SchemaVersion: "1.0",
				Game: domain.GameExportGame{
					Code:   "original",
					Name:   "Original Name",
					Engine: "GoldSource",
				},
			},
			opts: &gamesimport.Options{
				Name: new("A"),
			},
			wantError: "name must be between 2 and 128 characters",
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

			result, err := importer.Import(context.Background(), tt.export, tt.opts)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, gameRepo, gameModRepo, result)
			}
		})
	}
}

func TestMergeMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		existing domain.Metadata
		updated  domain.Metadata
		expected domain.Metadata
	}{
		{
			name:     "both_nil",
			existing: nil,
			updated:  nil,
			expected: nil,
		},
		{
			name:     "existing_nil",
			existing: nil,
			updated:  domain.Metadata{"key": "value"},
			expected: domain.Metadata{"key": "value"},
		},
		{
			name:     "updated_nil",
			existing: domain.Metadata{"key": "value"},
			updated:  nil,
			expected: domain.Metadata{"key": "value"},
		},
		{
			name:     "merge_keys",
			existing: domain.Metadata{"key1": "value1"},
			updated:  domain.Metadata{"key2": "value2"},
			expected: domain.Metadata{"key1": "value1", "key2": "value2"},
		},
		{
			name:     "updated_overwrites_existing",
			existing: domain.Metadata{"key": "old"},
			updated:  domain.Metadata{"key": "new"},
			expected: domain.Metadata{"key": "new"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := mergeMetadata(tt.existing, tt.updated)
			assert.Equal(t, tt.expected, result)
		})
	}
}
