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

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// controllableCache wraps a real in-memory cache so a test can force the
// short-lived-token failure branches that a real cache never exercises:
// a backend Get error (NOT key-not-found) and a Delete error. Get/Delete
// otherwise delegate, so the success path stays end-to-end real. A cache
// fake is acceptable here even in the classical style — the cache is the
// system boundary and these errors cannot be produced by the real one.
type controllableCache struct {
	delegate cache.Cache

	getErr    error
	deleteErr error
}

func newControllableCache() *controllableCache {
	return &controllableCache{delegate: cache.NewInMemory()}
}

func (c *controllableCache) Get(ctx context.Context, key string) (any, error) {
	if c.getErr != nil {
		return nil, c.getErr
	}

	return c.delegate.Get(ctx, key)
}

func (c *controllableCache) Set(
	ctx context.Context, key string, value any, options ...cache.Option,
) error {
	return c.delegate.Set(ctx, key, value, options...)
}

func (c *controllableCache) Delete(ctx context.Context, key string) error {
	if c.deleteErr != nil {
		return c.deleteErr
	}

	return c.delegate.Delete(ctx, key)
}

func (c *controllableCache) Clear(ctx context.Context) error {
	return c.delegate.Clear(ctx)
}

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

// TestAuthMiddleware_ShortLivedToken_CacheBackendError covers OWASP API2:2023:
// a cache backend failure (NOT key-not-found) must surface as a 500-class
// internal error — never a 401 and never an auth-rejection audit reason, so an
// infrastructure outage is not silently misreported as "token invalid" (which
// would also let an attacker probe backend health via the auth response).
func TestAuthMiddleware_ShortLivedToken_CacheBackendError(t *testing.T) {
	// ARRANGE
	userRepo := inmemory.NewUserRepository()
	c := newControllableCache()
	c.getErr = errors.New("redis cluster unreachable")

	recorder := &auditCapture{}
	mw := NewAuthMiddleware(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		inmemory.NewPersonalAccessTokenRepository(),
		auth.NoopRevocation{},
		c,
		api.NewResponder(),
		recorder,
	)
	handler := mw.Middleware(noopNextHandler())

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/ws/servers/1/console?token="+auth.ShortLivedTokenPrefix+"any-secret",
		http.NoBody,
	)
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"a cache backend outage must be a 500, not a 401; body=%s", w.Body.String())
	assert.NotEqual(t, http.StatusUnauthorized, w.Code,
		"an infrastructure error must never look like an auth rejection")

	_, rejected := findEvent(recorder.snapshot(), audit.EventAuthTokenRejected)
	assert.False(t, rejected,
		"an internal cache error must not be mislabelled as a token-rejected audit event")
}

// TestAuthMiddleware_ShortLivedToken_CorruptPayload covers OWASP API2:2023: a
// cached value that cannot be decoded into a payload must fail closed with 401
// and the stable shorttoken_invalid_or_expired audit reason — never construct
// a zero-valued session (which would authenticate as user id 0).
func TestAuthMiddleware_ShortLivedToken_CorruptPayload(t *testing.T) {
	// ARRANGE
	userRepo := inmemory.NewUserRepository()
	c := cache.NewInMemory()

	secret := "corruptPayloadSecret000111222333444555666777888"
	token := auth.ShortLivedTokenPrefix + secret
	require.NoError(t, c.Set(
		context.Background(), auth.ShortLivedCacheKey(token), "this-is-not-json",
	))

	recorder := &auditCapture{}
	mw := NewAuthMiddleware(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		inmemory.NewPersonalAccessTokenRepository(),
		auth.NoopRevocation{},
		c,
		api.NewResponder(),
		recorder,
	)
	handler := mw.Middleware(noopNextHandler())

	req := httptest.NewRequest(
		http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody,
	)
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid or expired short-lived token")

	ev, ok := findEvent(recorder.snapshot(), audit.EventAuthTokenRejected)
	require.True(t, ok, "a corrupt-payload rejection must be audited")
	assert.Equal(t, audit.OutcomeFailure, ev.Outcome)
	assert.Equal(t, "shorttoken_invalid_or_expired", ev.Reason,
		"the stable short-token rejection reason must be recorded")
}

// TestAuthMiddleware_ShortLivedToken_UserNotFound covers OWASP API2:2023: a
// cached payload whose user no longer exists must be refused with 401 and the
// user_not_found reason — a deleted account cannot be resurrected by a token
// minted before deletion.
func TestAuthMiddleware_ShortLivedToken_UserNotFound(t *testing.T) {
	// ARRANGE — empty user repo: the payload references a user that is absent.
	userRepo := inmemory.NewUserRepository()
	c := cache.NewInMemory()
	token := seedShortToken(t, c, auth.ShortLivedPayload{
		UserID: 9999, Login: "ghost", Email: "ghost@example.com",
	})

	recorder := &auditCapture{}
	mw := NewAuthMiddleware(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		inmemory.NewPersonalAccessTokenRepository(),
		auth.NoopRevocation{},
		c,
		api.NewResponder(),
		recorder,
	)
	handler := mw.Middleware(noopNextHandler())

	req := httptest.NewRequest(
		http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody,
	)
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "user not found")

	ev, ok := findEvent(recorder.snapshot(), audit.EventAuthTokenRejected)
	require.True(t, ok, "a missing-user rejection must be audited")
	assert.Equal(t, "user_not_found", ev.Reason)
}

// TestAuthMiddleware_ShortLivedToken_DeleteErrorStillAuthenticates covers
// OWASP API2:2023: deleting the consumed token is best-effort. If the delete
// fails the request must still authenticate (a transient delete failure must
// not deny a legitimate first use); single-use is enforced by the delete on
// the happy path, not by failing the request when the delete itself errors.
func TestAuthMiddleware_ShortLivedToken_DeleteErrorStillAuthenticates(t *testing.T) {
	// ARRANGE
	user := shortTokenUser()
	userRepo := inmemory.NewUserRepository()
	require.NoError(t, userRepo.Save(context.Background(), user))

	c := newControllableCache()
	c.deleteErr = errors.New("delete temporarily unavailable")
	token := seedShortToken(t, c, auth.ShortLivedPayload{
		UserID: user.ID, Login: user.Login, Email: user.Email,
	})

	var captured *auth.Session
	mw := NewAuthMiddleware(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		inmemory.NewPersonalAccessTokenRepository(),
		auth.NoopRevocation{},
		c,
		api.NewResponder(),
		nil,
	)
	handler := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = auth.SessionFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(
		http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody,
	)
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code,
		"a best-effort delete failure must not deny the first use; body=%s", w.Body.String())
	require.NotNil(t, captured)
	assert.True(t, captured.ShortLived, "the session must still be flagged short-lived")
	assert.Equal(t, user.ID, captured.User.ID)
}

// TestAuthMiddleware_ShortLivedToken_PATDerivedIsSingleUse covers OWASP
// API2:2023: the single-use guarantee must also hold for a PAT-derived token —
// the cache entry is consumed before the session is built, so a captured
// PAT-scoped short token cannot be replayed any more than a session-derived one.
func TestAuthMiddleware_ShortLivedToken_PATDerivedIsSingleUse(t *testing.T) {
	// ARRANGE
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
		},
	})

	handler := newShortTokenMiddleware(c, userRepo).Middleware(noopNextHandler())

	// ACT — present the same PAT-derived token twice.
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(
		http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody,
	))
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(
		http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody,
	))

	// ASSERT
	require.Equal(t, http.StatusOK, first.Code, "first use of a PAT-derived token must succeed")
	require.Equal(t, http.StatusUnauthorized, second.Code,
		"a PAT-derived short token must not authenticate twice")

	_, err := c.Get(context.Background(), auth.ShortLivedCacheKey(token))
	assert.ErrorIs(t, err, cache.ErrNotFound,
		"the consumed PAT-derived token must be gone from the cache")
}

// TestAuthMiddleware_ShortLivedToken_RejectionEmitsExactlyOneEvent covers
// OWASP API2:2023: an unknown/expired short-lived token presented through the
// middleware must emit exactly one auth.token.rejected event with the stable
// shorttoken_invalid_or_expired reason and an anonymous (never a user) actor.
func TestAuthMiddleware_ShortLivedToken_RejectionEmitsExactlyOneEvent(t *testing.T) {
	// ARRANGE
	userRepo := inmemory.NewUserRepository()
	c := cache.NewInMemory()

	recorder := &auditCapture{}
	mw := NewAuthMiddleware(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		inmemory.NewPersonalAccessTokenRepository(),
		auth.NoopRevocation{},
		c,
		api.NewResponder(),
		recorder,
	)
	handler := mw.Middleware(noopNextHandler())

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/ws/servers/1/console?token="+auth.ShortLivedTokenPrefix+"never-issued",
		http.NoBody,
	)
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())

	events := recorder.snapshot()
	require.Equal(t, 1, countEvents(events, audit.EventAuthTokenRejected),
		"exactly one rejected-token event must be emitted per rejected short-lived token")

	ev, ok := findEvent(events, audit.EventAuthTokenRejected)
	require.True(t, ok)
	assert.Equal(t, audit.OutcomeFailure, ev.Outcome, "a rejected short token is a failure")
	assert.Equal(t, audit.CategoryAuthentication, ev.Category)
	assert.Equal(t, "shorttoken_invalid_or_expired", ev.Reason,
		"the stable short-token rejection reason must be recorded")
	assert.Equal(t, audit.AuthMethodAnonymous, ev.AuthMethod,
		"a caller that never authenticated must be attributed anonymous")
	assert.Zero(t, ev.ActorID, "no actor id may be attributed to an unauthenticated rejection")
}

// TestAuthMiddleware_ShortLivedToken_RevocationDoesNotInterfere covers OWASP
// API2:2023: a real cache-backed revocation denylist over the SAME cache the
// short token lives in must not collide with it. The auth:revoked: and
// auth:shorttoken: namespaces are disjoint, so a freshly minted short token
// (never revoked) still authenticates and the post-auth revocation check is
// harmless for it.
func TestAuthMiddleware_ShortLivedToken_RevocationDoesNotInterfere(t *testing.T) {
	// ARRANGE
	user := shortTokenUser()
	userRepo := inmemory.NewUserRepository()
	require.NoError(t, userRepo.Save(context.Background(), user))

	c := cache.NewInMemory()
	token := seedShortToken(t, c, auth.ShortLivedPayload{
		UserID: user.ID, Login: user.Login, Email: user.Email,
	})

	var captured *auth.Session
	mw := NewAuthMiddleware(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		inmemory.NewPersonalAccessTokenRepository(),
		auth.NewCacheRevocation(c), // shares the very cache the short token is in
		c,
		api.NewResponder(),
		nil,
	)
	handler := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = auth.SessionFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(
		http.MethodGet, "/api/ws/servers/1/console?token="+token, http.NoBody,
	)
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code,
		"a non-revoked short token must authenticate even with a cache-backed revocation wired; body=%s",
		w.Body.String())
	require.NotNil(t, captured)
	assert.True(t, captured.ShortLived)
	assert.Equal(t, user.ID, captured.User.ID,
		"the revocation namespace must not shadow the short-token cache entry")
}
