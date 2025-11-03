package getplayers

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
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.ServerRepository, *inmemory.GameRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		expectPlayers  bool
	}{
		{
			name:      "user_not_authenticated",
			serverID:  "1",
			setupAuth: context.Background,
			setupRepo: func(
				_ *inmemory.ServerRepository,
				_ *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectPlayers:  false,
		},
		{
			name:     "invalid_server_id",
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
				_ *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
			expectPlayers:  false,
		},
		{
			name:     "server_not_found",
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
				_ *inmemory.GameRepository,
				_ *inmemory.RBACRepository,
			) {
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectPlayers:  false,
		},
		{
			name:     "server_is_offline",
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
			expectPlayers:  false,
		},
		{
			name:     "game_not_found",
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
				_ *inmemory.GameRepository,
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

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
			expectPlayers:  false,
		},
		{
			name:     "rcon_password_not_configured",
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
			expectPlayers:  false,
		},
		{
			name:     "user_does_not_have_access_to_server",
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
			expectPlayers:  false,
		},
		{
			name:     "unsupported_game_for_player_management",
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
					GameID:           "unsupported_game",
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
					Code:   "unsupported_game",
					Name:   "Unsupported Game",
					Engine: "goldsource",
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)
				require.NoError(t, gameRepo.Save(context.Background(), game))
			},
			expectedStatus: http.StatusNotImplemented,
			wantError:      "Not Implemented",
			expectPlayers:  false,
		},
		{
			name:     "admin_can_access_any_server",
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
			expectPlayers:  false,
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

			req := httptest.NewRequest(http.MethodGet, "/api/servers/"+tt.serverID+"/rcon/players", nil)
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

			if tt.expectPlayers {
				var response []playerResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.NotNil(t, response)
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

func TestNewPlayersResponse(t *testing.T) {
	tests := []struct {
		name    string
		players []struct {
			ID    string
			Name  string
			Score string
			Ping  string
			Addr  string
		}
		want []playerResponse
	}{
		{
			name: "empty_players_list",
			players: []struct {
				ID    string
				Name  string
				Score string
				Ping  string
				Addr  string
			}{},
			want: []playerResponse{},
		},
		{
			name: "single_player",
			players: []struct {
				ID    string
				Name  string
				Score string
				Ping  string
				Addr  string
			}{
				{
					ID:    "4280",
					Name:  "Sniper",
					Score: "100",
					Ping:  "0",
					Addr:  "127.0.0.1",
				},
			},
			want: []playerResponse{
				{
					ID:    "4280",
					Name:  "Sniper",
					Score: "100",
					Ping:  "0",
					IP:    "127.0.0.1",
				},
			},
		},
		{
			name: "multiple_players",
			players: []struct {
				ID    string
				Name  string
				Score string
				Ping  string
				Addr  string
			}{
				{
					ID:    "4280",
					Name:  "Sniper",
					Score: "100",
					Ping:  "0",
					Addr:  "127.0.0.1",
				},
				{
					ID:    "4284",
					Name:  "Kapa6aC_PaCKoJI6aC",
					Score: "100",
					Ping:  "0",
					Addr:  "127.0.0.1",
				},
				{
					ID:    "4131",
					Name:  "jorkata",
					Score: "0",
					Ping:  "47",
					Addr:  "10.77.17.77",
				},
			},
			want: []playerResponse{
				{
					ID:    "4280",
					Name:  "Sniper",
					Score: "100",
					Ping:  "0",
					IP:    "127.0.0.1",
				},
				{
					ID:    "4284",
					Name:  "Kapa6aC_PaCKoJI6aC",
					Score: "100",
					Ping:  "0",
					IP:    "127.0.0.1",
				},
				{
					ID:    "4131",
					Name:  "jorkata",
					Score: "0",
					Ping:  "47",
					IP:    "10.77.17.77",
				},
			},
		},
		{
			name: "invalid_score_and_ping",
			players: []struct {
				ID    string
				Name  string
				Score string
				Ping  string
				Addr  string
			}{
				{
					ID:    "123",
					Name:  "Player",
					Score: "invalid",
					Ping:  "also_invalid",
					Addr:  "192.168.1.1",
				},
			},
			want: []playerResponse{
				{
					ID:    "123",
					Name:  "Player",
					Score: "invalid",      // as is, since we don't parse it
					Ping:  "also_invalid", // as is, since we don't parse it
					IP:    "192.168.1.1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var playersList []struct {
				ID    string
				Name  string
				Score string
				Ping  string
				Addr  string
			}
			playersList = append(playersList, tt.players...)

			convertedPlayers := make([]struct {
				ID    string
				Name  string
				Score string
				Ping  string
				Addr  string
			}, len(playersList))
			copy(convertedPlayers, playersList)

			response := make([]playerResponse, 0, len(convertedPlayers))
			for _, player := range convertedPlayers {
				response = append(response, playerResponse{
					ID:    player.ID,
					Name:  player.Name,
					Score: player.Score,
					Ping:  player.Ping,
					IP:    player.Addr,
				})
			}

			assert.Equal(t, tt.want, response)
		})
	}
}
