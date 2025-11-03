package upgradegames

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGlobalAPIService struct {
	games []domain.GlobalAPIGame
	err   error
}

func (m *mockGlobalAPIService) Games(_ context.Context) ([]domain.GlobalAPIGame, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.games, nil
}

func TestHandler_ServeHTTP_Success(t *testing.T) {
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	tm := services.NewNilTransactionManager()

	mockAPI := &mockGlobalAPIService{
		games: []domain.GlobalAPIGame{
			{
				Code:                    "cs16",
				StartCode:               "hlds_run",
				Name:                    "Counter-Strike 1.6",
				Engine:                  "GoldSource",
				EngineVersion:           "1.0",
				SteamAppIDLinux:         90,
				SteamAppIDWindows:       90,
				SteamAppSetConfig:       "90 mod cstrike",
				RemoteRepositoryLinux:   "http://files.gameap.com/games/cs16-linux.tar.gz",
				RemoteRepositoryWindows: "http://files.gameap.com/games/cs16-windows.zip",
				Mods: []domain.GlobalAPIGameMod{
					{
						ID:                      1,
						GameCode:                "cs16",
						Name:                    "Classic",
						FastRcon:                domain.GameModFastRconList{},
						Vars:                    domain.GameModVarList{},
						RemoteRepositoryLinux:   "http://files.gameap.com/mods/cs16-classic-linux.tar.gz",
						RemoteRepositoryWindows: "http://files.gameap.com/mods/cs16-classic-windows.zip",
						StartCmdLinux:           "./hlds_run -game cstrike",
						StartCmdWindows:         "hlds.exe -game cstrike",
						KickCmd:                 "kick",
						BanCmd:                  "ban",
						ChnameCmd:               "hostname",
						SrestartCmd:             "restart",
						ChmapCmd:                "changelevel",
						SendmsgCmd:              "say",
						PasswdCmd:               "rcon_password",
					},
				},
			},
			{
				Code:          "hl2dm",
				StartCode:     "srcds_run",
				Name:          "Half-Life 2 Deathmatch",
				Engine:        "Source",
				EngineVersion: "1.0",
				Mods:          []domain.GlobalAPIGameMod{},
			},
		},
	}

	upgradeService := services.NewGameUpgradeService(mockAPI, gameRepo, gameModRepo, tm)
	handler := NewHandler(upgradeService, responder)

	req := httptest.NewRequest(http.MethodPost, "/api/games/upgrade", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "ok", response["status"])

	games, err := gameRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, games, 2)

	cs16Found := false
	hl2dmFound := false
	for _, game := range games {
		switch game.Code {
		case "cs16":
			cs16Found = true
			assert.Equal(t, "Counter-Strike 1.6", game.Name)
			assert.Equal(t, "GoldSource", game.Engine)
			assert.Equal(t, "1.0", game.EngineVersion)
			assert.Equal(t, 1, game.Enabled)
			require.NotNil(t, game.SteamAppIDLinux)
			assert.Equal(t, uint(90), *game.SteamAppIDLinux)
			require.NotNil(t, game.SteamAppIDWindows)
			assert.Equal(t, uint(90), *game.SteamAppIDWindows)
			require.NotNil(t, game.SteamAppSetConfig)
			assert.Equal(t, "90 mod cstrike", *game.SteamAppSetConfig)
			require.NotNil(t, game.RemoteRepositoryLinux)
			assert.Equal(t, "http://files.gameap.com/games/cs16-linux.tar.gz", *game.RemoteRepositoryLinux)
			require.NotNil(t, game.RemoteRepositoryWindows)
			assert.Equal(t, "http://files.gameap.com/games/cs16-windows.zip", *game.RemoteRepositoryWindows)
		case "hl2dm":
			hl2dmFound = true
			assert.Equal(t, "Half-Life 2 Deathmatch", game.Name)
			assert.Equal(t, "Source", game.Engine)
			assert.Equal(t, 1, game.Enabled)
		}
	}

	assert.True(t, cs16Found, "cs16 game should be saved")
	assert.True(t, hl2dmFound, "hl2dm game should be saved")

	mods, err := gameModRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, mods, 1)

	mod := mods[0]
	assert.Equal(t, "cs16", mod.GameCode)
	assert.Equal(t, "Classic", mod.Name)
	require.NotNil(t, mod.RemoteRepositoryLinux)
	assert.Equal(t, "http://files.gameap.com/mods/cs16-classic-linux.tar.gz", *mod.RemoteRepositoryLinux)
	require.NotNil(t, mod.RemoteRepositoryWindows)
	assert.Equal(t, "http://files.gameap.com/mods/cs16-classic-windows.zip", *mod.RemoteRepositoryWindows)
	require.NotNil(t, mod.StartCmdLinux)
	assert.Equal(t, "./hlds_run -game cstrike", *mod.StartCmdLinux)
	require.NotNil(t, mod.StartCmdWindows)
	assert.Equal(t, "hlds.exe -game cstrike", *mod.StartCmdWindows)
	require.NotNil(t, mod.KickCmd)
	assert.Equal(t, "kick", *mod.KickCmd)
	require.NotNil(t, mod.BanCmd)
	assert.Equal(t, "ban", *mod.BanCmd)
	require.NotNil(t, mod.ChnameCmd)
	assert.Equal(t, "hostname", *mod.ChnameCmd)
	require.NotNil(t, mod.SrestartCmd)
	assert.Equal(t, "restart", *mod.SrestartCmd)
	require.NotNil(t, mod.ChmapCmd)
	assert.Equal(t, "changelevel", *mod.ChmapCmd)
	require.NotNil(t, mod.SendmsgCmd)
	assert.Equal(t, "say", *mod.SendmsgCmd)
	require.NotNil(t, mod.PasswdCmd)
	assert.Equal(t, "rcon_password", *mod.PasswdCmd)
}

func TestHandler_ServeHTTP_GlobalAPIError(t *testing.T) {
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	tm := services.NewNilTransactionManager()

	mockAPI := &mockGlobalAPIService{
		err: errors.New("network error"),
	}

	upgradeService := services.NewGameUpgradeService(mockAPI, gameRepo, gameModRepo, tm)
	handler := NewHandler(upgradeService, responder)

	req := httptest.NewRequest(http.MethodPost, "/api/games/upgrade", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
	assert.Equal(t, "Internal Server Error", response["error"].(string))
}

func TestHandler_ServeHTTP_EmptyGamesList(t *testing.T) {
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	tm := services.NewNilTransactionManager()

	mockAPI := &mockGlobalAPIService{
		games: []domain.GlobalAPIGame{},
	}

	upgradeService := services.NewGameUpgradeService(mockAPI, gameRepo, gameModRepo, tm)
	handler := NewHandler(upgradeService, responder)

	req := httptest.NewRequest(http.MethodPost, "/api/games/upgrade", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "ok", response["status"])

	games, err := gameRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.Empty(t, games)
}

func TestHandler_ServeHTTP_UpdateExistingGame(t *testing.T) {
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	tm := services.NewNilTransactionManager()

	existingName := "Counter-Strike"
	existingGame := &domain.Game{
		Code:    "cs16",
		Name:    existingName,
		Engine:  "GoldSource",
		Enabled: 0,
	}
	require.NoError(t, gameRepo.Save(context.Background(), existingGame))

	mockAPI := &mockGlobalAPIService{
		games: []domain.GlobalAPIGame{
			{
				Code:          "cs16",
				StartCode:     "hlds_run",
				Name:          "Counter-Strike 1.6 Updated",
				Engine:        "GoldSource",
				EngineVersion: "2.0",
			},
		},
	}

	upgradeService := services.NewGameUpgradeService(mockAPI, gameRepo, gameModRepo, tm)
	handler := NewHandler(upgradeService, responder)

	req := httptest.NewRequest(http.MethodPost, "/api/games/upgrade", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	games, err := gameRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, games, 1)

	game := games[0]
	assert.Equal(t, "cs16", game.Code)
	assert.Equal(t, "Counter-Strike 1.6 Updated", game.Name)
	assert.Equal(t, "2.0", game.EngineVersion)
	assert.Equal(t, 1, game.Enabled)
}

func TestHandler_ServeHTTP_GameWithOptionalFieldsEmpty(t *testing.T) {
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	tm := services.NewNilTransactionManager()

	mockAPI := &mockGlobalAPIService{
		games: []domain.GlobalAPIGame{
			{
				Code:   "minimalgame",
				Name:   "Minimal Game",
				Engine: "TestEngine",
			},
		},
	}

	upgradeService := services.NewGameUpgradeService(mockAPI, gameRepo, gameModRepo, tm)
	handler := NewHandler(upgradeService, responder)

	req := httptest.NewRequest(http.MethodPost, "/api/games/upgrade", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	games, err := gameRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, games, 1)

	game := games[0]
	assert.Equal(t, "minimalgame", game.Code)
	assert.Equal(t, "Minimal Game", game.Name)
	assert.Equal(t, "TestEngine", game.Engine)
	assert.Equal(t, 1, game.Enabled)
	assert.Nil(t, game.SteamAppIDLinux)
	assert.Nil(t, game.SteamAppIDWindows)
	assert.Nil(t, game.SteamAppSetConfig)
	assert.Nil(t, game.RemoteRepositoryLinux)
	assert.Nil(t, game.RemoteRepositoryWindows)
}

func TestHandler_ServeHTTP_ModWithOptionalFieldsEmpty(t *testing.T) {
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()
	tm := services.NewNilTransactionManager()

	mockAPI := &mockGlobalAPIService{
		games: []domain.GlobalAPIGame{
			{
				Code:   "testgame",
				Name:   "Test Game",
				Engine: "TestEngine",
				Mods: []domain.GlobalAPIGameMod{
					{
						ID:       1,
						GameCode: "testgame",
						Name:     "Minimal Mod",
						FastRcon: domain.GameModFastRconList{},
						Vars:     domain.GameModVarList{},
					},
				},
			},
		},
	}

	upgradeService := services.NewGameUpgradeService(mockAPI, gameRepo, gameModRepo, tm)
	handler := NewHandler(upgradeService, responder)

	req := httptest.NewRequest(http.MethodPost, "/api/games/upgrade", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	mods, err := gameModRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, mods, 1)

	mod := mods[0]
	assert.Equal(t, "testgame", mod.GameCode)
	assert.Equal(t, "Minimal Mod", mod.Name)
	assert.Nil(t, mod.RemoteRepositoryLinux)
	assert.Nil(t, mod.RemoteRepositoryWindows)
	assert.Nil(t, mod.StartCmdLinux)
	assert.Nil(t, mod.StartCmdWindows)
	assert.Nil(t, mod.KickCmd)
	assert.Nil(t, mod.BanCmd)
	assert.Nil(t, mod.ChnameCmd)
	assert.Nil(t, mod.SrestartCmd)
	assert.Nil(t, mod.ChmapCmd)
	assert.Nil(t, mod.SendmsgCmd)
	assert.Nil(t, mod.PasswdCmd)
}
