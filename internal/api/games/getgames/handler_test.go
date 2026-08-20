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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGames(t *testing.T) {
	t.Parallel()
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
					SteamAppIDLinux:         new(uint(90)),
					SteamAppIDWindows:       new(uint(190)),
					SteamAppSetConfig:       new("some-config"),
					RemoteRepositoryLinux:   new("http://example.com/linux"),
					RemoteRepositoryWindows: new("http://example.com/windows"),
					LocalRepositoryLinux:    new("/var/repo/linux"),
					LocalRepositoryWindows:  new("C:\\repo\\windows"),
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
					"enabled": 1,
					"metadata": null
				}
			]`,
		},
		{
			name: "success_with_metadata",
			games: []domain.Game{
				{
					Code:          "csgo",
					Name:          "Counter-Strike: GO",
					Engine:        "Source",
					EngineVersion: "2.0",
					Enabled:       1,
					Metadata:      domain.Metadata{"docker_image": "ghcr.io/gameap/csgo:latest", "default_port": float64(27015)},
				},
			},
			want: `[
				{
					"code": "csgo",
					"name": "Counter-Strike: GO",
					"engine": "Source",
					"engine_version": "2.0",
					"steam_app_id_linux": null,
					"steam_app_id_windows": null,
					"steam_app_set_config": null,
					"remote_repository_linux": null,
					"remote_repository_windows": null,
					"local_repository_linux": null,
					"local_repository_windows": null,
					"enabled": 1,
					"metadata": {"docker_image": "ghcr.io/gameap/csgo:latest", "default_port": 27015}
				}
			]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
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
