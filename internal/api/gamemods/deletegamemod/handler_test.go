package deletegamemod

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		gameModID      string
		setupRepos     func(*inmemory.GameModRepository, *inmemory.ServerRepository)
		expectedStatus int
		wantError      string
	}{
		{
			name:      "successful_game_mod_deletion",
			gameModID: "1",
			setupRepos: func(gameModRepo *inmemory.GameModRepository, _ *inmemory.ServerRepository) {
				gameMod := &domain.GameMod{
					ID:                      1,
					GameCode:                "cs16",
					Name:                    "Classic",
					RemoteRepositoryLinux:   lo.ToPtr("https://example.com/cs16/classic/linux"),
					RemoteRepositoryWindows: lo.ToPtr("https://example.com/cs16/classic/windows"),
					LocalRepositoryLinux:    lo.ToPtr("/local/cs16/classic/linux"),
					LocalRepositoryWindows:  lo.ToPtr("C:\\local\\cs16\\classic\\windows"),
					StartCmdLinux:           lo.ToPtr("./hlds_run -game cstrike +map de_dust2"),
					StartCmdWindows:         lo.ToPtr("hlds.exe -game cstrike +map de_dust2"),
					KickCmd:                 lo.ToPtr("kick #{id}"),
					BanCmd:                  lo.ToPtr("ban #{id}"),
					ChnameCmd:               lo.ToPtr("hostname #{hostname}"),
					SrestartCmd:             lo.ToPtr("restart"),
					ChmapCmd:                lo.ToPtr("changelevel #{map}"),
					SendmsgCmd:              lo.ToPtr("say #{msg}"),
					PasswdCmd:               lo.ToPtr("rcon_password #{password}"),
				}
				require.NoError(t, gameModRepo.Save(context.Background(), gameMod))
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "cannot_delete_game_mod_with_servers",
			gameModID: "1",
			setupRepos: func(gameModRepo *inmemory.GameModRepository, serverRepo *inmemory.ServerRepository) {
				gameMod := &domain.GameMod{
					ID:       1,
					GameCode: "cs16",
					Name:     "Classic",
				}
				require.NoError(t, gameModRepo.Save(context.Background(), gameMod))

				server := &domain.Server{
					ID:         1,
					Name:       "Test Server",
					GameModID:  1,
					GameID:     "cs16",
					DSID:       1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					Dir:        "/servers/test",
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
			},
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "cannot delete game mod: servers are using this game mod",
		},
		{
			name:      "delete_non-existent_game_mod",
			gameModID: "999",
			setupRepos: func(_ *inmemory.GameModRepository, _ *inmemory.ServerRepository) {
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing_game_mod_id",
			gameModID:      "",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid_game_mod_id_non-numeric",
			gameModID:      "invalid",
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "invalid game mod id",
		},
		{
			name:           "invalid_game_mod_id_negative",
			gameModID:      "-1",
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "invalid game mod id",
		},
		{
			name:           "invalid_game_mod_id_zero",
			gameModID:      "0",
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "invalid game mod id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			gameModRepo := inmemory.NewGameModRepository()
			serverRepo := inmemory.NewServerRepository()
			responder := api.NewResponder()
			handler := NewHandler(gameModRepo, serverRepo, responder)

			if tt.setupRepos != nil {
				tt.setupRepos(gameModRepo, serverRepo)
			}

			// Create router to handle URL parameters
			router := mux.NewRouter()
			router.Handle("/api/game_mods/{id}", handler).Methods(http.MethodDelete)

			url := "/api/game_mods/" + tt.gameModID
			if tt.gameModID == "" {
				url = "/api/game_mods/"
			}

			req := httptest.NewRequest(http.MethodDelete, url, nil)
			w := httptest.NewRecorder()

			// ACT
			router.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Only check response body for non-404 responses
			if tt.expectedStatus != http.StatusNotFound {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

				if tt.wantError != "" {
					assert.Equal(t, "error", response["status"])
					errorMsg, ok := response["error"].(string)
					require.True(t, ok)
					assert.Contains(t, errorMsg, tt.wantError)
				} else {
					// Success response should contain status: "ok"
					assert.Equal(t, "ok", response["status"])
				}
			}
		})
	}
}

func TestHandler_GameModDeletion(t *testing.T) {
	// ARRANGE
	gameModRepo := inmemory.NewGameModRepository()
	serverRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()
	handler := NewHandler(gameModRepo, serverRepo, responder)

	// Add initial game mods
	mods := []*domain.GameMod{
		{
			ID:                      1,
			GameCode:                "cs16",
			Name:                    "Classic",
			RemoteRepositoryLinux:   lo.ToPtr("https://example.com/cs16/classic/linux"),
			RemoteRepositoryWindows: lo.ToPtr("https://example.com/cs16/classic/windows"),
			LocalRepositoryLinux:    lo.ToPtr("/local/cs16/classic/linux"),
			LocalRepositoryWindows:  lo.ToPtr("C:\\local\\cs16\\classic\\windows"),
			StartCmdLinux:           lo.ToPtr("./hlds_run -game cstrike +map de_dust2"),
			StartCmdWindows:         lo.ToPtr("hlds.exe -game cstrike +map de_dust2"),
			KickCmd:                 lo.ToPtr("kick #{id}"),
			BanCmd:                  lo.ToPtr("ban #{id}"),
			ChnameCmd:               lo.ToPtr("hostname #{hostname}"),
			SrestartCmd:             lo.ToPtr("restart"),
			ChmapCmd:                lo.ToPtr("changelevel #{map}"),
			SendmsgCmd:              lo.ToPtr("say #{msg}"),
			PasswdCmd:               lo.ToPtr("rcon_password #{password}"),
		},
		{
			ID:                      2,
			GameCode:                "hl2dm",
			Name:                    "Deathmatch",
			RemoteRepositoryLinux:   lo.ToPtr("https://example.com/hl2dm/dm/linux"),
			RemoteRepositoryWindows: lo.ToPtr("https://example.com/hl2dm/dm/windows"),
			LocalRepositoryLinux:    lo.ToPtr("/local/hl2dm/dm/linux"),
			LocalRepositoryWindows:  lo.ToPtr("C:\\local\\hl2dm\\dm\\windows"),
			StartCmdLinux:           lo.ToPtr("./srcds_run -game hl2mp +map dm_lockdown"),
			StartCmdWindows:         lo.ToPtr("srcds.exe -game hl2mp +map dm_lockdown"),
			KickCmd:                 lo.ToPtr("kick #{id}"),
			BanCmd:                  lo.ToPtr("banid 0 #{id}"),
			ChnameCmd:               lo.ToPtr("hostname #{hostname}"),
			SrestartCmd:             lo.ToPtr("restart"),
			ChmapCmd:                lo.ToPtr("changelevel #{map}"),
			SendmsgCmd:              lo.ToPtr("say #{msg}"),
			PasswdCmd:               lo.ToPtr("rcon_password #{password}"),
		},
	}
	gameMods := mods

	for _, gameMod := range gameMods {
		err := gameModRepo.Save(context.Background(), gameMod)
		require.NoError(t, err)
	}

	// Verify both game mods exist
	allGameMods, err := gameModRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, allGameMods, 2)

	// Create router
	router := mux.NewRouter()
	router.Handle("/api/game_mods/{id}", handler).Methods(http.MethodDelete)

	req := httptest.NewRequest(http.MethodDelete, "/api/game_mods/1", nil)
	w := httptest.NewRecorder()

	// ACT
	router.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code)

	// Verify the game mod was deleted
	allGameMods, err = gameModRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, allGameMods, 1)

	// Verify the remaining game mod is the hl2dm one
	assert.Equal(t, uint(2), allGameMods[0].ID)
	assert.Equal(t, "hl2dm", allGameMods[0].GameCode)
	assert.Equal(t, "Deathmatch", allGameMods[0].Name)
}

func TestHandler_IdempotentDeletion(t *testing.T) {
	// ARRANGE
	gameModRepo := inmemory.NewGameModRepository()
	serverRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()
	handler := NewHandler(gameModRepo, serverRepo, responder)

	// Add a game mod
	gameMod := &domain.GameMod{
		ID:       1,
		GameCode: "cs16",
		Name:     "Classic",
	}
	err := gameModRepo.Save(context.Background(), gameMod)
	require.NoError(t, err)

	// Create router
	router := mux.NewRouter()
	router.Handle("/api/game_mods/{id}", handler).Methods(http.MethodDelete)

	// First deletion
	req1 := httptest.NewRequest(http.MethodDelete, "/api/game_mods/1", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusOK, w1.Code)

	// Verify game mod was deleted
	allGameMods, err := gameModRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, allGameMods, 0)

	// Second deletion of the same ID (should succeed - idempotent)
	req2 := httptest.NewRequest(http.MethodDelete, "/api/game_mods/1", nil)
	w2 := httptest.NewRecorder()

	// ACT
	router.ServeHTTP(w2, req2)

	// ASSERT
	assert.Equal(t, http.StatusOK, w2.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &response))
	assert.Equal(t, "ok", response["status"])
}

func TestHandler_NewHandler(t *testing.T) {
	gameModRepo := inmemory.NewGameModRepository()
	serverRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()

	handler := NewHandler(gameModRepo, serverRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, gameModRepo, handler.repo)
	assert.Equal(t, serverRepo, handler.serverRepo)
	assert.Equal(t, responder, handler.responder)
}
