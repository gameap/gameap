package putgame

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		name        string
		gameCode    string
		requestBody string
		setupRepo   func(repo *inmemory.GameRepository)
		wantStatus  int
		wantError   string
	}{
		{
			name:     "successful game update",
			gameCode: "cs16",
			requestBody: `{
				"name": "Counter-Strike 1.6 Updated",
				"engine": "GoldSource",
				"enabled": 1
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				err := repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
				if err != nil {
					t.Fatalf("failed to setup test game: %v", err)
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "game not found",
			gameCode: "nonexistent",
			requestBody: `{
				"name": "Non-Existent Game",
				"engine": "GoldSource",
				"enabled": 1
			}`,
			setupRepo:  func(_ *inmemory.GameRepository) {},
			wantStatus: http.StatusNotFound,
			wantError:  "game not found",
		},
		{
			name:     "missing required name field",
			gameCode: "cs16",
			requestBody: `{
				"engine": "GoldSource",
				"enabled": 1
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				err := repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
				if err != nil {
					t.Fatalf("failed to setup test game: %v", err)
				}
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "game name is required",
		},
		{
			name:     "missing required engine field",
			gameCode: "cs16",
			requestBody: `{
				"name": "Counter-Strike 1.6",
				"enabled": 1
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				err := repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
				if err != nil {
					t.Fatalf("failed to setup test game: %v", err)
				}
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "engine is required",
		},
		{
			name:        "invalid JSON body",
			gameCode:    "cs16",
			requestBody: `{"invalid": json}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				err := repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
				if err != nil {
					t.Fatalf("failed to setup test game: %v", err)
				}
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid request",
		},
		{
			name:     "name too long",
			gameCode: "cs16",
			requestBody: `{
				"name": "` + strings.Repeat("a", 129) + `",
				"engine": "GoldSource",
				"enabled": 1
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "game name must not exceed 128 characters",
		},
		{
			name:     "engine too long",
			gameCode: "cs16",
			requestBody: `{
				"name": "Counter-Strike 1.6",
				"engine": "` + strings.Repeat("a", 129) + `",
				"enabled": 1
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "engine must not exceed 128 characters",
		},
		{
			name:     "engine version too long",
			gameCode: "cs16",
			requestBody: `{
				"name": "Counter-Strike 1.6",
				"engine": "GoldSource",
				"engine_version": "` + strings.Repeat("a", 129) + `",
				"enabled": 1
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "engine version must not exceed 128 characters",
		},
		{
			name:     "steam app set config too long",
			gameCode: "cs16",
			requestBody: `{
				"name": "Counter-Strike 1.6",
				"engine": "GoldSource",
				"steam_app_set_config": "` + strings.Repeat("a", 129) + `",
				"enabled": 1
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "steam app set config must not exceed 128 characters",
		},
		{
			name:     "remote repository too long",
			gameCode: "cs16",
			requestBody: `{
				"name": "Counter-Strike 1.6",
				"engine": "GoldSource",
				"remote_repository_linux": "` + strings.Repeat("a", 129) + `",
				"enabled": 1
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "remote repository must not exceed 128 characters",
		},
		{
			name:     "local repository too long",
			gameCode: "cs16",
			requestBody: `{
				"name": "Counter-Strike 1.6",
				"engine": "GoldSource",
				"local_repository_windows": "` + strings.Repeat("a", 129) + `",
				"enabled": 1
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  "local repository must not exceed 128 characters",
		},
		{
			name:     "update game with all optional fields",
			gameCode: "cs16",
			requestBody: `{
				"name": "Counter-Strike 1.6 Complete",
				"engine": "GoldSource",
				"engine_version": "1.1",
				"steam_app_id_linux": 10,
				"steam_app_id_windows": 10,
				"steam_app_set_config": "updated_config",
				"remote_repository_linux": "/remote/linux/updated",
				"remote_repository_windows": "/remote/windows/updated",
				"local_repository_linux": "/local/linux/updated",
				"local_repository_windows": "/local/windows/updated",
				"enabled": 1
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				err := repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
				if err != nil {
					t.Fatalf("failed to setup test game: %v", err)
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:     "disable game",
			gameCode: "cs16",
			requestBody: `{
				"name": "Counter-Strike 1.6",
				"engine": "GoldSource",
				"enabled": 0
			}`,
			setupRepo: func(repo *inmemory.GameRepository) {
				err := repo.Save(context.Background(), &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				})
				if err != nil {
					t.Fatalf("failed to setup test game: %v", err)
				}
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewGameRepository()
			responder := api.NewResponder()
			handler := NewHandler(repo, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(repo)
			}

			body := []byte(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, "/games/"+tt.gameCode, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"code": tt.gameCode})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			if tt.wantError != "" {
				assert.Equal(t, "error", response["status"])
				if errorMsg, ok := response["error"].(string); !ok || !strings.Contains(errorMsg, tt.wantError) {
					t.Errorf("want error containing '%s', got: %v", tt.wantError, response["error"])
				}
			} else {
				require.Equal(t, http.StatusOK, w.Code)
				assert.Equal(t, "ok", response["status"])
			}
		})
	}
}

func TestHandler_GameUpdatePersistence(t *testing.T) {
	repo := inmemory.NewGameRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	originalGame := &domain.Game{
		Code:    "hl2",
		Name:    "Half-Life 2",
		Engine:  "Source",
		Enabled: 1,
	}

	err := repo.Save(context.Background(), originalGame)
	require.NoError(t, err)

	updateData := map[string]any{
		"name":                      "Half-Life 2 Updated",
		"engine":                    "Source",
		"engine_version":            "2.0",
		"steam_app_id_linux":        lo.ToPtr(220),
		"steam_app_id_windows":      lo.ToPtr(2220),
		"steam_app_set_config":      lo.ToPtr("hl2_updated_config"),
		"remote_repository_linux":   lo.ToPtr("https://example.com/hl2/linux/updated"),
		"remote_repository_windows": lo.ToPtr("C:\\local\\hl2\\windows\\updated"),
		"local_repository_linux":    lo.ToPtr("C:\\local\\hl2\\windows\\updated"),
		"local_repository_windows":  lo.ToPtr("C:\\local\\hl2\\windows\\updated"),
		"enabled":                   0,
	}

	body, err := json.Marshal(updateData)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/games/hl2", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"code": "hl2"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	games, err := repo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, games, 1)

	game := games[0]
	assert.Equal(t, "hl2", game.Code)
	assert.Equal(t, "Half-Life 2 Updated", game.Name)
	assert.Equal(t, "Source", game.Engine)
	assert.Equal(t, "2.0", game.EngineVersion)
	require.NotNil(t, game.SteamAppIDLinux)
	assert.Equal(t, lo.ToPtr(uint(220)), game.SteamAppIDLinux)
	require.NotNil(t, game.SteamAppIDWindows)
	assert.Equal(t, lo.ToPtr(uint(2220)), game.SteamAppIDWindows)
	require.NotNil(t, game.SteamAppSetConfig)
	assert.Equal(t, lo.ToPtr("hl2_updated_config"), game.SteamAppSetConfig)
	require.NotNil(t, game.RemoteRepositoryLinux)
	assert.Equal(t, lo.ToPtr("https://example.com/hl2/linux/updated"), game.RemoteRepositoryLinux)
	require.NotNil(t, game.RemoteRepositoryWindows)
	assert.Equal(t, lo.ToPtr("C:\\local\\hl2\\windows\\updated"), game.RemoteRepositoryWindows)
	require.NotNil(t, game.LocalRepositoryLinux)
	assert.Equal(t, lo.ToPtr("C:\\local\\hl2\\windows\\updated"), game.LocalRepositoryLinux)
	require.NotNil(t, game.LocalRepositoryWindows)
	assert.Equal(t, lo.ToPtr("C:\\local\\hl2\\windows\\updated"), game.LocalRepositoryWindows)
	assert.Equal(t, 0, game.Enabled)
}

func TestHandler_EmptyGameCode(t *testing.T) {
	repo := inmemory.NewGameRepository()
	responder := api.NewResponder()
	handler := NewHandler(repo, responder)

	requestBody := `{
		"code": "cs16",
		"name": "Counter-Strike 1.6",
		"engine": "GoldSource",
		"enabled": 1
	}`

	req := httptest.NewRequest(http.MethodPut, "/games/", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"code": ""})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
	assert.Contains(t, response["error"].(string), "game code is required")
}
