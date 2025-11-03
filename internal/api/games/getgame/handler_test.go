package getgame

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		gameCode       string
		setupRepo      func(*inmemory.GameRepository)
		expectedStatus int
		wantError      string
		expectGame     bool
	}{
		{
			name:     "successful game retrieval",
			gameCode: "cs16",
			setupRepo: func(repo *inmemory.GameRepository) {
				game := &domain.Game{
					Code:          "cs16",
					Name:          "Counter-Strike 1.6",
					Engine:        "GoldSource",
					EngineVersion: "1.0",
					Enabled:       1,
				}
				require.NoError(t, repo.Save(context.Background(), game))
			},
			expectedStatus: http.StatusOK,
			expectGame:     true,
		},
		{
			name:           "game not found",
			gameCode:       "nonexistent",
			setupRepo:      func(_ *inmemory.GameRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "game not found",
			expectGame:     false,
		},
		{
			name:           "missing game code",
			gameCode:       "",
			expectedStatus: http.StatusNotFound,
			expectGame:     false,
		},
		{
			name:     "game with all optional fields",
			gameCode: "hl2",
			setupRepo: func(repo *inmemory.GameRepository) {
				steamAppLinux := uint(220)
				steamAppWindows := uint(220)
				steamAppConfig := "hl2_config"
				remoteRepoLinux := "https://example.com/hl2/linux"
				remoteRepoWindows := "https://example.com/hl2/windows"
				localRepoLinux := "/local/hl2/linux"
				localRepoWindows := "C:\\local\\hl2\\windows"

				game := &domain.Game{
					Code:                    "hl2",
					Name:                    "Half-Life 2",
					Engine:                  "Source",
					EngineVersion:           "1.0",
					SteamAppIDLinux:         &steamAppLinux,
					SteamAppIDWindows:       &steamAppWindows,
					SteamAppSetConfig:       &steamAppConfig,
					RemoteRepositoryLinux:   &remoteRepoLinux,
					RemoteRepositoryWindows: &remoteRepoWindows,
					LocalRepositoryLinux:    &localRepoLinux,
					LocalRepositoryWindows:  &localRepoWindows,
					Enabled:                 1,
				}
				require.NoError(t, repo.Save(context.Background(), game))
			},
			expectedStatus: http.StatusOK,
			expectGame:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewGameRepository()
			responder := api.NewResponder()
			handler := NewHandler(repo, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}

			// Create router to handle URL parameters
			router := mux.NewRouter()
			router.Handle("/api/games/{code}", handler).Methods(http.MethodGet)

			url := "/api/games/" + tt.gameCode
			if tt.gameCode == "" {
				url = "/api/games/"
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()

			// ACT
			router.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}

			if tt.expectGame {
				var gameResp gameResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &gameResp))
				assert.Equal(t, tt.gameCode, gameResp.Code)
				assert.NotEmpty(t, gameResp.Name)
				assert.NotEmpty(t, gameResp.Engine)
			}
		})
	}
}

func TestHandler_GameRetrieval(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewGameRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	// Add test games
	games := []*domain.Game{
		{
			Code:          "cs16",
			Name:          "Counter-Strike 1.6",
			Engine:        "GoldSource",
			EngineVersion: "1.0",
			Enabled:       1,
		},
		{
			Code:          "hl2",
			Name:          "Half-Life 2",
			Engine:        "Source",
			EngineVersion: "1.0",
			Enabled:       0,
		},
	}

	for _, game := range games {
		require.NoError(t, repo.Save(context.Background(), game))
	}

	// Create router
	router := mux.NewRouter()
	router.Handle("/api/games/{code}", handler).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/games/cs16", nil)
	w := httptest.NewRecorder()

	// ACT
	router.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code)

	var gameResp gameResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &gameResp))

	// Verify the correct game was returned
	assert.Equal(t, "cs16", gameResp.Code)
	assert.Equal(t, "Counter-Strike 1.6", gameResp.Name)
	assert.Equal(t, "GoldSource", gameResp.Engine)
	assert.Equal(t, "1.0", gameResp.EngineVersion)
	assert.True(t, gameResp.Enabled)
}

func TestHandler_GameResponseFields(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewGameRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	steamAppLinux := uint(220)
	steamAppWindows := uint(220)
	steamAppConfig := "hl2_config"
	remoteRepoLinux := "https://example.com/hl2/linux"
	remoteRepoWindows := "https://example.com/hl2/windows"
	localRepoLinux := "/local/hl2/linux"
	localRepoWindows := "C:\\local\\hl2\\windows"

	game := &domain.Game{
		Code:                    "hl2",
		Name:                    "Half-Life 2",
		Engine:                  "Source",
		EngineVersion:           "1.0",
		SteamAppIDLinux:         &steamAppLinux,
		SteamAppIDWindows:       &steamAppWindows,
		SteamAppSetConfig:       &steamAppConfig,
		RemoteRepositoryLinux:   &remoteRepoLinux,
		RemoteRepositoryWindows: &remoteRepoWindows,
		LocalRepositoryLinux:    &localRepoLinux,
		LocalRepositoryWindows:  &localRepoWindows,
		Enabled:                 1,
	}

	require.NoError(t, repo.Save(context.Background(), game))

	// Create router
	router := mux.NewRouter()
	router.Handle("/api/games/{code}", handler).Methods(http.MethodGet)

	req := httptest.NewRequest(http.MethodGet, "/api/games/hl2", nil)
	w := httptest.NewRecorder()

	// ACT
	router.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code)

	var gameResp gameResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &gameResp))

	// Verify all fields are correctly mapped
	assert.Equal(t, "hl2", gameResp.Code)
	assert.Equal(t, "Half-Life 2", gameResp.Name)
	assert.Equal(t, "Source", gameResp.Engine)
	assert.Equal(t, "1.0", gameResp.EngineVersion)
	require.NotNil(t, gameResp.SteamAppIDLinux)
	assert.Equal(t, uint(220), *gameResp.SteamAppIDLinux)
	require.NotNil(t, gameResp.SteamAppIDWindows)
	assert.Equal(t, uint(220), *gameResp.SteamAppIDWindows)
	require.NotNil(t, gameResp.SteamAppSetConfig)
	assert.Equal(t, "hl2_config", *gameResp.SteamAppSetConfig)
	require.NotNil(t, gameResp.RemoteRepositoryLinux)
	assert.Equal(t, "https://example.com/hl2/linux", *gameResp.RemoteRepositoryLinux)
	require.NotNil(t, gameResp.RemoteRepositoryWindows)
	assert.Equal(t, "https://example.com/hl2/windows", *gameResp.RemoteRepositoryWindows)
	require.NotNil(t, gameResp.LocalRepositoryLinux)
	assert.Equal(t, "/local/hl2/linux", *gameResp.LocalRepositoryLinux)
	require.NotNil(t, gameResp.LocalRepositoryWindows)
	assert.Equal(t, "C:\\local\\hl2\\windows", *gameResp.LocalRepositoryWindows)
	assert.True(t, gameResp.Enabled)
}

func TestHandler_NewHandler(t *testing.T) {
	repo := inmemory.NewGameRepository()
	responder := api.NewResponder()

	handler := NewHandler(repo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, repo, handler.repo)
	assert.Equal(t, responder, handler.responder)
}
