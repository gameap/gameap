package postcommand

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
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRconPassword = "test_password"

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
		requestBody    any
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.ServerRepository, *inmemory.GameRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		expectOutput   bool
	}{
		{
			name:     "user_not_authenticated",
			serverID: "1",
			requestBody: map[string]string{
				"command": "status",
			},
			setupAuth: context.Background,
			setupRepo: func(
				_ *inmemory.ServerRepository,
				_ *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectOutput:   false,
		},
		{
			name:     "invalid_server_id",
			serverID: "invalid",
			requestBody: map[string]string{
				"command": "status",
			},
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
				_ *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
			expectOutput:   false,
		},
		{
			name:     "server_not_found",
			serverID: "999",
			requestBody: map[string]string{
				"command": "status",
			},
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
				_ *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectOutput:   false,
		},
		{
			name:     "server_is_offline",
			serverID: "1",
			requestBody: map[string]string{
				"command": "status",
			},
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
				_ *inmemory.GameRepository,
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
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			expectedStatus: http.StatusServiceUnavailable,
			wantError:      "Service Unavailable",
			expectOutput:   false,
		},
		{
			name:     "missing_command_in_request",
			serverID: "1",
			requestBody: map[string]string{
				"not_command": "status",
			},
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
				_ *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()
				lastCheck := time.Now()

				server := &domain.Server{
					ID:               1,
					UUID:             uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:        "short1",
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Test Server 1",
					GameID:           "cs",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					ProcessActive:    true,
					LastProcessCheck: &lastCheck,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "command is required",
			expectOutput:   false,
		},
		{
			name:     "empty_command",
			serverID: "1",
			requestBody: map[string]string{
				"command": "",
			},
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
				_ *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()
				lastCheck := time.Now()

				server := &domain.Server{
					ID:               1,
					UUID:             uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:        "short1",
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Test Server 1",
					GameID:           "cs",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					ProcessActive:    true,
					LastProcessCheck: &lastCheck,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "command is required",
			expectOutput:   false,
		},
		{
			name:     "command_exceeds_maximum_length",
			serverID: "1",
			requestBody: map[string]string{
				"command": string(make([]byte, 128)),
			},
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
				_ *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()
				lastCheck := time.Now()

				server := &domain.Server{
					ID:               1,
					UUID:             uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:        "short1",
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Test Server 1",
					GameID:           "cs",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					ProcessActive:    true,
					LastProcessCheck: &lastCheck,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "command must not exceed 127 characters",
			expectOutput:   false,
		},
		{
			name:     "game_not_found",
			serverID: "1",
			requestBody: map[string]string{
				"command": "status",
			},
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
				_ *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()
				lastCheck := time.Now()
				rconPassword := "test_password"

				server := &domain.Server{
					ID:               1,
					UUID:             uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:        "short1",
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Test Server 1",
					GameID:           "cs",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					Rcon:             &rconPassword,
					ProcessActive:    true,
					LastProcessCheck: &lastCheck,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
			expectOutput:   false,
		},
		{
			name:     "rcon_password_not_configured",
			serverID: "1",
			requestBody: map[string]string{
				"command": "status",
			},
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
				gameRepo *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()
				lastCheck := time.Now()

				server := &domain.Server{
					ID:               1,
					UUID:             uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:        "short1",
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Test Server 1",
					GameID:           "cs",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27015,
					Rcon:             nil,
					ProcessActive:    true,
					LastProcessCheck: &lastCheck,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}

				game := &domain.Game{
					Code:   "cs",
					Name:   "Counter-Strike",
					Engine: "goldsource",
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				require.NoError(t, gameRepo.Save(context.Background(), game))
			},
			expectedStatus: http.StatusPreconditionFailed,
			wantError:      "rcon password not configured for server",
			expectOutput:   false,
		},
		{
			name:     "user_does_not_have_access_to_server",
			serverID: "2",
			requestBody: map[string]string{
				"command": "status",
			},
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
				gameRepo *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
				now := time.Now()
				lastCheck := time.Now()
				rconPassword := testRconPassword

				server := &domain.Server{
					ID:               2,
					UUID:             uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:        "short2",
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Other User Server",
					GameID:           "cs",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27016,
					Rcon:             &rconPassword,
					ProcessActive:    true,
					LastProcessCheck: &lastCheck,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}

				game := &domain.Game{
					Code:   "cs",
					Name:   "Counter-Strike",
					Engine: "goldsource",
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(2, 2)
				require.NoError(t, gameRepo.Save(context.Background(), game))
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectOutput:   false,
		},
		{
			name:     "admin_can_access_any_server",
			serverID: "2",
			requestBody: map[string]string{
				"command": "status",
			},
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
				gameRepo *inmemory.GameRepository,
				rbacRepo *inmemory.RBACRepository,
			) {
				now := time.Now()
				lastCheck := time.Now()
				rconPassword := testRconPassword

				server := &domain.Server{
					ID:               2,
					UUID:             uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:        "short2",
					Enabled:          true,
					Installed:        1,
					Blocked:          false,
					Name:             "Server 2",
					GameID:           "cs",
					DSID:             1,
					GameModID:        1,
					ServerIP:         "127.0.0.1",
					ServerPort:       27016,
					Rcon:             &rconPassword,
					ProcessActive:    true,
					LastProcessCheck: &lastCheck,
					CreatedAt:        &now,
					UpdatedAt:        &now,
				}

				game := &domain.Game{
					Code:   "cs",
					Name:   "Counter-Strike",
					Engine: "goldsource",
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 2)
				require.NoError(t, gameRepo.Save(context.Background(), game))

				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			expectedStatus: http.StatusServiceUnavailable,
			wantError:      "Service Unavailable",
			expectOutput:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			gameRepo := inmemory.NewGameRepository()
			rbacRepo := inmemory.NewRBACRepository()
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, gameRepo, rbacService, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, gameRepo, rbacRepo)
			}

			ctx := tt.setupAuth()

			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/servers/"+tt.serverID+"/rcon", bytes.NewReader(body))
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

			if tt.expectOutput {
				var response commandResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotEmpty(t, response.Output)
			}
		})
	}
}

func TestHandler_NewHandler(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	gameRepo := inmemory.NewGameRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, gameRepo, rbacService, responder)

	require.NotNil(t, handler)
	assert.NotNil(t, handler.serverFinder)
	assert.NotNil(t, handler.gameRepo)
	assert.Equal(t, responder, handler.responder)
}

func TestGetRconPort(t *testing.T) {
	tests := []struct {
		name   string
		server *domain.Server
		want   int
	}{
		{
			name: "custom_rcon_port",
			server: &domain.Server{
				ServerPort: 27015,
				RconPort:   intPtr(27020),
			},
			want: 27020,
		},
		{
			name: "default_to_server_port",
			server: &domain.Server{
				ServerPort: 27015,
				RconPort:   nil,
			},
			want: 27015,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := getRconPort(tt.server)
			assert.Equal(t, tt.want, port)
		})
	}
}

func intPtr(i int) *int {
	return &i
}

func TestCommandRequest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		request  commandRequest
		wantErr  bool
		errorMsg string
	}{
		{
			name: "valid_command",
			request: commandRequest{
				Command: "status",
			},
			wantErr: false,
		},
		{
			name: "valid_command_with_spaces",
			request: commandRequest{
				Command: "  status  ",
			},
			wantErr: false,
		},
		{
			name: "empty_command",
			request: commandRequest{
				Command: "",
			},
			wantErr:  true,
			errorMsg: "command is required",
		},
		{
			name: "command_exceeds_max_length",
			request: commandRequest{
				Command: string(make([]byte, 128)),
			},
			wantErr:  true,
			errorMsg: "command must not exceed 127 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewCommandResponse(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "simple_output",
			output: "Server is running",
			want:   "Server is running",
		},
		{
			name:   "empty_output",
			output: "",
			want:   "",
		},
		{
			name:   "multiline_output",
			output: "CPU   In    Out   Uptime  Users   FPS    Players\n 4.50  0.00  0.00      87     0  432.72       0",
			want:   "CPU   In    Out   Uptime  Users   FPS    Players\n 4.50  0.00  0.00      87     0  432.72       0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := newCommandResponse(tt.output)
			assert.Equal(t, tt.want, response.Output)
		})
	}
}
