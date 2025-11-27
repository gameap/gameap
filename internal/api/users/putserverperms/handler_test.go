package putserverperms

import (
	"bytes"
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
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
}

var testAdminUser = domain.User{
	ID:    2,
	Login: "adminuser",
	Email: "admin@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name              string
		userID            string
		serverID          string
		requestBody       any
		setupAuth         func() context.Context
		setupRepos        func(*inmemory.UserRepository, *inmemory.ServerRepository, *inmemory.RBACRepository)
		expectedStatus    int
		wantError         string
		expectPermissions bool
		verifyPermissions func(*testing.T, []PermissionResponse)
	}{
		{
			name:     "successful permission update - enable start permission",
			userID:   "1",
			serverID: "1",
			requestBody: UpdatePermissionsInput{
				{Permission: "game-server-start", Value: flexible.Bool(true)},
				{Permission: "game-server-stop", Value: flexible.Bool(false)},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				userRepo *inmemory.UserRepository,
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				require.NoError(t, userRepo.Save(context.Background(), &testUser))

				server := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "11111111",
					Enabled:    true,
					Name:       "Test Server",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
			},
			expectedStatus:    http.StatusOK,
			expectPermissions: true,
			verifyPermissions: func(t *testing.T, permissions []PermissionResponse) {
				t.Helper()

				startPerm := findPermission(permissions, "game-server-start")
				require.NotNil(t, startPerm)
				assert.True(t, startPerm.Value, "start permission should be enabled")

				stopPerm := findPermission(permissions, "game-server-stop")
				require.NotNil(t, stopPerm)
				assert.False(t, stopPerm.Value, "stop permission should be disabled")
			},
		},
		{
			name:     "successful permission update - multiple permissions",
			userID:   "1",
			serverID: "1",
			requestBody: UpdatePermissionsInput{
				{Permission: "game-server-start", Value: flexible.Bool(true)},
				{Permission: "game-server-stop", Value: flexible.Bool(true)},
				{Permission: "game-server-restart", Value: flexible.Bool(true)},
				{Permission: "game-server-files", Value: flexible.Bool(true)},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				userRepo *inmemory.UserRepository,
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				require.NoError(t, userRepo.Save(context.Background(), &testUser))

				server := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "11111111",
					Enabled:    true,
					Name:       "Test Server",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
			},
			expectedStatus:    http.StatusOK,
			expectPermissions: true,
			verifyPermissions: func(t *testing.T, permissions []PermissionResponse) {
				t.Helper()

				expectedEnabled := map[string]bool{
					"game-server-start":   true,
					"game-server-stop":    true,
					"game-server-restart": true,
					"game-server-files":   true,
				}

				for permName, shouldBeEnabled := range expectedEnabled {
					perm := findPermission(permissions, permName)
					require.NotNil(t, perm, "permission %s not found", permName)
					assert.Equal(t, shouldBeEnabled, perm.Value, "permission %s value mismatch", permName)
				}
			},
		},
		{
			name:     "user not authenticated",
			userID:   "1",
			serverID: "1",
			requestBody: UpdatePermissionsInput{
				{Permission: "game-server-start", Value: flexible.Bool(true)},
			},
			setupAuth: context.Background,
			setupRepos: func(
				_ *inmemory.UserRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:     "invalid user id",
			userID:   "invalid",
			serverID: "1",
			requestBody: UpdatePermissionsInput{
				{Permission: "game-server-start", Value: flexible.Bool(true)},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				_ *inmemory.UserRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid user id",
		},
		{
			name:     "invalid server id",
			userID:   "1",
			serverID: "invalid",
			requestBody: UpdatePermissionsInput{
				{Permission: "game-server-start", Value: flexible.Bool(true)},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				_ *inmemory.UserRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
		},
		{
			name:        "invalid request body",
			userID:      "1",
			serverID:    "1",
			requestBody: "invalid json",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				_ *inmemory.UserRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request body",
		},
		{
			name:        "empty permissions array",
			userID:      "1",
			serverID:    "1",
			requestBody: UpdatePermissionsInput{},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				_ *inmemory.UserRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "permissions array cannot be empty",
		},
		{
			name:     "invalid permission name",
			userID:   "1",
			serverID: "1",
			requestBody: UpdatePermissionsInput{
				{Permission: "invalid-permission", Value: flexible.Bool(true)},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				_ *inmemory.UserRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid permission name",
		},
		{
			name:     "user not found",
			userID:   "999",
			serverID: "1",
			requestBody: UpdatePermissionsInput{
				{Permission: "game-server-start", Value: flexible.Bool(true)},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				_ *inmemory.UserRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "user not found",
		},
		{
			name:     "server not found",
			userID:   "1",
			serverID: "999",
			requestBody: UpdatePermissionsInput{
				{Permission: "game-server-start", Value: flexible.Bool(true)},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				userRepo *inmemory.UserRepository,
				_ *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
				require.NoError(t, userRepo.Save(context.Background(), &testUser))
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
		},
		{
			name:     "admin user - permissions always true",
			userID:   "2",
			serverID: "1",
			requestBody: UpdatePermissionsInput{
				{Permission: "game-server-start", Value: flexible.Bool(false)},
			},
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "adminuser",
					Email: "admin@example.com",
					User:  &testAdminUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				userRepo *inmemory.UserRepository,
				serverRepo *inmemory.ServerRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				require.NoError(t, userRepo.Save(context.Background(), &testAdminUser))

				server := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "11111111",
					Enabled:    true,
					Name:       "Test Server",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				adminAbility := &domain.Ability{
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))

				entityTypeUser := domain.EntityTypeUser
				permission := &domain.Permission{
					AbilityID:  adminAbility.ID,
					EntityID:   lo.ToPtr(uint(2)),
					EntityType: &entityTypeUser,
					Forbidden:  false,
				}
				require.NoError(t, rbacRepo.SavePermission(context.Background(), permission))
			},
			expectedStatus:    http.StatusOK,
			expectPermissions: true,
			verifyPermissions: func(t *testing.T, permissions []PermissionResponse) {
				t.Helper()

				for _, perm := range permissions {
					assert.True(t, perm.Value, "admin should have all permissions set to true")
				}
			},
		},
		{
			name:        "flexible_bool_parsing_with_string_values",
			userID:      "1",
			serverID:    "1",
			requestBody: `[{"permission": "game-server-start", "value": "true"}, {"permission": "game-server-stop", "value": "1"}, {"permission": "game-server-restart", "value": "yes"}]`,
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				userRepo *inmemory.UserRepository,
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				require.NoError(t, userRepo.Save(context.Background(), &testUser))

				server := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "11111111",
					Enabled:    true,
					Name:       "Test Server",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
			},
			expectedStatus:    http.StatusOK,
			expectPermissions: true,
			verifyPermissions: func(t *testing.T, permissions []PermissionResponse) {
				t.Helper()

				startPerm := findPermission(permissions, "game-server-start")
				require.NotNil(t, startPerm)
				assert.True(t, startPerm.Value, "start permission should be enabled (parsed from string 'true')")

				stopPerm := findPermission(permissions, "game-server-stop")
				require.NotNil(t, stopPerm)
				assert.True(t, stopPerm.Value, "stop permission should be enabled (parsed from string '1')")

				restartPerm := findPermission(permissions, "game-server-restart")
				require.NotNil(t, restartPerm)
				assert.True(t, restartPerm.Value, "restart permission should be enabled (parsed from string 'yes')")
			},
		},
		{
			name:        "flexible_bool_parsing_with_integer_values",
			userID:      "1",
			serverID:    "1",
			requestBody: `[{"permission": "game-server-start", "value": 1}, {"permission": "game-server-stop", "value": 0}]`,
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				userRepo *inmemory.UserRepository,
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()

				require.NoError(t, userRepo.Save(context.Background(), &testUser))

				server := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "11111111",
					Enabled:    true,
					Name:       "Test Server",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
			},
			expectedStatus:    http.StatusOK,
			expectPermissions: true,
			verifyPermissions: func(t *testing.T, permissions []PermissionResponse) {
				t.Helper()

				startPerm := findPermission(permissions, "game-server-start")
				require.NotNil(t, startPerm)
				assert.True(t, startPerm.Value, "start permission should be enabled (parsed from integer 1)")

				stopPerm := findPermission(permissions, "game-server-stop")
				require.NotNil(t, stopPerm)
				assert.False(t, stopPerm.Value, "stop permission should be disabled (parsed from integer 0)")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRepo := inmemory.NewUserRepository()
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()

			handler := NewHandler(userRepo, serverRepo, rbacService, responder)

			if tt.setupRepos != nil {
				tt.setupRepos(userRepo, serverRepo, rbacRepo)
			}

			var body []byte
			var err error
			if strBody, ok := tt.requestBody.(string); ok {
				body = []byte(strBody)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(
				http.MethodPut,
				"/api/users/"+tt.userID+"/servers/"+tt.serverID+"/permissions",
				bytes.NewReader(body),
			)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{
				"id":     tt.userID,
				"server": tt.serverID,
			})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}

			if tt.expectPermissions {
				var permissions []PermissionResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &permissions))

				assert.Len(t, permissions, len(domain.ServersAbilities))

				for _, perm := range permissions {
					assert.NotEmpty(t, perm.Permission)
					assert.NotEmpty(t, perm.Name)
				}

				if tt.verifyPermissions != nil {
					tt.verifyPermissions(t, permissions)
				}
			}
		})
	}
}

func TestUpdatePermissionsInput_Validate(t *testing.T) {
	tests := []struct {
		name      string
		input     UpdatePermissionsInput
		wantError string
	}{
		{
			name: "valid input",
			input: UpdatePermissionsInput{
				{Permission: "game-server-start", Value: flexible.Bool(true)},
				{Permission: "game-server-stop", Value: flexible.Bool(false)},
			},
			wantError: "",
		},
		{
			name:      "empty array",
			input:     UpdatePermissionsInput{},
			wantError: "permissions array cannot be empty",
		},
		{
			name: "empty permission name",
			input: UpdatePermissionsInput{
				{Permission: "", Value: flexible.Bool(true)},
			},
			wantError: "permission at index 0 cannot be empty",
		},
		{
			name: "invalid permission name",
			input: UpdatePermissionsInput{
				{Permission: "invalid-permission", Value: flexible.Bool(true)},
			},
			wantError: "invalid permission name",
		},
		{
			name: "mixed valid and invalid permissions",
			input: UpdatePermissionsInput{
				{Permission: "game-server-start", Value: flexible.Bool(true)},
				{Permission: "invalid-permission", Value: flexible.Bool(false)},
			},
			wantError: "invalid permission name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpdatePermissionsInput_ToAbilities(t *testing.T) {
	input := UpdatePermissionsInput{
		{Permission: "game-server-start", Value: flexible.Bool(true)},
		{Permission: "game-server-stop", Value: flexible.Bool(false)},
		{Permission: "game-server-restart", Value: flexible.Bool(true)},
	}

	allow, revoke := input.ToAbilities()

	assert.Len(t, allow, 2, "should have 2 abilities to allow")
	assert.Len(t, revoke, 1, "should have 1 ability to revoke")

	allowNames := make(map[domain.AbilityName]bool)
	for _, ability := range allow {
		allowNames[ability] = true
	}

	assert.True(t, allowNames[domain.AbilityNameGameServerStart])
	assert.True(t, allowNames[domain.AbilityNameGameServerRestart])

	revokeNames := make(map[domain.AbilityName]bool)
	for _, ability := range revoke {
		revokeNames[ability] = true
	}

	assert.True(t, revokeNames[domain.AbilityNameGameServerStop])
}

func TestNewPermissionResponse(t *testing.T) {
	tests := []struct {
		name         string
		abilityName  domain.AbilityName
		value        bool
		wantPerm     string
		wantValue    bool
		wantDispName string
	}{
		{
			name:         "game server start",
			abilityName:  domain.AbilityNameGameServerStart,
			value:        true,
			wantPerm:     "game-server-start",
			wantValue:    true,
			wantDispName: "Start Game Server",
		},
		{
			name:         "game server stop",
			abilityName:  domain.AbilityNameGameServerStop,
			value:        false,
			wantPerm:     "game-server-stop",
			wantValue:    false,
			wantDispName: "Stop Game Server",
		},
		{
			name:         "rcon players",
			abilityName:  domain.AbilityNameGameServerRconPlayers,
			value:        true,
			wantPerm:     "game-server-rcon-players",
			wantValue:    true,
			wantDispName: "RCON players manage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := NewPermissionResponse(tt.abilityName, tt.value)

			assert.Equal(t, tt.wantPerm, resp.Permission)
			assert.Equal(t, tt.wantValue, resp.Value)
			assert.Equal(t, tt.wantDispName, resp.Name)
		})
	}
}

func findPermission(permissions []PermissionResponse, permission string) *PermissionResponse {
	for _, p := range permissions {
		if p.Permission == permission {
			return &p
		}
	}

	return nil
}
