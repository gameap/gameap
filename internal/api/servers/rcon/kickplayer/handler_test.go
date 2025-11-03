package kickplayer

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
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
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
		expectMessage  bool
	}{
		{
			name:     "user_not_authenticated",
			serverID: "1",
			requestBody: map[string]any{
				"player": "123",
				"reason": "test",
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
			expectMessage:  false,
		},
		{
			name:     "invalid_server_id",
			serverID: "invalid",
			requestBody: map[string]any{
				"player": "123",
				"reason": "test",
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
			expectMessage:  false,
		},
		{
			name:     "server_not_found",
			serverID: "999",
			requestBody: map[string]any{
				"player": "123",
				"reason": "test",
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
			expectMessage:  false,
		},
		{
			name:     "server_is_offline",
			serverID: "1",
			requestBody: map[string]any{
				"player": "123",
				"reason": "test",
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
			expectMessage:  false,
		},
		{
			name:     "missing_player_in_request",
			serverID: "1",
			requestBody: map[string]any{
				"reason": "test",
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
			wantError:      "player is required",
			expectMessage:  false,
		},
		{
			name:     "player_as_string_id",
			serverID: "1",
			requestBody: map[string]any{
				"player": "4841",
				"reason": "test reason",
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

				game := &domain.Game{
					Code:   "cs",
					Name:   "Counter-Strike",
					Engine: "goldsource",
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				require.NoError(t, gameRepo.Save(context.Background(), game))
			},
			expectedStatus: http.StatusServiceUnavailable,
			wantError:      "Service Unavailable",
			expectMessage:  false,
		},
		{
			name:     "player_as_full_object",
			serverID: "1",
			requestBody: map[string]any{
				"player": map[string]any{
					"id":    "4841",
					"name":  "hakan",
					"score": "202",
					"ping":  "99",
					"ip":    "31.223.13.178",
				},
				"reason": "test reason",
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

				game := &domain.Game{
					Code:   "cs",
					Name:   "Counter-Strike",
					Engine: "goldsource",
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				require.NoError(t, gameRepo.Save(context.Background(), game))
			},
			expectedStatus: http.StatusServiceUnavailable,
			wantError:      "Service Unavailable",
			expectMessage:  false,
		},
		{
			name:     "game_not_found",
			serverID: "1",
			requestBody: map[string]any{
				"player": "123",
				"reason": "test",
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
			expectMessage:  false,
		},
		{
			name:     "rcon_password_not_configured",
			serverID: "1",
			requestBody: map[string]any{
				"player": "123",
				"reason": "test",
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
			expectMessage:  false,
		},
		{
			name:     "user_does_not_have_access_to_server",
			serverID: "2",
			requestBody: map[string]any{
				"player": "123",
				"reason": "test",
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
			expectMessage:  false,
		},
		{
			name:     "admin_can_access_any_server",
			serverID: "2",
			requestBody: map[string]any{
				"player": "123",
				"reason": "test",
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
			expectMessage:  false,
		},
		{
			name:     "invalid_player_object_missing_id",
			serverID: "1",
			requestBody: map[string]any{
				"player": map[string]any{
					"name": "hakan",
				},
				"reason": "test",
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

				game := &domain.Game{
					Code:   "cs",
					Name:   "Counter-Strike",
					Engine: "goldsource",
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				require.NoError(t, gameRepo.Save(context.Background(), game))
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "player id is required",
			expectMessage:  false,
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

			req := httptest.NewRequest(http.MethodPost, "/api/servers/"+tt.serverID+"/rcon/players/kick", bytes.NewReader(body))
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{
				"server":  tt.serverID,
				"command": "kick",
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

			if tt.expectMessage {
				var response kickResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotEmpty(t, response.Message)
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

func TestKickRequest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		request  kickRequest
		wantErr  bool
		errorMsg string
	}{
		{
			name: "valid_string_player",
			request: kickRequest{
				Player: json.RawMessage(`"123"`),
				Reason: "test",
			},
			wantErr: false,
		},
		{
			name: "valid_object_player",
			request: kickRequest{
				Player: json.RawMessage(`{"id":"123","name":"test"}`),
				Reason: "test",
			},
			wantErr: false,
		},
		{
			name: "missing_player",
			request: kickRequest{
				Reason: "test",
			},
			wantErr:  true,
			errorMsg: "player is required",
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

func TestKickRequest_ToPlayer(t *testing.T) {
	tests := []struct {
		name        string
		request     kickRequest
		wantErr     bool
		errorMsg    string
		checkPlayer func(*testing.T, any)
	}{
		{
			name: "string_player_id",
			request: kickRequest{
				Player: json.RawMessage(`"4841"`),
			},
			wantErr: false,
			checkPlayer: func(t *testing.T, p any) {
				t.Helper()
				player := p.(players.Player)
				assert.Equal(t, "4841", player.ID)
				assert.Equal(t, "4841", player.UniqID)
			},
		},
		{
			name: "full_player_object",
			request: kickRequest{
				Player: json.RawMessage(`{"id":"4841","name":"hakan","score":"202","ping":"99","ip":"31.223.13.178"}`),
			},
			wantErr: false,
			checkPlayer: func(t *testing.T, p any) {
				t.Helper()
				player := p.(players.Player)
				assert.Equal(t, "4841", player.ID)
				assert.Equal(t, "hakan", player.Name)
				assert.Equal(t, "202", player.Score)
				assert.Equal(t, "99", player.Ping)
				assert.Equal(t, "31.223.13.178", player.Addr)
				assert.Equal(t, "4841", player.UniqID)
			},
		},
		{
			name: "player_object_missing_id",
			request: kickRequest{
				Player: json.RawMessage(`{"name":"hakan"}`),
			},
			wantErr:  true,
			errorMsg: "player id is required",
		},
		{
			name: "player_object_with_uniqid",
			request: kickRequest{
				Player: json.RawMessage(`{"id":"123","uniqid":"STEAM_0:1:12345"}`),
			},
			wantErr: false,
			checkPlayer: func(t *testing.T, p any) {
				t.Helper()
				player := p.(players.Player)
				assert.Equal(t, "123", player.ID)
				assert.Equal(t, "STEAM_0:1:12345", player.UniqID)
			},
		},
		{
			name: "invalid_player_format",
			request: kickRequest{
				Player: json.RawMessage(`[1,2,3]`),
			},
			wantErr:  true,
			errorMsg: "player must be a string ID or player object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player, err := tt.request.ToPlayer()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				if tt.checkPlayer != nil {
					tt.checkPlayer(t, player)
				}
			}
		})
	}
}

func TestNewKickResponse(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "simple_message",
			message: "Player kicked",
			want:    "Player kicked",
		},
		{
			name:    "empty_message",
			message: "",
			want:    "",
		},
		{
			name:    "multiline_message",
			message: "Player kicked\nReason: test",
			want:    "Player kicked\nReason: test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := newKickResponse(tt.message)
			assert.Equal(t, tt.want, response.Message)
		})
	}
}
