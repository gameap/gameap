package getquery

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
	"github.com/gameap/gameap/pkg/quercon/query"
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
		setupRepo      func(*inmemory.ServerRepository, *inmemory.GameRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		expectResponse bool
		expectOnline   bool
	}{
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
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectResponse: false,
		},
		{
			name:           "user not authenticated",
			serverID:       "1",
			setupAuth:      context.Background,
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectResponse: false,
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
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid server id",
			expectResponse: false,
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
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
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
				serverRepo.AddUserServer(2, 2)
			},
			expectedStatus: http.StatusNotFound,
			wantError:      "server not found",
			expectResponse: false,
		},
		{
			name:     "admin can access any server",
			serverID: "3",
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
					Code:   "cs",
					Name:   "Counter-Strike",
					Engine: "source",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				server := &domain.Server{
					ID:            3,
					UUID:          uuid.MustParse("33333333-3333-3333-3333-333333333333"),
					UUIDShort:     "short3",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Server 3",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27017,
					Dir:           "/home/gameap/servers/test3",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 3)

				adminAbility := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
				require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), testUser2.ID, adminAbility.ID))
			},
			expectedStatus: http.StatusOK,
			expectResponse: true,
			expectOnline:   false,
		},
		{
			name:     "successful query with custom query port",
			serverID: "4",
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
				queryPort := 27018

				game := &domain.Game{
					Code:   "cs",
					Name:   "Counter-Strike",
					Engine: "source",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				server := &domain.Server{
					ID:            4,
					UUID:          uuid.MustParse("44444444-4444-4444-4444-444444444444"),
					UUIDShort:     "short4",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "Server with Query Port",
					GameID:        "cs",
					DSID:          1,
					GameModID:     1,
					ServerIP:      "127.0.0.1",
					ServerPort:    27017,
					QueryPort:     &queryPort,
					Dir:           "/home/gameap/servers/test4",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server))
				serverRepo.AddUserServer(1, 4)
			},
			expectedStatus: http.StatusOK,
			expectResponse: true,
			expectOnline:   false,
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
			req := httptest.NewRequest(http.MethodGet, "/api/servers/"+tt.serverID+"/query", nil)
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

			if tt.expectResponse {
				var response queryResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))

				if tt.expectOnline {
					assert.Equal(t, "online", response.Status)
					assert.NotNil(t, response.Hostname)
					assert.NotNil(t, response.Map)
					assert.NotNil(t, response.Players)
					assert.NotNil(t, response.JoinLink)
				} else {
					assert.Equal(t, "offline", response.Status)
					assert.Nil(t, response.Hostname)
					assert.Nil(t, response.Map)
					assert.Nil(t, response.Players)
				}
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
	assert.Equal(t, gameRepo, handler.gameRepo)
	assert.Equal(t, responder, handler.responder)
}

func TestNewQueryResponse(t *testing.T) {
	tests := []struct {
		name         string
		result       *query.Result
		server       *domain.Server
		wantStatus   string
		wantHostname *string
		wantMap      *string
		wantPlayers  *string
		wantJoinLink *string
	}{
		{
			name:   "nil result returns offline",
			result: nil,
			server: &domain.Server{
				ServerIP:   "127.0.0.1",
				ServerPort: 27015,
			},
			wantStatus: "offline",
		},
		{
			name: "offline result returns offline",
			result: &query.Result{
				Online: false,
			},
			server: &domain.Server{
				ServerIP:   "127.0.0.1",
				ServerPort: 27015,
			},
			wantStatus: "offline",
		},
		{
			name: "online result returns full data",
			result: &query.Result{
				Online:        true,
				Name:          "Test Server",
				Map:           "de_dust2",
				PlayersNum:    5,
				MaxPlayersNum: 32,
			},
			server: &domain.Server{
				ServerIP:   "192.168.1.1",
				ServerPort: 27015,
			},
			wantStatus:   "online",
			wantHostname: stringPtr("Test Server"),
			wantMap:      stringPtr("de_dust2"),
			wantPlayers:  stringPtr("5/32"),
		},
		{
			name: "online result with zero players",
			result: &query.Result{
				Online:        true,
				Name:          "Empty Server",
				Map:           "cs_office",
				PlayersNum:    0,
				MaxPlayersNum: 16,
			},
			server: &domain.Server{
				ServerIP:   "10.0.0.1",
				ServerPort: 27016,
			},
			wantStatus:   "online",
			wantHostname: stringPtr("Empty Server"),
			wantMap:      stringPtr("cs_office"),
			wantPlayers:  stringPtr("0/16"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := newQueryResponse(tt.result, tt.server)

			assert.Equal(t, tt.wantStatus, response.Status)

			if tt.wantHostname != nil {
				require.NotNil(t, response.Hostname)
				assert.Equal(t, *tt.wantHostname, *response.Hostname)
			} else {
				assert.Nil(t, response.Hostname)
			}

			if tt.wantMap != nil {
				require.NotNil(t, response.Map)
				assert.Equal(t, *tt.wantMap, *response.Map)
			} else {
				assert.Nil(t, response.Map)
			}

			if tt.wantPlayers != nil {
				require.NotNil(t, response.Players)
				assert.Equal(t, *tt.wantPlayers, *response.Players)
			} else {
				assert.Nil(t, response.Players)
			}

			if tt.wantJoinLink != nil {
				require.NotNil(t, response.JoinLink)
				assert.Equal(t, *tt.wantJoinLink, *response.JoinLink)
			} else {
				assert.Nil(t, response.JoinLink)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
