package getgamemods_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/gamemods/getgamemods"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGameMods(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		gameMods []domain.GameMod
		want     string
	}{
		{
			name: "success with multiple game mods",
			gameMods: []domain.GameMod{
				{
					ID:       1,
					GameCode: "valve",
					Name:     "Half-Life Deathmatch",
					FastRcon: domain.GameModFastRconList{
						{
							Info:    "Status",
							Command: "status",
						},
					},
					Vars: domain.GameModVarList{
						{
							Var:     "maxplayers",
							Default: "32",
							Info:    "Maximum number of players",
						},
					},
					RemoteRepositoryLinux:   new("http://example.com/linux"),
					RemoteRepositoryWindows: new("http://example.com/windows"),
					LocalRepositoryLinux:    new("/var/repo/linux"),
					LocalRepositoryWindows:  new("C:\\repo\\windows"),
					StartCmdLinux:           new("./hlds_run"),
					StartCmdWindows:         new("hlds.exe"),
					KickCmd:                 new("kick"),
					BanCmd:                  new("ban"),
					ChnameCmd:               new("hostname"),
					SrestartCmd:             new("restart"),
					ChmapCmd:                new("changelevel"),
					SendmsgCmd:              new("say"),
					PasswdCmd:               new("password"),
				},
				{
					ID:       2,
					GameCode: "cstrike",
					Name:     "Counter-Strike",
					FastRcon: domain.GameModFastRconList{
						{
							Info:    "Status",
							Command: "status",
						},
					},
					Vars: domain.GameModVarList{
						{
							Var:     "maxplayers",
							Default: "32",
							Info:    "Maximum number of players",
						},
					},
					RemoteRepositoryLinux:   new("http://cs.example.com/linux"),
					RemoteRepositoryWindows: new("http://cs.example.com/windows"),
					LocalRepositoryLinux:    new("/var/repo/cs"),
					LocalRepositoryWindows:  new("C:\\repo\\cs"),
					StartCmdLinux:           new("./hlds_run -game cstrike"),
					StartCmdWindows:         new("hlds.exe -game cstrike"),
					KickCmd:                 new("kick"),
					BanCmd:                  new("banid"),
					ChnameCmd:               new("hostname"),
					SrestartCmd:             new("restart"),
					ChmapCmd:                new("changelevel"),
					SendmsgCmd:              new("say"),
					PasswdCmd:               new("rcon_password"),
				},
			},
			want: `[
				{
					"id": 2,
					"game_code": "cstrike",
					"name": "Counter-Strike",
					"fast_rcon": [
						{
							"info": "Status",
							"command": "status"
						}
					],
					"vars": [
						{
							"var": "maxplayers",
							"default": "32",
							"info": "Maximum number of players",
							"admin_var": false
						}
					],
					"remote_repository_linux": "http://cs.example.com/linux",
					"remote_repository_windows": "http://cs.example.com/windows",
					"local_repository_linux": "/var/repo/cs",
					"local_repository_windows": "C:\\repo\\cs",
					"start_cmd_linux": "./hlds_run -game cstrike",
					"start_cmd_windows": "hlds.exe -game cstrike",
					"kick_cmd": "kick",
					"ban_cmd": "banid",
					"chname_cmd": "hostname",
					"srestart_cmd": "restart",
					"chmap_cmd": "changelevel",
					"sendmsg_cmd": "say",
					"passwd_cmd": "rcon_password",
					"metadata": null
				},
				{
					"id": 1,
					"game_code": "valve",
					"name": "Half-Life Deathmatch",
					"fast_rcon": [
						{
							"info": "Status",
							"command": "status"
						}
					],
					"vars": [
						{
							"var": "maxplayers",
							"default": "32",
							"info": "Maximum number of players",
							"admin_var": false
						}
					],
					"remote_repository_linux": "http://example.com/linux",
					"remote_repository_windows": "http://example.com/windows",
					"local_repository_linux": "/var/repo/linux",
					"local_repository_windows": "C:\\repo\\windows",
					"start_cmd_linux": "./hlds_run",
					"start_cmd_windows": "hlds.exe",
					"kick_cmd": "kick",
					"ban_cmd": "ban",
					"chname_cmd": "hostname",
					"srestart_cmd": "restart",
					"chmap_cmd": "changelevel",
					"sendmsg_cmd": "say",
					"passwd_cmd": "password",
					"metadata": null
				}
			]`,
		},
		{
			name:     "success with empty result",
			gameMods: []domain.GameMod{},
			want:     `[]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			repo := inmemory.NewGameModRepository()

			for _, gameMod := range test.gameMods {
				err := repo.Save(context.Background(), &gameMod)
				require.NoError(t, err)
			}

			h := getgamemods.NewHandler(repo, api.NewResponder())
			recorder := httptest.NewRecorder()

			// ACT
			h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/game_mods", nil))

			// ASSERT
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.JSONEq(t, test.want, recorder.Body.String())
		})
	}
}
