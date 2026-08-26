package services

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGamesProvider struct {
	games []domain.GlobalAPIGame
	err   error
}

func (m *mockGamesProvider) Games(_ context.Context) ([]domain.GlobalAPIGame, error) {
	return m.games, m.err
}

func TestGameUpgradeService_UpgradeGames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		apiGames     []domain.GlobalAPIGame
		apiErr       error
		setupGameMod func(*inmemory.GameModRepository)
		wantErr      bool
		errContains  string
		validate     func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository)
	}{
		{
			name: "successful_upgrade_with_new_games_and_mods",
			apiGames: []domain.GlobalAPIGame{
				{
					Code:              "cstrike",
					StartCode:         "cstrike",
					Name:              "Counter-Strike 1.6",
					Engine:            "GoldSource",
					EngineVersion:     "1",
					SteamAppIDLinux:   90,
					SteamAppIDWindows: 90,
					Mods: []domain.GlobalAPIGameMod{
						{
							ID:            1,
							GameCode:      "cstrike",
							Name:          "Classic",
							StartCmdLinux: "./hlds_run -game cstrike",
						},
					},
				},
			},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				t.Helper()

				games, err := gameRepo.Find(context.Background(), &filters.FindGame{
					Codes: []string{"cstrike"},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)

				game := games[0]
				assert.Equal(t, "cstrike", game.Code)
				assert.Equal(t, "Counter-Strike 1.6", game.Name)
				assert.Equal(t, "GoldSource", game.Engine)
				assert.Equal(t, "1", game.EngineVersion)
				assert.Equal(t, uint(90), lo.FromPtr(game.SteamAppIDLinux))
				assert.Equal(t, uint(90), lo.FromPtr(game.SteamAppIDWindows))

				gameMods, err := gameModRepo.Find(context.Background(), &filters.FindGameMod{
					GameCodes: []string{"cstrike"},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, gameMods, 1)

				mod := gameMods[0]
				assert.Equal(t, "cstrike", mod.GameCode)
				assert.Equal(t, "Classic", mod.Name)
				assert.Equal(t, "./hlds_run -game cstrike", lo.FromPtr(mod.StartCmdLinux))
			},
		},
		{
			name: "successful_upgrade_with_existing_game_mod",
			apiGames: []domain.GlobalAPIGame{
				{
					Code:   "cstrike",
					Name:   "Counter-Strike 1.6",
					Engine: "GoldSource",
					Mods: []domain.GlobalAPIGameMod{
						{
							ID:              1,
							GameCode:        "cstrike",
							Name:            "Classic",
							StartCmdLinux:   "./hlds_run -game cstrike +maxplayers 32",
							StartCmdWindows: "hlds.exe -game cstrike +maxplayers 32",
						},
					},
				},
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "cstrike",
					Name:          "Classic",
					StartCmdLinux: new("./hlds_run -game cstrike"),
				})
			},
			wantErr: false,
			validate: func(t *testing.T, _ *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				t.Helper()

				gameMods, err := gameModRepo.Find(context.Background(), &filters.FindGameMod{
					GameCodes: []string{"cstrike"},
					Names:     []string{"Classic"},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, gameMods, 1)

				mod := gameMods[0]
				assert.Equal(t, "./hlds_run -game cstrike +maxplayers 32", lo.FromPtr(mod.StartCmdLinux))
				assert.Equal(t, "hlds.exe -game cstrike +maxplayers 32", lo.FromPtr(mod.StartCmdWindows))
			},
		},
		{
			name: "skip_mod_with_multiple_matches",
			apiGames: []domain.GlobalAPIGame{
				{
					Code:   "cstrike",
					Name:   "Counter-Strike 1.6",
					Engine: "GoldSource",
					Mods: []domain.GlobalAPIGameMod{
						{
							ID:            1,
							GameCode:      "cstrike",
							Name:          "Classic",
							StartCmdLinux: "./hlds_run -game cstrike +maxplayers 32",
						},
					},
				},
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "cstrike",
					Name:          "Classic",
					StartCmdLinux: new("./hlds_run -game cstrike"),
				})
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "cstrike",
					Name:          "Classic",
					StartCmdLinux: new("./hlds_run -game cstrike -duplicate"),
				})
			},
			wantErr: false,
			validate: func(t *testing.T, _ *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				t.Helper()

				gameMods, err := gameModRepo.Find(context.Background(), &filters.FindGameMod{
					GameCodes: []string{"cstrike"},
					Names:     []string{"Classic"},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, gameMods, 2)

				for _, mod := range gameMods {
					assert.NotEqual(t, "./hlds_run -game cstrike +maxplayers 32", lo.FromPtr(mod.StartCmdLinux))
				}
			},
		},
		{
			name: "multiple_games_with_multiple_mods",
			apiGames: []domain.GlobalAPIGame{
				{
					Code:   "cstrike",
					Name:   "Counter-Strike 1.6",
					Engine: "GoldSource",
					Mods: []domain.GlobalAPIGameMod{
						{
							ID:            1,
							GameCode:      "cstrike",
							Name:          "Classic",
							StartCmdLinux: "./hlds_run -game cstrike",
						},
						{
							ID:            2,
							GameCode:      "cstrike",
							Name:          "DeathMatch",
							StartCmdLinux: "./hlds_run -game cstrike +sv_deathmatch 1",
						},
					},
				},
				{
					Code:   "css",
					Name:   "Counter-Strike Source",
					Engine: "Source",
					Mods: []domain.GlobalAPIGameMod{
						{
							ID:            3,
							GameCode:      "css",
							Name:          "Classic",
							StartCmdLinux: "./srcds_run -game cstrike",
						},
					},
				},
			},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				t.Helper()

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 2)

				gameMods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, gameMods, 3)

				cstrikeMods, err := gameModRepo.Find(context.Background(), &filters.FindGameMod{
					GameCodes: []string{"cstrike"},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, cstrikeMods, 2)

				cssMods, err := gameModRepo.Find(context.Background(), &filters.FindGameMod{
					GameCodes: []string{"css"},
				}, nil, nil)
				require.NoError(t, err)
				require.Len(t, cssMods, 1)
			},
		},
		{
			name:         "api_returns_error",
			apiGames:     nil,
			apiErr:       errors.New("connection refused"),
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      true,
			errContains:  "failed to fetch games",
		},
		{
			name:         "empty_api_response",
			apiGames:     []domain.GlobalAPIGame{},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				t.Helper()

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				assert.Empty(t, games)

				gameMods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				assert.Empty(t, gameMods)
			},
		},
		{
			name: "game_without_mods",
			apiGames: []domain.GlobalAPIGame{
				{
					Code:   "cstrike",
					Name:   "Counter-Strike 1.6",
					Engine: "GoldSource",
					Mods:   nil,
				},
			},
			setupGameMod: func(_ *inmemory.GameModRepository) {},
			wantErr:      false,
			validate: func(t *testing.T, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				t.Helper()

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "cstrike", games[0].Code)

				gameMods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				assert.Empty(t, gameMods)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()

			tt.setupGameMod(gameModRepo)

			mockAPI := &mockGamesProvider{
				games: tt.apiGames,
				err:   tt.apiErr,
			}

			service := NewGameUpgradeService(
				mockAPI,
				gameRepo,
				gameModRepo,
				NewNilTransactionManager(),
			)

			err := service.UpgradeGames(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, gameRepo, gameModRepo)
				}
			}
		})
	}
}
