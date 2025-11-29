package getrconfeatures

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

//nolint:unparam
func allowUserAbilityForServer(
	t *testing.T,
	repo *inmemory.RBACRepository,
	userID uint,
	serverID uint,
	abilityName domain.AbilityName,
) {
	t.Helper()

	ability := domain.CreateAbilityForEntity(abilityName, serverID, domain.EntityTypeServer)
	require.NoError(t, repo.SaveAbility(context.Background(), &ability))

	require.NoError(t, repo.Allow(
		context.Background(),
		userID,
		domain.EntityTypeUser,
		[]domain.Ability{ability},
	))
}

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
		name                  string
		serverID              string
		setupAuth             func() context.Context
		setupRepo             func(*inmemory.ServerRepository, *inmemory.GameRepository, *inmemory.RBACRepository)
		expectedStatus        int
		wantError             string
		expectFeatures        bool
		expectedRcon          bool
		expectedPlayersManage bool
	}{
		{
			name:     "successful_features_retrieval__goldsource_engine",
			serverID: "1",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()

				game := &domain.Game{
					Code:   "cs",
					Name:   "Counter-Strike 1.6",
					Engine: "goldsource",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				server := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "short1",
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Test CS Server",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					Dir:        "/home/gameap/servers/test1",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 1)

				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, 1, domain.AbilityNameGameServerRconConsole)

				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, 1, domain.AbilityNameGameServerRconConsole)
			},
			expectedStatus:        http.StatusOK,
			expectFeatures:        true,
			expectedRcon:          true,
			expectedPlayersManage: true,
		},
		{
			name:     "successful_features_retrieval__source_engine",
			serverID: "2",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()

				game := &domain.Game{
					Code:   "cssource",
					Name:   "Counter-Strike: Source",
					Engine: "source",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				server := &domain.Server{
					ID:         2,
					UUID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:  "short2",
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Test CSS Server",
					GameID:     "cssource",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27016,
					Dir:        "/home/gameap/servers/test2",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 2)

				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, 2, domain.AbilityNameGameServerRconConsole)
			},
			expectedStatus:        http.StatusOK,
			expectFeatures:        true,
			expectedRcon:          true,
			expectedPlayersManage: false,
		},
		{
			name:     "successful_features_retrieval__unsupported_engine_supported_game_code",
			serverID: "3",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()

				game := &domain.Game{
					Code:   "minecraft",
					Name:   "Minecraft",
					Engine: "unsupported_engine",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				server := &domain.Server{
					ID:         3,
					UUID:       uuid.MustParse("33333333-3333-3333-3333-333333333333"),
					UUIDShort:  "short3",
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Test Minecraft Server",
					GameID:     "minecraft",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 25565,
					Dir:        "/home/gameap/servers/test3",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 3)

				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, 3, domain.AbilityNameGameServerRconConsole)
			},
			expectedStatus:        http.StatusOK,
			expectFeatures:        true,
			expectedRcon:          true,
			expectedPlayersManage: true,
		},
		{
			name:     "successful_features_retrieval__hl_game",
			serverID: "4",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()

				game := &domain.Game{
					Code:   "hl",
					Name:   "Half-Life",
					Engine: "goldsource",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				server := &domain.Server{
					ID:         4,
					UUID:       uuid.MustParse("44444444-4444-4444-4444-444444444444"),
					UUIDShort:  "short4",
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Test HL Server",
					GameID:     "hl",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27018,
					Dir:        "/home/gameap/servers/test4",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 4)

				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, 4, domain.AbilityNameGameServerRconConsole)
			},
			expectedStatus:        http.StatusOK,
			expectFeatures:        true,
			expectedRcon:          true,
			expectedPlayersManage: true,
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
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectFeatures: false,
		},
		{
			name:     "user_not_authenticated",
			serverID: "1",
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectFeatures: false,
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
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
			expectFeatures: false,
		},
		{
			name:     "user_does_not_have_access_to_server",
			serverID: "5",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				now := time.Now()

				game := &domain.Game{
					Code:   "test",
					Name:   "Test Game",
					Engine: "source",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				server := &domain.Server{
					ID:         5,
					UUID:       uuid.MustParse("55555555-5555-5555-5555-555555555555"),
					UUIDShort:  "short5",
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Other User Server",
					GameID:     "test",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27019,
					Dir:        "/home/gameap/servers/test5",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(2, 5)
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectFeatures: false,
		},
		{
			name:     "admin_can_access_any_server",
			serverID: "6",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "admin",
					Email: "admin@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()

				game := &domain.Game{
					Code:   "csgo",
					Name:   "Counter-Strike: Global Offensive",
					Engine: "source",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				server := &domain.Server{
					ID:         6,
					UUID:       uuid.MustParse("66666666-6666-6666-6666-666666666666"),
					UUIDShort:  "short6",
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Admin Server",
					GameID:     "csgo",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27020,
					Dir:        "/home/gameap/servers/test6",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 6)

				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			expectedStatus:        http.StatusOK,
			expectFeatures:        true,
			expectedRcon:          true,
			expectedPlayersManage: false,
		},
		{
			name:     "game_not_found_for_server",
			serverID: "7",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, rbacRepo *inmemory.RBACRepository) {
				now := time.Now()

				server := &domain.Server{
					ID:         7,
					UUID:       uuid.MustParse("77777777-7777-7777-7777-777777777777"),
					UUIDShort:  "short7",
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Server Without Game",
					GameID:     "nonexistent",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27021,
					Dir:        "/home/gameap/servers/test7",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 7)

				allowUserAbilityForServer(t, rbacRepo, testUser1.ID, 7, domain.AbilityNameGameServerRconConsole)
			},
			expectedStatus: http.StatusInternalServerError,
			wantError:      "Internal Server Error",
			expectFeatures: false,
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
			req := httptest.NewRequest(http.MethodGet, "/api/servers/"+tt.serverID+"/rcon/features", nil)
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

			if tt.expectFeatures {
				var features featuresResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &features))
				assert.Equal(t, tt.expectedRcon, features.Rcon)
				assert.Equal(t, tt.expectedPlayersManage, features.PlayersManage)
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

func TestNewFeaturesResponse(t *testing.T) {
	tests := []struct {
		name                  string
		game                  domain.Game
		expectedRcon          bool
		expectedPlayersManage bool
	}{
		{
			name:                  "goldsource_engine_with_cs_game",
			game:                  domain.Game{Code: "cs", Engine: "goldsource"},
			expectedRcon:          true,
			expectedPlayersManage: true,
		},
		{
			name:                  "goldsource_engine_with_hl_game",
			game:                  domain.Game{Code: "hl", Engine: "goldsource"},
			expectedRcon:          true,
			expectedPlayersManage: true,
		},
		{
			name:                  "goldsource_engine_with_tfc_game",
			game:                  domain.Game{Code: "tfc", Engine: "goldsource"},
			expectedRcon:          true,
			expectedPlayersManage: true,
		},
		{
			name:                  "goldsource_engine_with_unsupported_game",
			game:                  domain.Game{Code: "unknown_game", Engine: "goldsource"},
			expectedRcon:          true,
			expectedPlayersManage: false,
		},
		{
			name:                  "source_engine_with_unsupported_game",
			game:                  domain.Game{Code: "csgo", Engine: "source"},
			expectedRcon:          true,
			expectedPlayersManage: false,
		},
		{
			name:                  "unsupported_engine_with_supported_game",
			game:                  domain.Game{Code: "cs", Engine: "unreal"},
			expectedRcon:          true,
			expectedPlayersManage: true,
		},
		{
			name:                  "unsupported_engine_with_unsupported_game",
			game:                  domain.Game{Code: "rust", Engine: "rust"},
			expectedRcon:          false,
			expectedPlayersManage: false,
		},
		{
			name:                  "empty_engine_and_game",
			game:                  domain.Game{Code: "", Engine: ""},
			expectedRcon:          false,
			expectedPlayersManage: false,
		},
		{
			name:                  "cstrike_game_code",
			game:                  domain.Game{Code: "cstrike", Engine: "goldsource"},
			expectedRcon:          true,
			expectedPlayersManage: true,
		},
		{
			name:                  "valve_game_code",
			game:                  domain.Game{Code: "valve", Engine: "goldsource"},
			expectedRcon:          true,
			expectedPlayersManage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := newFeaturesResponse(tt.game)
			assert.Equal(t, tt.expectedRcon, response.Rcon)
			assert.Equal(t, tt.expectedPlayersManage, response.PlayersManage)
		})
	}
}
