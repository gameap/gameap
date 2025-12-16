package getabilities

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
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.UserRepository, *inmemory.ServerRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		expectedCount  int
	}{
		{
			name: "successful abilities retrieval for regular user",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(userRepo *inmemory.UserRepository, serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				userName := "Test User"
				user := &domain.User{
					ID:        1,
					Login:     "testuser",
					Email:     "test@example.com",
					Name:      &userName,
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				require.NoError(t, userRepo.Save(context.Background(), user))

				// Create test servers
				server1 := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 1",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				server2 := &domain.Server{
					ID:            2,
					UUID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:     "short2",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server 2",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27016,
					Dir:           "/home/gameap/servers/test2",
					ProcessActive: true,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				require.NoError(t, serverRepo.Save(context.Background(), server2))

				// Associate servers with user
				serverRepo.AddUserServer(1, 1)
				serverRepo.AddUserServer(1, 2)

				// Set up some abilities for the user
				entityID := uint(1)
				ability := &domain.Ability{
					ID:         1,
					Name:       domain.AbilityNameGameServerCommon,
					EntityID:   &entityID,
					EntityType: lo.ToPtr(domain.EntityTypeServer),
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), ability))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), 1, 1))
			},
			expectedStatus: http.StatusOK,
			expectedCount:  2,
		},
		{
			name: "successful abilities retrieval for admin user",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(userRepo *inmemory.UserRepository, serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				userName := "Admin User"
				user := &domain.User{
					ID:        1,
					Login:     "admin",
					Email:     "admin@example.com",
					Name:      &userName,
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				require.NoError(t, userRepo.Save(context.Background(), user))

				// Create test server
				server := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Test Server",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/home/gameap/servers/test",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))

				// Set up admin abilities
				adminAbility := &domain.Ability{
					ID:        1,
					Name:      domain.AbilityNameAdminRolesPermissions,
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), 1, 1))
			},
			expectedStatus: http.StatusOK,
			expectedCount:  1,
		},
		{
			name:           "user not authenticated",
			setupRepo:      func(_ *inmemory.UserRepository, _ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name: "authenticated user not found in database",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "nonexistent",
					Email: "nonexistent@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.UserRepository, _ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "user not found",
		},
		{
			name: "user with no servers",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "usernoservers",
					Email: "noservers@example.com",
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(userRepo *inmemory.UserRepository, _ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now()
				user := &domain.User{
					ID:        2,
					Login:     "usernoservers",
					Email:     "noservers@example.com",
					CreatedAt: &now,
					UpdatedAt: &now,
				}
				require.NoError(t, userRepo.Save(context.Background(), user))
			},
			expectedStatus: http.StatusOK,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userRepo := inmemory.NewUserRepository()
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			handler := NewHandler(userRepo, serverRepo, rbacService, responder, nil)

			if tt.setupRepo != nil {
				tt.setupRepo(userRepo, serverRepo, rbacRepo)
			}

			ctx := context.Background()

			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/user/servers_abilities", nil)
			req = req.WithContext(ctx)
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
			} else if tt.expectedStatus == http.StatusOK {
				var response ServersAbilitiesResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Len(t, response, tt.expectedCount)

				// If we have servers, verify the structure
				if tt.expectedCount > 0 {
					for serverID, abilities := range response {
						assert.Greater(t, serverID, uint(0))
						assert.IsType(t, ServerAbilities{}, abilities)

						// Verify all server abilities are present
						for _, abilityName := range domain.ServersAbilities {
							_, exists := abilities[abilityName]
							assert.True(t, exists, "ability %s should exist for server %d", abilityName, serverID)
						}
					}
				}
			}
		})
	}
}

func TestHandler_AdminUserHasAllAbilities(t *testing.T) {
	userRepo := inmemory.NewUserRepository()
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()
	handler := NewHandler(userRepo, serverRepo, rbacService, responder, nil)

	now := time.Now()
	userName := "Admin User"
	user := &domain.User{
		ID:        1,
		Login:     "admin",
		Email:     "admin@example.com",
		Name:      &userName,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, userRepo.Save(context.Background(), user))

	// Create test server
	server := &domain.Server{
		ID:            1,
		UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UUIDShort:     "short1",
		Enabled:       true,
		Installed:     1,
		Blocked:       false,
		Name:          "Test Server",
		GameID:        "cs",
		DSID:          1,
		GameModID:     1,
		ServerIP:      "127.0.0.1",
		ServerPort:    27015,
		Dir:           "/home/gameap/servers/test",
		ProcessActive: false,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))

	// Set up admin abilities
	adminAbility := &domain.Ability{
		ID:        1,
		Name:      domain.AbilityNameAdminRolesPermissions,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
	require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), 1, 1))

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/user/servers_abilities", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response ServersAbilitiesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	require.Len(t, response, 1)
	serverAbilities := response[1]

	// Admin should have all abilities set to true
	for _, abilityName := range domain.ServersAbilities {
		hasAbility, exists := serverAbilities[abilityName]
		require.True(t, exists, "ability %s should exist", abilityName)
		assert.True(t, hasAbility, "admin should have ability %s", abilityName)
	}
}

func TestHandler_RegularUserAbilities(t *testing.T) {
	userRepo := inmemory.NewUserRepository()
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()
	handler := NewHandler(userRepo, serverRepo, rbacService, responder, nil)

	now := time.Now()
	userName := "Regular User"
	user := &domain.User{
		ID:        1,
		Login:     "user",
		Email:     "user@example.com",
		Name:      &userName,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, userRepo.Save(context.Background(), user))

	// Create test server
	server := &domain.Server{
		ID:            1,
		UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UUIDShort:     "short1",
		Enabled:       true,
		Installed:     1,
		Blocked:       false,
		Name:          "Test Server",
		GameID:        "cs",
		DSID:          1,
		GameModID:     1,
		ServerIP:      "127.0.0.1",
		ServerPort:    27015,
		Dir:           "/home/gameap/servers/test",
		ProcessActive: false,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))
	serverRepo.AddUserServer(1, 1)

	// Set up specific ability for this server
	entityID := uint(1)
	ability := &domain.Ability{
		ID:         1,
		Name:       domain.AbilityNameGameServerCommon,
		EntityID:   &entityID,
		EntityType: lo.ToPtr(domain.EntityTypeServer),
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), ability))
	require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), 1, 1))

	session := &auth.Session{
		Login: "user",
		Email: "user@example.com",
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/user/servers_abilities", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response ServersAbilitiesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

	require.Len(t, response, 1)
	serverAbilities := response[1]

	// User should have game-server-common ability
	hasCommon, exists := serverAbilities[domain.AbilityNameGameServerCommon]
	require.True(t, exists)
	assert.True(t, hasCommon)

	// User should not have other abilities
	hasStart, exists := serverAbilities[domain.AbilityNameGameServerStart]
	require.True(t, exists)
	assert.False(t, hasStart)
}

func TestNewHandler(t *testing.T) {
	userRepo := inmemory.NewUserRepository()
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := NewHandler(userRepo, serverRepo, rbacService, responder, nil)

	require.NotNil(t, handler)
	assert.Equal(t, userRepo, handler.userRepo)
	assert.Equal(t, serverRepo, handler.serverRepo)
	assert.Equal(t, rbacService, handler.rbac)
	assert.Equal(t, responder, handler.responder)
}

func TestNewServersAbilitiesResponse(t *testing.T) {
	abilities := map[uint]map[domain.AbilityName]bool{
		1: {
			"game-server-common": true,
			"game-server-start":  false,
		},
		2: {
			"game-server-common": false,
			"game-server-start":  true,
		},
	}

	response := NewServersAbilitiesResponse(abilities)

	require.Len(t, response, 2)

	assert.True(t, response[1]["game-server-common"])
	assert.False(t, response[1]["game-server-start"])

	assert.False(t, response[2]["game-server-common"])
	assert.True(t, response[2]["game-server-start"])
}
