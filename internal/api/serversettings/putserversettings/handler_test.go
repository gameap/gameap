package putserversettings_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gameap/gameap/internal/api/serversettings/putserversettings"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
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

func TestPutServerSettings(t *testing.T) {
	tests := []struct {
		name             string
		serverID         uint
		userID           uint
		gameMod          *domain.GameMod
		existingSettings []domain.ServerSetting
		inputSettings    []map[string]string
		abilities        []domain.Ability
		permissions      []domain.Permission
		roles            []domain.Role
		expectedStatus   int
		verifySettings   bool
		wantFinalVals    map[string]string
	}{
		{
			name:     "success updating existing settings",
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
			existingSettings: []domain.ServerSetting{
				{
					ServerID: 1,
					Name:     "autostart",
					Value:    domain.NewServerSettingValue(false),
				},
				{
					ServerID: 1,
					Name:     "maxplayers",
					Value:    domain.NewServerSettingValue("24"),
				},
			},
			inputSettings: []map[string]string{
				{
					"name":  "autostart",
					"value": "true",
				},
				{
					"name":  "maxplayers",
					"value": "32",
				},
				{
					"name":  "hostname",
					"value": "Updated Server",
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
			verifySettings: true,
			wantFinalVals: map[string]string{
				"autostart":  "true",
				"maxplayers": "32",
				"hostname":   "Updated Server",
			},
		},
		{
			name:     "success with admin vars when user is admin",
			serverID: 1,
			userID:   1,
			gameMod: &domain.GameMod{
				ID:       1,
				GameCode: "valve",
				Name:     "Half-Life Deathmatch",
				Vars: domain.GameModVarList{
					{
						Var:      "rcon_password",
						Default:  "",
						Info:     "RCON Password",
						AdminVar: true,
					},
					{
						Var:     "maxplayers",
						Default: "32",
						Info:    "Maximum number of players",
					},
				},
			},
			existingSettings: []domain.ServerSetting{},
			inputSettings: []map[string]string{
				{
					"name":  "rcon_password",
					"value": "secret123",
				},
				{
					"name":  "maxplayers",
					"value": "16",
				},
			},
			abilities: []domain.Ability{
				{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				},
				{
					ID:         2,
					Name:       domain.AbilityNameGameServerCommon,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   lo.ToPtr(uint(1)),
				},
				{
					ID:         3,
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
				{
					ID:         3,
					AbilityID:  3,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   lo.ToPtr(uint(1)),
					Forbidden:  false,
				},
			},
			roles:          []domain.Role{},
			expectedStatus: http.StatusOK,
			verifySettings: true,
			wantFinalVals: map[string]string{
				"rcon_password": "secret123",
				"maxplayers":    "16",
			},
		},
		{
			name:     "skip admin vars when user is not admin",
			serverID: 1,
			userID:   1,
			gameMod: &domain.GameMod{
				ID:       1,
				GameCode: "valve",
				Name:     "Half-Life Deathmatch",
				Vars: domain.GameModVarList{
					{
						Var:      "rcon_password",
						Default:  "",
						Info:     "RCON Password",
						AdminVar: true,
					},
					{
						Var:     "maxplayers",
						Default: "32",
						Info:    "Maximum number of players",
					},
				},
			},
			existingSettings: []domain.ServerSetting{},
			inputSettings: []map[string]string{
				{
					"name":  "rcon_password",
					"value": "secret123",
				},
				{
					"name":  "maxplayers",
					"value": "16",
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
			verifySettings: true,
			wantFinalVals: map[string]string{
				"maxplayers": "16",
			},
		},
		{
			name:     "ignore unknown settings",
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
				},
			},
			existingSettings: []domain.ServerSetting{},
			inputSettings: []map[string]string{
				{
					"name":  "maxplayers",
					"value": "16",
				},
				{
					"name":  "unknown_var",
					"value": "should_be_ignored",
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
			verifySettings: true,
			wantFinalVals: map[string]string{
				"maxplayers": "16",
			},
		},
		{
			name:           "unauthorized when user not authenticated",
			serverID:       1,
			userID:         0,
			inputSettings:  []map[string]string{},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:     "forbidden when user has no server control ability",
			serverID: 1,
			userID:   1,
			inputSettings: []map[string]string{
				{
					"name":  "autostart",
					"value": "true",
				},
			},
			abilities:      []domain.Ability{},
			permissions:    []domain.Permission{},
			roles:          []domain.Role{},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "forbidden when user has control but no settings ability",
			serverID: 1,
			userID:   1,
			inputSettings: []map[string]string{
				{
					"name":  "autostart",
					"value": "true",
				},
			},
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
		{
			name:     "validation error when setting name is empty",
			serverID: 1,
			userID:   1,
			gameMod: &domain.GameMod{
				ID:       1,
				GameCode: "valve",
				Name:     "Half-Life Deathmatch",
				Vars:     domain.GameModVarList{},
			},
			existingSettings: []domain.ServerSetting{},
			inputSettings: []map[string]string{
				{
					"name":  "",
					"value": "some_value",
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
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()

			serverSettingsRepo := inmemory.NewServerSettingRepository()
			serversRepo := inmemory.NewServerRepository()
			gameModsRepo := inmemory.NewGameModRepository()
			rbacRepo := inmemory.NewRBACRepository()
			usersRepo := inmemory.NewUserRepository()

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

			if test.gameMod != nil {
				err := gameModsRepo.Save(ctx, test.gameMod)
				require.NoError(t, err)
			}

			for _, setting := range test.existingSettings {
				err := serverSettingsRepo.Save(ctx, &setting)
				require.NoError(t, err)
			}

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

			h := putserversettings.NewHandler(
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

			body, err := json.Marshal(test.inputSettings)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/servers/%s/settings", serverIDStr), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			if user != nil {
				req = req.WithContext(auth.ContextWithSession(ctx, &auth.Session{
					Login: user.Login,
					Email: user.Email,
					User:  user,
				}))
			}

			req = mux.SetURLVars(req, map[string]string{"server": serverIDStr})

			recorder := httptest.NewRecorder()

			h.ServeHTTP(recorder, req)

			if !assert.Equal(t, test.expectedStatus, recorder.Code) {
				t.Logf("Response body: %s", recorder.Body.String())
			}

			if recorder.Code != http.StatusOK && test.expectedStatus == http.StatusOK {
				t.Logf("Expected OK but got %d. Response: %s", recorder.Code, recorder.Body.String())
			}

			if test.verifySettings && test.expectedStatus == http.StatusOK {
				savedSettings, err := serverSettingsRepo.Find(ctx, &filters.FindServerSetting{
					ServerIDs: []uint{test.serverID},
				}, nil, nil)
				require.NoError(t, err)

				savedSettingsMap := make(map[string]any)
				for _, setting := range savedSettings {
					savedSettingsMap[setting.Name] = setting.Value
				}

				for name, expectedValue := range test.wantFinalVals {
					actualValue, exists := savedSettingsMap[name]
					require.True(t, exists, "setting %s should exist", name)

					actualValueStr, ok := actualValue.(domain.ServerSettingValue)
					require.True(t, ok, "setting %s has wrong type", name)

					assert.Equal(t, expectedValue, actualValueStr.Any(), "setting %s value mismatch", name)
				}
			}
		})
	}
}
