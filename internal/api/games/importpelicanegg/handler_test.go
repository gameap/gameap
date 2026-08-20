package importpelicanegg

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/services/pelicaneggimporter"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		requestBody    string
		expectedStatus int
		wantError      string
		validate       func(t *testing.T, response map[string]any, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository)
	}{
		{
			name: "successful_import",
			requestBody: `{
				"uuid": "test-uuid",
				"name": "Test Game",
				"startup": "./server -port {{server.build.default.port}}",
				"variables": [
					{
						"name": "Server Name",
						"env_variable": "SERVER_NAME",
						"default_value": "My Server",
						"user_editable": true
					}
				]
			}`,
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, response map[string]any, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				t.Helper()

				assert.Equal(t, "test_game", response["game_code"])
				assert.Equal(t, "Test Game", response["game_name"])
				assert.NotNil(t, response["game_mod_id"])

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "Test Game", games[0].Name)

				gameMods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, gameMods, 1)
			},
		},
		{
			name: "successful_import_with_all_fields",
			requestBody: `{
				"uuid": "82c049db-06e3-416a-8ed3-805cc53105a9",
				"author": "test@example.com",
				"name": "Rage.MP",
				"description": "Rage Multiplayer Server",
				"docker_images": {
					"ghcr.io/parkervcp/yolks:debian": "ghcr.io/parkervcp/yolks:debian"
				},
				"startup": "./ragemp-server",
				"config": {
					"startup": {
						"done": "The server is ready to accept connections"
					},
					"stop": "^X"
				},
				"scripts": {
					"installation": {
						"script": "#!/bin/bash\nmkdir -p /mnt/server",
						"container": "ghcr.io/parkervcp/installers:debian",
						"entrypoint": "bash"
					}
				},
				"variables": [
					{
						"name": "Server Name",
						"description": "Name of your server",
						"env_variable": "SERVER_NAME",
						"default_value": "My Rage.MP Server",
						"user_viewable": true,
						"user_editable": true,
						"rules": "required|string|max:64",
						"field_type": "text"
					}
				]
			}`,
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, response map[string]any, gameRepo *inmemory.GameRepository, gameModRepo *inmemory.GameModRepository) {
				t.Helper()

				assert.Equal(t, "rage_mp", response["game_code"])
				assert.Equal(t, "Rage.MP", response["game_name"])

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)

				game := games[0]
				assert.Equal(t, "pelican", game.Engine)
				require.NotNil(t, game.Metadata)

				gameMods, err := gameModRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, gameMods, 1)

				mod := gameMods[0]
				assert.Equal(t, "./ragemp-server", *mod.StartCmdLinux)
				assert.Equal(t, "ghcr.io/parkervcp/yolks:debian", mod.Metadata["docker_image"])
				assert.Equal(t, "The server is ready to accept connections", mod.Metadata["docker_startup_done"])
			},
		},
		{
			name:           "invalid_json",
			requestBody:    `{invalid json}`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "failed to parse pelican egg",
		},
		{
			name:           "empty_name",
			requestBody:    `{"name": "", "startup": "./server"}`,
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
		},
		{
			name:           "missing_name",
			requestBody:    `{"startup": "./server"}`,
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			responder := api.NewResponder()

			importer := pelicaneggimporter.NewImporter(
				gameRepo,
				gameModRepo,
				services.NewNilTransactionManager(),
			)

			handler := NewHandler(importer, responder)

			body := []byte(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/games/import/pelican-egg", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			if tt.wantError != "" {
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.True(t, strings.Contains(errorMsg, tt.wantError),
					"Expected error containing '%s', got: %s", tt.wantError, errorMsg)
			} else if tt.validate != nil {
				tt.validate(t, response, gameRepo, gameModRepo)
			}
		})
	}
}

func TestHandler_EmptyBody(t *testing.T) {
	t.Parallel()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()

	importer := pelicaneggimporter.NewImporter(
		gameRepo,
		gameModRepo,
		services.NewNilTransactionManager(),
	)

	handler := NewHandler(importer, responder)

	req := httptest.NewRequest(http.MethodPost, "/api/games/import/pelican-egg", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "error", response["status"])
}

func TestHandler_ResponseStructure(t *testing.T) {
	t.Parallel()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()

	importer := pelicaneggimporter.NewImporter(
		gameRepo,
		gameModRepo,
		services.NewNilTransactionManager(),
	)

	handler := NewHandler(importer, responder)

	requestBody := `{
		"name": "Test Game",
		"startup": "./server"
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/games/import/pelican-egg", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response Response
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	assert.NotEmpty(t, response.GameCode)
	assert.NotEmpty(t, response.GameName)
	assert.NotZero(t, response.GameModID)
}

func TestHandler_WithQueryParameters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		requestBody    string
		queryParams    string
		expectedStatus int
		validate       func(t *testing.T, response map[string]any, gameRepo *inmemory.GameRepository)
	}{
		{
			name: "override_name_via_query",
			requestBody: `{
				"name": "Original Name",
				"startup": "./server"
			}`,
			queryParams:    "?name=Overridden%20Name",
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, response map[string]any, gameRepo *inmemory.GameRepository) {
				t.Helper()

				assert.Equal(t, "original_name", response["game_code"])
				assert.Equal(t, "Overridden Name", response["game_name"])

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "Overridden Name", games[0].Name)
			},
		},
		{
			name: "override_code_via_query",
			requestBody: `{
				"name": "Test Game",
				"startup": "./server"
			}`,
			queryParams:    "?code=custom",
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, response map[string]any, gameRepo *inmemory.GameRepository) {
				t.Helper()

				assert.Equal(t, "custom", response["game_code"])
				assert.Equal(t, "Test Game", response["game_name"])

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "custom", games[0].Code)
			},
		},
		{
			name: "override_both_code_and_name_via_query",
			requestBody: `{
				"name": "Original Name",
				"startup": "./server"
			}`,
			queryParams:    "?name=Custom%20Name&code=custom",
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, response map[string]any, gameRepo *inmemory.GameRepository) {
				t.Helper()

				assert.Equal(t, "custom", response["game_code"])
				assert.Equal(t, "Custom Name", response["game_name"])

				games, err := gameRepo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, games, 1)
				assert.Equal(t, "custom", games[0].Code)
				assert.Equal(t, "Custom Name", games[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			responder := api.NewResponder()

			importer := pelicaneggimporter.NewImporter(
				gameRepo,
				gameModRepo,
				services.NewNilTransactionManager(),
			)

			handler := NewHandler(importer, responder)

			req := httptest.NewRequest(http.MethodPost, "/api/games/import/pelican-egg"+tt.queryParams, bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.validate != nil {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				tt.validate(t, response, gameRepo)
			}
		})
	}
}

func TestHandler_InvalidQueryParameters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		requestBody    string
		queryParams    string
		expectedStatus int
		wantError      string
	}{
		{
			name:           "invalid_code_uppercase",
			requestBody:    `{"name": "Test", "startup": "./server"}`,
			queryParams:    "?code=INVALID",
			expectedStatus: http.StatusBadRequest,
			wantError:      "code must match pattern",
		},
		{
			name:           "code_too_short",
			requestBody:    `{"name": "Test", "startup": "./server"}`,
			queryParams:    "?code=a",
			expectedStatus: http.StatusBadRequest,
			wantError:      "code must be between 2 and 16 characters",
		},
		{
			name:           "name_too_short",
			requestBody:    `{"name": "Test", "startup": "./server"}`,
			queryParams:    "?name=A",
			expectedStatus: http.StatusBadRequest,
			wantError:      "name must be between 2 and 128 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			responder := api.NewResponder()

			importer := pelicaneggimporter.NewImporter(
				gameRepo,
				gameModRepo,
				services.NewNilTransactionManager(),
			)

			handler := NewHandler(importer, responder)

			req := httptest.NewRequest(http.MethodPost, "/api/games/import/pelican-egg"+tt.queryParams, bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var errorResp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errorResp))
			assert.Equal(t, "error", errorResp["status"])
			errorMsg, ok := errorResp["error"].(string)
			require.True(t, ok)
			assert.Contains(t, errorMsg, tt.wantError)
		})
	}
}
