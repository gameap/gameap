package getgames_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/games/getgames"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGames(t *testing.T) {
	tests := []struct {
		name  string
		games []domain.Game
		want  string
	}{
		{
			name: "success",
			games: []domain.Game{
				{
					Code:                    "valve",
					Name:                    "Half-Life 1",
					Engine:                  "GoldSource",
					EngineVersion:           "1.0",
					SteamAppIDLinux:         lo.ToPtr(uint(90)),
					SteamAppIDWindows:       lo.ToPtr(uint(190)),
					SteamAppSetConfig:       lo.ToPtr("some-config"),
					RemoteRepositoryLinux:   lo.ToPtr("http://example.com/linux"),
					RemoteRepositoryWindows: lo.ToPtr("http://example.com/windows"),
					LocalRepositoryLinux:    lo.ToPtr("/var/repo/linux"),
					LocalRepositoryWindows:  lo.ToPtr("C:\\repo\\windows"),
					Enabled:                 1,
				},
			},
			want: `[
				{
					"code": "valve",
					"name": "Half-Life 1",
					"engine": "GoldSource",
					"engine_version": "1.0",
					"steam_app_id_linux": 90,
					"steam_app_id_windows": 190,
					"steam_app_set_config": "some-config",
					"remote_repository_linux": "http://example.com/linux",
					"remote_repository_windows": "http://example.com/windows",
					"local_repository_linux": "/var/repo/linux",
					"local_repository_windows": "C:\\repo\\windows",
					"enabled": 1
				}
			]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewGameRepository()

			for _, game := range test.games {
				err := repo.Save(context.Background(), &game)
				require.NoError(t, err)
			}

			h := getgames.NewHandler(repo, api.NewResponder())
			recorder := httptest.NewRecorder()

			// ACT
			h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/games", nil))

			// ASSERT
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.JSONEq(t, test.want, recorder.Body.String())
		})
	}
}
