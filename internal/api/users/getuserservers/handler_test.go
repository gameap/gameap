package getuserservers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
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
	Login: "usernoservers",
	Email: "noservers@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func() context.Context
		setupRepos     func(*inmemory.ServerRepository, *inmemory.GameRepository, *inmemory.GameModRepository)
		expectedStatus int
		wantError      string
		expectServers  bool
		expectedCount  int
	}{
		{
			name: "successful servers retrieval with game and game mod",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				serverRepo *inmemory.ServerRepository,
				gameRepo *inmemory.GameRepository,
				gameModRepo *inmemory.GameModRepository,
			) {
				now := time.Now()

				game := &domain.Game{
					Code:          "minecraft",
					Name:          "Minecraft",
					Engine:        "Minecraft",
					EngineVersion: "1",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				gameMod := &domain.GameMod{
					ID:       33,
					GameCode: "minecraft",
					Name:     "Multicore",
				}
				require.NoError(t, gameModRepo.Save(context.Background(), gameMod))

				server1 := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("9fe8c1a1-41a0-4a16-9f86-553fe3f1f3f6"),
					UUIDShort:  "9fe8c1a1",
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "First",
					GameID:     "minecraft",
					DSID:       2,
					GameModID:  33,
					ServerIP:   "172.17.0.2",
					ServerPort: 25565,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				serverRepo.AddUserServer(1, 1)
			},
			expectedStatus: http.StatusOK,
			expectServers:  true,
			expectedCount:  1,
		},
		{
			name: "user not authenticated",
			//nolint:gocritic
			setupAuth: func() context.Context {
				return context.Background()
			},
			setupRepos:     func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {},
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
			setupRepos:     func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.GameModRepository) {},
			expectedStatus: http.StatusOK,
			expectServers:  true,
			expectedCount:  0,
		},
		{
			name: "multiple servers with sorting by name",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepos: func(
				serverRepo *inmemory.ServerRepository,
				gameRepo *inmemory.GameRepository,
				gameModRepo *inmemory.GameModRepository,
			) {
				now := time.Now()

				game := &domain.Game{
					Code:          "cs",
					Name:          "Counter-Strike 1.6",
					Engine:        "GoldSource",
					EngineVersion: "1",
				}
				require.NoError(t, gameRepo.Save(context.Background(), game))

				gameMod := &domain.GameMod{
					ID:       1,
					GameCode: "cs",
					Name:     "Classic",
				}
				require.NoError(t, gameModRepo.Save(context.Background(), gameMod))

				server1 := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "11111111",
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Zebra Server",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				server2 := &domain.Server{
					ID:         2,
					UUID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:  "22222222",
					Enabled:    true,
					Installed:  1,
					Blocked:    false,
					Name:       "Alpha Server",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27016,
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				require.NoError(t, serverRepo.Save(context.Background(), server2))

				serverRepo.AddUserServer(1, 1)
				serverRepo.AddUserServer(1, 2)
			},
			expectedStatus: http.StatusOK,
			expectServers:  true,
			expectedCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			gameRepo := inmemory.NewGameRepository()
			gameModRepo := inmemory.NewGameModRepository()
			responder := api.NewResponder()

			handler := NewHandler(serverRepo, gameRepo, gameModRepo, responder)

			if tt.setupRepos != nil {
				tt.setupRepos(serverRepo, gameRepo, gameModRepo)
			}

			ctx := tt.setupAuth()
			req := httptest.NewRequest(http.MethodGet, "/api/users/1/servers", nil)
			req = req.WithContext(ctx)
			req = mux.SetURLVars(req, map[string]string{"id": "1"})
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
					server := servers[0]
					assert.NotZero(t, server.ID)
					assert.NotEmpty(t, server.UUID)
					assert.NotEmpty(t, server.Name)
					assert.NotEmpty(t, server.GameID)
					assert.NotZero(t, server.ServerPort)
				}

				if tt.name == "multiple servers with sorting by name" && tt.expectedCount == 2 {
					assert.Equal(t, "Alpha Server", servers[0].Name)
					assert.Equal(t, "Zebra Server", servers[1].Name)
				}
			}
		})
	}
}

func TestHandler_ServersResponseFields(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	gameRepo := inmemory.NewGameRepository()
	gameModRepo := inmemory.NewGameModRepository()
	responder := api.NewResponder()

	handler := NewHandler(serverRepo, gameRepo, gameModRepo, responder)

	now := time.Now()

	game := &domain.Game{
		Code:          "minecraft",
		Name:          "Minecraft",
		Engine:        "Minecraft",
		EngineVersion: "1",
	}
	require.NoError(t, gameRepo.Save(context.Background(), game))

	gameMod := &domain.GameMod{
		ID:       33,
		GameCode: "minecraft",
		Name:     "Multicore",
	}
	require.NoError(t, gameModRepo.Save(context.Background(), gameMod))

	queryPort := 25565
	rconPort := 25566
	server := &domain.Server{
		ID:         1,
		UUID:       uuid.MustParse("9fe8c1a1-41a0-4a16-9f86-553fe3f1f3f6"),
		UUIDShort:  "9fe8c1a1",
		Enabled:    true,
		Installed:  1,
		Blocked:    false,
		Name:       "First",
		GameID:     "minecraft",
		DSID:       2,
		GameModID:  33,
		ServerIP:   "172.17.0.2",
		ServerPort: 25565,
		QueryPort:  &queryPort,
		RconPort:   &rconPort,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))
	serverRepo.AddUserServer(1, 1)

	session := &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/users/1/servers", nil)
	req = req.WithContext(ctx)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var servers []serverResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &servers))

	require.Len(t, servers, 1)
	serverResp := servers[0]

	assert.Equal(t, uint(1), serverResp.ID)
	assert.Equal(t, "9fe8c1a1-41a0-4a16-9f86-553fe3f1f3f6", serverResp.UUID)
	assert.Equal(t, "9fe8c1a1", serverResp.UUIDShort)
	assert.True(t, serverResp.Enabled)
	assert.Equal(t, 1, serverResp.Installed)
	assert.False(t, serverResp.Blocked)
	assert.Equal(t, "First", serverResp.Name)
	assert.Equal(t, "minecraft", serverResp.GameID)
	assert.Equal(t, uint(2), serverResp.DSID)
	assert.Equal(t, uint(33), serverResp.GameModID)
	assert.Equal(t, "172.17.0.2", serverResp.ServerIP)
	assert.Equal(t, 25565, serverResp.ServerPort)
	require.NotNil(t, serverResp.QueryPort)
	assert.Equal(t, 25565, *serverResp.QueryPort)
	require.NotNil(t, serverResp.RconPort)
	assert.Equal(t, 25566, *serverResp.RconPort)

	require.NotNil(t, serverResp.Game)
	assert.Equal(t, "minecraft", serverResp.Game.Code)
	assert.Equal(t, "Minecraft", serverResp.Game.Name)
	assert.Equal(t, "Minecraft", serverResp.Game.Engine)
	assert.Equal(t, "1", serverResp.Game.EngineVersion)

	require.NotNil(t, serverResp.GameMod)
	assert.Equal(t, uint(33), serverResp.GameMod.ID)
	assert.Equal(t, "Multicore", serverResp.GameMod.Name)
}

func TestNewServersResponseFromServers(t *testing.T) {
	now := time.Now()

	games := []domain.Game{
		{
			Code:          "cs",
			Name:          "Counter-Strike 1.6",
			Engine:        "GoldSource",
			EngineVersion: "1",
		},
	}

	gameMods := []domain.GameMod{
		{
			ID:       1,
			GameCode: "cs",
			Name:     "Classic",
		},
	}

	servers := []domain.Server{
		{
			ID:         1,
			UUID:       uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			UUIDShort:  "44444444",
			Enabled:    true,
			Name:       "Server 1",
			GameID:     "cs",
			GameModID:  1,
			ServerPort: 27015,
			CreatedAt:  &now,
		},
		{
			ID:         2,
			UUID:       uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			UUIDShort:  "55555555",
			Enabled:    false,
			Name:       "Server 2",
			GameID:     "cs",
			GameModID:  1,
			ServerPort: 27016,
			CreatedAt:  &now,
		},
	}

	response := newServersResponseFromServers(servers, games, gameMods)

	require.Len(t, response, 2)

	assert.Equal(t, uint(1), response[0].ID)
	assert.Equal(t, "44444444-4444-4444-4444-444444444444", response[0].UUID)
	assert.Equal(t, "Server 1", response[0].Name)
	assert.True(t, response[0].Enabled)

	assert.Equal(t, uint(2), response[1].ID)
	assert.Equal(t, "55555555-5555-5555-5555-555555555555", response[1].UUID)
	assert.Equal(t, "Server 2", response[1].Name)
	assert.False(t, response[1].Enabled)

	for _, server := range response {
		require.NotNil(t, server.Game)
		assert.Equal(t, "cs", server.Game.Code)
		assert.Equal(t, "Counter-Strike 1.6", server.Game.Name)

		require.NotNil(t, server.GameMod)
		assert.Equal(t, uint(1), server.GameMod.ID)
		assert.Equal(t, "Classic", server.GameMod.Name)
	}
}

func TestNewServerResponseFromServer(t *testing.T) {
	now := time.Now()
	queryPort := 27016

	gameMap := map[string]domain.Game{
		"cs": {
			Code:          "cs",
			Name:          "Counter-Strike 1.6",
			Engine:        "GoldSource",
			EngineVersion: "1",
		},
	}

	gameModMap := map[uint]domain.GameMod{
		1: {
			ID:       1,
			GameCode: "cs",
			Name:     "Classic",
		},
	}

	server := &domain.Server{
		ID:         1,
		UUID:       uuid.MustParse("66666666-6666-6666-6666-666666666666"),
		UUIDShort:  "66666666",
		Enabled:    true,
		Installed:  1,
		Blocked:    false,
		Name:       "Test Server",
		GameID:     "cs",
		DSID:       1,
		GameModID:  1,
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
		QueryPort:  &queryPort,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}

	response := newServerResponseFromServer(server, gameMap, gameModMap)

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "66666666-6666-6666-6666-666666666666", response.UUID)
	assert.Equal(t, "66666666", response.UUIDShort)
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

	require.NotNil(t, response.Game)
	assert.Equal(t, "cs", response.Game.Code)
	assert.Equal(t, "Counter-Strike 1.6", response.Game.Name)
	assert.Equal(t, "GoldSource", response.Game.Engine)
	assert.Equal(t, "1", response.Game.EngineVersion)

	require.NotNil(t, response.GameMod)
	assert.Equal(t, uint(1), response.GameMod.ID)
	assert.Equal(t, "Classic", response.GameMod.Name)
}

func TestServerResponseWithMissingGameAndGameMod(t *testing.T) {
	now := time.Now()

	server := &domain.Server{
		ID:         1,
		UUID:       uuid.MustParse("77777777-7777-7777-7777-777777777777"),
		UUIDShort:  "77777777",
		Enabled:    true,
		Installed:  1,
		Blocked:    false,
		Name:       "Test Server",
		GameID:     "nonexistent",
		DSID:       1,
		GameModID:  999,
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}

	response := newServerResponseFromServer(server, map[string]domain.Game{}, map[uint]domain.GameMod{})

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "nonexistent", response.GameID)
	assert.Equal(t, uint(999), response.GameModID)
	assert.Nil(t, response.Game)
	assert.Nil(t, response.GameMod)
}
