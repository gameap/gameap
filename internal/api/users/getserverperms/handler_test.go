package getserverperms

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
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
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
		name       string
		userID     string
		serverID   string
		setupAuth  func() context.Context
		setupRepos func(
			*inmemory.UserRepository,
			*inmemory.ServerRepository,
			*inmemory.RBACRepository,
		)
		expectedStatus     int
		wantError          string
		expectPermissions  bool
		expectedPermission *string
	}{
		{
			name:     "successful permissions retrieval for regular user",
			userID:   "1",
			serverID: "1",
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
				rbacRepo *inmemory.RBACRepository,
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

				serverEntityType := domain.EntityTypeServer
				ability := &domain.Ability{
					Name:       domain.AbilityNameGameServerStart,
					EntityType: &serverEntityType,
					EntityID:   lo.ToPtr(uint(1)),
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), ability))

				entityTypeUser := domain.EntityTypeUser
				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   lo.ToPtr(uint(1)),
					EntityType: &entityTypeUser,
					Forbidden:  false,
				}
				require.NoError(t, rbacRepo.SavePermission(context.Background(), permission))
			},
			expectedStatus:     http.StatusOK,
			expectPermissions:  true,
			expectedPermission: lo.ToPtr("game-server-start"),
		},
		{
			name:     "successful permissions retrieval for admin user",
			userID:   "2",
			serverID: "1",
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
		},
		{
			name:     "user not authenticated",
			userID:   "1",
			serverID: "1",
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
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
			name:     "user not found",
			userID:   "999",
			serverID: "1",
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
			name:     "user with no permissions",
			userID:   "1",
			serverID: "1",
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRepo := inmemory.NewUserRepository()
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(nil, rbacRepo, 0)
			responder := api.NewResponder()

			handler := NewHandler(userRepo, serverRepo, rbacService, responder)

			if tt.setupRepos != nil {
				tt.setupRepos(userRepo, serverRepo, rbacRepo)
			}

			ctx := tt.setupAuth()
			req := httptest.NewRequest(
				http.MethodGet,
				"/api/users/"+tt.userID+"/servers/"+tt.serverID+"/permissions",
				nil,
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

			//nolint:nestif
			if tt.expectPermissions {
				var permissions []PermissionResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &permissions))

				assert.Len(t, permissions, len(domain.ServersAbilities))

				for _, perm := range permissions {
					assert.NotEmpty(t, perm.Permission)
					assert.NotEmpty(t, perm.Name)
				}

				if tt.expectedPermission != nil {
					found := false
					for _, perm := range permissions {
						if perm.Permission == *tt.expectedPermission {
							found = true
							if tt.name == "successful permissions retrieval for regular user" {
								assert.True(t, perm.Value)
							}

							break
						}
					}
					assert.True(t, found, "expected permission %s not found", *tt.expectedPermission)
				}

				if tt.name == "successful permissions retrieval for admin user" {
					for _, perm := range permissions {
						assert.True(t, perm.Value, "admin should have all permissions")
					}
				}

				if tt.name == "user with no permissions" {
					for _, perm := range permissions {
						assert.False(t, perm.Value, "user with no permissions should have all false")
					}
				}
			}
		})
	}
}

func TestHandler_PermissionResponseStructure(t *testing.T) {
	userRepo := inmemory.NewUserRepository()
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(nil, rbacRepo, 0)
	responder := api.NewResponder()

	handler := NewHandler(userRepo, serverRepo, rbacService, responder)

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

	serverEntityType := domain.EntityTypeServer
	startAbility := &domain.Ability{
		Name:       domain.AbilityNameGameServerStart,
		EntityType: &serverEntityType,
		EntityID:   lo.ToPtr(uint(1)),
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), startAbility))

	stopAbility := &domain.Ability{
		Name:       domain.AbilityNameGameServerStop,
		EntityType: &serverEntityType,
		EntityID:   lo.ToPtr(uint(1)),
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), stopAbility))

	entityTypeUser := domain.EntityTypeUser
	startPermission := &domain.Permission{
		AbilityID:  startAbility.ID,
		EntityID:   lo.ToPtr(uint(1)),
		EntityType: &entityTypeUser,
		Forbidden:  false,
	}
	require.NoError(t, rbacRepo.SavePermission(context.Background(), startPermission))

	session := &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  &testUser,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/users/1/servers/1/permissions", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1", "server": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var permissions []PermissionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &permissions))

	require.Len(t, permissions, len(domain.ServersAbilities))

	startPerm := findPermission(permissions, "game-server-start")
	require.NotNil(t, startPerm)
	assert.Equal(t, "game-server-start", startPerm.Permission)
	assert.True(t, startPerm.Value)
	assert.Equal(t, "Start Game Server", startPerm.Name)

	stopPerm := findPermission(permissions, "game-server-stop")
	require.NotNil(t, stopPerm)
	assert.Equal(t, "game-server-stop", stopPerm.Permission)
	assert.False(t, stopPerm.Value)
	assert.Equal(t, "Stop Game Server", stopPerm.Name)
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
