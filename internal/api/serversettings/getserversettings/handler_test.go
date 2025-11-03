package getserversettings_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gameap/gameap/internal/api/serversettings/getserversettings"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetServerSettings(t *testing.T) {
	tests := []struct {
		name           string
		serverID       uint
		userID         uint
		gameMod        *domain.GameMod
		serverSettings []domain.ServerSetting
		abilities      []domain.Ability
		permissions    []domain.Permission
		roles          []domain.Role
		expectedStatus int
		expectedBody   string
	}{
		{
			name:     "success with server settings and game mod vars",
			serverID: 1,
			userID:   1,
			gameMod: &domain.GameMod{
				ID:       1,
				GameCode: "valve",
				Name:     "Half-Life Deathmatch",
				Vars: domain.GameModVarList{
					{
						Var:     "maxplayers",
						Default: "32",
						Info:    "Maximum number of players",
					},
					{
						Var:     "hostname",
						Default: "My Server",
						Info:    "Server hostname",
					},
				},
			},
			serverSettings: []domain.ServerSetting{
				{
					ID:       1,
					ServerID: 1,
					Name:     "autostart",
					Value:    domain.NewServerSettingValue(true),
				},
				{
					ID:       2,
					ServerID: 1,
					Name:     "maxplayers",
					Value:    domain.NewServerSettingValue("24"),
				},
			},
			abilities: []domain.Ability{
				{
					ID:         1,
					Name:       domain.AbilityNameGameServerCommon,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   lo.ToPtr(uint(1)),
				},
				{
					ID:         2,
					Name:       domain.AbilityNameGameServerSettings,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   lo.ToPtr(uint(1)),
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   lo.ToPtr(uint(1)),
					Forbidden:  false,
				},
				{
					ID:         2,
					AbilityID:  2,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   lo.ToPtr(uint(1)),
					Forbidden:  false,
				},
			},
			roles:          []domain.Role{},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "unauthorized when user not authenticated",
			serverID:       1,
			userID:         0,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "forbidden when user has no abilities",
			serverID:       999,
			userID:         1,
			abilities:      []domain.Ability{},
			permissions:    []domain.Permission{},
			roles:          []domain.Role{},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "forbidden when user lacks permissions",
			serverID: 1,
			userID:   1,
			abilities: []domain.Ability{
				{
					ID:         1,
					Name:       domain.AbilityNameGameServerCommon,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   lo.ToPtr(uint(1)),
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   lo.ToPtr(uint(1)),
					Forbidden:  false,
				},
			},
			roles:          []domain.Role{},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// ARRANGE
			ctx := context.Background()

			serverSettingsRepo := inmemory.NewServerSettingRepository()
			serversRepo := inmemory.NewServerRepository()
			gameModsRepo := inmemory.NewGameModRepository()
			rbacRepo := inmemory.NewRBACRepository()
			usersRepo := inmemory.NewUserRepository()

			// Create user if userID is set
			var user *domain.User
			if test.userID > 0 {
				user = &domain.User{
					ID:    test.userID,
					Login: "testuser",
					Email: "test@example.com",
				}
				err := usersRepo.Save(ctx, user)
				require.NoError(t, err)
			}

			// Create server
			if test.serverID > 0 {
				server := &domain.Server{
					ID:        test.serverID,
					Name:      "Test Server",
					GameModID: 1,
				}
				err := serversRepo.Save(ctx, server)
				require.NoError(t, err)

				if test.userID > 0 {
					serversRepo.AddUserServer(test.userID, test.serverID)
				}
			}

			// Create game mod
			if test.gameMod != nil {
				err := gameModsRepo.Save(ctx, test.gameMod)
				require.NoError(t, err)
			}

			// Create server settings
			for _, setting := range test.serverSettings {
				err := serverSettingsRepo.Save(ctx, &setting)
				require.NoError(t, err)
			}

			// Setup RBAC
			for _, ability := range test.abilities {
				err := rbacRepo.SaveAbility(ctx, &ability)
				require.NoError(t, err)
			}

			for _, permission := range test.permissions {
				if permission.Ability == nil {
					for _, ability := range test.abilities {
						if ability.ID == permission.AbilityID {
							permission.Ability = &ability

							break
						}
					}
				}
				err := rbacRepo.SavePermission(ctx, &permission)
				require.NoError(t, err)
			}

			for _, role := range test.roles {
				err := rbacRepo.SaveRole(ctx, &role)
				require.NoError(t, err)
			}

			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)

			h := getserversettings.NewHandler(
				serverSettingsRepo,
				serversRepo,
				gameModsRepo,
				rbacService,
				api.NewResponder(),
			)

			serverIDStr := "999"
			if test.serverID > 0 {
				serverIDStr = strconv.FormatUint(uint64(test.serverID), 10)
			}

			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/servers/%s/settings", serverIDStr), nil)

			if user != nil {
				req = req.WithContext(auth.ContextWithSession(ctx, &auth.Session{
					Login: user.Login,
					Email: user.Email,
					User:  user,
				}))
			}

			req = mux.SetURLVars(req, map[string]string{"server": serverIDStr})

			recorder := httptest.NewRecorder()

			// ACT
			h.ServeHTTP(recorder, req)

			// ASSERT
			if !assert.Equal(t, test.expectedStatus, recorder.Code) {
				t.Logf("Response body: %s", recorder.Body.String())
			}

			if test.expectedBody != "" {
				assert.JSONEq(t, test.expectedBody, recorder.Body.String())
			}
		})
	}
}
