package searchservers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
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

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		query          string
		omitQuery      bool // if true, don't include ?q= in URL
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.ServerRepository, *inmemory.GameRepository)
		expectedStatus int
		wantError      string
		wantServerIDs  []uint
	}{
		{
			name:  "successful search with matching results",
			query: "TestServer",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository) {
				now := time.Now()

				// Setup games
				require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
					Code: "cs",
					Name: "Counter-Strike",
				}))
				require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
					Code: "cs16",
					Name: "Counter-Strike 1.6",
				}))
				require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
					Code: "hl",
					Name: "Half-Life",
				}))

				server1 := &domain.Server{
					ID:            1,
					UUID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:     "short1",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "TestServer1",
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
					Name:          "TestServer2",
					GameID:        "cs16",
					DSID:          1,
					GameModID:     2,
					ServerIP:      "192.168.1.100",
					ServerPort:    27016,
					Dir:           "/home/gameap/servers/test2",
					ProcessActive: true,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}
				server3 := &domain.Server{
					ID:            3,
					UUID:          uuid.MustParse("33333333-3333-3333-3333-333333333333"),
					UUIDShort:     "short3",
					Enabled:       true,
					Installed:     1,
					Blocked:       false,
					Name:          "AnotherServer",
					GameID:        "hl",
					DSID:          1,
					GameModID:     3,
					ServerIP:      "10.0.0.1",
					ServerPort:    27017,
					Dir:           "/home/gameap/servers/test3",
					ProcessActive: false,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				require.NoError(t, serverRepo.Save(context.Background(), server2))
				require.NoError(t, serverRepo.Save(context.Background(), server3))
			},
			expectedStatus: http.StatusOK,
			wantServerIDs:  []uint{1, 2},
		},
		{
			name:  "search by IP address",
			query: "192.168",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository) {
				now := time.Now()

				// Setup game
				require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
					Code: "cs",
					Name: "Counter-Strike",
				}))

				server1 := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "short1",
					Name:       "Server1",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "192.168.1.100",
					ServerPort: 27015,
					Dir:        "/test",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				server2 := &domain.Server{
					ID:         2,
					UUID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:  "22222222",
					Name:       "Server2",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "10.20.30.40",
					ServerPort: 27016,
					Dir:        "/test",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				require.NoError(t, serverRepo.Save(context.Background(), server2))
			},
			expectedStatus: http.StatusOK,
			wantServerIDs:  []uint{1},
		},
		{
			name:  "search by port",
			query: "27015",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository) {
				now := time.Now()

				// Setup games
				require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
					Code: "cs",
					Name: "Counter-Strike",
				}))
				require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
					Code: "cs16",
					Name: "Counter-Strike 1.6",
				}))

				server1 := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "short1",
					Name:       "Server1",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					Dir:        "/test",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}
				server2 := &domain.Server{
					ID:         2,
					UUID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
					UUIDShort:  "short2",
					Name:       "Server2",
					GameID:     "cs16",
					DSID:       1,
					GameModID:  2,
					ServerIP:   "127.0.0.2",
					ServerPort: 27016,
					Dir:        "/test2",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
				require.NoError(t, serverRepo.Save(context.Background(), server2))
			},
			expectedStatus: http.StatusOK,
			wantServerIDs:  []uint{1},
		},
		{
			name:  "search with short query returns up to 10 servers",
			query: "ab",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository) {
				now := time.Now()

				// Setup game
				require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
					Code: "cs",
					Name: "Counter-Strike",
				}))

				for i := 1; i <= 15; i++ {
					server := &domain.Server{
						ID:         uint(i),
						UUID:       uuid.New(),
						UUIDShort:  uuid.New().String()[0:8],
						Name:       "Server" + string(rune(i)),
						GameID:     "cs",
						DSID:       1,
						GameModID:  1,
						ServerIP:   "127.0.0.1",
						ServerPort: 27000 + i,
						Dir:        "/test",
						CreatedAt:  &now,
						UpdatedAt:  &now,
					}
					require.NoError(t, serverRepo.Save(context.Background(), server))
				}
			},
			expectedStatus: http.StatusOK,
			wantServerIDs:  nil, // Will check count instead of exact IDs
		},
		{
			name: "missing query parameter",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			omitQuery: true,
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository) {
				now := time.Now()

				// Setup game
				require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
					Code: "cs",
					Name: "Counter-Strike",
				}))

				for i := 1; i <= 15; i++ {
					server := &domain.Server{
						ID:         uint(i),
						UUID:       uuid.New(),
						UUIDShort:  uuid.New().String()[0:8],
						Name:       "Server" + string(rune(i)),
						GameID:     "cs",
						DSID:       1,
						GameModID:  1,
						ServerIP:   "127.0.0.1",
						ServerPort: 27000 + i,
						Dir:        "/test",
						CreatedAt:  &now,
						UpdatedAt:  &now,
					}
					require.NoError(t, serverRepo.Save(context.Background(), server))
				}
			},
			expectedStatus: http.StatusOK,
			wantServerIDs:  nil, // Will check count instead of exact IDs
		},
		{
			name:  "empty query parameter",
			query: "",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository) {
				now := time.Now()

				// Setup game
				require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
					Code: "cs",
					Name: "Counter-Strike",
				}))

				for i := 1; i <= 15; i++ {
					server := &domain.Server{
						ID:         uint(i),
						UUID:       uuid.New(),
						UUIDShort:  uuid.New().String()[0:8],
						Name:       "Server" + string(rune(i)),
						GameID:     "cs",
						DSID:       1,
						GameModID:  1,
						ServerIP:   "127.0.0.1",
						ServerPort: 27000 + i,
						Dir:        "/test",
						CreatedAt:  &now,
						UpdatedAt:  &now,
					}
					require.NoError(t, serverRepo.Save(context.Background(), server))
				}
			},
			expectedStatus: http.StatusOK,
			wantServerIDs:  nil, // Will check count instead of exact IDs
		},
		{
			name:           "user not authenticated",
			query:          "test",
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:  "no results found",
			query: "nonexistent",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, gameRepo *inmemory.GameRepository) {
				now := time.Now()

				// Setup game
				require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
					Code: "cs",
					Name: "Counter-Strike",
				}))

				server1 := &domain.Server{
					ID:         1,
					UUID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
					UUIDShort:  "short1",
					Name:       "TestServer",
					GameID:     "cs",
					DSID:       1,
					GameModID:  1,
					ServerIP:   "127.0.0.1",
					ServerPort: 27015,
					Dir:        "/test",
					CreatedAt:  &now,
					UpdatedAt:  &now,
				}

				require.NoError(t, serverRepo.Save(context.Background(), server1))
			},
			expectedStatus: http.StatusOK,
			wantServerIDs:  []uint{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverRepo := inmemory.NewServerRepository()
			gameRepo := inmemory.NewGameRepository()
			responder := api.NewResponder()
			handler := NewHandler(serverRepo, gameRepo, responder)

			if tt.setupRepo != nil {
				tt.setupRepo(serverRepo, gameRepo)
			}

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			url := "/api/servers/search"
			if !tt.omitQuery {
				url += "?q=" + tt.query
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			//nolint:nestif
			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])

				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)
			} else {
				var servers []searchServerResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &servers))

				// Extract actual server IDs
				var actualServerIDs []uint
				for _, server := range servers {
					actualServerIDs = append(actualServerIDs, server.ID)
				}

				slices.Sort(actualServerIDs)

				// Assert server IDs match expected (if specific IDs are provided)
				if tt.wantServerIDs != nil {
					// For empty slices, both nil and empty slice should be treated as equal
					if len(tt.wantServerIDs) == 0 {
						assert.Empty(t, actualServerIDs)
					} else {
						assert.Equal(t, tt.wantServerIDs, actualServerIDs)
					}
				} else {
					// If no specific IDs are expected, just check the count (for short queries)
					assert.LessOrEqual(t, len(actualServerIDs), 10)
				}

				// Additional validations for non-empty results
				if len(tt.wantServerIDs) > 0 && len(servers) > 0 {
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

func TestHandler_SearchResponseFields(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	gameRepo := inmemory.NewGameRepository()
	responder := api.NewResponder()
	handler := NewHandler(serverRepo, gameRepo, responder)

	// Setup game
	require.NoError(t, gameRepo.Save(context.Background(), &domain.Game{
		Code: "cs16",
		Name: "Counter-Strike 1.6",
	}))

	now := time.Now()
	server := &domain.Server{
		ID:         1,
		UUID:       uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		UUIDShort:  "shorttest",
		Enabled:    true,
		Installed:  1,
		Blocked:    false,
		Name:       "Test Server",
		GameID:     "cs16",
		DSID:       1,
		GameModID:  2,
		ServerIP:   "192.168.1.100",
		ServerPort: 27015,
		Dir:        "/home/gameap/servers/testserver",
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	require.NoError(t, serverRepo.Save(context.Background(), server))

	session := &auth.Session{
		Login: "testuser",
		Email: "test@example.com",
		User:  &testUser1,
	}
	ctx := auth.ContextWithSession(context.Background(), session)

	req := httptest.NewRequest(http.MethodGet, "/api/servers/search?q=Test", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var servers []searchServerResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &servers))

	require.Len(t, servers, 1)
	serverResp := servers[0]

	assert.Equal(t, uint(1), serverResp.ID)
	assert.Equal(t, "Test Server", serverResp.Name)
	assert.Equal(t, "cs16", serverResp.GameID)
	assert.Equal(t, uint(2), serverResp.GameModID)
	assert.Equal(t, "192.168.1.100", serverResp.ServerIP)
	assert.Equal(t, 27015, serverResp.ServerPort)
	assert.NotNil(t, serverResp.Game)
	assert.Equal(t, "cs16", serverResp.Game.Code)
	assert.Equal(t, "Counter-Strike 1.6", serverResp.Game.Name)
}

func TestNewSearchServersResponseFromServers(t *testing.T) {
	servers := []*domain.Server{
		{
			ID:         1,
			Name:       "Server 1",
			GameID:     "cs",
			GameModID:  1,
			ServerIP:   "127.0.0.1",
			ServerPort: 27015,
		},
		{
			ID:         2,
			Name:       "Server 2",
			GameID:     "hl",
			GameModID:  2,
			ServerIP:   "192.168.1.100",
			ServerPort: 27016,
		},
	}

	games := map[string]domain.Game{
		"cs": {
			Code: "cs",
			Name: "Counter-Strike",
		},
		"hl": {
			Code: "hl",
			Name: "Half-Life",
		},
	}

	response := newSearchServersResponseFromServers(servers, games)

	require.Len(t, response, 2)

	assert.Equal(t, uint(1), response[0].ID)
	assert.Equal(t, "Server 1", response[0].Name)
	assert.Equal(t, "cs", response[0].GameID)
	assert.Equal(t, uint(1), response[0].GameModID)
	assert.Equal(t, "127.0.0.1", response[0].ServerIP)
	assert.Equal(t, 27015, response[0].ServerPort)
	assert.NotNil(t, response[0].Game)
	assert.Equal(t, "cs", response[0].Game.Code)
	assert.Equal(t, "Counter-Strike", response[0].Game.Name)

	assert.Equal(t, uint(2), response[1].ID)
	assert.Equal(t, "Server 2", response[1].Name)
	assert.Equal(t, "hl", response[1].GameID)
	assert.Equal(t, uint(2), response[1].GameModID)
	assert.Equal(t, "192.168.1.100", response[1].ServerIP)
	assert.Equal(t, 27016, response[1].ServerPort)
	assert.NotNil(t, response[1].Game)
	assert.Equal(t, "hl", response[1].Game.Code)
	assert.Equal(t, "Half-Life", response[1].Game.Name)
}

func TestNewSearchServerResponseFromServer(t *testing.T) {
	server := &domain.Server{
		ID:         1,
		Name:       "Test Server",
		GameID:     "cs",
		GameModID:  1,
		ServerIP:   "127.0.0.1",
		ServerPort: 27015,
	}

	game := &domain.Game{
		Code: "cs",
		Name: "Counter-Strike",
	}

	response := newSearchServerResponseFromServer(server, game)

	assert.Equal(t, uint(1), response.ID)
	assert.Equal(t, "Test Server", response.Name)
	assert.Equal(t, "cs", response.GameID)
	assert.Equal(t, uint(1), response.GameModID)
	assert.Equal(t, "127.0.0.1", response.ServerIP)
	assert.Equal(t, 27015, response.ServerPort)
	assert.NotNil(t, response.Game)
	assert.Equal(t, "cs", response.Game.Code)
	assert.Equal(t, "Counter-Strike", response.Game.Name)
}
