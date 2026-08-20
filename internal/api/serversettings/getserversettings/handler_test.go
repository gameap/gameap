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
	t.Parallel()

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
					EntityID:   new(uint(1)),
				},
				{
					ID:         2,
					Name:       domain.AbilityNameGameServerSettings,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(1)),
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
				{
					ID:         2,
					AbilityID:  2,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
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
					EntityID:   new(uint(1)),
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
			},
			roles:          []domain.Role{},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "admin_user_bypasses_server_permissions",
			serverID: 1,
			userID:   1,
			gameMod: &domain.GameMod{
				ID:       1,
				GameCode: "valve",
				Name:     "Half-Life Deathmatch",
				Vars:     domain.GameModVarList{},
			},
			abilities: []domain.Ability{
				{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
			},
			roles:          []domain.Role{},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "user_with_only_common_permission",
			serverID: 1,
			userID:   1,
			abilities: []domain.Ability{
				{
					ID:         1,
					Name:       domain.AbilityNameGameServerCommon,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(1)),
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
			},
			roles:          []domain.Role{},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "user_with_only_settings_permission",
			serverID: 1,
			userID:   1,
			abilities: []domain.Ability{
				{
					ID:         1,
					Name:       domain.AbilityNameGameServerSettings,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(1)),
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
			},
			roles:          []domain.Role{},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "forbidden_permission_overrides_allowed",
			serverID: 1,
			userID:   1,
			abilities: []domain.Ability{
				{
					ID:         1,
					Name:       domain.AbilityNameGameServerCommon,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(1)),
				},
				{
					ID:         2,
					Name:       domain.AbilityNameGameServerSettings,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(1)),
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
				{
					ID:         2,
					AbilityID:  2,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
					Forbidden:  true,
				},
			},
			roles:          []domain.Role{},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "user_with_permissions_for_different_server",
			serverID: 1,
			userID:   1,
			abilities: []domain.Ability{
				{
					ID:         1,
					Name:       domain.AbilityNameGameServerCommon,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(2)),
				},
				{
					ID:         2,
					Name:       domain.AbilityNameGameServerSettings,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(2)),
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
				{
					ID:         2,
					AbilityID:  2,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
			},
			roles:          []domain.Role{},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "user_with_role_based_permissions",
			serverID: 1,
			userID:   1,
			gameMod: &domain.GameMod{
				ID:       1,
				GameCode: "valve",
				Name:     "Half-Life Deathmatch",
				Vars:     domain.GameModVarList{},
			},
			abilities: []domain.Ability{
				{
					ID:         1,
					Name:       domain.AbilityNameGameServerCommon,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(1)),
				},
				{
					ID:         2,
					Name:       domain.AbilityNameGameServerSettings,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(1)),
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeRole),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
				{
					ID:         2,
					AbilityID:  2,
					EntityType: lo.ToPtr(domain.EntityTypeRole),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
			},
			roles: []domain.Role{
				{
					ID:   1,
					Name: "server_admin",
				},
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "typed_vars_carry_their_definition_for_admins",
			serverID: 1,
			userID:   1,
			gameMod: &domain.GameMod{
				ID:       1,
				GameCode: "minecraft",
				Name:     "Default",
				Vars: domain.GameModVarList{
					{
						Var:         "version",
						Default:     "1.20.4",
						Info:        "Minecraft version",
						AdminVar:    true,
						Type:        domain.GameModVarTypeSelect,
						Description: "The version the server runs",
						AllowCustom: true,
						Options: domain.GameModVarOptions{
							{Value: "1.21"},
							{Value: "1.20.4", Label: "1.20.4 (LTS)"},
						},
						I18n: domain.GameModVarI18n{"ru": {Info: "Версия Minecraft"}},
					},
					{
						Var:     "maxplayers",
						Default: "20",
						Info:    "Max players",
						Type:    domain.GameModVarTypeInt,
						Rules:   &domain.GameModVarRules{Min: new(1.0), Max: new(64.0)},
					},
					{
						Var:        "pvp",
						Default:    "on",
						Info:       "PvP",
						Type:       domain.GameModVarTypeBool,
						TrueValue:  new("on"),
						FalseValue: new("off"),
					},
					{
						Var:  "rcon_password",
						Info: "RCON password",
						Type: domain.GameModVarTypePassword,
					},
				},
			},
			serverSettings: []domain.ServerSetting{
				{ID: 1, ServerID: 1, Name: "maxplayers", Value: domain.NewServerSettingValue("32")},
				{ID: 2, ServerID: 1, Name: "pvp", Value: domain.NewServerSettingValue("off")},
				{ID: 3, ServerID: 1, Name: "rcon_password", Value: domain.NewServerSettingValue("007")},
			},
			abilities: []domain.Ability{
				{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeUser),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
			},
			roles:          []domain.Role{},
			expectedStatus: http.StatusOK,
			expectedBody: `[
				{"name":"autostart","value":false,"default":false,"type":"bool","label":"Autostart"},
				{"name":"update_before_start","value":false,"default":false,"type":"bool","label":"Update before start"},
				{
					"name":"version","value":"1.20.4","default":"1.20.4","type":"select",
					"label":"Minecraft version","description":"The version the server runs",
					"allow_custom":true,"admin_var":true,
					"options":[
						{"value":"1.21","label":"1.21"},
						{"value":"1.20.4","label":"1.20.4 (LTS)"}
					],
					"i18n":{"ru":{"info":"Версия Minecraft"}}
				},
				{
					"name":"maxplayers","value":32,"default":20,"type":"int","label":"Max players",
					"rules":{"min":1,"max":64}
				},
				{"name":"pvp","value":false,"default":true,"type":"bool","label":"PvP"},
				{"name":"rcon_password","value":"007","default":"","type":"password","label":"RCON password"}
			]`,
		},
		{
			name:     "admin_var_is_hidden_from_regular_users",
			serverID: 1,
			userID:   1,
			gameMod: &domain.GameMod{
				ID:       1,
				GameCode: "minecraft",
				Name:     "Default",
				Vars: domain.GameModVarList{
					{
						Var:         "version",
						Default:     "1.20.4",
						Info:        "Minecraft version",
						AdminVar:    true,
						Type:        domain.GameModVarTypeSelect,
						Description: "The version the server runs",
						AllowCustom: true,
						Options: domain.GameModVarOptions{
							{Value: "1.21"},
							{Value: "1.20.4", Label: "1.20.4 (LTS)"},
						},
						I18n: domain.GameModVarI18n{"ru": {Info: "Версия Minecraft"}},
					},
					{
						Var:     "maxplayers",
						Default: "20",
						Info:    "Max players",
						Type:    domain.GameModVarTypeInt,
						Rules:   &domain.GameModVarRules{Min: new(1.0), Max: new(64.0)},
					},
					{
						Var:        "pvp",
						Default:    "on",
						Info:       "PvP",
						Type:       domain.GameModVarTypeBool,
						TrueValue:  new("on"),
						FalseValue: new("off"),
					},
					{
						Var:  "rcon_password",
						Info: "RCON password",
						Type: domain.GameModVarTypePassword,
					},
				},
			},
			serverSettings: []domain.ServerSetting{
				{ID: 1, ServerID: 1, Name: "maxplayers", Value: domain.NewServerSettingValue("32")},
				{ID: 2, ServerID: 1, Name: "pvp", Value: domain.NewServerSettingValue("off")},
				{ID: 3, ServerID: 1, Name: "rcon_password", Value: domain.NewServerSettingValue("007")},
			},
			abilities: []domain.Ability{
				{
					ID:         1,
					Name:       domain.AbilityNameGameServerCommon,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(1)),
				},
				{
					ID:         2,
					Name:       domain.AbilityNameGameServerSettings,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					EntityID:   new(uint(1)),
				},
			},
			permissions: []domain.Permission{
				{
					ID:         1,
					AbilityID:  1,
					EntityType: lo.ToPtr(domain.EntityTypeRole),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
				{
					ID:         2,
					AbilityID:  2,
					EntityType: lo.ToPtr(domain.EntityTypeRole),
					EntityID:   new(uint(1)),
					Forbidden:  false,
				},
			},
			roles: []domain.Role{
				{
					ID:   1,
					Name: "server_admin",
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody: `[
				{"name":"autostart","value":false,"default":false,"type":"bool","label":"Autostart"},
				{"name":"update_before_start","value":false,"default":false,"type":"bool","label":"Update before start"},
				{
					"name":"maxplayers","value":32,"default":20,"type":"int","label":"Max players",
					"rules":{"min":1,"max":64}
				},
				{"name":"pvp","value":false,"default":true,"type":"bool","label":"PvP"},
				{"name":"rcon_password","value":"007","default":"","type":"password","label":"RCON password"}
			]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

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

				if test.userID > 0 {
					err = rbacRepo.AssignRolesForEntity(
						ctx,
						test.userID,
						domain.EntityTypeUser,
						[]domain.RestrictedRole{domain.NewRestrictedRoleFromRole(role)},
					)
					require.NoError(t, err)
				}
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
