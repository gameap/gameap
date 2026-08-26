// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — the 2FA challenge token issued by the
//     login endpoint must NOT be accepted by the auth middleware as a session
//     credential. It uses a prefix the prefix-router does not recognise, so it
//     can only ever be redeemed at /api/auth/2fa/verify (which reads it from
//     the cache directly) and never exchanged for an authenticated session by
//     presenting it as a Bearer/query/cookie token. This scope confinement is
//     the core security invariant of the two-step login.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package middlewares

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedTwoFactorChallenge(t *testing.T, c cache.Cache, userID uint, login, email string) string {
	t.Helper()

	token := twofactor.ChallengeTokenPrefix + "seeded-2fa-challenge-secret-0123456789abcdef"

	encoded, err := twofactor.MarshalChallengePayload(twofactor.ChallengePayload{
		UserID:    userID,
		Login:     login,
		Email:     email,
		ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
	})
	require.NoError(t, err)
	require.NoError(t, c.Set(context.Background(), twofactor.ChallengeCacheKey(token), encoded))

	return token
}

// TestAuthMiddleware_TwoFactorChallenge_NotAcceptedAsBearer covers OWASP
// API2:2023: even with a valid challenge payload seeded in the very cache the
// middleware uses, presenting the challenge token in the Authorization header
// must be refused with 401, must not reach the protected handler, and must not
// consume the challenge cache entry.
func TestAuthMiddleware_TwoFactorChallenge_NotAcceptedAsBearer(t *testing.T) {
	t.Parallel()
	user := &domain.User{ID: 77, Login: "bob", Email: "bob@example.com", TwoFactorEnabled: true}
	userRepo := inmemory.NewUserRepository()
	require.NoError(t, userRepo.Save(context.Background(), user))

	c := cache.NewInMemory()
	token := seedTwoFactorChallenge(t, c, user.ID, user.Login, user.Email)

	reached := false
	handler := newShortTokenMiddleware(c, userRepo).Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/profile", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	assert.False(t, reached, "a 2FA challenge token must never reach a protected handler")
	assert.Contains(t, w.Body.String(), "invalid or expired token")

	_, getErr := c.Get(context.Background(), twofactor.ChallengeCacheKey(token))
	assert.NoError(t, getErr,
		"the auth middleware must not read or consume the 2FA challenge cache entry")
}

// TestAuthMiddleware_TwoFactorChallenge_NotAcceptedAsQueryToken covers OWASP
// API2:2023: the WebSocket-style ?token= vector must also reject the challenge
// token — scope confinement cannot be side-stepped by moving the token to the
// URL.
func TestAuthMiddleware_TwoFactorChallenge_NotAcceptedAsQueryToken(t *testing.T) {
	t.Parallel()
	user := &domain.User{ID: 78, Login: "carol", Email: "carol@example.com", TwoFactorEnabled: true}
	userRepo := inmemory.NewUserRepository()
	require.NoError(t, userRepo.Save(context.Background(), user))

	c := cache.NewInMemory()
	token := seedTwoFactorChallenge(t, c, user.ID, user.Login, user.Email)

	handler := newShortTokenMiddleware(c, userRepo).Middleware(noopNextHandler())

	req := httptest.NewRequest(
		http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody,
	)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid or expired token")
}
