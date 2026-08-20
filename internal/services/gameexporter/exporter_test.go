package gameexporter

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExporter_Export(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		gameCode     string
		setupGame    func(*inmemory.GameRepository)
		setupGameMod func(*inmemory.GameModRepository)
		wantError    string
		validate     func(t *testing.T, yamlData []byte)
	}{
		{
			name:     "successful_export_game_with_mods",
			gameCode: "cstrike",
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:              "cstrike",
					Name:              "Counter-Strike 1.6",
					Engine:            "GoldSource",
					EngineVersion:     "1.0",
					SteamAppIDLinux:   new(uint(90)),
					SteamAppIDWindows: new(uint(90)),
					Enabled:           1,
				})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "cstrike",
					Name:          "Classic",
					StartCmdLinux: new("./hlds_run -game cstrike +port {port}"),
					KickCmd:       new("kick {name}"),
					FastRcon: domain.GameModFastRconList{
						{Info: "Restart", Command: "changelevel {map}"},
					},
					Vars: domain.GameModVarList{
						{Var: "maxplayers", Default: "32", Info: "Max players"},
					},
				})
			},
			validate: func(t *testing.T, yamlData []byte) {
				t.Helper()

				export, err := domain.ParseGameExport(yamlData)
				require.NoError(t, err)

				assert.Equal(t, domain.CurrentSchemaVersion, export.SchemaVersion)
				assert.NotEmpty(t, export.ExportedAt)
				assert.Contains(t, export.ExportedBy, "GameAP")

				assert.Equal(t, "cstrike", export.Game.Code)
				assert.Equal(t, "Counter-Strike 1.6", export.Game.Name)
				assert.Equal(t, "GoldSource", export.Game.Engine)
				assert.Equal(t, "1.0", export.Game.EngineVersion)
				assert.Equal(t, uint(90), *export.Game.SteamAppIDLinux)
				assert.Equal(t, uint(90), *export.Game.SteamAppIDWindows)

				require.Len(t, export.Mods, 1)
				assert.Equal(t, "Classic", export.Mods[0].Name)
				assert.Equal(t, "./hlds_run -game cstrike +port {port}", *export.Mods[0].StartCmdLinux)
				assert.Equal(t, "kick {name}", *export.Mods[0].KickCmd)
				require.Len(t, export.Mods[0].FastRcon, 1)
				require.Len(t, export.Mods[0].Vars, 1)
			},
		},
		{
			name:     "successful_export_game_without_mods",
			gameCode: "test",
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "test",
					Name:    "Test Game",
					Engine:  "Test Engine",
					Enabled: 1,
				})
			},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			validate: func(t *testing.T, yamlData []byte) {
				t.Helper()

				export, err := domain.ParseGameExport(yamlData)
				require.NoError(t, err)

				assert.Equal(t, "test", export.Game.Code)
				assert.Equal(t, "Test Game", export.Game.Name)
				require.Len(t, export.Mods, 0)
			},
		},
		{
			name:     "successful_export_game_with_multiple_mods",
			gameCode: "multi",
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "multi",
					Name:    "Multi Mod Game",
					Engine:  "Multi Engine",
					Enabled: 1,
				})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "multi",
					Name:          "Mod1",
					StartCmdLinux: new("./mod1"),
				})
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "multi",
					Name:          "Mod2",
					StartCmdLinux: new("./mod2"),
				})
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "multi",
					Name:          "Mod3",
					StartCmdLinux: new("./mod3"),
				})
			},
			validate: func(t *testing.T, yamlData []byte) {
				t.Helper()

				export, err := domain.ParseGameExport(yamlData)
				require.NoError(t, err)

				assert.Equal(t, "multi", export.Game.Code)
				require.Len(t, export.Mods, 3)
			},
		},
		{
			name:         "empty_game_code",
			gameCode:     "",
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantError:    "game code is required",
		},
		{
			name:         "game_not_found",
			gameCode:     "notexist",
			setupGame:    func(_ *inmemory.GameRepository) {},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantError:    "game with code \"notexist\" not found",
		},
		{
			name:     "export_preserves_metadata",
			gameCode: "pelican",
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "pelican",
					Name:    "Pelican Game",
					Engine:  "pelican",
					Enabled: 1,
					Metadata: domain.Metadata{
						"pelican_egg": map[string]any{"uuid": "test-uuid"},
						"custom_key":  "keep_me",
					},
				})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "pelican",
					Name:          "Default",
					StartCmdLinux: new("./server"),
					Metadata: domain.Metadata{
						"pelican_egg": map[string]any{"data": "value"},
					},
				})
			},
			validate: func(t *testing.T, yamlData []byte) {
				t.Helper()

				export, err := domain.ParseGameExport(yamlData)
				require.NoError(t, err)

				require.NotNil(t, export.Game.Metadata)
				assert.Equal(t, "keep_me", export.Game.Metadata["custom_key"])
				assert.NotNil(t, export.Game.Metadata["pelican_egg"])

				require.Len(t, export.Mods, 1)
				require.NotNil(t, export.Mods[0].Metadata)
				assert.NotNil(t, export.Mods[0].Metadata["pelican_egg"])
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

			exporter := NewExporter(gameRepo, gameModRepo, "v3.0.0")

			yamlData, err := exporter.Export(context.Background(), tt.gameCode)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, yamlData)

			if tt.validate != nil {
				tt.validate(t, yamlData)
			}
		})
	}
}

func TestExporter_ExportToStruct(t *testing.T) {
	t.Parallel()

	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()

	_ = gameRepo.Save(context.Background(), &domain.Game{
		Code:    "test",
		Name:    "Test Game",
		Engine:  "Test Engine",
		Enabled: 1,
	})

	_ = gameModRepo.Save(context.Background(), &domain.GameMod{
		GameCode:      "test",
		Name:          "Default",
		StartCmdLinux: new("./test"),
	})

	exporter := NewExporter(gameRepo, gameModRepo, "v3.0.0")

	export, err := exporter.ExportToStruct(context.Background(), "test")
	require.NoError(t, err)

	assert.Equal(t, domain.CurrentSchemaVersion, export.SchemaVersion)
	assert.Equal(t, "test", export.Game.Code)
	assert.Equal(t, "Test Game", export.Game.Name)
	require.Len(t, export.Mods, 1)
	assert.Equal(t, "Default", export.Mods[0].Name)
}
