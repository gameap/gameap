package getgamemod

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gmBase "github.com/gameap/gameap/internal/api/gamemods/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		gameModID      string
		setupRepo      func(*inmemory.GameModRepository)
		expectedStatus int
		wantError      string
		expectGameMod  bool
	}{
		{
			name:      "successful game mod retrieval",
			gameModID: "1",
			setupRepo: func(repo *inmemory.GameModRepository) {
				gameMod := &domain.GameMod{
					ID:       1,
					GameCode: "cs16",
					Name:     "Classic",
					FastRcon: domain.GameModFastRconList{
						{
							Info:    "Status",
							Command: "status",
						},
					},
					RemoteRepositoryLinux:   new("https://example.com/cs16/classic/linux"),
					RemoteRepositoryWindows: new("https://example.com/cs16/classic/windows"),
					LocalRepositoryLinux:    new("/local/cs16/classic/linux"),
					LocalRepositoryWindows:  new("C:\\local\\cs16\\classic\\windows"),
					StartCmdLinux:           new("./hlds_run -game cstrike +map de_dust2"),
					StartCmdWindows:         new("hlds.exe -game cstrike +map de_dust2"),
					KickCmd:                 new("kick #{id}"),
					BanCmd:                  new("ban #{id}"),
					ChnameCmd:               new("hostname #{hostname}"),
					SrestartCmd:             new("restart"),
					ChmapCmd:                new("changelevel #{map}"),
					SendmsgCmd:              new("say #{msg}"),
					PasswdCmd:               new("rcon_password #{password}"),
				}
				require.NoError(t, repo.Save(context.Background(), gameMod))
			},
			expectedStatus: http.StatusOK,
			expectGameMod:  true,
		},
		{
			name:           "game mod not found",
			gameModID:      "999",
			setupRepo:      func(_ *inmemory.GameModRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "game mod not found",
			expectGameMod:  false,
		},
		{
			name:           "missing game mod id",
			gameModID:      "",
			expectedStatus: http.StatusNotFound,
			expectGameMod:  false,
		},
		{
			name:           "invalid game mod id",
			gameModID:      "invalid",
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid game mod id",
			expectGameMod:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := inmemory.NewGameModRepository()
			responder := api.NewResponder()
			handler := NewHandler(repo, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}

			router := mux.NewRouter()
			router.Handle("/api/game_mods/{id}", handler).Methods(http.MethodGet)

			url := "/api/game_mods/" + tt.gameModID
			if tt.gameModID == "" {
				url = "/api/game_mods/"
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}

			if tt.expectGameMod {
				var gameModResp gmBase.GameModResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &gameModResp))
				assert.Equal(t, uint(1), gameModResp.ID)
				assert.Equal(t, "cs16", gameModResp.GameCode)
				assert.Equal(t, "Classic", gameModResp.Name)
				assert.NotEmpty(t, gameModResp.FastRcon)
			}
		})
	}
}

func TestHandler_GameModRetrieval(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	gameMods := []*domain.GameMod{
		{
			ID:                      1,
			GameCode:                "cs16",
			Name:                    "Classic",
			RemoteRepositoryLinux:   new("https://example.com/cs16/classic/linux"),
			RemoteRepositoryWindows: new("https://example.com/cs16/classic/windows"),
			LocalRepositoryLinux:    new("/local/cs16/classic/linux"),
			LocalRepositoryWindows:  new("C:\\local\\cs16\\classic\\windows"),
			StartCmdLinux:           new("./hlds_run -game cstrike +map de_dust2"),
			StartCmdWindows:         new("hlds.exe -game cstrike +map de_dust2"),
			KickCmd:                 new("kick #{id}"),
			BanCmd:                  new("ban #{id}"),
			ChnameCmd:               new("hostname #{hostname}"),
			SrestartCmd:             new("restart"),
			ChmapCmd:                new("changelevel #{map}"),
			SendmsgCmd:              new("say #{msg}"),
			PasswdCmd:               new("rcon_password #{password}"),
		},
		{
			ID:                      2,
			GameCode:                "hl2dm",
			Name:                    "Deathmatch",
			RemoteRepositoryLinux:   new("https://example.com/hl2dm/dm/linux"),
			RemoteRepositoryWindows: new("https://example.com/hl2dm/dm/windows"),
			LocalRepositoryLinux:    new("/local/hl2dm/dm/linux"),
			LocalRepositoryWindows:  new("C:\\local\\hl2dm\\dm\\windows"),
			StartCmdLinux:           new("./srcds_run -game hl2mp +map dm_lockdown"),
			StartCmdWindows:         new("srcds.exe -game hl2mp +map dm_lockdown"),
			KickCmd:                 new("kick #{id}"),
			BanCmd:                  new("banid 0 #{id}"),
			ChnameCmd:               new("hostname #{hostname}"),
			SrestartCmd:             new("restart"),
			ChmapCmd:                new("changelevel #{map}"),
			SendmsgCmd:              new("say #{msg}"),
			PasswdCmd:               new("rcon_password #{password}"),
		},
	}

	for _, gameMod := range gameMods {
		require.NoError(t, repo.Save(context.Background(), gameMod))
	}

	router := mux.NewRouter()
	router.Handle("/api/game_mods/{id}", handler).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/game_mods/1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var gameModResp gmBase.GameModResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &gameModResp))

	assert.Equal(t, uint(1), gameModResp.ID)
	assert.Equal(t, "cs16", gameModResp.GameCode)
	assert.Equal(t, "Classic", gameModResp.Name)
}

func TestHandler_GameModResponseFields(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	gameMod := &domain.GameMod{
		ID:                      1,
		GameCode:                "cs16",
		Name:                    "Classic",
		RemoteRepositoryLinux:   new("https://example.com/cs16/classic/linux"),
		RemoteRepositoryWindows: new("https://example.com/cs16/classic/windows"),
		LocalRepositoryLinux:    new("/local/cs16/classic/linux"),
		LocalRepositoryWindows:  new("C:\\local\\cs16\\classic\\windows"),
		StartCmdLinux:           new("./hlds_run -game cstrike +map de_dust2 +maxplayers 32"),
		StartCmdWindows:         new("hlds.exe -game cstrike +map de_dust2 +maxplayers 32"),
		KickCmd:                 new("kick #{id}"),
		BanCmd:                  new("ban #{id}"),
		ChnameCmd:               new("hostname #{hostname}"),
		SrestartCmd:             new("restart"),
		ChmapCmd:                new("changelevel #{map}"),
		SendmsgCmd:              new("say #{msg}"),
		PasswdCmd:               new("rcon_password #{password}"),
		Metadata:                domain.Metadata{"docker_image": "ghcr.io/gameap/cs16:latest"},
	}

	require.NoError(t, repo.Save(context.Background(), gameMod))

	router := mux.NewRouter()
	router.Handle("/api/game_mods/{id}", handler).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/game_mods/1", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var gameModResp gmBase.GameModResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &gameModResp))

	assert.Equal(t, uint(1), gameModResp.ID)
	assert.Equal(t, "cs16", gameModResp.GameCode)
	assert.Equal(t, "Classic", gameModResp.Name)
	assert.Equal(t, "https://example.com/cs16/classic/linux", lo.FromPtr(gameModResp.RemoteRepositoryLinux))
	assert.Equal(t, "https://example.com/cs16/classic/windows", lo.FromPtr(gameModResp.RemoteRepositoryWindows))
	assert.Equal(t, "/local/cs16/classic/linux", lo.FromPtr(gameModResp.LocalRepositoryLinux))
	assert.Equal(t, "C:\\local\\cs16\\classic\\windows", lo.FromPtr(gameModResp.LocalRepositoryWindows))
	assert.Equal(t, "./hlds_run -game cstrike +map de_dust2 +maxplayers 32", lo.FromPtr(gameModResp.StartCmdLinux))
	assert.Equal(t, "hlds.exe -game cstrike +map de_dust2 +maxplayers 32", lo.FromPtr(gameModResp.StartCmdWindows))
	assert.Equal(t, "kick #{id}", lo.FromPtr(gameModResp.KickCmd))
	assert.Equal(t, "ban #{id}", lo.FromPtr(gameModResp.BanCmd))
	assert.Equal(t, "hostname #{hostname}", lo.FromPtr(gameModResp.ChnameCmd))
	assert.Equal(t, "restart", lo.FromPtr(gameModResp.SrestartCmd))
	assert.Equal(t, "changelevel #{map}", lo.FromPtr(gameModResp.ChmapCmd))
	assert.Equal(t, "say #{msg}", lo.FromPtr(gameModResp.SendmsgCmd))
	assert.Equal(t, "rcon_password #{password}", lo.FromPtr(gameModResp.PasswdCmd))
	assert.Equal(t, map[string]any{"docker_image": "ghcr.io/gameap/cs16:latest"}, gameModResp.Metadata)
}

func TestHandler_NewHandler(t *testing.T) {
	t.Parallel()
	repo := inmemory.NewGameModRepository()
	responder := api.NewResponder()

	handler := NewHandler(repo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, repo, handler.repo)
	assert.Equal(t, responder, handler.responder)
}
