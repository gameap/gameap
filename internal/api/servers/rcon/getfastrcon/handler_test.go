package getfastrcon

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
		name           string
		serverID       string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.ServerRepository, *inmemory.GameModRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		expectFastRcon bool
	}{
		{
			name:     "successful fast rcon retrieval",
			serverID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				gameModRepo *inmemory.GameModRepository,
				_ *inmemory.RBACRepository,
			) {
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

				gameMod := &domain.GameMod{
					ID:       1,
					GameCode: "cs",
					Name:     "Counter-Strike 1.6",
					FastRcon: domain.GameModFastRconList{
						{Info: "Status", Command: "status"},
						{Info: "Restart Map", Command: "sv_restart 1"},
					},
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				require.NoError(t, gameModRepo.Save(context.Background(), gameMod))
			},
			expectedStatus: http.StatusOK,
			expectFastRcon: true,
		},
		{
			name:     "empty fast rcon list",
			serverID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				gameModRepo *inmemory.GameModRepository,
				_ *inmemory.RBACRepository,
			) {
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

				gameMod := &domain.GameMod{
					ID:       1,
					GameCode: "cs",
					Name:     "Counter-Strike 1.6",
					FastRcon: domain.GameModFastRconList{},
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				require.NoError(t, gameModRepo.Save(context.Background(), gameMod))
			},
			expectedStatus: http.StatusOK,
			expectFastRcon: true,
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
			setupRepo: func(
				_ *inmemory.ServerRepository,
				_ *inmemory.GameModRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectFastRcon: false,
		},
		{
			name:     "user not authenticated",
			serverID: "1",
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepo: func(
				_ *inmemory.ServerRepository,
				_ *inmemory.GameModRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectFastRcon: false,
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
			setupRepo: func(
				_ *inmemory.ServerRepository,
				_ *inmemory.GameModRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
			expectFastRcon: false,
		},
		{
			name:     "game mod not found",
			serverID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				_ *inmemory.GameModRepository,
				_ *inmemory.RBACRepository,
			) {
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
					GameModID:     999,
					ServerIP:      "127.0.0.1",
					ServerPort:    27015,
					Dir:           "/home/gameap/servers/test1",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "game mod for server not found",
			expectFastRcon: false,
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
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				gameModRepo *inmemory.GameModRepository,
				_ *inmemory.RBACRepository,
			) {
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

				gameMod := &domain.GameMod{
					ID:       1,
					GameCode: "cs",
					Name:     "Counter-Strike 1.6",
					FastRcon: domain.GameModFastRconList{
						{Info: "Status", Command: "status"},
					},
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(2, 2)
				require.NoError(t, gameModRepo.Save(context.Background(), gameMod))
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectFastRcon: false,
		},
		{
			name:     "admin can access any server",
			serverID: "2",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(
				serverRepo *inmemory.ServerRepository,
				gameModRepo *inmemory.GameModRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()

				server := &domain.Server{
					ID:            2,
					UUID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:     "short2",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Server 2",
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

				gameMod := &domain.GameMod{
					ID:       1,
					GameCode: "cs",
					Name:     "Counter-Strike 1.6",
					FastRcon: domain.GameModFastRconList{
						{Info: "Status", Command: "status"},
						{Info: "Restart Map", Command: "sv_restart 1"},
					},
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 2)
				require.NoError(t, gameModRepo.Save(context.Background(), gameMod))

				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			expectedStatus: http.StatusOK,
			expectFastRcon: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			gameModRepo := inmemory.NewGameModRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, gameModRepo, rbacService, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, gameModRepo, rbacRepo)
			}

			ctx := tt.setupAuth()
			req := httptest.NewRequest(http.MethodGet, "/api/servers/"+tt.serverID+"/rcon/fast_rcon", nil)
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

			if tt.expectFastRcon {
				var fastRcon fastRconResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &fastRcon))
				assert.NotNil(t, fastRcon)
			}
		})
	}
}

func TestHandler_FastRconContent(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	gameModRepo := inmemory.NewGameModRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, gameModRepo, rbacService, responder)

	now := time.Now()

	server := &domain.Server{
		ID:            1,
		UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UUIDShort:     "short1",
		Enabled:       true,
		Installed:     1,
		Blocked:       false,
		Name:          "Test Server",
		GameID:        "hl",
		DSID:          1,
		GameModID:     2,
		ServerIP:      "127.0.0.1",
		ServerPort:    27015,
		Dir:           "/home/gameap/servers/test1",
		ProcessActive: false,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	gameMod := &domain.GameMod{
		ID:       2,
		GameCode: "hl",
		Name:     "Half-Life",
		FastRcon: domain.GameModFastRconList{
			{Info: "Server Status", Command: "status"},
			{Info: "List Users", Command: "users"},
			{Info: "Restart Round", Command: "sv_restart 1"},
			{Info: "Change Map to de_dust2", Command: "changelevel de_dust2"},
		},
	}

	require.NoError(t, serverRepo.Save(context.Background(), server))
	serverRepo.AddUserServer(1, 1)
	require.NoError(t, gameModRepo.Save(context.Background(), gameMod))

	session := &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/servers/1/rcon/fast_rcon", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"server": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var fastRcon fastRconResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &fastRcon))

	require.Len(t, fastRcon, 4)
	assert.Equal(t, "Server Status", fastRcon[0].Info)
	assert.Equal(t, "status", fastRcon[0].Command)
	assert.Equal(t, "List Users", fastRcon[1].Info)
	assert.Equal(t, "users", fastRcon[1].Command)
	assert.Equal(t, "Restart Round", fastRcon[2].Info)
	assert.Equal(t, "sv_restart 1", fastRcon[2].Command)
	assert.Equal(t, "Change Map to de_dust2", fastRcon[3].Info)
	assert.Equal(t, "changelevel de_dust2", fastRcon[3].Command)
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	gameModRepo := inmemory.NewGameModRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, gameModRepo, rbacService, responder)

	require.NotNil(t, handler)
	assert.NotNil(t, handler.serverFinder)
	assert.NotNil(t, handler.gameModRepo)
	assert.Equal(t, responder, handler.responder)
}

func TestNewFastRconResponse(t *testing.T) {
	tests := []struct {
		name     string
		fastRcon domain.GameModFastRconList
		want     int
	}{
		{
			name: "multiple items",
			fastRcon: domain.GameModFastRconList{
				{Info: "Status", Command: "status"},
				{Info: "Restart", Command: "restart"},
			},
			want: 2,
		},
		{
			name:     "empty list",
			fastRcon: domain.GameModFastRconList{},
			want:     0,
		},
		{
			name:     "nil list",
			fastRcon: nil,
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := newFastRconResponse(tt.fastRcon)

			assert.Len(t, response, tt.want)

			if tt.want > 0 && tt.fastRcon != nil {
				for i, item := range tt.fastRcon {
					assert.Equal(t, item.Info, response[i].Info)
					assert.Equal(t, item.Command, response[i].Command)
				}
			}
		})
	}
}
