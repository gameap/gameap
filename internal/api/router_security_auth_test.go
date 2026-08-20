// API Security Tests for OWASP API Security Top 10:2023.
// Category: API2:2023 — Broken Authentication.
// Reference: https://owasp.org/API-Security/editions/2023/en/0xa2-broken-authentication/
//
// These tests verify that the API correctly rejects requests with missing,
// malformed, expired, tampered, or revoked credentials, and that authenticated
// endpoints cannot be reached without valid authentication.

package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tokenKind int

const (
	tokenKindNone tokenKind = iota
	tokenKindValidRegularPASETO
	tokenKindValidAdminPASETO
	tokenKindExpiredRegularPASETO
	tokenKindValidAdminPAT
	tokenKindValidRegularPAT
	tokenKindSignedWithWrongKey
	tokenKindGarbageJWTPrefix
	tokenKindGarbagePASETOPrefix
	tokenKindPATUnknownID
	tokenKindPATWrongSecret
	tokenKindPATNegativeID
	tokenKindPATZeroID
	tokenKindPATForRemovedUser
	tokenKindRawGarbage
)

// TestRouterSecurity_API2_BrokenAuthentication covers OWASP API2:2023.
// Asserts 401 Unauthorized for the most common authentication-bypass attempts:
// missing token, malformed token, signed-with-wrong-key, expired token,
// invalid PAT id/secret, PAT for a removed user, garbage payloads.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API2_BrokenAuthentication(t *testing.T) {
	tests := []struct {
		name           string
		request        string
		kind           tokenKind
		wantStatusCode int
	}{
		// Missing token paths
		{"no_token_on_user_endpoint_returns_401", "GET /api/user", tokenKindNone, http.StatusUnauthorized},
		{"no_token_on_servers_list_returns_401", "GET /api/servers", tokenKindNone, http.StatusUnauthorized},
		{"no_token_on_admin_endpoint_returns_401", "GET /api/users", tokenKindNone, http.StatusUnauthorized},
		{"no_token_on_tokens_endpoint_returns_401", "GET /api/tokens", tokenKindNone, http.StatusUnauthorized},

		// Public endpoints must remain reachable without authentication
		{"public_config_allows_anon", "GET /api/config/public", tokenKindNone, http.StatusOK},

		// Malformed / garbage tokens
		{"garbage_token_returns_401", "GET /api/user", tokenKindRawGarbage, http.StatusUnauthorized},
		{"jwt_prefix_only_returns_401", "GET /api/user", tokenKindGarbageJWTPrefix, http.StatusUnauthorized},
		{"paseto_prefix_only_returns_401", "GET /api/user", tokenKindGarbagePASETOPrefix, http.StatusUnauthorized},

		// Signature attacks
		{"signed_with_wrong_key_returns_401", "GET /api/user", tokenKindSignedWithWrongKey, http.StatusUnauthorized},

		// Expiration
		{"expired_paseto_returns_401", "GET /api/user", tokenKindExpiredRegularPASETO, http.StatusUnauthorized},

		// Personal Access Token edge cases
		{"pat_with_unknown_id_returns_401", "GET /api/servers", tokenKindPATUnknownID, http.StatusUnauthorized},
		{"pat_with_wrong_secret_returns_401", "GET /api/servers", tokenKindPATWrongSecret, http.StatusUnauthorized},
		{"pat_with_negative_id_returns_401", "GET /api/servers", tokenKindPATNegativeID, http.StatusUnauthorized},
		{"pat_with_zero_id_returns_401", "GET /api/servers", tokenKindPATZeroID, http.StatusUnauthorized},
		{"pat_for_removed_user_returns_401", "GET /api/servers", tokenKindPATForRemovedUser, http.StatusUnauthorized},

		// Sanity checks: valid tokens grant access (the inverse case for confidence)
		{"valid_regular_paseto_grants_access_to_user", "GET /api/user", tokenKindValidRegularPASETO, http.StatusOK},
		{"valid_admin_paseto_grants_access_to_users", "GET /api/users", tokenKindValidAdminPASETO, http.StatusOK},
		{"valid_regular_pat_grants_access_to_servers", "GET /api/servers", tokenKindValidRegularPAT, http.StatusOK},
		{"valid_admin_pat_grants_access_to_servers", "GET /api/servers", tokenKindValidAdminPAT, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupSecurityTest(t)

			method, path := parseMethodPath(tt.request)
			token := buildAuthTokenForKind(t, env, tt.kind)

			w := doRequest(t, env, method, path, token)

			assert.Equal(t,
				tt.wantStatusCode,
				w.Code,
				"unexpected status code, body=%s",
				w.Body.String(),
			)
		})
	}
}

// buildAuthTokenForKind centralises construction of all the various invalid/valid
// token variants used in API2 tests.
func buildAuthTokenForKind(t *testing.T, env *securityTestEnv, kind tokenKind) string {
	t.Helper()

	switch kind {
	case tokenKindNone:
		return ""
	case tokenKindValidRegularPASETO:
		return issuePASETOToken(t, env, env.fixtures.RegularUser)
	case tokenKindValidAdminPASETO:
		return issuePASETOToken(t, env, env.fixtures.AdminUser)
	case tokenKindExpiredRegularPASETO:
		return issueExpiredPASETOToken(t, env, env.fixtures.RegularUser)
	case tokenKindValidAdminPAT:
		return issuePAT(t, env, env.fixtures.AdminUser, []domain.PATAbility{
			domain.PATAbilityServerList,
		})
	case tokenKindValidRegularPAT:
		return issuePAT(t, env, env.fixtures.RegularUser, []domain.PATAbility{
			domain.PATAbilityServerList,
		})
	case tokenKindSignedWithWrongKey:
		// Sign a token with a different secret; the API's authService will fail validation.
		other := auth.NewJWTService([]byte("completely-different-secret-key-do-not-trust"))
		token, err := other.GenerateTokenForUser(env.fixtures.RegularUser, 0)
		require.NoError(t, err)

		return token
	case tokenKindGarbageJWTPrefix:
		return "eyJaaaa.bbbb.cccc"
	case tokenKindGarbagePASETOPrefix:
		return "v4.local.aaaaaaaaaaaa"
	case tokenKindPATUnknownID:
		return "99999999|some-random-secret-string-here"
	case tokenKindPATWrongSecret:
		// Issue a real PAT and then send a request with the right ID but a wrong secret.
		realToken := issuePAT(t, env, env.fixtures.RegularUser, []domain.PATAbility{
			domain.PATAbilityServerList,
		})
		var id uint
		_, err := fmt.Sscanf(realToken, "%d|", &id)
		require.NoError(t, err)

		return fmt.Sprintf("%d|wrong-secret-value", id)
	case tokenKindPATNegativeID:
		return "-1|whatever"
	case tokenKindPATZeroID:
		return "0|whatever"
	case tokenKindPATForRemovedUser:
		token := issuePAT(t, env, env.fixtures.RegularUser, []domain.PATAbility{
			domain.PATAbilityServerList,
		})
		require.NoError(t, env.container.UserRepository().Delete(env.ctx, env.fixtures.RegularUser.ID))

		return token
	case tokenKindRawGarbage:
		return "not.a.valid.token.at.all"
	default:
		t.Fatalf("unknown token kind: %d", kind)

		return ""
	}
}

// TestRouterSecurity_API2_TokenSchemes verifies how the API handles the Authorization header
// schemes other than Bearer (Basic / Digest / empty / case variants).
//
// Although by design the AuthMiddleware also reads the token from the `?token=` query string
// (for WebSocket connections) and from a `token` cookie (for browser apps), this test pins
// down the *current* contract so future regressions are caught.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API2_TokenSchemes(t *testing.T) {
	env := setupSecurityTest(t)
	validToken := issuePASETOToken(t, env, env.fixtures.RegularUser)

	tests := []struct {
		name           string
		header         string
		wantStatusCode int
	}{
		{"basic_auth_scheme_returns_401", "Basic " + validToken, http.StatusUnauthorized},
		{"digest_scheme_returns_401", "Digest " + validToken, http.StatusUnauthorized},
		{"empty_authorization_returns_401", "", http.StatusUnauthorized},
		{"bearer_with_no_value_returns_401", "Bearer ", http.StatusUnauthorized},
		// The middleware lowercases the scheme before matching, so this must succeed.
		{"lowercase_bearer_works", "bearer " + validToken, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}

			w := doRequestRaw(t, env, req)
			assert.Equal(t,
				tt.wantStatusCode,
				w.Code,
				"header=%q body=%s",
				tt.header,
				w.Body.String(),
			)
		})
	}
}

// TestRouterSecurity_API2_TokenViaQueryAndCookie verifies the per-source
// token transport policy after ASVS_L2 C-4:
//   - ?token= rejects every token type EXCEPT the short-lived glst_ token
//     (covered by the shorttoken handler tests); URLs leak into proxy logs.
//   - cookies accept session PASETOs but reject PATs (PAT is API-only).
//   - invalid tokens via either channel return 401.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API2_TokenViaQueryAndCookie(t *testing.T) {
	env := setupSecurityTest(t)
	valid := issuePASETOToken(t, env, env.fixtures.RegularUser)

	// C-4: long-lived PASETO via ?token= must be rejected so a logged URL
	// cannot replay the session. Authorization header is the canonical
	// channel for this credential.
	t.Run("paseto_in_query_string_rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/user?token="+valid, nil)
		w := doRequestRaw(t, env, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	})

	t.Run("invalid_token_via_query_string_returns_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/user?token=v4.local.bogus", nil)
		w := doRequestRaw(t, env, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	})

	t.Run("valid_token_via_cookie_works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
		req.AddCookie(&http.Cookie{Name: "token", Value: valid})
		w := doRequestRaw(t, env, req)
		assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	})

	t.Run("invalid_token_via_cookie_returns_401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/user", nil)
		req.AddCookie(&http.Cookie{Name: "token", Value: "eyJgarbage"})
		w := doRequestRaw(t, env, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	})
}

// TestRouterSecurity_API2_UserDeletedAfterTokenIssue verifies that a previously valid
// PASETO/JWT token cannot be used after the underlying user account is removed —
// the auth middleware re-resolves the user on every request via login lookup.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API2_UserDeletedAfterTokenIssue(t *testing.T) {
	env := setupSecurityTest(t)

	token := issuePASETOToken(t, env, env.fixtures.RegularUser)

	// Confirm token works while user exists.
	w := doRequest(t, env, http.MethodGet, "/api/user", token)
	require.Equal(t, http.StatusOK, w.Code, "precondition failed: token should work pre-deletion")

	require.NoError(t, env.container.UserRepository().Delete(env.ctx, env.fixtures.RegularUser.ID))

	w = doRequest(t, env, http.MethodGet, "/api/user", token)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "deleted user must lose API access; body=%s", w.Body.String())
}

// TestRouterSecurity_API2_PATSecretMustBeOpaque ensures that the secret half of a PAT
// is hashed before storage and is never accepted in raw form.
//
// This guards against a regression where the database might be queried with the
// plaintext secret instead of the SHA-256 hash.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API2_PATSecretMustBeOpaque(t *testing.T) {
	env := setupSecurityTest(t)

	// Create a PAT whose `Token` column stores the SHA-256 of "rawSecret123".
	hashed := pkgstrings.SHA256("rawSecret123")
	storedToken := &domain.PersonalAccessToken{
		TokenableType: domain.EntityTypeUser,
		TokenableID:   env.fixtures.RegularUser.ID,
		Name:          "opaque-test",
		Token:         hashed,
		Abilities: &[]domain.PATAbility{
			domain.PATAbilityServerList,
		},
	}
	require.NoError(t, env.container.PersonalAccessTokenRepository().Save(env.ctx, storedToken))

	// Sending the *hash* as the secret must fail (the middleware re-hashes the supplied secret).
	bearerWithHash := fmt.Sprintf("%d|%s", storedToken.ID, hashed)
	w := doRequest(t, env, http.MethodGet, "/api/servers", bearerWithHash)
	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"sending the stored hash directly must not authenticate; body=%s", w.Body.String())

	// Sending the original secret must succeed.
	bearerWithRaw := fmt.Sprintf("%d|%s", storedToken.ID, "rawSecret123")
	w = doRequest(t, env, http.MethodGet, "/api/servers", bearerWithRaw)
	assert.Equal(t, http.StatusOK, w.Code,
		"original secret should authenticate; body=%s", w.Body.String())
}

// TestRouterSecurity_API2_LogoutInvalidatesToken covers OWASP API2:2023 — once
// the user POSTs /api/auth/logout with a valid token, that exact token must be
// rejected on subsequent requests even though its `exp` is still in the future.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API2_LogoutInvalidatesToken(t *testing.T) {
	env := setupSecurityTest(t)
	token := issuePASETOToken(t, env, env.fixtures.RegularUser)

	// Sanity: the token works before logout.
	w := doRequest(t, env, http.MethodGet, "/api/user", token)
	require.Equalf(t, http.StatusOK, w.Code, "fresh token must work; body=%s", w.Body.String())

	// Logout returns 204 No Content.
	w = doRequest(t, env, http.MethodPost, "/api/auth/logout", token)
	require.Equalf(t, http.StatusNoContent, w.Code,
		"logout must succeed; body=%s", w.Body.String())

	// The same token must now be rejected.
	w = doRequest(t, env, http.MethodGet, "/api/user", token)
	assert.Equalf(t, http.StatusUnauthorized, w.Code,
		"revoked token must be rejected on subsequent requests; body=%s", w.Body.String())
}

// TestRouterSecurity_API2_LogoutRequiresAuth verifies that the logout endpoint
// itself requires authentication — anonymous callers get 401, not 204.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API2_LogoutRequiresAuth(t *testing.T) {
	env := setupSecurityTest(t)

	w := doRequest(t, env, http.MethodPost, "/api/auth/logout", "")
	assert.Equalf(t, http.StatusUnauthorized, w.Code,
		"logout without a token must return 401; body=%s", w.Body.String())
}

// TestRouterSecurity_API2_LoginBruteForceProtection covers OWASP API2:2023 and
// API4:2023 — repeated failed credential checks against /api/auth/login must
// trip the rate limiter and return 429 Too Many Requests, mitigating CWE-307.
//
// The default middleware permits 20 failures per IP and 5 per username within
// a 15-minute window. We exercise the per-username path because it is tightest
// and easiest to assert against without adjusting headers.
//
//nolint:paralleltest // api.CreateRouter mutates the unsynchronized package-global ability-check audit sink (data race in servers/base.SetAuditLogger).
func TestRouterSecurity_API2_LoginBruteForceProtection(t *testing.T) {
	env := setupSecurityTest(t)
	body := []byte(`{"login":"never-existed","password":"wrong"}`)

	// First 5 failures are simply 401 — the limiter only refuses on the 6th.
	for i := range 5 {
		w := doRequestWithBody(t, env, http.MethodPost, "/api/auth/login", "", body)
		require.Equalf(t, http.StatusUnauthorized, w.Code,
			"failed login %d should still return 401; body=%s", i+1, w.Body.String())
	}

	w := doRequestWithBody(t, env, http.MethodPost, "/api/auth/login", "", body)
	assert.Equalf(t, http.StatusTooManyRequests, w.Code,
		"6th failed login for the same username must be rate-limited; body=%s", w.Body.String())
	assert.NotEmptyf(t, w.Header().Get("Retry-After"),
		"429 response must include a Retry-After header")
}
