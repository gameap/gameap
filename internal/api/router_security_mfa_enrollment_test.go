// API Security Tests for OWASP API Security Top 10:2023.
// Categories:
//   - API2:2023 — Broken Authentication: a session minted from an
//     MFA-enrollment token (issued at login to an admin who crossed the MFA
//     hard-fail threshold, see services/mfanudge) must authenticate only the
//     2FA-enrollment endpoints that explicitly opted in.
//   - API1:2023 — Broken Object Level Authorization: on every route that did
//     NOT opt in the same token is refused (403) and the denial is audited, so
//     a forced-enrollment admin cannot reach the wider API until 2FA is
//     enrolled.
//
// These tests drive the production router end-to-end via the in-memory
// container. The enrollment token is minted through the container's real auth
// service (the very service the auth middleware validates against), so the
// scope claim travels the full auth → scope-guard path, not a stubbed one.
// The guard enforces the scope only while AUTH_REQUIRE_MFA_FOR_ADMINS is
// enabled — with the feature off a leftover enrollment token is honoured as a
// full session — so each test pins the flag explicitly before CreateRouter.
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
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/testcontainer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// issueMFAEnrollmentToken mints an MFA-enrollment-scoped token for user through
// the container's real auth service, mirroring what the login handler issues to
// an admin past the MFA hard-fail threshold.
func issueMFAEnrollmentToken(tb testing.TB, env *securityTestEnv, user *domain.User) string {
	tb.Helper()

	token, err := env.container.AuthService().GenerateMFAEnrollmentToken(user, time.Hour)
	require.NoError(tb, err)

	return token
}

// setupMFARouterEnv mirrors setupAuditRouterEnv but pins
// AUTH_REQUIRE_MFA_FOR_ADMINS before CreateRouter — the enrollment scope guard
// captures the flag at wiring time, like production reading the env at startup.
func setupMFARouterEnv(t *testing.T, requireMFAForAdmins bool) (*securityTestEnv, *auditCapture) {
	t.Helper()

	c, err := testcontainer.LoadInmemoryContainer()
	require.NoError(t, err)
	c.Config().Auth.RequireMFAForAdmins = requireMFAForAdmins

	recorder := &auditCapture{}
	c.SetAuditLogger(recorder)

	ctx := context.Background()
	fixtures, err := testcontainer.SetupFixtures(ctx, c)
	require.NoError(t, err)

	env := &securityTestEnv{
		container: c,
		fixtures:  fixtures,
		router:    api.CreateRouter(c),
		ctx:       ctx,
	}

	return env, recorder
}

// TestRouterSecurity_MFAEnrollmentToken_DeniedOnNonOptedInRoute covers OWASP
// API1:2023 / API2:2023. An enrollment-scoped token presented to GET
// /api/servers (a route that did NOT opt in) must be refused with 403, the
// handler must not run, and the denial must be audited with the stable
// mfa_enrollment_scope reason attributed to the authenticated admin.
func TestRouterSecurity_MFAEnrollmentToken_DeniedOnNonOptedInRoute(t *testing.T) {
	// ARRANGE
	env, recorder := setupMFARouterEnv(t, true)
	token := issueMFAEnrollmentToken(t, env, env.fixtures.AdminUser)

	// ACT
	w := doRequest(t, env, http.MethodGet, "/api/servers", token)

	// ASSERT
	require.Equal(t, http.StatusForbidden, w.Code,
		"an enrollment-scoped token must be refused on a non-opted-in route; body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "restricted to two-factor enrollment")

	ev, ok := findEvent(recorder.snapshot(), audit.EventAccessDenied)
	require.True(t, ok, "a scope denial on a non-opted-in route must be audited")
	assert.Equal(t, audit.OutcomeDenied, ev.Outcome)
	assert.Equal(t, "mfa_enrollment_scope", ev.Reason, "the stable scope-denial reason must be recorded")
	assert.Equal(t, "endpoint", ev.ResourceType)
	assert.Equal(t, "/api/servers", ev.ResourceID, "the refused endpoint must be recorded for forensics")
	assert.Equal(t, env.fixtures.AdminUser.ID, ev.ActorID,
		"the denied principal must be attributed to the enrollment session's user")
}

// TestRouterSecurity_MFAEnrollmentToken_AuthenticatesOnProfileRead covers OWASP
// API2:2023. The profile read opted in (AllowMFAEnrollmentToken), so an
// enrollment-scoped admin token must clear auth and the scope guard there: the
// response must NOT be 401 or 403, and no scope denial may be audited.
func TestRouterSecurity_MFAEnrollmentToken_AuthenticatesOnProfileRead(t *testing.T) {
	// ARRANGE
	env, recorder := setupMFARouterEnv(t, true)
	token := issueMFAEnrollmentToken(t, env, env.fixtures.AdminUser)

	// ACT
	w := doRequest(t, env, http.MethodGet, "/api/profile", token)

	// ASSERT
	require.NotEqual(t, http.StatusUnauthorized, w.Code,
		"a valid enrollment token must pass auth on the profile read; body=%s", w.Body.String())
	require.NotEqual(t, http.StatusForbidden, w.Code,
		"the scope guard must allow the opted-in profile read; body=%s", w.Body.String())
	assert.Equal(t, http.StatusOK, w.Code, "the profile read must succeed for an enrollment session; body=%s",
		w.Body.String())

	_, denied := findEvent(recorder.snapshot(), audit.EventAccessDenied)
	assert.False(t, denied,
		"no scope/authorization denial may be recorded for an allowed enrollment request")
}

// TestRouterSecurity_MFAEnrollmentToken_AllowedOnSetupRoute covers OWASP
// API2:2023. The 2FA setup endpoint opted in, so the scope guard must NOT
// refuse an enrollment-scoped token there. The setup handler itself may answer
// some other status, which is fine — the contract under test is only that the
// scope guard does not turn it into a 403 (nor the auth layer into a 401).
func TestRouterSecurity_MFAEnrollmentToken_AllowedOnSetupRoute(t *testing.T) {
	// ARRANGE
	env, recorder := setupMFARouterEnv(t, true)
	token := issueMFAEnrollmentToken(t, env, env.fixtures.AdminUser)

	// ACT
	w := doRequest(t, env, http.MethodPost, "/api/profile/2fa/setup", token)

	// ASSERT
	require.NotEqual(t, http.StatusForbidden, w.Code,
		"the scope guard must allow the opted-in 2FA setup route; body=%s", w.Body.String())
	require.NotEqual(t, http.StatusUnauthorized, w.Code,
		"a valid enrollment token must pass auth on the 2FA setup route; body=%s", w.Body.String())
	assert.NotContains(t, w.Body.String(), "restricted to two-factor enrollment",
		"an opted-in route must not return the scope-restriction error")

	_, denied := findEvent(recorder.snapshot(), audit.EventAccessDenied)
	assert.False(t, denied, "no scope denial may be recorded for the opted-in setup route")
}

// TestRouterSecurity_MFAEnrollmentToken_HonoredWhenEnforcementDisabled covers
// OWASP API2:2023. With AUTH_REQUIRE_MFA_FOR_ADMINS=false a still-valid
// enrollment-scoped token (minted before the operator turned the flag off)
// must be honoured as a full session: no scope-403, no mfa_enrollment_scope
// audit denial — otherwise its bearer stays locked out of the whole API with
// no enrollment modal to escape through.
func TestRouterSecurity_MFAEnrollmentToken_HonoredWhenEnforcementDisabled(t *testing.T) {
	// ARRANGE
	env, recorder := setupMFARouterEnv(t, false)
	token := issueMFAEnrollmentToken(t, env, env.fixtures.AdminUser)

	// ACT
	w := doRequest(t, env, http.MethodGet, "/api/servers", token)

	// ASSERT
	require.NotEqual(t, http.StatusUnauthorized, w.Code,
		"a valid enrollment token must still authenticate; body=%s", w.Body.String())
	assert.NotContains(t, w.Body.String(), "restricted to two-factor enrollment",
		"the enrollment scope guard must stand down when the feature is disabled")

	ev, denied := findEvent(recorder.snapshot(), audit.EventAccessDenied)
	assert.False(t, denied,
		"no scope denial may be recorded when enforcement is disabled; got reason=%s", ev.Reason)
}

// TestRouterSecurity_NormalToken_NotBlockedByEnrollmentGuard covers OWASP
// API2:2023. A control: an ordinary (unscoped) admin token on GET /api/servers
// must NOT be touched by the enrollment scope guard — the guard fires only on
// the scope claim. The request may legitimately reach the handler (200) or hit
// backend behavior, but it must never be the scope-403 nor carry the
// scope-restriction body.
func TestRouterSecurity_NormalToken_NotBlockedByEnrollmentGuard(t *testing.T) {
	// ARRANGE
	env := setupSecurityTest(t)
	token := issuePASETOToken(t, env, env.fixtures.AdminUser)

	// ACT
	w := doRequest(t, env, http.MethodGet, "/api/servers", token)

	// ASSERT
	assert.NotContains(t, w.Body.String(), "restricted to two-factor enrollment",
		"an unscoped session must never trip the enrollment scope guard")
	if w.Code == http.StatusForbidden {
		assert.NotContains(t, w.Body.String(), "restricted to two-factor enrollment",
			"any 403 here must come from another gate, never the enrollment scope guard")
	}
}

// TestRouterSecurity_MFANudge_ProfileEmitsNudgeWhenEnabled covers OWASP API2:2023.
// End-to-end through the real router AND real RBAC (not a stubbed admin check):
// with AUTH_REQUIRE_MFA_FOR_ADMINS enabled, GET /api/profile must return the
// mfa_nudge block for an admin without 2FA — that block is exactly what the
// frontend enforcement modal binds to. A non-admin must never receive it.
//
// The flag is set BEFORE CreateRouter on purpose: the profile handler captures
// the nudge service (config-by-value) at wiring time, mirroring production,
// which reads the env var at startup before building the router.
func TestRouterSecurity_MFANudge_ProfileEmitsNudgeWhenEnabled(t *testing.T) {
	// ARRANGE
	c, err := testcontainer.LoadInmemoryContainer()
	require.NoError(t, err)
	c.Config().Auth.RequireMFAForAdmins = true

	fixtures, err := testcontainer.SetupFixtures(context.Background(), c)
	require.NoError(t, err)

	env := &securityTestEnv{
		container: c,
		fixtures:  fixtures,
		router:    api.CreateRouter(c),
		ctx:       context.Background(),
	}

	// ACT + ASSERT — admin without 2FA receives the nudge.
	adminResp := doRequest(t, env, http.MethodGet, "/api/profile", issuePASETOToken(t, env, env.fixtures.AdminUser))
	require.Equal(t, http.StatusOK, adminResp.Code, "body=%s", adminResp.Body.String())
	assert.Contains(t, adminResp.Body.String(), `"mfa_nudge"`,
		"an admin without 2FA must receive mfa_nudge when AUTH_REQUIRE_MFA_FOR_ADMINS is enabled")
	assert.Contains(t, adminResp.Body.String(), `"required":true`)
	assert.Contains(t, adminResp.Body.String(), `"show_now":true`)

	// Control — a non-admin must not receive the nudge.
	userResp := doRequest(t, env, http.MethodGet, "/api/profile", issuePASETOToken(t, env, env.fixtures.RegularUser))
	require.Equal(t, http.StatusOK, userResp.Code, "body=%s", userResp.Body.String())
	assert.NotContains(t, userResp.Body.String(), "mfa_nudge",
		"a non-admin must not receive the MFA nudge")
}
