package postgames

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		wantError      string
	}{
		{
			name: "valid game creation",
			requestBody: `{
				"code": "cs16",
				"name": "Counter-Strike 1.6",
				"engine": "GoldSource",
				"enabled": 1
			}`,
			expectedStatus: http.StatusOK,
		},
		{
			name: "missing required code field",
			requestBody: `{
				"name": "Counter-Strike 1.6",
				"engine": "GoldSource",
				"enabled": 1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game code is required",
		},
		{
			name: "missing required name field",
			requestBody: `{
				"code": "cs16",
				"engine": "GoldSource",
				"enabled": 1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game name is required",
		},
		{
			name: "missing required engine field",
			requestBody: `{
				"code": "cs16",
				"name": "Counter-Strike 1.6",
				"enabled": 1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "engine is required",
		},
		{
			name:           "invalid JSON body",
			requestBody:    `{"invalid": json}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request",
		},
		{
			name: "code too long",
			requestBody: `{
				"code": "this_code_is_way_too_long",
				"name": "Test Game",
				"engine": "TestEngine",
				"enabled": 1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game code must not exceed 16 characters",
		},
		{
			name: "name too long",
			requestBody: `{
				"code": "test",
				"name": "` + strings.Repeat("a", 129) + `",
				"engine": "TestEngine",
				"enabled": 1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "game name must not exceed 128 characters",
		},
		{
			name: "engine too long",
			requestBody: `{
				"code": "test",
				"name": "Test Game",
				"engine": "` + strings.Repeat("a", 129) + `",
				"enabled": 1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "engine must not exceed 128 characters",
		},
		{
			name: "engine version too long",
			requestBody: `{
				"code": "test",
				"name": "Test Game",
				"engine": "TestEngine",
				"engine_version": "` + strings.Repeat("a", 129) + `",
				"enabled": 1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "engine version must not exceed 128 characters",
		},
		{
			name: "steam app set config too long",
			requestBody: `{
				"code": "test",
				"name": "Test Game",
				"engine": "TestEngine",
				"steam_app_set_config": "` + strings.Repeat("a", 129) + `",
				"enabled": 1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "steam app set config must not exceed 128 characters",
		},
		{
			name: "remote repository too long",
			requestBody: `{
				"code": "test",
				"name": "Test Game",
				"engine": "TestEngine",
				"remote_repository_linux": "` + strings.Repeat("a", 129) + `",
				"enabled": 1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "remote repository must not exceed 128 characters",
		},
		{
			name: "local repository too long",
			requestBody: `{
				"code": "test",
				"name": "Test Game",
				"engine": "TestEngine",
				"local_repository_windows": "` + strings.Repeat("a", 129) + `",
				"enabled": 1
			}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "local repository must not exceed 128 characters",
		},
		{
			name: "complete game with all optional fields",
			requestBody: `{
				"code": "cs16",
				"name": "Counter-Strike 1.6",
				"engine": "GoldSource",
				"engine_version": "1.0",
				"steam_app_id_linux": 10,
				"steam_app_id_windows": 10,
				"steam_app_set_config": "config",
				"remote_repository_linux": "/remote/linux",
				"remote_repository_windows": "/remote/windows",
				"local_repository_linux": "/local/linux",
				"local_repository_windows": "/local/windows",
				"enabled": 1
			}`,
			expectedStatus: http.StatusOK,
		},
		{
			name: "game with disabled state",
			requestBody: `{
				"code": "hl1",
				"name": "Half-Life",
				"engine": "GoldSource",
				"enabled": 0
			}`,
			expectedStatus: http.StatusOK,
		},
		{
			name: "game with empty optional strings",
			requestBody: `{
				"code": "dod",
				"name": "Day of Defeat",
				"engine": "GoldSource",
				"engine_version": "",
				"enabled": 1
			}`,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := inmemory.NewGameRepository()
			responder := api.NewResponder()
			handler := NewHandler(repo, responder)

			body := []byte(tt.requestBody)

			req := httptest.NewRequest(http.MethodPost, "/games", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			if tt.wantError != "" {
				assert.Equal(t, "error", response["status"])
				if errorMsg, ok := response["error"].(string); !ok || !strings.Contains(errorMsg, tt.wantError) {
					t.Errorf("Expected error containing '%s', got: %v", tt.wantError, response["error"])
				}
			} else {
				require.Equal(t, http.StatusOK, w.Code)

				games, err := repo.FindAll(context.Background(), nil, nil)
				if err != nil {
					t.Errorf("Failed to retrieve games from repository: %v", err)
				}
				if len(games) == 0 {
					t.Error("Expected game to be saved to repository, but none found")
				}
			}
		})
	}
}

func TestHandler_GamePersistence(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewGameRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	gameData := map[string]any{
		"code":                      "hl2",
		"name":                      "Half-Life 2",
		"engine":                    "Source",
		"engine_version":            "1.0",
		"steam_app_id_linux":        220,
		"steam_app_id_windows":      2220,
		"steam_app_set_config":      "hl2_config",
		"remote_repository_linux":   "https://example.com/hl2/linux",
		"remote_repository_windows": "https://example.com/hl2/windows",
		"local_repository_linux":    "/local/hl2/linux",
		"local_repository_windows":  "C:\\local\\hl2\\windows",
		"enabled":                   1,
	}

	body, err := json.Marshal(gameData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/games", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code)

	games, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, games, 1)

	game := games[0]
	assert.Equal(t, "hl2", game.Code)
	assert.Equal(t, "Half-Life 2", game.Name)
	assert.Equal(t, "Source", game.Engine)
	assert.Equal(t, "1.0", game.EngineVersion)
	require.NotNil(t, game.SteamAppIDLinux)
	assert.Equal(t, uint(220), *game.SteamAppIDLinux)
	require.NotNil(t, game.SteamAppIDWindows)
	assert.Equal(t, uint(2220), *game.SteamAppIDWindows)
	require.NotNil(t, game.SteamAppSetConfig)
	assert.Equal(t, "hl2_config", *game.SteamAppSetConfig)
	require.NotNil(t, game.RemoteRepositoryLinux)
	assert.Equal(t, "https://example.com/hl2/linux", *game.RemoteRepositoryLinux)
	require.NotNil(t, game.RemoteRepositoryWindows)
	assert.Equal(t, "https://example.com/hl2/windows", *game.RemoteRepositoryWindows)
	require.NotNil(t, game.LocalRepositoryLinux)
	assert.Equal(t, "/local/hl2/linux", *game.LocalRepositoryLinux)
	require.NotNil(t, game.LocalRepositoryWindows)
	assert.Equal(t, "C:\\local\\hl2\\windows", *game.LocalRepositoryWindows)
	assert.Equal(t, 1, game.Enabled)
}

func TestHandler_DuplicateGameCode(t *testing.T) {
	// ARRANGE
	repo := inmemory.NewGameRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	// Create the first game
	firstGameData := `{
		"code": "cs16",
		"name": "Counter-Strike 1.6",
		"engine": "GoldSource",
		"enabled": 1
	}`

	req1 := httptest.NewRequest(http.MethodPost, "/games", bytes.NewBufferString(firstGameData))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()

	handler.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusOK, w1.Code)

	// ACT: Try to create a game with the same code
	duplicateGameData := `{
		"code": "cs16",
		"name": "Counter-Strike 1.6 Duplicate",
		"engine": "GoldSource",
		"enabled": 1
	}`

	req2 := httptest.NewRequest(http.MethodPost, "/games", bytes.NewBufferString(duplicateGameData))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()

	handler.ServeHTTP(w2, req2)

	// ASSERT
	assert.Equal(t, http.StatusConflict, w2.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"].(string), "game with this code already exists")

	// Verify only one game exists in the repository
	games, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.Len(t, games, 1)
	assert.Equal(t, "Counter-Strike 1.6", games[0].Name)
}
