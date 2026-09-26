package getserver

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

func TestHandler_Suspension(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		suspended      bool
		isAdmin        bool
		wantStatus     int
		wantSuspension map[string]any
	}{
		{
			name:       "admin_sees_the_suspension",
			suspended:  true,
			isAdmin:    true,
			wantStatus: http.StatusOK,
			wantSuspension: map[string]any{
				"since":  "2026-09-01T08:00:00Z",
				"reason": "unpaid",
			},
		},
		{
			name:           "admin_sees_null_for_a_server_that_is_not_suspended",
			isAdmin:        true,
			wantStatus:     http.StatusOK,
			wantSuspension: nil,
		},
		{
			name:           "owner_sees_null_for_a_server_that_is_not_suspended",
			wantStatus:     http.StatusOK,
			wantSuspension: nil,
		},
		{
			name:       "owner_is_refused_a_suspended_server",
			suspended:  true,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			handler := NewHandler(
				serverRepo,
				inmemory.NewGameRepository(),
				inmemory.NewGameModRepository(),
				inmemory.NewServerSettingRepository(),
				rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0),
				api.NewResponder(),
			)

			user := testUser1
			if tt.isAdmin {
				user = testUser2
				adminAbility := &domain.Ability{ID: 1, Name: domain.AbilityNameAdminRolesPermissions}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), user.ID, adminAbility.ID))
			}

			server := &domain.Server{
				ID:         1,
				UID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				Enabled:    true,
				Installed:  domain.ServerInstalledStatusInstalled,
				Name:       "Test Server",
				GameID:     "cs",
				DSID:       1,
				GameModID:  1,
				ServerIP:   "10.0.0.5",
				ServerPort: 27015,
				Metadata:   domain.Metadata{"docker_image": "gameap/debian"},
			}
			if tt.suspended {
				server.Suspend(since, new("unpaid"))
			}
			require.NoError(t, serverRepo.Save(context.Background(), server))
			serverRepo.AddUserServer(testUser1.ID, 1)

			req := httptest.NewRequest(http.MethodGet, "/api/servers/1", nil)
			req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{User: &user}))
			req = mux.SetURLVars(req, map[string]string{"id": "1"})
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, tt.wantStatus, w.Code, w.Body.String())

			var response map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			if tt.wantStatus != http.StatusOK {
				assert.Equal(t, "server is blocked", response["error"])

				return
			}

			require.Contains(t, response, "suspension")
			if tt.wantSuspension == nil {
				assert.Nil(t, response["suspension"])
			} else {
				assert.Equal(t, tt.wantSuspension, response["suspension"])
			}

			if tt.isAdmin {
				assert.Equal(t, map[string]any{"docker_image": "gameap/debian"}, response["metadata"],
					"the panel's own record is not part of the metadata")
			}
		})
	}
}
