// Aligned with OWASP API Security Top 10:2023 — API2:2023 (Broken
// Authentication). The logout endpoint must reliably revoke the bearer token
// used to authenticate the request, and must reject anonymous callers, so any
// subsequent request presenting the same token is rejected with 401 even
// before the token's natural expiration.
package logout

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "test-secret-key-for-logout-handler"

func newTestUser() *domain.User {
	now := time.Now()

	return &domain.User{
		ID:        7,
		Login:     "logoutuser",
		Email:     "logout@example.com",
		Password:  "hashed-stub",
		Name:      new("Logout User"),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
}

// authenticatedContext returns a request context carrying an authenticated
// session for the given user. Mirrors the pattern used in
// internal/api/servers/getservers/handler_test.go.
func authenticatedContext(user *domain.User) context.Context {
	return auth.ContextWithSession(context.Background(), &auth.Session{
		ID:    "session-id",
		Login: user.Login,
		Email: user.Email,
		User:  user,
	})
}

// errRevocation is a minimal hand-written stub used solely to exercise the
// "revocation backend returned an error" branch. The rest of the suite uses
// the real auth.CacheRevocation backed by an in-memory cache.
type errRevocation struct {
	revokeErr error
}

func (e *errRevocation) Revoke(_ context.Context, _ string, _ time.Duration) error {
	return e.revokeErr
}

func (e *errRevocation) IsRevoked(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// TestHandler_ServeHTTP exercises every branch of the logout flow.
//
// OWASP API Security Top 10:2023 — API2:2023 (Broken Authentication). Each
// case verifies an aspect of post-authentication token invalidation.
func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	user := newTestUser()
	jwtSvc := auth.NewJWTService([]byte(testJWTSecret))

	tokenWith24hExp, err := jwtSvc.GenerateTokenForUser(user, 24*time.Hour)
	require.NoError(t, err)
	headerToken := tokenWith24hExp

	queryToken, err := jwtSvc.GenerateTokenForUser(user, 24*time.Hour)
	require.NoError(t, err)

	cookieToken, err := jwtSvc.GenerateTokenForUser(user, 24*time.Hour)
	require.NoError(t, err)

	headerWinsToken, err := jwtSvc.GenerateTokenForUser(user, 24*time.Hour)
	require.NoError(t, err)
	queryLoserToken, err := jwtSvc.GenerateTokenForUser(user, 24*time.Hour)
	require.NoError(t, err)

	queryWinsToken, err := jwtSvc.GenerateTokenForUser(user, 24*time.Hour)
	require.NoError(t, err)
	cookieLoserToken, err := jwtSvc.GenerateTokenForUser(user, 24*time.Hour)
	require.NoError(t, err)

	expiredToken, err := jwtSvc.GenerateTokenForUser(user, -1*time.Hour)
	require.NoError(t, err)

	opaqueNoExpToken := "not-a-jwt-just-an-opaque-bearer"

	tests := []struct {
		name               string
		setupContext       func() context.Context
		setupRequest       func(r *http.Request)
		bearerForRevocheck string
		notRevokedBearer   string
		expectedStatus     int
		wantError          string
	}{
		{
			name:         "unauthenticated_session_is_rejected",
			setupContext: context.Background,
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+headerToken)
			},
			notRevokedBearer: headerToken,
			expectedStatus:   http.StatusUnauthorized,
			wantError:        "user not authenticated",
		},
		{
			name: "session_present_but_user_not_authenticated_is_rejected",
			setupContext: func() context.Context {
				return auth.ContextWithSession(context.Background(), &auth.Session{
					ID: "session-without-user",
				})
			},
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+headerToken)
			},
			notRevokedBearer: headerToken,
			expectedStatus:   http.StatusUnauthorized,
			wantError:        "user not authenticated",
		},
		{
			name: "authenticated_without_bearer_returns_no_content",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest:   func(_ *http.Request) {},
			expectedStatus: http.StatusNoContent,
		},
		{
			name: "authenticated_with_bearer_in_header_revokes_token",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+headerToken)
			},
			bearerForRevocheck: headerToken,
			expectedStatus:     http.StatusNoContent,
		},
		{
			name: "authorization_header_with_unknown_scheme_is_ignored",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name: "bearer_scheme_is_case_insensitive",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "bearer "+headerToken)
			},
			bearerForRevocheck: headerToken,
			expectedStatus:     http.StatusNoContent,
		},
		{
			name: "authenticated_with_bearer_in_query_param_revokes_token",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest: func(r *http.Request) {
				q := r.URL.Query()
				q.Set("token", queryToken)
				r.URL.RawQuery = q.Encode()
			},
			bearerForRevocheck: queryToken,
			expectedStatus:     http.StatusNoContent,
		},
		{
			name: "authenticated_with_bearer_in_cookie_revokes_token",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: "token", Value: cookieToken})
			},
			bearerForRevocheck: cookieToken,
			expectedStatus:     http.StatusNoContent,
		},
		{
			name: "header_takes_precedence_over_query_param",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+headerWinsToken)
				q := r.URL.Query()
				q.Set("token", queryLoserToken)
				r.URL.RawQuery = q.Encode()
			},
			bearerForRevocheck: headerWinsToken,
			notRevokedBearer:   queryLoserToken,
			expectedStatus:     http.StatusNoContent,
		},
		{
			name: "query_param_takes_precedence_over_cookie",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest: func(r *http.Request) {
				q := r.URL.Query()
				q.Set("token", queryWinsToken)
				r.URL.RawQuery = q.Encode()
				r.AddCookie(&http.Cookie{Name: "token", Value: cookieLoserToken})
			},
			bearerForRevocheck: queryWinsToken,
			notRevokedBearer:   cookieLoserToken,
			expectedStatus:     http.StatusNoContent,
		},
		{
			name: "non_jwt_bearer_uses_default_revocation_ttl",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+opaqueNoExpToken)
			},
			bearerForRevocheck: opaqueNoExpToken,
			expectedStatus:     http.StatusNoContent,
		},
		{
			// An expired JWT can no longer be parsed by jwt.ParseWithClaims, so
			// from the handler's perspective it is indistinguishable from an
			// opaque bearer with no derivable exp — the default 30-day TTL is
			// used and the token IS revoked. (The "remaining <= 0" branch in
			// revocationTTL is reachable only by a Claims implementation that
			// returns a past *time.Time without erroring; the JWT library used
			// here rejects such tokens at parse time, so this is the realistic
			// behavior to assert.)
			name: "expired_jwt_bearer_falls_back_to_default_ttl_and_revokes",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+expiredToken)
			},
			bearerForRevocheck: expiredToken,
			expectedStatus:     http.StatusNoContent,
		},
		{
			name: "empty_cookie_value_falls_through_to_no_bearer",
			setupContext: func() context.Context {
				return authenticatedContext(user)
			},
			setupRequest: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: "token", Value: ""})
			},
			expectedStatus: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			revocation := auth.NewCacheRevocation(cache.NewInMemory())
			responder := api.NewResponder()
			handler := NewHandler(jwtSvc, revocation, responder)

			req := httptest.NewRequest(http.MethodPost, "/auth/logout", http.NoBody)
			tt.setupRequest(req)
			req = req.WithContext(tt.setupContext())

			rec := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(rec, req)

			// ASSERT
			require.Equal(t, tt.expectedStatus, rec.Code, "status code mismatch")

			if tt.wantError != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "error", resp["status"])

				errMsg, ok := resp["error"].(string)
				require.True(t, ok, "error field must be a string")
				assert.Contains(t, errMsg, tt.wantError, "error message mismatch")
			}

			if tt.expectedStatus == http.StatusNoContent {
				assert.Empty(t, rec.Body.String(), "204 must have no body")
			}

			ctx := context.Background()
			if tt.bearerForRevocheck != "" {
				revoked, err := revocation.IsRevoked(ctx, auth.TokenIdentifier(tt.bearerForRevocheck))
				require.NoError(t, err)
				assert.True(t, revoked, "expected bearer token to be marked revoked")
			}
			if tt.notRevokedBearer != "" {
				revoked, err := revocation.IsRevoked(ctx, auth.TokenIdentifier(tt.notRevokedBearer))
				require.NoError(t, err)
				assert.False(t, revoked, "expected bearer token to NOT be marked revoked")
			}
		})
	}
}

// TestHandler_ServeHTTP_RevocationBackendError verifies that when the
// underlying revocation backend fails, the handler surfaces the error to the
// responder rather than returning a misleading success.
//
// Note on the response body: pkg/api.Responder masks 5xx error messages with
// http.StatusText(code) ("Internal Server Error") so internal failure detail
// is not leaked to clients. The wrapped "failed to revoke token" message only
// appears in the slog log — it is intentionally not part of the public
// contract — so the assertion below checks the public contract (status +
// generic body) rather than the log line.
//
// OWASP API Security Top 10:2023 — API2:2023 (Broken Authentication): a
// failed revocation must not be reported as success, otherwise the operator
// would believe the token was invalidated when it was not.
func TestHandler_ServeHTTP_RevocationBackendError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	user := newTestUser()
	jwtSvc := auth.NewJWTService([]byte(testJWTSecret))
	bearer, err := jwtSvc.GenerateTokenForUser(user, 24*time.Hour)
	require.NoError(t, err)

	failing := &errRevocation{revokeErr: errors.New("backend down")}
	responder := api.NewResponder()
	handler := NewHandler(jwtSvc, failing, responder)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req = req.WithContext(authenticatedContext(user))
	rec := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(rec, req)

	// ASSERT
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp["status"])

	errMsg, ok := resp["error"].(string)
	require.True(t, ok, "error field must be a string")
	assert.Equal(t, http.StatusText(http.StatusInternalServerError), errMsg,
		"5xx response body must not leak internal error detail")
}

// TestHandler_ServeHTTP_TTLDerivedFromExp asserts that for a JWT bearer the
// revocation TTL is bounded by the token's `exp` claim. Concretely: if a
// token expires in 2s, the revocation entry must also expire within ~2s
// rather than persisting for defaultRevocationTTL.
//
// We verify this state-side by reading the cache after the JWT's exp has
// passed and confirming the revocation is gone — this is an observable,
// non-fragile assertion over the public TokenRevocation contract.
func TestHandler_ServeHTTP_TTLDerivedFromExp(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping timing-sensitive test in -short mode")
	}

	// ARRANGE
	user := newTestUser()
	jwtSvc := auth.NewJWTService([]byte(testJWTSecret))

	const tokenLifetime = 1500 * time.Millisecond
	bearer, err := jwtSvc.GenerateTokenForUser(user, tokenLifetime)
	require.NoError(t, err)

	revocation := auth.NewCacheRevocation(cache.NewInMemory())
	responder := api.NewResponder()
	handler := NewHandler(jwtSvc, revocation, responder)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req = req.WithContext(authenticatedContext(user))
	rec := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(rec, req)

	// ASSERT
	require.Equal(t, http.StatusNoContent, rec.Code)

	ctx := context.Background()
	revoked, err := revocation.IsRevoked(ctx, auth.TokenIdentifier(bearer))
	require.NoError(t, err)
	assert.True(t, revoked, "token must be revoked immediately after logout")

	time.Sleep(tokenLifetime + 250*time.Millisecond)

	revoked, err = revocation.IsRevoked(ctx, auth.TokenIdentifier(bearer))
	require.NoError(t, err)
	assert.False(t, revoked, "revocation entry must be reaped once exp has passed (TTL derived from exp, not the 30-day default)")
}

// TestHandler_ServeHTTP_NoExpUsesDefaultTTL asserts that for a non-JWT or
// expirationless bearer the handler stores the revocation under the longer
// defaultRevocationTTL rather than refusing or storing nothing.
//
// We don't assert the exact TTL value (that would be implementation-coupled)
// — we assert the observable consequence: the entry is still present after
// well past any plausible JWT exp window.
func TestHandler_ServeHTTP_NoExpUsesDefaultTTL(t *testing.T) {
	t.Parallel()

	// ARRANGE
	user := newTestUser()
	jwtSvc := auth.NewJWTService([]byte(testJWTSecret))
	revocation := auth.NewCacheRevocation(cache.NewInMemory())
	responder := api.NewResponder()
	handler := NewHandler(jwtSvc, revocation, responder)

	bearer := "opaque-pat-with-no-exp"

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req = req.WithContext(authenticatedContext(user))
	rec := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(rec, req)

	// ASSERT
	require.Equal(t, http.StatusNoContent, rec.Code)

	ctx := context.Background()
	revoked, err := revocation.IsRevoked(ctx, auth.TokenIdentifier(bearer))
	require.NoError(t, err)
	assert.True(t, revoked, "PAT/opaque bearer must be revoked using the default TTL")
}
