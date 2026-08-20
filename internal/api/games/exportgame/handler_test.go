package exportgame

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services/gameexporter"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		gameCode       string
		setupGame      func(*inmemory.GameRepository)
		setupGameMod   func(*inmemory.GameModRepository)
		expectedStatus int
		validate       func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			name:     "successful_export_game_with_mods",
			gameCode: "cstrike",
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:              "cstrike",
					Name:              "Counter-Strike 1.6",
					Engine:            "GoldSource",
					EngineVersion:     "1.0",
					SteamAppIDLinux:   new(uint(90)),
					SteamAppIDWindows: new(uint(90)),
					Enabled:           1,
				})
			},
			setupGameMod: func(repo *inmemory.GameModRepository) {
				_ = repo.Save(context.Background(), &domain.GameMod{
					GameCode:      "cstrike",
					Name:          "Classic",
					StartCmdLinux: new("./hlds_run -game cstrike +port {port}"),
					KickCmd:       new("kick {name}"),
				})
			},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, resp *httptest.ResponseRecorder) {
				t.Helper()

				assert.Equal(t, "application/x-yaml", resp.Header().Get("Content-Type"))
				assert.Contains(t, resp.Header().Get("Content-Disposition"), "cstrike.gameap.yaml")

				export, err := domain.ParseGameExport(resp.Body.Bytes())
				require.NoError(t, err)

				assert.Equal(t, domain.CurrentSchemaVersion, export.SchemaVersion)
				assert.Equal(t, "cstrike", export.Game.Code)
				assert.Equal(t, "Counter-Strike 1.6", export.Game.Name)
				assert.Equal(t, "GoldSource", export.Game.Engine)
				require.Len(t, export.Mods, 1)
				assert.Equal(t, "Classic", export.Mods[0].Name)
			},
		},
		{
			name:     "successful_export_game_without_mods",
			gameCode: "test",
			setupGame: func(repo *inmemory.GameRepository) {
				_ = repo.Save(context.Background(), &domain.Game{
					Code:    "test",
					Name:    "Test Game",
					Engine:  "Test Engine",
					Enabled: 1,
				})
			},
			setupGameMod:   func(_ *inmemory.GameModRepository) {},
			expectedStatus: http.StatusOK,
			validate: func(t *testing.T, resp *httptest.ResponseRecorder) {
				t.Helper()

				export, err := domain.ParseGameExport(resp.Body.Bytes())
				require.NoError(t, err)

				assert.Equal(t, "test", export.Game.Code)
				require.Len(t, export.Mods, 0)
			},
		},
		{
			name:           "game_not_found",
			gameCode:       "notexist",
			setupGame:      func(_ *inmemory.GameRepository) {},
			setupGameMod:   func(_ *inmemory.GameModRepository) {},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "empty_game_code",
			gameCode:       "",
			setupGame:      func(_ *inmemory.GameRepository) {},
			setupGameMod:   func(_ *inmemory.GameModRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			responder := api.NewResponder()

			tt.setupGame(gameRepo)
			tt.setupGameMod(gameModRepo)

			exporter := gameexporter.NewExporter(gameRepo, gameModRepo, "v3.0.0")
			handler := NewHandler(exporter, responder)

			req := httptest.NewRequest(http.MethodGet, "/api/games/"+tt.gameCode+"/export", nil)
			req = mux.SetURLVars(req, map[string]string{"code": tt.gameCode})

			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.validate != nil && w.Code == http.StatusOK {
				tt.validate(t, w)
			}
		})
	}
}

func TestHandler_ContentDisposition(t *testing.T) {
	t.Parallel()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()

	_ = gameRepo.Save(context.Background(), &domain.Game{
		Code:    "my_game",
		Name:    "My Game",
		Engine:  "Test",
		Enabled: 1,
	})

	exporter := gameexporter.NewExporter(gameRepo, gameModRepo, "v3.0.0")
	handler := NewHandler(exporter, responder)

	req := httptest.NewRequest(http.MethodGet, "/api/games/my_game/export", nil)
	req = mux.SetURLVars(req, map[string]string{"code": "my_game"})

	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `attachment; filename="my_game.gameap.yaml"`, w.Header().Get("Content-Disposition"))
}
