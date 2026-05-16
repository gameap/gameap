// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — the short-lived token validator must
//     accept a token exactly once (single-use: consumed before the session is
//     built), reject an unknown/expired token, mark the resulting session so
//     the scope guard can constrain it, and never widen authority beyond the
//     PAT that minted it.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package middlewares

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func shortTokenUser() *domain.User {
	return &domain.User{ID: 42, Login: "alice", Email: "alice@example.com"}
}

func seedShortToken(t *testing.T, c cache.Cache, payload auth.ShortLivedPayload) string {
	t.Helper()

	secret := "aB3dE5fG7hJ9kL1mN3pQ5rS7tU9vW1xY3zA5bC7dE9fG1hJ"
	token := auth.ShortLivedTokenPrefix + secret

	encoded, err := auth.MarshalShortLivedPayload(payload)
	require.NoError(t, err)
	require.NoError(t, c.Set(context.Background(), auth.ShortLivedCacheKey(token), encoded))

	return token
}

func newShortTokenMiddleware(c cache.Cache, userRepo *inmemory.UserRepository) *AuthMiddleware {
	return NewAuthMiddleware(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		inmemory.NewPersonalAccessTokenRepository(),
		auth.NoopRevocation{},
		c,
		api.NewResponder(),
		nil,
	)
}

// TestAuthMiddleware_ShortLivedToken_Accepted covers OWASP API2:2023: a valid
// short-lived token authenticates the request and the reconstructed session is
// flagged ShortLived so downstream scope enforcement can apply.
func TestAuthMiddleware_ShortLivedToken_Accepted(t *testing.T) {
	user := shortTokenUser()
	userRepo := inmemory.NewUserRepository()
	require.NoError(t, userRepo.Save(context.Background(), user))

	c := cache.NewInMemory()
	token := seedShortToken(t, c, auth.ShortLivedPayload{
		UserID: user.ID, Login: user.Login, Email: user.Email,
	})

	var captured *auth.Session
	handler := newShortTokenMiddleware(c, userRepo).Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = auth.SessionFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.NotNil(t, captured)
	assert.True(t, captured.IsAuthenticated())
	assert.True(t, captured.ShortLived, "a short-lived token must flag the session")
	assert.Equal(t, user.ID, captured.User.ID)
	assert.Nil(t, captured.Token, "a session-derived short token carries no PAT")
}

// TestAuthMiddleware_ShortLivedToken_SingleUse covers OWASP API2:2023: the
// token must authenticate at most once — the second presentation is rejected
// because the first consumed (deleted) the cache entry.
func TestAuthMiddleware_ShortLivedToken_SingleUse(t *testing.T) {
	user := shortTokenUser()
	userRepo := inmemory.NewUserRepository()
	require.NoError(t, userRepo.Save(context.Background(), user))

	c := cache.NewInMemory()
	token := seedShortToken(t, c, auth.ShortLivedPayload{
		UserID: user.ID, Login: user.Login, Email: user.Email,
	})

	handler := newShortTokenMiddleware(c, userRepo).Middleware(noopNextHandler())

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(
		http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody,
	))
	require.Equal(t, http.StatusOK, first.Code, "first use must succeed")

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(
		http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody,
	))
	require.Equal(t, http.StatusUnauthorized, second.Code,
		"a short-lived token must not authenticate twice")

	_, err := c.Get(context.Background(), auth.ShortLivedCacheKey(token))
	assert.ErrorIs(t, err, cache.ErrNotFound, "the consumed token must be gone from the cache")
}

// TestAuthMiddleware_ShortLivedToken_Rejected covers OWASP API2:2023: an
// unknown or expired short-lived token is refused with 401.
func TestAuthMiddleware_ShortLivedToken_Rejected(t *testing.T) {
	userRepo := inmemory.NewUserRepository()
	c := cache.NewInMemory()

	handler := newShortTokenMiddleware(c, userRepo).Middleware(noopNextHandler())

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/ws/servers/1/console?token="+auth.ShortLivedTokenPrefix+"never-issued-secret",
		http.NoBody,
	)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid or expired short-lived token")
}

// TestAuthMiddleware_ShortLivedToken_InheritsPATAbilities covers OWASP
// API2:2023: a token minted from a PAT session must reconstruct a session
// bound to the PAT id and abilities, so it is never broader than its origin.
func TestAuthMiddleware_ShortLivedToken_InheritsPATAbilities(t *testing.T) {
	user := shortTokenUser()
	userRepo := inmemory.NewUserRepository()
	require.NoError(t, userRepo.Save(context.Background(), user))

	c := cache.NewInMemory()
	token := seedShortToken(t, c, auth.ShortLivedPayload{
		UserID: user.ID,
		Login:  user.Login,
		Email:  user.Email,
		PATID:  7,
		Abilities: []domain.PATAbility{
			domain.PATAbilityServerList,
			domain.PATAbilityServerConsole,
		},
	})

	var captured *auth.Session
	handler := newShortTokenMiddleware(c, userRepo).Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = auth.SessionFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	require.NotNil(t, captured)
	assert.True(t, captured.ShortLived)
	require.True(t, captured.IsTokenSession(), "a PAT-derived short token must be a token session")
	assert.Equal(t, uint(7), captured.Token.ID)
	require.NotNil(t, captured.Token.Abilities)
	assert.Equal(t, []domain.PATAbility{
		domain.PATAbilityServerList,
		domain.PATAbilityServerConsole,
	}, *captured.Token.Abilities)
}

// TestAuthMiddleware_ShortLivedToken_DisabledWithoutCache covers OWASP
// API2:2023: with no cache wired the prefix must not be treated as a valid
// credential — it falls through to the unknown-token rejection.
func TestAuthMiddleware_ShortLivedToken_DisabledWithoutCache(t *testing.T) {
	mw := NewAuthMiddleware(
		auth.NewJWTService([]byte(testJWTSecret)),
		inmemory.NewUserRepository(),
		inmemory.NewPersonalAccessTokenRepository(),
		auth.NoopRevocation{},
		nil,
		api.NewResponder(),
		nil,
	)
	handler := mw.Middleware(noopNextHandler())

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/ws/servers/1/console?token="+auth.ShortLivedTokenPrefix+"whatever",
		http.NoBody,
	)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid or expired token")
}
