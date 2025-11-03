package deletegame

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
		setupRepos     func(*inmemory.GameRepository, *inmemory.ServerRepository)
		expectedStatus int
		wantError      string
	}{
		{
			name:     "successful_game_deletion",
			gameCode: "cs16",
			setupRepos: func(gameRepo *inmemory.GameRepository, _ *inmemory.ServerRepository) {
				game := &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "cannot_delete_game_with_servers",
			gameCode: "cs16",
			setupRepos: func(gameRepo *inmemory.GameRepository, serverRepo *inmemory.ServerRepository) {
				game := &domain.Game{
					Code:    "cs16",
					Name:    "Counter-Strike 1.6",
					Engine:  "GoldSource",
					Enabled: 1,
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

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
			wantError:      "cannot delete game: servers are using this game",
		},
		{
			name:     "delete_non-existent_game",
			gameCode: "nonexistent",
			setupRepos: func(_ *inmemory.GameRepository, _ *inmemory.ServerRepository) {
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			gameRepo := inmemory.NewGameRepository()
			serverRepo := inmemory.NewServerRepository()
			responder := api.NewResponder()
			handler := NewHandler(gameRepo, serverRepo, responder)

			if tt.setupRepos != nil {
				tt.setupRepos(gameRepo, serverRepo)
			}

			// Create router to handle URL parameters
			router := mux.NewRouter()
			router.Handle("/api/games/{code}", handler).Methods(http.MethodDelete)

			url := "/api/games/" + tt.gameCode
			if tt.gameCode == "" {
				url = "/api/games/"
			}

			req := httptest.NewRequest(http.MethodDelete, url, nil)
			w := httptest.NewRecorder()

			// ACT
			router.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			if tt.wantError != "" {
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}
		})
	}
}

func TestHandler_GameDeletion(t *testing.T) {
	// ARRANGE
	gameRepo := inmemory.NewGameRepository()
	serverRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()
	handler := NewHandler(gameRepo, serverRepo, responder)

	// Add initial games
	games := []*domain.Game{
		{
			Code:    "cs16",
			Name:    "Counter-Strike 1.6",
			Engine:  "GoldSource",
			Enabled: 1,
		},
		{
			Code:    "hl2",
			Name:    "Half-Life 2",
			Engine:  "Source",
			Enabled: 1,
		},
	}

	for _, game := range games {
		err := gameRepo.Save(context.Background(), game)
		require.NoError(t, err)
	}

	// Verify both games exist
	allGames, err := gameRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, allGames, 2)

	// Create router
	router := mux.NewRouter()
	router.Handle("/api/games/{code}", handler).Methods(http.MethodDelete)

	req := httptest.NewRequest(http.MethodDelete, "/api/games/cs16", nil)
	w := httptest.NewRecorder()

	// ACT
	router.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code)

	// Verify the game was deleted
	allGames, err = gameRepo.FindAll(context.Background(), nil, nil)
	require.NoError(t, err)
	require.Len(t, allGames, 1)

	// Verify the remaining game is hl2
	assert.Equal(t, "hl2", allGames[0].Code)
}

func TestHandler_NewHandler(t *testing.T) {
	gameRepo := inmemory.NewGameRepository()
	serverRepo := inmemory.NewServerRepository()
	responder := api.NewResponder()

	handler := NewHandler(gameRepo, serverRepo, responder)

	require.NotNil(t, handler)
	assert.Equal(t, gameRepo, handler.repo)
	assert.Equal(t, serverRepo, handler.serverRepo)
	assert.Equal(t, responder, handler.responder)
}
