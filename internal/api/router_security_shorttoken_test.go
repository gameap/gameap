// API Security Tests for OWASP API Security Top 10:2023.
// Categories:
//   - API2:2023 — Broken Authentication: a single-use short-lived token,
//     minted for the URL-borne download/WebSocket use case, must authenticate
//     a request on the routes that opted in.
//   - API1:2023 — Broken Object Level Authorization: the same token must be
//     refused (403) on every route that did NOT opt in, and the refusal must
//     be audited, so a token leaked from a URL/log/proxy cannot be replayed
//     against the wider API.
//
// These tests drive the production router end-to-end via the in-memory
// container. The short-lived token is seeded directly into the container's
// cache (the very cache the router wires into both the issuer and the auth
// middleware) — exactly what a successful POST /api/auth/short-lived-token
// would have stored — so the test exercises the consume path, not the mint.
//
// NOTE: as documented in router_security_auditlog_test.go, CreateRouter builds
// the mux only; RequestContextMiddleware is wired in createHTTPServer, so
// request_id/ip enrichment is not asserted here.
//
// Reference: https://owasp.org/API-Security/editions/2023/

package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedShortLivedToken stores a short-lived token for user in the container's
// cache and returns the wire token, mirroring what the issue handler persists.
func seedShortLivedToken(t *testing.T, env *securityTestEnv, user *domain.User) string {
	t.Helper()

	secret := "rTrouterIntegrationSecret00112233445566778899xZ"
	token := auth.ShortLivedTokenPrefix + secret

	encoded, err := auth.MarshalShortLivedPayload(auth.ShortLivedPayload{
		UserID: user.ID,
		Login:  user.Login,
		Email:  user.Email,
	})
	require.NoError(t, err)
	require.NoError(t, env.container.Cache().Set(
		context.Background(), auth.ShortLivedCacheKey(token), encoded,
	))

	return token
}

// TestRouterSecurity_ShortLivedToken_AuthenticatesOnOptedInRoute covers OWASP
// API2:2023. A short-lived token minted for the admin user must authenticate
// GET /api/nodes/{id}/logs.zip — the scope-allowed download route. The request
// must clear auth, the short-lived scope guard, and the admin gate (it must
// NOT be 401 or 403); the only thing that may still fail is the absent daemon
// backend in the in-memory container, which is not the behavior under test.
func TestRouterSecurity_ShortLivedToken_AuthenticatesOnOptedInRoute(t *testing.T) {
	// ARRANGE
	env, recorder := setupAuditRouterEnv(t)
	token := seedShortLivedToken(t, env, env.fixtures.AdminUser)

	// ACT
	req := httptest.NewRequest(http.MethodGet, "/api/nodes/1/logs.zip?token="+token, http.NoBody)
	w := doRequestRaw(t, env, req)

	// ASSERT
	require.NotEqual(t, http.StatusUnauthorized, w.Code,
		"a valid short-lived token must pass auth on the opted-in route; body=%s", w.Body.String())
	require.NotEqual(t, http.StatusForbidden, w.Code,
		"the scope guard must allow the opted-in route for a short-lived admin token; body=%s",
		w.Body.String())

	_, denied := findEvent(recorder.snapshot(), audit.EventAccessDenied)
	assert.False(t, denied,
		"no scope/authorization denial may be recorded for an allowed short-lived request")
	_, rejected := findEvent(recorder.snapshot(), audit.EventAuthTokenRejected)
	assert.False(t, rejected,
		"a valid short-lived token must not produce a token-rejected audit event")
}

// TestRouterSecurity_ShortLivedToken_DeniedOnNonOptedInRoute covers OWASP
// API1:2023 / API2:2023. The same kind of token presented to GET /api/servers
// (a route that did NOT opt in) must be refused with 403, the handler must not
// run, and the denial must be audited with the stable shorttoken_scope reason
// attributed to the authenticated principal.
func TestRouterSecurity_ShortLivedToken_DeniedOnNonOptedInRoute(t *testing.T) {
	// ARRANGE
	env, recorder := setupAuditRouterEnv(t)
	token := seedShortLivedToken(t, env, env.fixtures.AdminUser)

	// ACT
	req := httptest.NewRequest(http.MethodGet, "/api/servers?token="+token, http.NoBody)
	w := doRequestRaw(t, env, req)

	// ASSERT
	require.Equal(t, http.StatusForbidden, w.Code,
		"a short-lived token must be refused on a non-opted-in route; body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "short-lived token is not accepted")

	ev, ok := findEvent(recorder.snapshot(), audit.EventAccessDenied)
	require.True(t, ok, "a scope denial on a non-opted-in route must be audited")
	assert.Equal(t, audit.OutcomeDenied, ev.Outcome)
	assert.Equal(t, "shorttoken_scope", ev.Reason, "the stable scope-denial reason must be recorded")
	assert.Equal(t, "endpoint", ev.ResourceType)
	assert.Equal(t, "/api/servers", ev.ResourceID, "the refused endpoint must be recorded for forensics")
	assert.Equal(t, env.fixtures.AdminUser.ID, ev.ActorID,
		"the denied principal must be attributed to the short-lived session's user")
	assert.Equal(t, audit.AuthMethodShortLived, ev.AuthMethod,
		"the denial must be attributed to the short-lived auth method")
}

// TestRouterSecurity_ShortLivedToken_SingleUseAcrossRouter covers OWASP
// API2:2023. End-to-end through the real router, a short-lived token must
// authenticate at most once: the first consume deletes the cache entry, so a
// second presentation of the same token on the same opted-in route is a 401.
func TestRouterSecurity_ShortLivedToken_SingleUseAcrossRouter(t *testing.T) {
	// ARRANGE
	env, _ := setupAuditRouterEnv(t)
	token := seedShortLivedToken(t, env, env.fixtures.AdminUser)
	path := "/api/nodes/1/logs.zip?token=" + token

	// ACT
	first := doRequestRaw(t, env, httptest.NewRequest(http.MethodGet, path, http.NoBody))
	second := doRequestRaw(t, env, httptest.NewRequest(http.MethodGet, path, http.NoBody))

	// ASSERT
	require.NotEqual(t, http.StatusUnauthorized, first.Code,
		"the first use of the short-lived token must authenticate; body=%s", first.Body.String())
	require.Equal(t, http.StatusUnauthorized, second.Code,
		"the second use of a consumed short-lived token must be rejected; body=%s",
		second.Body.String())
}
