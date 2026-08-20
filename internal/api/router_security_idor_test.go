// API Security Tests for OWASP API Security Top 10:2023.
// Category: API1:2023 — Broken Object Level Authorization (BOLA / IDOR).
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/
//
// Fixture invariants used by these tests (see pkg/testcontainer/inmemory.go):
//   - RegularUser is bound to Server1 only via server_user.
//   - Server2 has NO regular user attached, so it is reachable only by admin.
//   - The ServerFinder applies `filter.UserIDs = [user.ID]` for non-admins
//     (see internal/api/servers/base/serverfinder.go), which makes any direct
//     reference to Server2 by RegularUser yield 404 — that's the contract we pin here.

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serverScopedEndpoints lists every authenticated endpoint that takes a
// `{server}`/`{id}` path parameter and that should perform per-server access
// control. POST/PUT bodies are intentionally minimal — we only verify the
// authorization layer, not request validation.
//
// Endpoints purely admin-gated (DELETE/PUT /api/servers/{id}) are NOT in this
// list — they belong to API5/BFLA tests.
//
//nolint:gochecknoglobals
var serverScopedEndpoints = []struct {
	method string
	// pathFmt is a Go-format string with one %d placeholder where the server ID goes.
	pathFmt string
	// wantBody is sent as a JSON body for POST/PUT to avoid 400 bad-request before authz.
	wantBody string
}{
	{http.MethodGet, "/api/servers/%d", ""},
	{http.MethodGet, "/api/servers/%d/abilities", ""},
	{http.MethodGet, "/api/servers/%d/status", ""},
	{http.MethodGet, "/api/servers/%d/query", ""},
	{http.MethodGet, "/api/servers/%d/console", ""},
	{http.MethodPost, "/api/servers/%d/console", `{"command":"status"}`},
	{http.MethodGet, "/api/servers/%d/rcon/features", ""},
	{http.MethodPost, "/api/servers/%d/rcon", `{"command":"status"}`},
	{http.MethodGet, "/api/servers/%d/rcon/players", ""},
	{http.MethodPost, "/api/servers/%d/start", ""},
	{http.MethodPost, "/api/servers/%d/stop", ""},
	{http.MethodPost, "/api/servers/%d/restart", ""},
	{http.MethodPost, "/api/servers/%d/update", ""},
	{http.MethodPost, "/api/servers/%d/install", ""},
	{http.MethodPost, "/api/servers/%d/reinstall", ""},
	{http.MethodGet, "/api/servers/%d/tasks", ""},
	{http.MethodPost, "/api/servers/%d/tasks", `{"command":"start"}`},
	{http.MethodGet, "/api/servers/%d/settings", ""},
	{http.MethodPut, "/api/servers/%d/settings", `{}`},
	{http.MethodGet, "/api/file-manager/%d/initialize", ""},
	{http.MethodGet, "/api/file-manager/%d/content?path=/", ""},
	{http.MethodGet, "/api/file-manager/%d/tree?path=/", ""},
	{http.MethodGet, "/api/file-manager/%d/download?path=/file", ""},
	{http.MethodGet, "/api/file-manager/%d/stream-file?path=/file", ""},
	{http.MethodPost, "/api/file-manager/%d/delete", `{"path":"/file"}`},
	{http.MethodPost, "/api/file-manager/%d/upload", `{}`},
	{http.MethodPost, "/api/file-manager/%d/update-file", `{"path":"/file"}`},
	{http.MethodPost, "/api/file-manager/%d/rename", `{"path":"/file","name":"new"}`},
	{http.MethodPost, "/api/file-manager/%d/create-directory", `{"path":"/dir"}`},
	{http.MethodPost, "/api/file-manager/%d/create-file", `{"path":"/file"}`},
	{http.MethodPost, "/api/file-manager/%d/paste", `{"action":"copy","files":[]}`},
}

// TestRouterSecurity_API1_BOLA_ForeignServerReturnsNotFound verifies API1:2023.
// For every server-scoped endpoint, a RegularUser hitting Server2 (which they
// don't own) must be rejected with 403 or 404. We accept both because the
// rejection happens at slightly different layers depending on the handler:
// ServerFinder returns NotFound, while a few handlers short-circuit via
// AbilityChecker which returns 403.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API1_BOLA_ForeignServerReturnsNotFound(t *testing.T) {
	const foreignServerID = 2 // Server2 — not attached to RegularUser

	for _, ep := range serverScopedEndpoints {
		t.Run(testNameForEndpoint("regular_user_cannot_access_foreign_server", ep.method, ep.pathFmt), func(t *testing.T) {
			env := setupSecurityTest(t)
			token := issuePASETOToken(t, env, env.fixtures.RegularUser)
			path := formatServerPath(ep.pathFmt, foreignServerID)

			w := doRequestWithBody(t, env, ep.method, path, token, []byte(ep.wantBody))

			assert.Containsf(t,
				[]int{http.StatusNotFound, http.StatusForbidden},
				w.Code,
				"BOLA: regular user must not reach foreign server resource %s %s; got %d body=%s",
				ep.method, path, w.Code, w.Body.String(),
			)
		})
	}
}

// TestRouterSecurity_API1_BOLA_OwnServerNotRejectedByAuthz verifies that the same
// matrix of endpoints does NOT reject the user from reaching their OWN server
// at the authorization layer. We only check the authz contract: response status
// must not be 401 (auth) or 403 (authz). 200/400/404/500 are all accepted —
// they reflect downstream business logic, not access control.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API1_BOLA_OwnServerNotRejectedByAuthz(t *testing.T) {
	const ownServerID = 1 // Server1 — attached to RegularUser

	for _, ep := range serverScopedEndpoints {
		t.Run(testNameForEndpoint("regular_user_can_reach_own_server", ep.method, ep.pathFmt), func(t *testing.T) {
			env := setupSecurityTest(t)
			token := issuePASETOToken(t, env, env.fixtures.RegularUser)
			path := formatServerPath(ep.pathFmt, ownServerID)

			w := doRequestWithBody(t, env, ep.method, path, token, []byte(ep.wantBody))

			assert.NotEqualf(t, http.StatusUnauthorized, w.Code,
				"authn must not block: %s %s body=%s", ep.method, path, w.Body.String())
			assert.NotEqualf(t, http.StatusForbidden, w.Code,
				"authz must not block own server: %s %s body=%s", ep.method, path, w.Body.String())
		})
	}
}

// TestRouterSecurity_API1_BOLA_AdminCanReachAnyServer verifies that an admin
// is not blocked at the authz layer for any of the server-scoped endpoints,
// regardless of whether the server is attached to them via server_user.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API1_BOLA_AdminCanReachAnyServer(t *testing.T) {
	const foreignToAdmin = 2

	for _, ep := range serverScopedEndpoints {
		t.Run(testNameForEndpoint("admin_can_reach_any_server", ep.method, ep.pathFmt), func(t *testing.T) {
			env := setupSecurityTest(t)
			token := issuePASETOToken(t, env, env.fixtures.AdminUser)
			path := formatServerPath(ep.pathFmt, foreignToAdmin)

			w := doRequestWithBody(t, env, ep.method, path, token, []byte(ep.wantBody))

			assert.NotEqualf(t, http.StatusUnauthorized, w.Code,
				"authn must not block admin: %s %s body=%s", ep.method, path, w.Body.String())
			assert.NotEqualf(t, http.StatusForbidden, w.Code,
				"authz must not block admin: %s %s body=%s", ep.method, path, w.Body.String())
		})
	}
}

// TestRouterSecurity_API1_BOLA_ServerListLeaksOnlyOwnServers verifies that GET /api/servers,
// when called by a regular user, returns ONLY the servers attached to that user, not all
// servers in the system. Otherwise an attacker could enumerate other users' resources.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API1_BOLA_ServerListLeaksOnlyOwnServers(t *testing.T) {
	env := setupSecurityTest(t)
	regularToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	w := doRequest(t, env, http.MethodGet, "/api/servers", regularToken)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Data []struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	for _, server := range resp.Data {
		assert.NotEqualf(t,
			env.fixtures.Server2.ID,
			server.ID,
			"BOLA: regular user must not see Server2 in /api/servers listing",
		)
	}

	// Sanity: admin should see both
	adminToken := issuePASETOToken(t, env, env.fixtures.AdminUser)
	w = doRequest(t, env, http.MethodGet, "/api/servers", adminToken)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	ids := make(map[uint]bool, len(resp.Data))
	for _, s := range resp.Data {
		ids[s.ID] = true
	}
	assert.Truef(t, ids[env.fixtures.Server1.ID] && ids[env.fixtures.Server2.ID],
		"admin must see both servers in listing, got=%v", ids)
}

// TestRouterSecurity_API1_BOLA_ServerSummaryLeaksOnlyOwnServers verifies the same
// per-user filtering for /api/servers/summary.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API1_BOLA_ServerSummaryLeaksOnlyOwnServers(t *testing.T) {
	env := setupSecurityTest(t)
	regularToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	w := doRequest(t, env, http.MethodGet, "/api/servers/summary", regularToken)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	body := w.Body.String()
	assert.NotContainsf(t, body, `"name":"Test Server 2"`,
		"BOLA: server summary must not leak Server2 details to regular user; body=%s", body)
}

// TestRouterSecurity_API1_BOLA_AbilitiesEndpointDoesNotLeakForeignServer verifies
// that a regular user cannot enumerate abilities/permissions for a server they
// don't own.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API1_BOLA_AbilitiesEndpointDoesNotLeakForeignServer(t *testing.T) {
	env := setupSecurityTest(t)
	regularToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	w := doRequest(t, env, http.MethodGet, "/api/servers/2/abilities", regularToken)

	assert.Containsf(t,
		[]int{http.StatusNotFound, http.StatusForbidden},
		w.Code,
		"BOLA: foreign server abilities must not be enumerable; got %d body=%s", w.Code, w.Body.String(),
	)
}

// TestRouterSecurity_API1_BOLA_PathTraversalAndMalformedIDs verifies that
// malformed/encoded server IDs cannot be used to bypass the access check or
// crash the router with a 500.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API1_BOLA_PathTraversalAndMalformedIDs(t *testing.T) {
	env := setupSecurityTest(t)
	regularToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	tests := []struct {
		name string
		path string
	}{
		{"encoded_slash_in_id", "/api/servers/2%2F1"},
		{"path_traversal_dotdot", "/api/servers/..%2Fservers%2F2"},
		{"zero_id", "/api/servers/0"},
		{"negative_id", "/api/servers/-1"},
		{"non_numeric_id", "/api/servers/abc"},
		{"huge_overflow_id", "/api/servers/99999999999999999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doRequest(t, env, http.MethodGet, tt.path, regularToken)
			assert.NotEqualf(t,
				http.StatusInternalServerError,
				w.Code,
				"malformed id %s should not yield 500; body=%s", tt.path, w.Body.String(),
			)
			// 3xx is acceptable: gorilla/mux URL-canonicalises path-traversal
			// attempts into a redirect; the redirected target is itself enforced
			// by the same middleware chain on the next request.
			acceptable := []int{
				http.StatusMovedPermanently,
				http.StatusFound,
				http.StatusBadRequest,
				http.StatusForbidden,
				http.StatusNotFound,
				http.StatusMethodNotAllowed,
			}
			assert.Containsf(t,
				acceptable,
				w.Code,
				"malformed id %s expected 3xx/4xx, got %d body=%s", tt.path, w.Code, w.Body.String(),
			)
		})
	}
}

// formatServerPath substitutes %d with the server ID in the endpoint pattern.
// Implemented manually instead of fmt.Sprintf to keep tests fast and avoid
// unintended formatting of query strings.
func formatServerPath(pattern string, serverID int) string {
	out := make([]byte, 0, len(pattern)+8)
	idStr := []byte(itoa(serverID))
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '%' && i+1 < len(pattern) && pattern[i+1] == 'd' {
			out = append(out, idStr...)
			i++

			continue
		}
		out = append(out, pattern[i])
	}

	return string(out)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}

// testNameForEndpoint produces a stable, underscore-only test case name out of
// METHOD + path pattern, suitable for `go test -run`.
func testNameForEndpoint(prefix, method, pathFmt string) string {
	out := make([]byte, 0, len(prefix)+1+len(method)+1+len(pathFmt))
	out = append(out, prefix...)
	out = append(out, '_')
	out = append(out, method...)
	out = append(out, '_')
	for i := 0; i < len(pathFmt); i++ {
		ch := pathFmt[i]
		switch {
		case ch >= 'a' && ch <= 'z',
			ch >= 'A' && ch <= 'Z',
			ch >= '0' && ch <= '9':
			out = append(out, ch)
		default:
			out = append(out, '_')
		}
	}

	return string(out)
}
