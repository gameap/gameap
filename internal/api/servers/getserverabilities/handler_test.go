package getserverabilities

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

var testUser1 = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
}

var testUser2 = domain.User{
	ID:    2,
	Login: "admin",
	Email: "admin@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name            string
		serverID        string
		setupAuth       func() context.Context
		setupRepo       func(*inmemory.ServerRepository, *inmemory.RBACRepository)
		expectedStatus  int
		wantError       string
		expectAbilities bool
	}{
		{
			name:     "successful abilities retrieval for regular user",
			serverID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()

				server := &domain.Server{
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

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				startAbility := domain.CreateAbilityForEntity(
					domain.AbilityNameGameServerStart,
					server.ID,
					domain.EntityTypeServer,
				)
				require.NoError(t, rbacRepo.Allow(
					context.Background(),
					testUser1.ID,
					domain.EntityTypeUser,
					[]domain.Ability{startAbility},
				))
			},
			expectedStatus:  http.StatusOK,
			expectAbilities: true,
		},
		{
			name:     "admin has all abilities",
			serverID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()

				server := &domain.Server{
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

				require.NoError(t, serverRepo.Save(context.Background(), server))

				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			expectedStatus:  http.StatusOK,
			expectAbilities: true,
		},
		{
			name:     "server not found",
			serverID: "999",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:       func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus:  http.StatusNotFound,
			wantError:       "server not found",
			expectAbilities: false,
		},
		{
			name:     "user not authenticated",
			serverID: "1",
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepo:       func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus:  http.StatusUnauthorized,
			wantError:       "user not authenticated",
			expectAbilities: false,
		},
		{
			name:     "invalid server id",
			serverID: "invalid",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:       func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus:  http.StatusBadRequest,
			wantError:       "invalid server id",
			expectAbilities: false,
		},
		{
			name:     "user does not have access to server",
			serverID: "2",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now()

				server := &domain.Server{
					ID:            2,
					UUID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:     "short2",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Other User Server",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27016,
					Dir:           "/home/gameap/servers/test2",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(3, 2)
			},
			expectedStatus:  http.StatusNotFound,
			wantError:       "server not found",
			expectAbilities: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, rbacService, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, rbacRepo)
			}

			ctx := tt.setupAuth()
			req := httptest.NewRequest(http.MethodGet, "/api/servers/"+tt.serverID+"/abilities", nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"server": tt.serverID})
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

			if tt.expectAbilities {
				var abilities abilitiesResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &abilities))
			}
		})
	}
}

func TestHandler_AdminHasAllAbilities(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, rbacService, responder)

	now := time.Now()

	server := &domain.Server{
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

	require.NoError(t, serverRepo.Save(context.Background(), server))

	adminAbility := &domain.Ability{
		ID:   1,
		Name: domain.AbilityNameAdminRolesPermissions,
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
	require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  &testUser2,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/servers/1/abilities", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var abilities abilitiesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &abilities))

	assert.True(t, abilities.GameServerCommon)
	assert.True(t, abilities.GameServerStart)
	assert.True(t, abilities.GameServerStop)
	assert.True(t, abilities.GameServerRestart)
	assert.True(t, abilities.GameServerPause)
	assert.True(t, abilities.GameServerUpdate)
	assert.True(t, abilities.GameServerFiles)
	assert.True(t, abilities.GameServerTasks)
	assert.True(t, abilities.GameServerSettings)
	assert.True(t, abilities.GameServerConsoleView)
	assert.True(t, abilities.GameServerConsoleSend)
	assert.True(t, abilities.GameServerRconConsole)
	assert.True(t, abilities.GameServerRconPlayers)
}

func TestHandler_RegularUserHasLimitedAbilities(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, rbacService, responder)

	now := time.Now()

	server := &domain.Server{
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

	require.NoError(t, serverRepo.Save(context.Background(), server))
	serverRepo.AddUserServer(1, 1)

	startAbility := domain.CreateAbilityForEntity(
		domain.AbilityNameGameServerStart,
		server.ID,
		domain.EntityTypeServer,
	)
	stopAbility := domain.CreateAbilityForEntity(
		domain.AbilityNameGameServerStop,
		server.ID,
		domain.EntityTypeServer,
	)
	require.NoError(t, rbacRepo.Allow(
		context.Background(),
		testUser1.ID,
		domain.EntityTypeUser,
		[]domain.Ability{startAbility, stopAbility},
	))

	session := &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/servers/1/abilities", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var abilities abilitiesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &abilities))

	assert.False(t, abilities.GameServerCommon)
	assert.True(t, abilities.GameServerStart)
	assert.True(t, abilities.GameServerStop)
	assert.False(t, abilities.GameServerRestart)
	assert.False(t, abilities.GameServerPause)
	assert.False(t, abilities.GameServerUpdate)
	assert.False(t, abilities.GameServerFiles)
	assert.False(t, abilities.GameServerTasks)
	assert.False(t, abilities.GameServerSettings)
	assert.False(t, abilities.GameServerConsoleView)
	assert.False(t, abilities.GameServerConsoleSend)
	assert.False(t, abilities.GameServerRconConsole)
	assert.False(t, abilities.GameServerRconPlayers)
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, rbacService, responder)

	require.NotNil(t, handler)
	assert.Equal(t, rbacService, handler.rbac)
	assert.Equal(t, responder, handler.responder)
}

func TestNewAbilitiesResponse(t *testing.T) {
	abilities := map[domain.AbilityName]bool{
		domain.AbilityNameGameServerCommon:      true,
		domain.AbilityNameGameServerStart:       true,
		domain.AbilityNameGameServerStop:        false,
		domain.AbilityNameGameServerRestart:     false,
		domain.AbilityNameGameServerPause:       false,
		domain.AbilityNameGameServerUpdate:      false,
		domain.AbilityNameGameServerFiles:       false,
		domain.AbilityNameGameServerTasks:       false,
		domain.AbilityNameGameServerSettings:    false,
		domain.AbilityNameGameServerConsoleView: false,
		domain.AbilityNameGameServerConsoleSend: false,
		domain.AbilityNameGameServerRconConsole: false,
		domain.AbilityNameGameServerRconPlayers: false,
	}

	response := newAbilitiesResponse(abilities)

	assert.True(t, response.GameServerCommon)
	assert.True(t, response.GameServerStart)
	assert.False(t, response.GameServerStop)
	assert.False(t, response.GameServerRestart)
	assert.False(t, response.GameServerPause)
	assert.False(t, response.GameServerUpdate)
	assert.False(t, response.GameServerFiles)
	assert.False(t, response.GameServerTasks)
	assert.False(t, response.GameServerSettings)
	assert.False(t, response.GameServerConsoleView)
	assert.False(t, response.GameServerConsoleSend)
	assert.False(t, response.GameServerRconConsole)
	assert.False(t, response.GameServerRconPlayers)
}
