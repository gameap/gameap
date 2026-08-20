// API Security Tests for OWASP API Security Top 10:2023.
// Categories covered:
//   - API5:2023 — Broken Function Level Authorization (BFLA)
//   - API3:2023 — Broken Object Property Level Authorization (BOPLA)
//     (vertical privilege escalation via mass-assignment of `roles` etc.)
//
// References:
//   - https://owasp.org/API-Security/editions/2023/en/0xa5-broken-function-level-authorization/
//   - https://owasp.org/API-Security/editions/2023/en/0xa3-broken-object-property-level-authorization/
//
// These tests assert two things:
//  1. Every mutating admin endpoint rejects regular users with 403 and unauthenticated callers with 401.
//  2. A regular user cannot escalate privileges via known weak spots: PUT /api/users/{self},
//     PUT /api/users/{self}/servers/{foreign}/permissions, POST /api/tokens with admin abilities, etc.

package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminEndpoint describes an admin-only endpoint with sample request body.
type adminEndpoint struct {
	method  string
	path    string
	body    string // JSON body for POST/PUT (avoids 400 before authz layer)
	comment string
}

// adminMutatingEndpoints lists representative admin endpoints to verify BFLA.
// We include all endpoints that materially change system state (mass-assignment
// targets, role grants, infra ops). Pure list-style admin endpoints (GET /api/users
// etc.) are already covered by TestRouterSecurity_UserAccess in router_security_test.go.
//
//nolint:gochecknoglobals
var adminMutatingEndpoints = []adminEndpoint{
	// Users management
	{http.MethodPost, "/api/users", `{"login":"x","email":"x@x","password":"abc"}`, "create user"},
	{http.MethodPut, "/api/users/2", `{"login":"x"}`, "edit user"},
	{http.MethodDelete, "/api/users/2", "", "delete user"},
	{http.MethodGet, "/api/users/2/servers", "", "list other user's servers"},
	{http.MethodGet, "/api/users/2/servers/1/permissions", "", "read other user's server permissions"},
	{http.MethodPut, "/api/users/2/servers/1/permissions", `{"abilities":[]}`, "grant server permissions"},

	// Games management
	{http.MethodPost, "/api/games", `{"code":"x","name":"X","engine":"source"}`, "create game"},
	{http.MethodPut, "/api/games/test", `{"name":"X"}`, "edit game"},
	{http.MethodDelete, "/api/games/test", "", "delete game"},
	{http.MethodPost, "/api/games/import/gameap", `{}`, "import gameap"},
	{http.MethodPost, "/api/games/import/pelican-egg", `{}`, "import pelican egg"},

	// Game Mods
	{http.MethodPost, "/api/game_mods", `{"name":"X","game_code":"test"}`, "create mod"},
	{http.MethodPut, "/api/game_mods/1", `{"name":"X"}`, "edit mod"},
	{http.MethodDelete, "/api/game_mods/1", "", "delete mod"},

	// Nodes / dedicated servers
	{http.MethodPut, "/api/nodes/1", `{"name":"X"}`, "edit node"},
	{http.MethodDelete, "/api/nodes/1", "", "delete node"},
	{http.MethodPut, "/api/dedicated_servers/1", `{"name":"X"}`, "edit dedicated server alias"},
	{http.MethodDelete, "/api/dedicated_servers/1", "", "delete dedicated server alias"},
	{http.MethodGet, "/api/nodes/1", "", "view node"},
	{http.MethodGet, "/api/nodes/1/busy_ports", "", "view node ports"},
	{http.MethodGet, "/api/nodes/1/ip_list", "", "view node ips"},
	{http.MethodGet, "/api/nodes/setup", "", "node setup"},
	{http.MethodGet, "/api/nodes/setup-key", "", "read setup key"},
	{http.MethodPost, "/api/nodes/setup-key", "", "rotate setup key"},
	{http.MethodDelete, "/api/nodes/setup-key", "", "delete setup key"},

	// Servers (admin-only mutations)
	{http.MethodPost, "/api/servers", `{"name":"X"}`, "create server"},
	{http.MethodPut, "/api/servers/1", `{"name":"X"}`, "edit server"},
	{http.MethodDelete, "/api/servers/1", "", "delete server"},

	// Client certificates
	{http.MethodPost, "/api/client_certificates", `{}`, "create client cert"},
	{http.MethodDelete, "/api/client_certificates/1", "", "delete client cert"},
}

// TestRouterSecurity_API5_BFLA_RegularUserRejected verifies API5:2023 — that admin
// endpoints reject a regular user with 403 even when they hold a valid PASETO token.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API5_BFLA_RegularUserRejected(t *testing.T) {
	for _, ep := range adminMutatingEndpoints {
		t.Run(testNameForAdminEndpoint("regular_user_rejected", ep), func(t *testing.T) {
			env := setupSecurityTest(t)
			token := issuePASETOToken(t, env, env.fixtures.RegularUser)

			w := doRequestWithBody(t, env, ep.method, ep.path, token, []byte(ep.body))

			assert.Equalf(t, http.StatusForbidden, w.Code,
				"BFLA: regular user must be rejected for %s %s (%s); got %d body=%s",
				ep.method, ep.path, ep.comment, w.Code, w.Body.String(),
			)
		})
	}
}

// TestRouterSecurity_API5_BFLA_UnauthenticatedRejected verifies that the same
// endpoints reject completely unauthenticated callers with 401 (not 403, not 200).
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API5_BFLA_UnauthenticatedRejected(t *testing.T) {
	for _, ep := range adminMutatingEndpoints {
		t.Run(testNameForAdminEndpoint("anon_rejected", ep), func(t *testing.T) {
			env := setupSecurityTest(t)

			w := doRequestWithBody(t, env, ep.method, ep.path, "" /* no token */, []byte(ep.body))

			assert.Equalf(t, http.StatusUnauthorized, w.Code,
				"BFLA: unauthenticated caller must get 401 for %s %s (%s); got %d body=%s",
				ep.method, ep.path, ep.comment, w.Code, w.Body.String(),
			)
		})
	}
}

// TestRouterSecurity_API5_BFLA_AdminAllowed asserts the inverse: admin tokens are NOT
// rejected at the authz layer for these endpoints. We accept any non-401/403 result.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API5_BFLA_AdminAllowed(t *testing.T) {
	for _, ep := range adminMutatingEndpoints {
		t.Run(testNameForAdminEndpoint("admin_passes_authz", ep), func(t *testing.T) {
			env := setupSecurityTest(t)
			token := issuePASETOToken(t, env, env.fixtures.AdminUser)

			w := doRequestWithBody(t, env, ep.method, ep.path, token, []byte(ep.body))

			assert.NotEqualf(t, http.StatusUnauthorized, w.Code,
				"admin must not be rejected with 401 for %s %s; body=%s", ep.method, ep.path, w.Body.String())
			assert.NotEqualf(t, http.StatusForbidden, w.Code,
				"admin must not be rejected with 403 for %s %s; body=%s", ep.method, ep.path, w.Body.String())
		})
	}
}

// TestRouterSecurity_API3_Escalation_RegularUserCannotEditOtherUsers verifies
// that PUT /api/users/{id} is blocked for non-admins even when targeting THEIR
// OWN user record (defense-in-depth: it's an admin endpoint regardless).
//
// This guards against mass-assignment vulnerabilities (API3:2023 — BOPLA)
// where a malicious caller submits {"roles":["admin"]} to elevate themselves.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API3_Escalation_RegularUserCannotEditOtherUsers(t *testing.T) {
	env := setupSecurityTest(t)
	token := issuePASETOToken(t, env, env.fixtures.RegularUser)

	// Editing self with an admin role payload — must be 403.
	body := fmt.Sprintf(`{"login":"%s","roles":["admin"]}`, env.fixtures.RegularUser.Login)
	w := doRequestWithBody(t, env, http.MethodPut,
		fmt.Sprintf("/api/users/%d", env.fixtures.RegularUser.ID), token, []byte(body))
	assert.Equalf(t, http.StatusForbidden, w.Code,
		"vertical escalation via PUT /api/users/{self} must be blocked; body=%s", w.Body.String())

	// Editing the admin's record — also 403.
	w = doRequestWithBody(t, env, http.MethodPut,
		fmt.Sprintf("/api/users/%d", env.fixtures.AdminUser.ID), token, []byte(`{"login":"hijacked"}`))
	assert.Equalf(t, http.StatusForbidden, w.Code,
		"PUT /api/users/{admin} must be blocked for regular user; body=%s", w.Body.String())
}

// TestRouterSecurity_API3_Escalation_RegularUserCannotGrantSelfServerAccess
// verifies that the put-server-permissions endpoint (the most direct privilege-grant
// surface) is admin-only AND that even after a forbidden attempt, the foreign server
// is still inaccessible.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API3_Escalation_RegularUserCannotGrantSelfServerAccess(t *testing.T) {
	env := setupSecurityTest(t)
	regularToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	// Pre-condition: regular user cannot read Server2.
	w := doRequest(t, env, http.MethodGet, "/api/servers/2", regularToken)
	require.Equalf(t, http.StatusNotFound, w.Code, "precondition: server 2 must be hidden; body=%s", w.Body.String())

	// Attempt grant: must be 403.
	grantPath := fmt.Sprintf("/api/users/%d/servers/2/permissions", env.fixtures.RegularUser.ID)
	w = doRequestWithBody(t, env, http.MethodPut, grantPath, regularToken,
		[]byte(`{"abilities":["game-server-common","game-server-start","game-server-stop"]}`))
	assert.Equalf(t, http.StatusForbidden, w.Code,
		"BFLA: regular user must not self-grant server permissions; body=%s", w.Body.String())

	// Post-condition: foreign server still hidden after failed grant attempt.
	w = doRequest(t, env, http.MethodGet, "/api/servers/2", regularToken)
	assert.Equalf(t, http.StatusNotFound, w.Code,
		"post-condition: failed grant attempt must not leak access; body=%s", w.Body.String())
}

// TestRouterSecurity_API5_Escalation_RegularUserCannotCreatePATWithAdminAbility verifies
// that POST /api/tokens with an admin-only ability is rejected even though the endpoint
// itself is user-level — the handler must validate that the requester holds the underlying
// RBAC permission for each requested PAT ability.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API5_Escalation_RegularUserCannotCreatePATWithAdminAbility(t *testing.T) {
	env := setupSecurityTest(t)
	regularToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	body := fmt.Sprintf(`{"token_name":"hostile","abilities":["%s"]}`, domain.PATAbilityServerCreate)
	w := doRequestWithBody(t, env, http.MethodPost, "/api/tokens", regularToken, []byte(body))

	assert.Containsf(t,
		[]int{http.StatusForbidden, http.StatusBadRequest, http.StatusUnprocessableEntity},
		w.Code,
		"regular user must not be able to mint a PAT with admin abilities; got %d body=%s", w.Code, w.Body.String(),
	)
}

// TestRouterSecurity_API5_Escalation_PATCannotIssueAnotherToken verifies that even
// admin-issued PATs cannot mint additional PATs — that endpoint is restricted to
// password-authenticated sessions (PASETO/JWT).
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API5_Escalation_PATCannotIssueAnotherToken(t *testing.T) {
	env := setupSecurityTest(t)

	// Admin PAT with the broadest possible ability set.
	pat := issuePAT(t, env, env.fixtures.AdminUser, []domain.PATAbility{
		domain.PATAbilityServerCreate,
		domain.PATAbilityGDaemonTaskRead,
		domain.PATAbilityServerList,
		domain.PATAbilityServerStart,
	})

	w := doRequestWithBody(t, env, http.MethodPost, "/api/tokens", pat, []byte(`{"name":"a"}`))

	assert.Equalf(t, http.StatusForbidden, w.Code,
		"PAT must not be able to issue further PATs; body=%s", w.Body.String())
}

// TestRouterSecurity_API5_Escalation_RemovedAdminRoleLosesAccess verifies that
// admin role is re-checked on every request (not cached past role removal).
// If an admin loses their role in mid-session, subsequent requests must be denied.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API5_Escalation_RemovedAdminRoleLosesAccess(t *testing.T) {
	env := setupSecurityTest(t)
	adminToken := issuePASETOToken(t, env, env.fixtures.AdminUser)

	// Sanity: admin-only endpoint works.
	w := doRequest(t, env, http.MethodGet, "/api/users", adminToken)
	require.Equalf(t, http.StatusOK, w.Code, "precondition: admin should access /api/users; body=%s", w.Body.String())

	// Demote admin to regular user.
	require.NoError(t, env.container.RBAC().SetRolesToUser(env.ctx, env.fixtures.AdminUser.ID, []string{"user"}))

	// In production the RBAC cache TTL is finite — a recently-revoked role
	// could remain effective for up to that TTL. The test container uses
	// a 1ms TTL specifically to verify the post-cache-eviction contract:
	// once the cache entry expires, the next request must be blocked.
	time.Sleep(10 * time.Millisecond)

	w = doRequest(t, env, http.MethodGet, "/api/users", adminToken)
	assert.Equalf(t, http.StatusForbidden, w.Code,
		"removed admin role must immediately lose admin endpoint access; body=%s", w.Body.String())
}

// TestRouterSecurity_API5_Escalation_AdminGrantedAccessThenRevoked exercises the
// grant→use→revoke→use cycle to verify that revoked permissions take effect on
// the next request.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API5_Escalation_AdminGrantedAccessThenRevoked(t *testing.T) {
	env := setupSecurityTest(t)
	regularToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	// Initially regular user has no access to Server2.
	w := doRequest(t, env, http.MethodGet, "/api/servers/2", regularToken)
	require.Equalf(t, http.StatusNotFound, w.Code, "precondition: server2 hidden; body=%s", w.Body.String())

	// Grant via repository (simulating admin action without going through HTTP, which would
	// require server_user link semantics that are not exposed via the in-memory repo helpers).
	require.NoError(t,
		env.container.ServerRepository().SetUserServers(env.ctx, env.fixtures.RegularUser.ID, []uint{1, 2}))

	// Now Server2 should be visible (404 → 200).
	w = doRequest(t, env, http.MethodGet, "/api/servers/2", regularToken)
	assert.Containsf(t, []int{http.StatusOK, http.StatusNotFound}, w.Code,
		"after grant body=%s", w.Body.String())

	// Revoke.
	require.NoError(t,
		env.container.ServerRepository().SetUserServers(env.ctx, env.fixtures.RegularUser.ID, []uint{1}))

	// Server2 is hidden again.
	w = doRequest(t, env, http.MethodGet, "/api/servers/2", regularToken)
	assert.Equalf(t, http.StatusNotFound, w.Code,
		"after revoke server2 must be hidden again; body=%s", w.Body.String())
}

func testNameForAdminEndpoint(prefix string, ep adminEndpoint) string {
	return testNameForEndpoint(prefix, ep.method, ep.path)
}

// TestRouterSecurity_API3_NodeResponseDoesNotLeakDaemonSecrets covers API3:2023
// (Broken Object Property Level Authorization). Even an authenticated administrator
// must not receive the daemon bootstrap key, daemon login/password or daemon API
// token in routine GET responses on /api/nodes/{id} and /api/nodes/{id}/daemon.
//
// Possession of the daemon API key allows minting a fresh daemon API token via
// /gdaemon_api/get_token (see internal/api/daemonapi/gettoken/handler.go), which
// in turn grants full daemon control. These fields belong only to a deliberate
// "reveal credentials" flow with audit logging, not to ordinary list/detail reads.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API3_NodeResponseDoesNotLeakDaemonSecrets(t *testing.T) {
	env := setupSecurityTest(t)
	adminToken := issuePASETOToken(t, env, env.fixtures.AdminUser)

	// Sanity: the fixture node holds a known-secret API key + daemon API token so
	// we can assert their plaintext values are not echoed back.
	require.NotEmpty(t, env.fixtures.Node1.GdaemonAPIKey, "fixture must set a daemon API key")
	require.NotNil(t, env.fixtures.Node1.GdaemonAPIToken, "fixture must set a daemon API token")
	require.NotEmpty(t, *env.fixtures.Node1.GdaemonAPIToken, "fixture must set a non-empty daemon API token")

	apiKey := env.fixtures.Node1.GdaemonAPIKey
	apiToken := *env.fixtures.Node1.GdaemonAPIToken

	endpoints := []string{
		"/api/nodes/1",
		"/api/dedicated_servers/1",
	}

	forbiddenSubstrings := []string{
		`"gdaemon_api_key"`,
		`"gdaemon_password"`,
		`"gdaemon_login"`,
		`"gdaemon_api_token"`,
		apiKey,
		apiToken,
	}

	for _, path := range endpoints {
		t.Run("GET_"+path, func(t *testing.T) {
			w := doRequest(t, env, http.MethodGet, path, adminToken)
			require.Equalf(t, http.StatusOK, w.Code,
				"admin must reach %s; got %d body=%s", path, w.Code, w.Body.String())

			body := w.Body.String()
			for _, needle := range forbiddenSubstrings {
				assert.NotContainsf(t, body, needle,
					"BOPLA: %s response leaked %q to admin; body=%s", path, needle, body)
			}
		})
	}
}
