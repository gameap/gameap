package postcommand

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP_SuspendedServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		command    string
		asAdmin    bool
		wantStatus int
		wantError  string
		wantTaskID bool
	}{
		{name: "admin_start_is_refused", command: "start", asAdmin: true, wantStatus: http.StatusForbidden, wantError: "server is blocked"},
		{name: "admin_restart_is_refused", command: "restart", asAdmin: true, wantStatus: http.StatusForbidden, wantError: "server is blocked"},
		{name: "admin_update_is_refused", command: "update", asAdmin: true, wantStatus: http.StatusForbidden, wantError: "server is blocked"},
		{name: "admin_install_is_refused", command: "install", asAdmin: true, wantStatus: http.StatusForbidden, wantError: "server is blocked"},
		{name: "admin_reinstall_is_refused", command: "reinstall", asAdmin: true, wantStatus: http.StatusForbidden, wantError: "server is blocked"},
		{name: "admin_stop_is_allowed", command: "stop", asAdmin: true, wantStatus: http.StatusOK, wantTaskID: true},
		{name: "owner_stop_is_refused", command: "stop", asAdmin: false, wantStatus: http.StatusForbidden, wantError: "server is blocked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			daemonTaskRepo := inmemory.NewDaemonTaskRepository()

			server := &domain.Server{
				ID:           1,
				UID:          uuid.New(),
				Enabled:      true,
				Installed:    domain.ServerInstalledStatusInstalled,
				Blocked:      true,
				Name:         "Suspended Server",
				GameID:       "cstrike",
				DSID:         1,
				GameModID:    1,
				ServerIP:     "192.168.1.1",
				ServerPort:   27015,
				StartCommand: new(testStartCommand),
			}
			require.NoError(t, serverRepo.Save(context.Background(), server))
			serverRepo.AddUserServer(testUser1.ID, server.ID)

			for _, ability := range []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStart,
				domain.AbilityNameGameServerStop,
				domain.AbilityNameGameServerRestart,
				domain.AbilityNameGameServerUpdate,
			} {
				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, server.ID, ability)
			}

			user := &testUser1
			if tt.asAdmin {
				user = &testUser2
				adminAbility := &domain.Ability{ID: 100, Name: domain.AbilityNameAdminRolesPermissions}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			}

			handler := NewHandler(
				serverRepo,
				servercontrol.NewService(
					daemonTaskRepo,
					inmemory.NewServerSettingRepository(),
					services.NewNilTransactionManager(),
				),
				rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0),
				api.NewResponder(),
			)

			req := httptest.NewRequest(http.MethodPost, "/api/servers/1/"+tt.command, nil)
			req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{User: user}))
			req = mux.SetURLVars(req, map[string]string{"server": "1"})
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, tt.wantStatus, w.Code)

			tasks, err := daemonTaskRepo.FindAll(context.Background(), nil, nil)
			require.NoError(t, err)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.wantError, response["error"], "the refusal must not be wrapped")
				assert.Empty(t, tasks)
			}

			if tt.wantTaskID {
				var response commandResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotZero(t, response.DaemonTaskID)
				require.Len(t, tasks, 1)
				assert.Equal(t, domain.DaemonTaskTypeServerStop, tasks[0].Task)
			}
		})
	}
}
