package getrconfeatures

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The UDP protocols added alongside Quake, SA-MP and BattlEye can read a player list but have no
// kick or ban command the panel can build, so the features endpoint must advertise listing
// without moderation — otherwise the UI shows buttons that cannot work.
func TestHandler_ListOnlyProtocolsAdvertiseNoModeration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		game   *domain.Game
		server *domain.Server
	}{
		{
			name: "quake3",
			game: &domain.Game{Code: "q3", Name: "Quake 3", Engine: "q3", EngineVersion: "3"},
		},
		{
			name: "call_of_duty",
			game: &domain.Game{Code: "cod4", Name: "Call of Duty 4", Engine: "cod4", EngineVersion: "4"},
		},
		{
			name: "samp",
			game: &domain.Game{Code: "samp", Name: "SA-MP", Engine: "samp", EngineVersion: "1.0"},
		},
		{
			name: "arma3",
			game: &domain.Game{Code: "arma3", Name: "Arma 3", Engine: "arma3", EngineVersion: "3"},
		},
		{
			name: "legacy_idtech_snapshot",
			game: &domain.Game{Code: "q2", Name: "Quake 2", Engine: "idtech", EngineVersion: "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			serverRepo := inmemory.NewServerRepository()
			gameRepo := inmemory.NewGameRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			handler := NewHandler(serverRepo, gameRepo, testResolver(), rbacService, api.NewResponder())

			now := time.Now()
			server := &domain.Server{
				ID:         1,
				UID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				UUIDShort:  "short1",
				Enabled:    true,
				Installed:  1,
				Name:       "Test Server",
				GameID:     tt.game.Code,
				DSID:       1,
				GameModID:  1,
				ServerIP:   "127.0.0.1",
				ServerPort: 27015,
				CreatedAt:  &now,
				UpdatedAt:  &now,
			}

			require.NoError(t, gameRepo.Save(context.Background(), tt.game))
			require.NoError(t, serverRepo.Save(context.Background(), server))
			serverRepo.AddUserServer(1, 1)
			allowUserAbilityForServer(t, rbacRepo, testUser1.ID, 1, domain.AbilityNameGameServerRconPlayers)

			session := &auth.Session{Login: "testuser", Email: "test@example.com", User: &testUser1}
			req := httptest.NewRequest(http.MethodGet, "/api/servers/1/rcon/features", nil)
			req = req.WithContext(auth.ContextWithSession(context.Background(), session))
			req = mux.SetURLVars(req, map[string]string{"server": "1"})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var features featuresResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &features))

			assert.True(t, features.Rcon, "%s must have rcon", tt.name)
			assert.True(t, features.PlayersList, "%s must be able to list players", tt.name)
			assert.True(t, features.PlayersManage, "the legacy alias must follow playersList")
			assert.False(t, features.PlayersKick, "%s has no kick command", tt.name)
			assert.False(t, features.PlayersBan, "%s has no ban command", tt.name)
		})
	}
}
