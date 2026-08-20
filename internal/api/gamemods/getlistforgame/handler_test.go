package getlistforgame_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/gamemods/getlistforgame"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetListForGame(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		gameCode string
		gameMods []domain.GameMod
		want     string
		wantCode int
	}{
		{
			name:     "success with multiple game mods for cstrike",
			gameCode: "cstrike",
			gameMods: []domain.GameMod{
				{
					ID:       8,
					GameCode: "cstrike",
					Name:     "Classic (AMX Mod)",
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
					ID:       9,
					GameCode: "cstrike",
					Name:     "Classic (ReHLDS)",
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
					ID:       7,
					GameCode: "cstrike",
					Name:     "Default",
					FastRcon: domain.GameModFastRconList{
						{
							Info:    "Status",
							Command: "status",
						},
					},
					Vars:                    domain.GameModVarList{},
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
					ID:       10,
					GameCode: "valve",
					Name:     "Half-Life Deathmatch",
					FastRcon: domain.GameModFastRconList{
						{
							Info:    "Status",
							Command: "status",
						},
					},
					Vars:                    domain.GameModVarList{},
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
			},
			want: `[
				{
					"id": 8,
					"name": "Classic (AMX Mod)"
				},
				{
					"id": 9,
					"name": "Classic (ReHLDS)"
				},
				{
					"id": 7,
					"name": "Default"
				}
			]`,
			wantCode: http.StatusOK,
		},
		{
			name:     "success with empty result for non-existent game",
			gameCode: "nonexistent",
			gameMods: []domain.GameMod{
				{
					ID:       1,
					GameCode: "cstrike",
					Name:     "Counter-Strike",
					FastRcon: domain.GameModFastRconList{},
					Vars:     domain.GameModVarList{},
				},
			},
			want:     `[]`,
			wantCode: http.StatusOK,
		},
		{
			name:     "success with empty result when no game mods exist",
			gameCode: "valve",
			gameMods: []domain.GameMod{},
			want:     `[]`,
			wantCode: http.StatusOK,
		},
		{
			name:     "error when game code is empty",
			gameCode: "",
			gameMods: []domain.GameMod{},
			want:     `{"error":"game code is required","http_code":422,"message":"game code is required","status":"error"}`,
			wantCode: http.StatusUnprocessableEntity,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewGameModRepository()

			for _, gameMod := range test.gameMods {
				err := repo.Save(context.Background(), &gameMod)
				require.NoError(t, err)
			}

			h := getlistforgame.NewHandler(repo, api.NewResponder())
			recorder := httptest.NewRecorder()

			req := httptest.NewRequest(http.MethodGet, "/api/game_mods/get_list_for_game/"+test.gameCode, nil)
			req = mux.SetURLVars(req, map[string]string{"game": test.gameCode})

			h.ServeHTTP(recorder, req)

			assert.Equal(t, test.wantCode, recorder.Code)
			assert.JSONEq(t, test.want, recorder.Body.String())
		})
	}
}
