package getservers

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
	Login: "usernoservers",
	Email: "noservers@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.ServerRepository)
		expectedStatus int
		wantError      string
		expectServers  bool
		expectedCount  int
	}{
		{
			name: "successful servers retrieval",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository) {
				now := time.Now()

				// Create test servers for the user
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
			},
			expectedStatus: http.StatusOK,
			expectServers:  true,
			expectedCount:  2,
		},
		{
			name: "user not authenticated",
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepo:      func(_ *inmemory.ServerRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
			expectServers:  false,
		},
		{
			name: "user with no servers",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "usernoservers",
					Email: "noservers@example.com",
					User:  &testUser2,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo:      func(_ *inmemory.ServerRepository) {},
			expectedStatus: http.StatusOK,
			expectServers:  true,
			expectedCount:  0,
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
				tt.setupRepo(serverRepo)
			}

			ctx := tt.setupAuth()
			req := httptest.NewRequest(http.MethodGet, "/api/servers", nil)
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
			}

			if tt.expectServers {
				var servers []serverResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &servers))
				assert.Len(t, servers, tt.expectedCount)

				if tt.expectedCount > 0 {
					// Verify first server structure
					server := servers[0]
					assert.NotZero(t, server.ID)
					assert.NotEmpty(t, server.Name)
					assert.NotEmpty(t, server.GameID)
					assert.NotZero(t, server.ServerPort)
				}
			}
		})
	}
}

func TestHandler_ServersResponseFields(t *testing.T) {
	userRepo := inmemory.NewUserRepository()
	serverRepo := inmemory.NewServerRepository()
	gameRepo := inmemory.NewGameRepository()
	rbacRepo := inmemory.NewRBACRepository()
	rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, gameRepo, rbacService, responder)

	now := time.Now()
	userName := "John Doe"
	user := &domain.User{
		ID:        1,
		Login:     "johndoe",
		Email:     "john@example.com",
		Name:      &userName,
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	require.NoError(t, userRepo.Save(context.Background(), user))

	queryPort := 27016
	rconPort := 27017
	rcon := "rconpassword"
	server := &domain.Server{
		ID:            1,
		UUID:          uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		UUIDShort:     "shorttest",
		Enabled:       true,
		Installed:     1,
		Blocked:       false,
		Name:          "Test Server",
		GameID:        "cs16",
		DSID:          1,
		GameModID:     2,
		ServerIP:      "192.168.1.100",
		ServerPort:    27015,
		QueryPort:     &queryPort,
		RconPort:      &rconPort,
		Rcon:          &rcon,
		Dir:           "/home/gameap/servers/testserver",
		ProcessActive: true,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))
	serverRepo.AddUserServer(1, 1)

	session := &auth.Session{
		Login: "johndoe",
		Email: "john@example.com",
		User:  user,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/servers", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var servers []serverResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &servers))

	require.Len(t, servers, 1)
	serverResp := servers[0]

	assert.Equal(t, uint(1), serverResp.ID)
	assert.True(t, serverResp.Enabled)
	assert.Equal(t, 1, serverResp.Installed)
	assert.False(t, serverResp.Blocked)
	assert.Equal(t, "Test Server", serverResp.Name)
	assert.Equal(t, "cs16", serverResp.GameID)
	assert.Equal(t, uint(1), serverResp.DSID)
	assert.Equal(t, uint(2), serverResp.GameModID)
	assert.Equal(t, "192.168.1.100", serverResp.ServerIP)
	assert.Equal(t, 27015, serverResp.ServerPort)
	require.NotNil(t, serverResp.QueryPort)
	assert.Equal(t, 27016, *serverResp.QueryPort)
	require.NotNil(t, serverResp.RconPort)
	assert.Equal(t, 27017, *serverResp.RconPort)
	assert.True(t, serverResp.ProcessActive)
}

func TestNewServersResponseFromServers(t *testing.T) {
	now := time.Now()
	servers := []domain.Server{
		{
			ID:            1,
			UUID:          uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			UUIDShort:     "short1",
			Enabled:       true,
			Name:          "Server 1",
			GameID:        "cs",
			ServerPort:    27015,
			ProcessActive: false,
			CreatedAt:     &now,
		},
		{
			ID:            2,
			UUID:          uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			UUIDShort:     "short2",
			Enabled:       false,
			Name:          "Server 2",
			GameID:        "hl",
			ServerPort:    27016,
			ProcessActive: true,
			CreatedAt:     &now,
		},
	}

	games := []domain.Game{
		{
			Code:          "cs",
			Name:          "Counter-Strike",
			Engine:        "GoldSource",
			EngineVersion: "1",
		},
		{
			Code:          "hl",
			Name:          "Half-Life",
			Engine:        "GoldSource",
			EngineVersion: "1",
		},
	}

	response := newServersResponseFromServers(servers, games)

	require.Len(t, response, 2)

	assert.Equal(t, uint(1), response[0].ID)
	assert.Equal(t, "Server 1", response[0].Name)
	assert.True(t, response[0].Enabled)
	assert.False(t, response[0].ProcessActive)
	assert.False(t, response[0].Online)
	require.NotNil(t, response[0].Game)
	assert.Equal(t, "cs", response[0].Game.Code)
	assert.Equal(t, "Counter-Strike", response[0].Game.Name)

	assert.Equal(t, uint(2), response[1].ID)
	assert.Equal(t, "Server 2", response[1].Name)
	assert.False(t, response[1].Enabled)
	assert.True(t, response[1].ProcessActive)
	assert.False(t, response[1].Online)
	require.NotNil(t, response[1].Game)
	assert.Equal(t, "hl", response[1].Game.Code)
	assert.Equal(t, "Half-Life", response[1].Game.Name)
}

func TestNewServerResponseFromServer(t *testing.T) {
	now := time.Now()
	queryPort := 27016
	server := &domain.Server{
		ID:            1,
		UUID:          uuid.MustParse("66666666-6666-6666-6666-666666666666"),
		UUIDShort:     "test-short",
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

	game := domain.Game{
		Code:          "cs",
		Name:          "Counter-Strike",
		Engine:        "GoldSource",
		EngineVersion: "1",
	}

	gamesByCode := map[string]*domain.Game{
		"cs": &game,
	}

	response := newServerResponseFromServer(server, gamesByCode)

	assert.Equal(t, uint(1), response.ID)
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
	assert.True(t, response.ProcessActive)
	assert.False(t, response.Online)
	require.NotNil(t, response.Game)
	assert.Equal(t, "cs", response.Game.Code)
	assert.Equal(t, "Counter-Strike", response.Game.Name)
	assert.Equal(t, "GoldSource", response.Game.Engine)
	assert.Equal(t, "1", response.Game.EngineVersion)
}
