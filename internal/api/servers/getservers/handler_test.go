package getservers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/base"
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

var testAdminUser = domain.User{
	ID:    3,
	Login: "admin",
	Email: "admin@example.com",
}

func setupAdminUser(t *testing.T, rbacRepo *inmemory.RBACRepository, userID uint) {
	t.Helper()

	adminAbility := &domain.Ability{
		ID:   100,
		Name: domain.AbilityNameAdminRolesPermissions,
	}
	require.NoError(t, rbacRepo.SaveAbility(context.Background(), adminAbility))
	require.NoError(t, rbacRepo.AssignAbilityToUser(context.Background(), userID, adminAbility.ID))
}

func sessionFor(user *domain.User) context.Context {
	session := &auth.Session{
		Login: user.Login,
		Email: user.Email,
		User:  user,
	}

	return auth.ContextWithSession(context.Background(), session)
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupAuth      func() context.Context
		setupRepo      func(*inmemory.ServerRepository, *inmemory.GameRepository, *inmemory.RBACRepository)
		expectedStatus int
		wantError      string
		checkResponse  func(*testing.T, *base.PaginatedResponse[serverResponse])
	}{
		{
			name: "successful_default_pagination",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "short1", Enabled: true, Name: "Server 1",
					GameID: "cs", DSID: 1, GameModID: 1, ServerIP: "127.0.0.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "short2", Enabled: true, Name: "Server 2",
					GameID: "cs", DSID: 1, GameModID: 1, ServerIP: "127.0.0.1", ServerPort: 27016,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, 1, resp.CurrentPage)
				assert.Equal(t, base.DefaultPageSize, resp.PerPage)
				assert.Equal(t, 2, resp.Total)
				assert.Equal(t, 1, resp.LastPage)
				assert.Equal(t, 1, resp.From)
				require.Len(t, resp.Data, 2)
			},
		},
		{
			name:        "successful_custom_page_size_first_page",
			queryParams: "?page[size]=1&page[number]=1",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "S1", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "s2", Name: "S2", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27016,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, 1, resp.CurrentPage)
				assert.Equal(t, 1, resp.PerPage)
				assert.Equal(t, 2, resp.Total)
				assert.Equal(t, 2, resp.LastPage)
				assert.Equal(t, 1, resp.From)
				require.Len(t, resp.Data, 1)
			},
		},
		{
			name:        "successful_custom_page_size_second_page",
			queryParams: "?page[size]=1&page[number]=2",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "S1", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "s2", Name: "S2", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27016,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, 2, resp.CurrentPage)
				assert.Equal(t, 1, resp.PerPage)
				assert.Equal(t, 2, resp.Total)
				assert.Equal(t, 2, resp.LastPage)
				assert.Equal(t, 2, resp.From)
				require.Len(t, resp.Data, 1)
			},
		},
		{
			name:        "page_size_over_cap_is_clamped_to_max",
			queryParams: "?page[size]=1000",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "S1", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, base.MaxPageSize, resp.PerPage,
					"page[size] above the cap must be clamped to MaxPageSize, not rejected")
				assert.Equal(t, 1, resp.CurrentPage)
			},
		},
		{
			name:        "successful_filter_by_ds_id",
			queryParams: "?filter[ds_id]=2",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "S1", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "s2", Name: "S2", GameID: "cs", DSID: 2, ServerIP: "1.1.1.1", ServerPort: 27016,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, 1, resp.Total)
				require.Len(t, resp.Data, 1)
				assert.Equal(t, uint(2), resp.Data[0].ID)
				assert.Equal(t, uint(2), resp.Data[0].DSID)
			},
		},
		{
			name:        "successful_filter_by_game_id",
			queryParams: "?filter[game_id]=cs,hl",
			setupAuth: func() context.Context {
				return sessionFor(&testAdminUser)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, rbacRepo *inmemory.RBACRepository) {
				setupAdminUser(t, rbacRepo, testAdminUser.ID)
				saveServer(t, serverRepo, testAdminUser.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "S1", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testAdminUser.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "s2", Name: "S2", GameID: "hl", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27016,
				})
				saveServer(t, serverRepo, testAdminUser.ID, &domain.Server{
					ID: 3, UID: uuid.New(), UUIDShort: "s3", Name: "S3", GameID: "csgo", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27017,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, 2, resp.Total)
				require.Len(t, resp.Data, 2)
			},
		},
		{
			name:        "successful_filter_by_enabled_true",
			queryParams: "?filter[enabled]=true",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "S1", Enabled: true, GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "s2", Name: "S2", Enabled: false, GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27016,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, 1, resp.Total)
				require.Len(t, resp.Data, 1)
				assert.True(t, resp.Data[0].Enabled)
			},
		},
		{
			name:        "successful_filter_by_enabled_false",
			queryParams: "?filter[enabled]=false",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "S1", Enabled: true, GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "s2", Name: "S2", Enabled: false, GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27016,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, 1, resp.Total)
				require.Len(t, resp.Data, 1)
				assert.False(t, resp.Data[0].Enabled)
			},
		},
		{
			name:        "successful_sort_by_name_asc",
			queryParams: "?sort=name",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "Zebra", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "s2", Name: "Alpha", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27016,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				require.Len(t, resp.Data, 2)
				assert.Equal(t, "Alpha", resp.Data[0].Name)
				assert.Equal(t, "Zebra", resp.Data[1].Name)
			},
		},
		{
			name:        "successful_sort_by_id_asc_default",
			queryParams: "",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "S1", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "s2", Name: "S2", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27016,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				require.Len(t, resp.Data, 2)
				assert.Equal(t, uint(1), resp.Data[0].ID)
				assert.Equal(t, uint(2), resp.Data[1].ID)
			},
		},
		{
			name: "successful_empty_result",
			setupAuth: func() context.Context {
				return sessionFor(&testUser2)
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, 1, resp.CurrentPage)
				assert.Equal(t, base.DefaultPageSize, resp.PerPage)
				assert.Equal(t, 0, resp.Total)
				assert.Equal(t, 1, resp.LastPage)
				assert.Equal(t, 0, resp.From)
				require.Len(t, resp.Data, 0)
			},
		},
		{
			name: "successful_non_admin_sees_only_own_servers",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "Own", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testAdminUser.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "s2", Name: "NotOwn", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27016,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, 1, resp.Total)
				require.Len(t, resp.Data, 1)
				assert.Equal(t, uint(1), resp.Data[0].ID)
			},
		},
		{
			name: "successful_admin_sees_all_servers",
			setupAuth: func() context.Context {
				return sessionFor(&testAdminUser)
			},
			setupRepo: func(serverRepo *inmemory.ServerRepository, _ *inmemory.GameRepository, rbacRepo *inmemory.RBACRepository) {
				setupAdminUser(t, rbacRepo, testAdminUser.ID)
				saveServer(t, serverRepo, testUser1.ID, &domain.Server{
					ID: 1, UID: uuid.New(), UUIDShort: "s1", Name: "User1Own", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27015,
				})
				saveServer(t, serverRepo, testUser2.ID, &domain.Server{
					ID: 2, UID: uuid.New(), UUIDShort: "s2", Name: "User2Own", GameID: "cs", DSID: 1, ServerIP: "1.1.1.1", ServerPort: 27016,
				})
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *base.PaginatedResponse[serverResponse]) {
				t.Helper()
				assert.Equal(t, 2, resp.Total)
				require.Len(t, resp.Data, 2)
			},
		},
		{
			name:           "error_user_not_authenticated",
			setupAuth:      context.Background,
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusUnauthorized,
			wantError:      "user not authenticated",
		},
		{
			name:        "error_invalid_page_size",
			queryParams: "?page[size]=invalid",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "failed to read input",
		},
		{
			name:        "error_negative_page_size",
			queryParams: "?page[size]=-1",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "page[size] must be positive",
		},
		{
			name:        "error_zero_page_number",
			queryParams: "?page[number]=0",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "page[number] must be positive",
		},
		{
			name:        "error_invalid_enabled_filter",
			queryParams: "?filter[enabled]=notbool",
			setupAuth: func() context.Context {
				return sessionFor(&testUser1)
			},
			setupRepo:      func(_ *inmemory.ServerRepository, _ *inmemory.GameRepository, _ *inmemory.RBACRepository) {},
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid filter[enabled] value",
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

			tt.setupRepo(serverRepo, gameRepo, rbacRepo)

			ctx := tt.setupAuth()
			req := httptest.NewRequest(http.MethodGet, "/api/servers"+tt.queryParams, nil)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.wantError != "" {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
				assert.Equal(t, "error", errResp["status"])
				errMsg, ok := errResp["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errMsg, tt.wantError)

				return
			}

			var resp base.PaginatedResponse[serverResponse]
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			tt.checkResponse(t, &resp)
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
		UID:           uuid.MustParse("33333333-3333-3333-3333-333333333333"),
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

	var resp base.PaginatedResponse[serverResponse]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.Len(t, resp.Data, 1)
	serverResp := resp.Data[0]

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
			UID:           uuid.MustParse("44444444-4444-4444-4444-444444444444"),
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
			UID:           uuid.MustParse("55555555-5555-5555-5555-555555555555"),
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
		UID:           uuid.MustParse("66666666-6666-6666-6666-666666666666"),
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

func saveServer(t *testing.T, repo *inmemory.ServerRepository, userID uint, server *domain.Server) {
	t.Helper()
	require.NoError(t, repo.Save(context.Background(), server))
	repo.AddUserServer(userID, server.ID)
}
