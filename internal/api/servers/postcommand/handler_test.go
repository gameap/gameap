package postcommand

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

const testStartCommand = "./start.sh"

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
		name       string
		serverID   string
		command    string
		setupAuth  func() context.Context
		setupRepo  func(*inmemory.ServerRepository, *inmemory.RBACRepository)
		wantStatus int
		wantError  string
		wantTaskID bool
	}{
		{
			name:     "successful_start_command",
			serverID: "1",
			command:  "start",
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
				startCmd := testStartCommand
				server := &domain.Server{
					ID:           1,
					UUID:         uuid.New(),
					UUIDShort:    "short1",
					Enabled:      true,
					Installed:    1,
					Name:         "Test Server",
					GameID:       "cstrike",
					DSID:         1,
					GameModID:    1,
					ServerIP:     "192.168.1.1",
					ServerPort:   27015,
					StartCommand: &startCmd,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(testUser1.ID, server.ID)
			},
			wantStatus: http.StatusOK,
			wantTaskID: true,
		},
		{
			name:     "successful_stop_command",
			serverID: "1",
			command:  "stop",
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
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "short1",
					Enabled:    true,
					Installed:  1,
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(testUser1.ID, server.ID)
			},
			wantStatus: http.StatusOK,
			wantTaskID: true,
		},
		{
			name:     "successful_restart_command",
			serverID: "1",
			command:  "restart",
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
				startCmd := testStartCommand
				server := &domain.Server{
					ID:           1,
					UUID:         uuid.New(),
					UUIDShort:    "short1",
					Enabled:      true,
					Installed:    1,
					Name:         "Test Server",
					GameID:       "cstrike",
					DSID:         1,
					GameModID:    1,
					ServerIP:     "192.168.1.1",
					ServerPort:   27015,
					StartCommand: &startCmd,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(testUser1.ID, server.ID)
			},
			wantStatus: http.StatusOK,
			wantTaskID: true,
		},
		{
			name:     "successful_update_command",
			serverID: "1",
			command:  "update",
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
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "short1",
					Enabled:    true,
					Installed:  1,
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(testUser1.ID, server.ID)
			},
			wantStatus: http.StatusOK,
			wantTaskID: true,
		},
		{
			name:     "successful_install_command",
			serverID: "1",
			command:  "install",
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
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "short1",
					Enabled:    true,
					Installed:  1,
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(testUser1.ID, server.ID)
			},
			wantStatus: http.StatusOK,
			wantTaskID: true,
		},
		{
			name:     "successful_reinstall_command",
			serverID: "1",
			command:  "reinstall",
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
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "short1",
					Enabled:    true,
					Installed:  1,
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(testUser1.ID, server.ID)
			},
			wantStatus: http.StatusOK,
			wantTaskID: true,
		},
		{
			name:       "user_not_authenticated",
			serverID:   "1",
			command:    "start",
			setupRepo:  func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			wantStatus: http.StatusUnauthorized,
			wantError:  "user not authenticated",
		},
		{
			name:     "invalid_server_id",
			serverID: "invalid",
			command:  "start",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:  func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid server id",
		},
		{
			name:     "invalid_command_-_empty",
			serverID: "1",
			command:  "",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:  func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			wantStatus: http.StatusNotFound,
			wantError:  "invalid command",
		},
		{
			name:     "invalid_command_-_unknown",
			serverID: "1",
			command:  "unknown_command",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:  func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			wantStatus: http.StatusNotFound,
			wantError:  "invalid command",
		},
		{
			name:     "server_not_found",
			serverID: "999",
			command:  "start",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:  func(_ *inmemory.ServerRepository, _ *inmemory.RBACRepository) {},
			wantStatus: http.StatusNotFound,
			wantError:  "server not found",
		},
		{
			name:     "user_does_not_have_access_to_server",
			serverID: "2",
			command:  "start",
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
				startCmd := testStartCommand
				server := &domain.Server{
					ID:           2,
					UUID:         uuid.New(),
					UUIDShort:    "short2",
					Enabled:      true,
					Installed:    1,
					Name:         "Other User Server",
					GameID:       "cstrike",
					DSID:         1,
					GameModID:    1,
					ServerIP:     "192.168.1.2",
					ServerPort:   27016,
					StartCommand: &startCmd,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				// Server is assigned to user 2, not user 1
				serverRepo.AddUserServer(2, server.ID)
			},
			wantStatus: http.StatusNotFound,
			wantError:  "server not found",
		},
		{
			name:     "admin_can_access_any_server",
			serverID: "2",
			command:  "start",
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
				startCmd := testStartCommand
				server := &domain.Server{
					ID:           2,
					UUID:         uuid.New(),
					UUIDShort:    "short2",
					Enabled:      true,
					Installed:    1,
					Name:         "Server 2",
					GameID:       "cstrike",
					DSID:         1,
					GameModID:    1,
					ServerIP:     "192.168.1.2",
					ServerPort:   27016,
					StartCommand: &startCmd,
					CreatedAt:    &now,
					UpdatedAt:    &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				// Server is assigned to user 1, not the admin
				serverRepo.AddUserServer(1, server.ID)

				// Setup admin ability
				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			wantStatus: http.StatusOK,
			wantTaskID: true,
		},
		{
			name:     "start_command_fails_when_server_has_no_start_command",
			serverID: "1",
			command:  "start",
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
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "short1",
					Enabled:    true,
					Installed:  1,
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(testUser1.ID, server.ID)
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			daemonTaskRepo := inmemory.NewDaemonTaskRepository()
			serverSettingRepo := inmemory.NewServerSettingRepository()

			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			serverControlService := servercontrol.NewService(
				daemonTaskRepo,
				serverSettingRepo,
				services.NewNilTransactionManager(),
			)
			responder := api.NewResponder()

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, rbacRepo)
			}

			handler := NewHandler(serverRepo, serverControlService, rbacService, responder)

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			body := strings.NewReader(`{"server": "` + tt.serverID + `"}`)
			requestURL := "/api/servers/" + tt.serverID + "/command/" + tt.command
			req := httptest.NewRequest(http.MethodPost, requestURL, body)
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"server": tt.serverID})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			}

			if tt.wantTaskID {
				var response commandResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotZero(t, response.DaemonTaskID)
			}
		})
	}
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingRepo := inmemory.NewServerSettingRepository()

	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	serverControlService := servercontrol.NewService(
		daemonTaskRepo,
		serverSettingRepo,
		services.NewNilTransactionManager(),
	)
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, serverControlService, rbacService, responder)

	require.NotNil(t, handler)
	assert.NotNil(t, handler.serverFinder)
	assert.Equal(t, responder, handler.responder)
	assert.NotNil(t, handler.commandMap)
	assert.NotNil(t, handler.abilitiesMap)
	assert.Len(t, handler.commandMap, 6)
	assert.Len(t, handler.abilitiesMap, 6)
}

func TestHandler_CommandMapContainsAllCommands(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingRepo := inmemory.NewServerSettingRepository()

	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	serverControlService := servercontrol.NewService(
		daemonTaskRepo,
		serverSettingRepo,
		services.NewNilTransactionManager(),
	)
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, serverControlService, rbacService, responder)

	expectedCommands := []string{"start", "stop", "restart", "update", "install", "reinstall"}

	for _, cmd := range expectedCommands {
		assert.Contains(t, handler.commandMap, cmd, "commandMap should contain "+cmd)
		assert.Contains(t, handler.abilitiesMap, cmd, "abilitiesMap should contain "+cmd)
	}
}

func TestHandler_AbilitiesMapContainsCorrectAbilities(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	rbacRepo := inmemory.NewRBACRepository()
	daemonTaskRepo := inmemory.NewDaemonTaskRepository()
	serverSettingRepo := inmemory.NewServerSettingRepository()

	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	serverControlService := servercontrol.NewService(
		daemonTaskRepo,
		serverSettingRepo,
		services.NewNilTransactionManager(),
	)
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, serverControlService, rbacService, responder)

	tests := []struct {
		command           string
		expectedAbilities []domain.AbilityName
	}{
		{
			command: "start",
			expectedAbilities: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStart,
			},
		},
		{
			command: "stop",
			expectedAbilities: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStop,
			},
		},
		{
			command: "restart",
			expectedAbilities: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
			},
		},
		{
			command: "update",
			expectedAbilities: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
		},
		{
			command: "install",
			expectedAbilities: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
		},
		{
			command: "reinstall",
			expectedAbilities: []domain.AbilityName{
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			abilities, exists := handler.abilitiesMap[tt.command]
			require.True(t, exists)
			assert.Equal(t, tt.expectedAbilities, abilities)
		})
	}
}

func TestNewCommandResponse(t *testing.T) {
	taskID := uint(42)
	response := newCommandResponse(taskID)

	require.NotNil(t, response)
	assert.Equal(t, taskID, response.DaemonTaskID)
}

func TestHandler_FindUserServer(t *testing.T) {
	tests := []struct {
		name         string
		userID       uint
		serverID     uint
		setupRepo    func(*inmemory.ServerRepository, *inmemory.RBACRepository)
		expectError  bool
		errorMessage string
	}{
		{
			name:     "regular_user_finds_their_server",
			userID:   1,
			serverID: 1,
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now()
				server := &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "short1",
					Enabled:    true,
					Installed:  1,
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			expectError: false,
		},
		{
			name:     "regular_user_cannot_find_other_user's_server",
			userID:   1,
			serverID: 2,
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.RBACRepository) {
				now := time.Now()
				server := &domain.Server{
					ID:         2,
					UUID:       uuid.New(),
					UUIDShort:  "short2",
					Enabled:    true,
					Installed:  1,
					Name:       "Other Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.2",
					ServerPort: 27016,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(2, 2)
			},
			expectError:  true,
			errorMessage: "server not found",
		},
		{
			name:     "admin_can_find_any_server",
			userID:   2,
			serverID: 1,
			setupRepo: func(serverRepo *inmemory.ServerRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()
				server := &domain.Server{
					ID:         1,
					UUID:       uuid.New(),
					UUIDShort:  "short1",
					Enabled:    true,
					Installed:  1,
					Name:       "Test Server",
					GameID:     "cstrike",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), 2, adminAbility.ID))
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			daemonTaskRepo := inmemory.NewDaemonTaskRepository()
			serverSettingRepo := inmemory.NewServerSettingRepository()

			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			serverControlService := servercontrol.NewService(
				daemonTaskRepo,
				serverSettingRepo,
				services.NewNilTransactionManager(),
			)
			responder := api.NewResponder()

			handler := NewHandler(serverRepo, serverControlService, rbacService, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, rbacRepo)
			}

			user := &domain.User{ID: tt.userID}
			server, err := handler.serverFinder.FindUserServer(context.Background(), user, tt.serverID)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
				assert.Nil(t, server)
			} else {
				require.NoError(t, err)
				require.NotNil(t, server)
				assert.Equal(t, tt.serverID, server.ID)
			}
		})
	}
}

func TestHandler_DaemonTaskCreation(t *testing.T) {
	tests := []struct {
		name             string
		command          string
		expectedTaskType domain.DaemonTaskType
	}{
		{
			name:             "start_command_creates_start_task",
			command:          "start",
			expectedTaskType: domain.DaemonTaskTypeServerStart,
		},
		{
			name:             "stop_command_creates_stop_task",
			command:          "stop",
			expectedTaskType: domain.DaemonTaskTypeServerStop,
		},
		{
			name:             "restart_command_creates_restart_task",
			command:          "restart",
			expectedTaskType: domain.DaemonTaskTypeServerRestart,
		},
		{
			name:             "update_command_creates_update_task",
			command:          "update",
			expectedTaskType: domain.DaemonTaskTypeServerUpdate,
		},
		{
			name:             "install_command_creates_install_task",
			command:          "install",
			expectedTaskType: domain.DaemonTaskTypeServerInstall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			rbacRepo := inmemory.NewRBACRepository()
			daemonTaskRepo := inmemory.NewDaemonTaskRepository()
			serverSettingRepo := inmemory.NewServerSettingRepository()

			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			serverControlService := servercontrol.NewService(
				daemonTaskRepo,
				serverSettingRepo,
				services.NewNilTransactionManager(),
			)
			responder := api.NewResponder()

			// Setup server
			now := time.Now()
			startCmd := testStartCommand
			server := &domain.Server{
				ID:           1,
				UUID:         uuid.New(),
				UUIDShort:    "short1",
				Enabled:      true,
				Installed:    1,
				Name:         "Test Server",
				GameID:       "cstrike",
				DSID:         1,
				GameModID:    1,
				ServerIP:     "192.168.1.1",
				ServerPort:   27015,
				StartCommand: &startCmd,
				CreatedAt:    &now,
				UpdatedAt:    &now,
			}
			require.NoError(t, serverRepo.Save(context.Background(), server))
			serverRepo.AddUserServer(testUser1.ID, server.ID)

			handler := NewHandler(serverRepo, serverControlService, rbacService, responder)

			session := &auth.Session{
				Login: "testuser",
				Email: "test@example.com",
				User:  &testUser1,
			}
			ctx := auth.ContextWithSession(context.Background(), session)

			body := strings.NewReader(`{"server": "1"}`)
			requestURL := "/api/servers/1/command/" + tt.command
			req := httptest.NewRequest(http.MethodPost, requestURL, body)
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"server": "1"})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var response commandResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.NotZero(t, response.DaemonTaskID)

			// Verify daemon task was created
			tasks, err := daemonTaskRepo.FindAll(ctx, nil, nil)
			require.NoError(t, err)
			require.Len(t, tasks, 1)
			assert.Equal(t, tt.expectedTaskType, tasks[0].Task)
			assert.Equal(t, server.ID, *tasks[0].ServerID)
			assert.Equal(t, server.DSID, tasks[0].DedicatedServerID)
		})
	}
}
