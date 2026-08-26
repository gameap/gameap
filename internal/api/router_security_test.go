package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/gameap/gameap/pkg/testcontainer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseRequest(request string) (method, path string) {
	parts := strings.SplitN(request, " ", 2)

	return parts[0], parts[1]
}

//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_TokenAccess(t *testing.T) {
	tests := []struct {
		name               string
		request            string
		isAdmin            bool
		tokenAbilities     []domain.PATAbility
		expectedStatusCode int
	}{
		// "GET /api/servers" endpoint tests
		{
			// Token with server list ability can access servers.
			name:               "token_with_server_list_can_access_servers",
			request:            "GET /api/servers",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerList},
			expectedStatusCode: http.StatusOK,
		},
		{
			// Admin without server list ability cannot access servers.
			name:               "admin_without_server_list_cannot_access_servers",
			request:            "GET /api/servers",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			// Admin with server list ability can access servers.
			name:               "admin_without_server_list_cannot_access_servers",
			request:            "GET /api/servers",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerList},
			expectedStatusCode: http.StatusOK,
		},
		{
			// Token without server list ability cannot access servers.
			name:               "token_without_server_list_cannot_access_servers",
			request:            "GET /api/servers",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerStart},
			expectedStatusCode: http.StatusForbidden,
		},

		// "POST /api/servers" endpoint tests
		{
			name:               "token_with_server_create_can_create_server",
			request:            "POST /api/servers",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerCreate},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			// Only admin can create servers.
			name:               "token_without_server_create_cannot_create_server",
			request:            "POST /api/servers",
			isAdmin:            false,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerList},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			// Only admin can create servers, even with ability.
			name:               "regular_user_with_server_create_cannot_create_server",
			request:            "POST /api/servers",
			isAdmin:            false,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerCreate},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			// Admin can create servers with ability.
			name:               "admin_with_server_create_can_create_server",
			request:            "POST /api/servers",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerCreate},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			// Admin cannot create servers without ability.
			name:               "admin_without_server_create_cannot_create_server",
			request:            "POST /api/servers",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{},
			expectedStatusCode: http.StatusForbidden,
		},

		// "GET /api/servers/1/rcon/features" endpoint tests
		{
			name:               "token_with_rcon_console_can_access_rcon_features",
			request:            "GET /api/servers/1/rcon/features",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerRconConsole},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "token_without_rcon_console_cannot_access_rcon_features",
			request:            "GET /api/servers/1/rcon/features",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerList},
			expectedStatusCode: http.StatusForbidden,
		},

		// "GET /api/servers/1/console" endpoint tests
		{
			name:               "token_with_console_can_access_console",
			request:            "GET /api/servers/1/console",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerConsole},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name:               "token_without_console_cannot_access_console",
			request:            "GET /api/servers/1/console",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerStart},
			expectedStatusCode: http.StatusForbidden,
		},

		// "GET /api/servers/1/tasks" endpoint tests
		{
			name:               "token_with_tasks_manage_can_access_server_tasks",
			request:            "GET /api/servers/1/tasks",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerTasksManage},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "token_without_tasks_manage_cannot_access_server_tasks",
			request:            "GET /api/servers/1/tasks",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerList},
			expectedStatusCode: http.StatusForbidden,
		},

		// "GET /api/servers/1/settings" endpoint tests
		{
			name:               "token_with_settings_manage_can_access_server_settings",
			request:            "GET /api/servers/1/settings",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerSettingsManage},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name:               "token_without_settings_manage_cannot_access_server_settings",
			request:            "GET /api/servers/1/settings",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerList},
			expectedStatusCode: http.StatusForbidden,
		},

		// "POST /api/servers/{id}/start" endpoint tests
		{
			name:               "token_with_server_start_can_start_server",
			request:            "POST /api/servers/1/start",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerStart},
			expectedStatusCode: http.StatusOK,
		},
		{
			// Token has ability but server is not accessible by user.
			name:               "regular_user_with_server_start_cannot_start_other_user_server",
			request:            "POST /api/servers/2/start",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerStart},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			// Admin has access to all servers.
			name:               "admin_with_server_start_can_start_any_server",
			request:            "POST /api/servers/2/start",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerStart},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "token_without_server_start_cannot_start_server",
			request:            "POST /api/servers/1/start",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerList},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			// Admin without ability cannot start server.
			name:               "admin_without_server_start_cannot_start_server",
			request:            "POST /api/servers/1/start",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerList},
			expectedStatusCode: http.StatusForbidden,
		},

		// "GET /api/gdaemon_tasks/1" endpoint tests
		{
			name:               "token_with_gdaemon_task_read_can_access_gdaemon_task",
			request:            "GET /api/gdaemon_tasks/1",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityGDaemonTaskRead},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name:               "token_without_gdaemon_task_read_cannot_access_gdaemon_task",
			request:            "GET /api/gdaemon_tasks/1",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerList},
			expectedStatusCode: http.StatusForbidden,
		},

		// Provisioning scopes. A billing integration drives these routes with
		// a token, so each one must be reachable with its ability and closed
		// without it.
		{
			name:               "admin_with_user_read_can_list_users",
			request:            "GET /api/users",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityUserRead},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "admin_without_user_read_cannot_list_users",
			request:            "GET /api/users",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerList},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "regular_user_with_user_read_cannot_list_users",
			request:            "GET /api/users",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityUserRead},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_with_user_manage_reaches_user_creation",
			request:            "POST /api/users",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityUserManage},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "admin_with_only_user_read_cannot_create_user",
			request:            "POST /api/users",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityUserRead},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			// Account deletion is deliberately left out of the token scopes:
			// a leaked billing token must not be able to erase accounts.
			name:    "admin_with_every_provisioning_ability_cannot_delete_user",
			request: "DELETE /api/users/2",
			isAdmin: true,
			tokenAbilities: []domain.PATAbility{
				domain.PATAbilityUserRead,
				domain.PATAbilityUserManage,
				domain.PATAbilityNodeRead,
				domain.PATAbilityGameRead,
			},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_with_node_read_can_list_nodes",
			request:            "GET /api/nodes",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityNodeRead},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "admin_without_node_read_cannot_list_nodes",
			request:            "GET /api/nodes",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityGameRead},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_with_node_read_can_read_busy_ports",
			request:            "GET /api/nodes/1/busy_ports",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityNodeRead},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "admin_with_game_read_can_list_games",
			request:            "GET /api/games",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityGameRead},
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "admin_without_game_read_cannot_list_games",
			request:            "GET /api/games",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityNodeRead},
			expectedStatusCode: http.StatusForbidden,
		},

		// "POST /api/auth/sso/tickets" endpoint tests
		{
			name:               "admin_with_sso_issue_can_reach_ticket_minting",
			request:            "POST /api/auth/sso/tickets",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilitySSOIssue},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			// Managing users must not silently include handing out logins.
			name:               "admin_with_user_manage_alone_cannot_mint_tickets",
			request:            "POST /api/auth/sso/tickets",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityUserManage},
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "regular_user_with_sso_issue_cannot_mint_tickets",
			request:            "POST /api/auth/sso/tickets",
			tokenAbilities:     []domain.PATAbility{domain.PATAbilitySSOIssue},
			expectedStatusCode: http.StatusForbidden,
		},

		// "POST /api/tokens" endpoint tests
		{
			// Creating tokens is forbidden even for admin tokens.
			// Tokens are created only via user password authentication.
			name:               "admin_with_token_create_cannot_create_token",
			request:            "POST /api/tokens",
			isAdmin:            true,
			tokenAbilities:     []domain.PATAbility{domain.PATAbilityServerCreate, domain.PATAbilityGDaemonTaskRead},
			expectedStatusCode: http.StatusForbidden,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, err := testcontainer.LoadInmemoryContainer()
			require.NoError(t, err)

			ctx := context.Background()

			fixtures, err := testcontainer.SetupFixtures(ctx, c)
			require.NoError(t, err)

			user := fixtures.RegularUser

			if test.isAdmin {
				user = fixtures.AdminUser
			}

			tokenString, err := pkgstrings.CryptoRandomString(40)
			require.NoError(t, err)
			token := &domain.PersonalAccessToken{
				TokenableType: domain.EntityTypeUser,
				TokenableID:   user.ID,
				Name:          "Test Token",
				Token:         pkgstrings.SHA256(tokenString),
				Abilities:     &test.tokenAbilities,
			}
			err = c.PersonalAccessTokenRepository().Save(ctx, token)
			require.NoError(t, err)

			router := api.CreateRouter(c)

			method, path := parseRequest(test.request)
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Authorization", "Bearer "+fmt.Sprintf("%d|%s", token.ID, tokenString))

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if !assert.Equal(
				t,
				test.expectedStatusCode,
				w.Code,
				"Expected status code %d, got %d",
				test.expectedStatusCode,
				w.Code,
			) {
				t.Logf("Response body: %s", w.Body.String())
			}
		})
	}
}

//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_UserAccess(t *testing.T) {
	tests := []struct {
		name               string
		request            string
		isAdmin            bool
		expectedStatusCode int
	}{
		{
			name:               "regular_user_cannot_access_users_list",
			request:            "GET /api/users",
			isAdmin:            false,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_can_access_users_list",
			request:            "GET /api/users",
			isAdmin:            true,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "regular_user_cannot_create_user",
			request:            "POST /api/users",
			isAdmin:            false,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_can_create_user",
			request:            "POST /api/users",
			isAdmin:            true,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "regular_user_cannot_access_games",
			request:            "GET /api/games",
			isAdmin:            false,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_can_access_games",
			request:            "GET /api/games",
			isAdmin:            true,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "regular_user_cannot_access_nodes",
			request:            "GET /api/nodes",
			isAdmin:            false,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_can_access_nodes",
			request:            "GET /api/nodes",
			isAdmin:            true,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "regular_user_cannot_access_dedicated_servers",
			request:            "GET /api/dedicated_servers",
			isAdmin:            false,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_can_access_dedicated_servers",
			request:            "GET /api/dedicated_servers",
			isAdmin:            true,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "regular_user_cannot_access_game_mods",
			request:            "GET /api/game_mods",
			isAdmin:            false,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_can_access_game_mods",
			request:            "GET /api/game_mods",
			isAdmin:            true,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "regular_user_cannot_search_servers",
			request:            "GET /api/servers/search",
			isAdmin:            false,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_can_search_servers",
			request:            "GET /api/servers/search",
			isAdmin:            true,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "regular_user_cannot_access_gdaemon_tasks",
			request:            "GET /api/gdaemon_tasks",
			isAdmin:            false,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_can_access_gdaemon_tasks",
			request:            "GET /api/gdaemon_tasks",
			isAdmin:            true,
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "regular_user_cannot_cancel_gdaemon_task",
			request:            "POST /api/gdaemon_tasks/1/cancel",
			isAdmin:            false,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_can_reach_gdaemon_task_cancel_endpoint",
			request:            "POST /api/gdaemon_tasks/1/cancel",
			isAdmin:            true,
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name:               "regular_user_cannot_access_client_certificates",
			request:            "GET /api/client_certificates",
			isAdmin:            false,
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:               "admin_can_access_client_certificates",
			request:            "GET /api/client_certificates",
			isAdmin:            true,
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, err := testcontainer.LoadInmemoryContainer()
			require.NoError(t, err)

			ctx := context.Background()

			fixtures, err := testcontainer.SetupFixtures(ctx, c)
			require.NoError(t, err)

			var user *domain.User
			if test.isAdmin {
				user = fixtures.AdminUser
			} else {
				user = fixtures.RegularUser
			}

			token, err := c.AuthService().GenerateTokenForUser(user, time.Hour)
			require.NoError(t, err)

			router := api.CreateRouter(c)

			method, path := parseRequest(test.request)
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Authorization", "Bearer "+token)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, test.expectedStatusCode, w.Code, "Expected status code %d, got %d", test.expectedStatusCode, w.Code)
		})
	}
}

// An SSO ticket travels in a URL, so it must be worthless as a credential
// anywhere except the exchange endpoint. The auth middleware does not know its
// prefix, which is what makes every other presentation a 401.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_SSOTicketIsNotACredential(t *testing.T) {
	ticket := auth.SSOTicketPrefix + "0123456789abcdef0123456789abcdef0123456789abcdef"

	tests := []struct {
		name    string
		prepare func(req *http.Request)
	}{
		{
			name: "as_bearer_header",
			prepare: func(req *http.Request) {
				req.Header.Set("Authorization", "Bearer "+ticket)
			},
		},
		{
			name: "as_query_parameter",
			prepare: func(req *http.Request) {
				query := req.URL.Query()
				query.Set("token", ticket)
				req.URL.RawQuery = query.Encode()
			},
		},
		{
			name: "as_cookie",
			prepare: func(req *http.Request) {
				req.AddCookie(&http.Cookie{Name: "token", Value: ticket})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// ARRANGE
			c, err := testcontainer.LoadInmemoryContainer()
			require.NoError(t, err)

			_, err = testcontainer.SetupFixtures(context.Background(), c)
			require.NoError(t, err)

			router := api.CreateRouter(c)

			req := httptest.NewRequest(http.MethodGet, "/api/servers", nil)
			test.prepare(req)

			w := httptest.NewRecorder()

			// ACT
			router.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
		})
	}
}
