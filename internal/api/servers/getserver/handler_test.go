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
		setupRepo      func(*inmemory.ServerRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		expectServer   bool
		isAdmin        bool
	}{
		{
			name:     "successful server retrieval",
			serverID: "1",
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
					ID:            1,
					UID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
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
			},
			expectedStatus: http.StatusOK,
			expectServer:   true,
			isAdmin:        false,
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
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectServer:   false,
			isAdmin:        false,
		},
		{
			name:     "user not authenticated",
			serverID: "1",
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectServer:   false,
			isAdmin:        false,
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
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
			expectServer:   false,
			isAdmin:        false,
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
					UID:           uuid.MustParse("22222222-2222-2222-2222-222222222222"),
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
				// Server is assigned to user 2, not user 1
				serverRepo.AddUserServer(2, 2)
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectServer:   false,
			isAdmin:        false,
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
			setupRepo: func(serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()

				server := &domain.Server{
					ID:            2,
					UID:           uuid.MustParse("22222222-2222-2222-2222-222222222222"),
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

				require.NoError(t, serverRepo.Save(context.Background(), server))
				// Server is not assigned to the admin user
				serverRepo.AddUserServer(1, 2)

				// Setup admin ability
				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			expectedStatus: http.StatusOK,
			expectServer:   true,
			isAdmin:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			serverSettingRepo := inmemory.NewServerSettingRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, gameRepo, gameModRepo, serverSettingRepo, rbacService, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, rbacRepo)
			}

			ctx := tt.setupAuth()
			req := httptest.NewRequest(http.MethodGet, "/api/servers/"+tt.serverID, nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"id": tt.serverID})
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

			if tt.expectServer {
				if tt.isAdmin {
					var server adminServerResponse
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &server))
					assert.NotZero(t, server.ID)
					assert.NotEmpty(t, server.UUID)
					assert.NotEmpty(t, server.Name)
					assert.NotEmpty(t, server.GameID)
					assert.NotZero(t, server.ServerPort)
				} else {
					var server userServerResponse
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &server))
					assert.NotZero(t, server.ID)
					assert.NotEmpty(t, server.Name)
					assert.NotEmpty(t, server.GameID)
					assert.NotZero(t, server.ServerPort)
				}
			}
		})
	}
}

func TestHandler_PublicIPVisibility(t *testing.T) {
	tests := []struct {
		name     string
		metadata domain.Metadata
		isAdmin  bool
		wantIP   string
	}{
		{
			name:     "user_gets_public_ip",
			metadata: domain.Metadata{"public_ip": "203.0.113.10"},
			isAdmin:  false,
			wantIP:   "203.0.113.10",
		},
		{
			name:     "user_gets_real_ip_without_public_ip",
			metadata: domain.Metadata{"docker_image": "gameap/debian"},
			isAdmin:  false,
			wantIP:   "10.0.0.5",
		},
		{
			name:     "user_gets_real_ip_when_public_ip_blank",
			metadata: domain.Metadata{"public_ip": "  "},
			isAdmin:  false,
			wantIP:   "10.0.0.5",
		},
		{
			name:     "admin_gets_public_ip_too",
			metadata: domain.Metadata{"public_ip": "203.0.113.10"},
			isAdmin:  true,
			wantIP:   "203.0.113.10",
		},
		{
			name:     "admin_gets_real_ip_without_public_ip",
			metadata: nil,
			isAdmin:  true,
			wantIP:   "10.0.0.5",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			serverSettingRepo := inmemory.NewServerSettingRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, gameRepo, gameModRepo, serverSettingRepo, rbacService, responder)

			user := testUser1
			if test.isAdmin {
				user = testUser2

				adminAbility := &domain.Ability{ID: 1, Name: domain.AbilityNameAdminRolesPermissions}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), user.ID, adminAbility.ID))
			}

			require.NoError(t, serverRepo.Save(context.Background(), &domain.Server{
				ID:         1,
				UID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				UUIDShort:  "short1",
				Enabled:    true,
				Installed:  1,
				Name:       "Test Server",
				GameID:     "cs",
				DSID:       1,
				GameModID:  1,
				ServerIP:   "10.0.0.5",
				ServerPort: 27015,
				Dir:        "/home/gameap/servers/test1",
				Metadata:   test.metadata,
			}))
			serverRepo.AddUserServer(user.ID, 1)

			req := httptest.NewRequest(http.MethodGet, "/api/servers/1", nil)
			req = req.WithContext(auth.ContextWithSession(context.Background(), &auth.Session{
				Login: user.Login,
				Email: user.Email,
				User:  &user,
			}))
			req = mux.SetURLVars(req, map[string]string{"id": "1"})

			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())

			var response struct {
				ServerIP         string         `json:"server_ip"`
				InternalServerIP string         `json:"internal_server_ip"`
				Aliases          map[string]any `json:"aliases"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

			assert.Equal(t, test.wantIP, response.ServerIP)

			if test.isAdmin {
				// The edit form round-trips internal_server_ip, and aliases feed the
				// start command: both must keep the address the daemon connects to.
				assert.Equal(t, "10.0.0.5", response.InternalServerIP)
				assert.Equal(t, "10.0.0.5", response.Aliases["ip"])
			}
		})
	}
}

func TestHandler_ServerResponseFields(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	serverSettingRepo := inmemory.NewServerSettingRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, gameRepo, gameModRepo, serverSettingRepo, rbacService, responder)

	now := time.Now()
	userName := "Admin User"
	user := &domain.User{
		ID:        2,
		Login:     "admin",
		Email:     "admin@example.com",
		Name:      &userName,
		CreatedAt: &now,
		UpdatedAt: &now,
	}

	adminAbility := &domain.Ability{
		ID:   1,
		Name: domain.AbilityNameAdminRolesPermissions,
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
	require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), user.ID, adminAbility.ID))

	queryPort := 27016
	rconPort := 27017
	rcon := "rconpassword"
	suUser := "gameap"
	cpuLimit := 50
	ramLimit := 2048
	netLimit := 100
	startCmd := "./start.sh"
	stopCmd := "./stop.sh"
	forceStopCmd := "./force_stop.sh"
	restartCmd := "./restart.sh"
	expires := now.Add(30 * 24 * time.Hour)
	lastCheck := now.Add(-1 * time.Hour)

	server := &domain.Server{
		ID:               1,
		UID:              uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Enabled:          true,
		Installed:        1,
		Blocked:          false,
		Name:             "Test Server",
		GameID:           "cs16",
		DSID:             1,
		GameModID:        2,
		Expires:          &expires,
		ServerIP:         "192.168.1.100",
		ServerPort:       27015,
		QueryPort:        &queryPort,
		RconPort:         &rconPort,
		Rcon:             &rcon,
		Dir:              "/home/gameap/servers/testserver",
		SuUser:           &suUser,
		CPULimit:         &cpuLimit,
		RAMLimit:         &ramLimit,
		NetLimit:         &netLimit,
		StartCommand:     &startCmd,
		StopCommand:      &stopCmd,
		ForceStopCommand: &forceStopCmd,
		RestartCommand:   &restartCmd,
		ProcessActive:    true,
		LastProcessCheck: &lastCheck,
		Vars:             domain.ServerVars{"key": "value"},
		Metadata:         domain.Metadata{"docker_image": "ghcr.io/gameap/cs16:latest", "default_port": float64(27015)},
		CreatedAt:        &now,
		UpdatedAt:        &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))
	serverRepo.AddUserServer(1, 1)

	session := &auth.Session{
		Login: "admin",
		Email: "admin@example.com",
		User:  user,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/servers/1", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var serverResp adminServerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &serverResp))

	assert.Equal(t, uint(1), serverResp.ID)
	assert.Equal(t, "33333333-3333-3333-3333-333333333333", serverResp.UUID)
	assert.Equal(t, "33333333", serverResp.UUIDShort)
	assert.True(t, serverResp.Enabled)
	assert.Equal(t, 1, serverResp.Installed)
	assert.False(t, serverResp.Blocked)
	assert.Equal(t, "Test Server", serverResp.Name)
	assert.Equal(t, "cs16", serverResp.GameID)
	assert.Equal(t, uint(1), serverResp.DSID)
	assert.Equal(t, uint(2), serverResp.GameModID)
	require.NotNil(t, serverResp.Expires)
	assert.Equal(t, expires.Unix(), serverResp.Expires.Unix())
	assert.Equal(t, "192.168.1.100", serverResp.ServerIP)
	assert.Equal(t, 27015, serverResp.ServerPort)
	require.NotNil(t, serverResp.QueryPort)
	assert.Equal(t, 27016, *serverResp.QueryPort)
	require.NotNil(t, serverResp.RconPort)
	assert.Equal(t, 27017, *serverResp.RconPort)
	require.NotNil(t, serverResp.Rcon)
	assert.Equal(t, "rconpassword", *serverResp.Rcon)
	assert.Equal(t, "/home/gameap/servers/testserver", serverResp.Dir)
	require.NotNil(t, serverResp.SuUser)
	assert.Equal(t, "gameap", *serverResp.SuUser)
	require.NotNil(t, serverResp.CPULimit)
	assert.Equal(t, 50, *serverResp.CPULimit)
	require.NotNil(t, serverResp.RAMLimit)
	assert.Equal(t, 2048, *serverResp.RAMLimit)
	require.NotNil(t, serverResp.NetLimit)
	assert.Equal(t, 100, *serverResp.NetLimit)
	require.NotNil(t, serverResp.StartCommand)
	assert.Equal(t, "./start.sh", *serverResp.StartCommand)
	require.NotNil(t, serverResp.StopCommand)
	assert.Equal(t, "./stop.sh", *serverResp.StopCommand)
	require.NotNil(t, serverResp.ForceStopCommand)
	assert.Equal(t, "./force_stop.sh", *serverResp.ForceStopCommand)
	require.NotNil(t, serverResp.RestartCommand)
	assert.Equal(t, "./restart.sh", *serverResp.RestartCommand)
	assert.True(t, serverResp.ProcessActive)
	require.NotNil(t, serverResp.LastProcessCheck)
	assert.Equal(t, lastCheck.Unix(), serverResp.LastProcessCheck.Unix())
	require.NotNil(t, serverResp.Vars)
	assert.Equal(t, map[string]string{"key": "value"}, serverResp.Vars)
	assert.Equal(t, map[string]any{"docker_image": "ghcr.io/gameap/cs16:latest", "default_port": float64(27015)}, serverResp.Metadata)
	assert.NotNil(t, serverResp.CreatedAt)
	assert.NotNil(t, serverResp.UpdatedAt)
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	serverSettingRepo := inmemory.NewServerSettingRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, gameRepo, gameModRepo, serverSettingRepo, rbacService, responder)

	require.NotNil(t, handler)
	assert.NotNil(t, handler.serverFinder)
	assert.Equal(t, responder, handler.responder)
}

func TestNewServerResponseFromServer(t *testing.T) {
	now := time.Now()
	queryPort := 27016
	server := &domain.Server{
		ID:            1,
		UID:           uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		Enabled:       true,
		Installed:     1,
		Blocked:       false,
		Name:          "Test Server",
		GameID:        "cs",
		DSID:          1,
		GameModID:     1,
		ServerIP:      "127.0.0.1",
		ServerPort:    27015,
		QueryPort:     &queryPort,
		Dir:           "/test/dir",
		ProcessActive: true,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	server.Hydrate()

	response := newAdminServerResponseFromServer(server, nil, nil, nil)

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "44444444-4444-4444-4444-444444444444", response.UUID)
	assert.Equal(t, "44444444", response.UUIDShort)
	assert.True(t, response.Enabled)
	assert.Equal(t, 1, response.Installed)
	assert.False(t, response.Blocked)
	assert.Equal(t, "Test Server", response.Name)
	assert.Equal(t, "cs", response.GameID)
	assert.Equal(t, uint(1), response.DSID)
	assert.Equal(t, uint(1), response.GameModID)
	assert.Equal(t, "127.0.0.1", response.ServerIP)
	assert.Equal(t, 27015, response.ServerPort)
	require.NotNil(t, response.QueryPort)
	assert.Equal(t, 27016, *response.QueryPort)
	assert.Equal(t, "/test/dir", response.Dir)
	assert.True(t, response.ProcessActive)
	assert.Equal(t, &now, response.CreatedAt)
	assert.Equal(t, &now, response.UpdatedAt)
}
