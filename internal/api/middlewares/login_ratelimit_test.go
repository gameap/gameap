package middlewares

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loginAttemptHandler is a stub for the wrapped login handler. It echoes the
// configured outcome without inspecting the request body.
type loginAttemptHandler struct {
	outcome int
}

func (h *loginAttemptHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(h.outcome)
}

func newLoginRequest(t *testing.T, login, password, remoteAddr string) *http.Request {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"login":    login,
		"password": password,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", "application/json")

	return req
}

func TestLoginRateLimitMiddleware_BlocksAfterMaxFailuresPerUsername(t *testing.T) {
	t.Parallel()
	c := cache.NewInMemory()
	mw := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerUsername(3),
		WithLoginRateLimitPerIP(100),
		WithLoginRateLimitWindow(time.Minute),
	)
	handler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

	for i := range 3 {
		req := newLoginRequest(t, "alice", "wrong", "10.0.0.1:1234")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equalf(t, http.StatusUnauthorized, w.Code,
			"attempt %d should reach the inner handler; body=%s", i, w.Body.String())
	}

	req := newLoginRequest(t, "alice", "wrong", "10.0.0.1:1234")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "fourth attempt must be 429")
	assert.NotEmpty(t, w.Header().Get("Retry-After"), "429 must set Retry-After")
}

func TestLoginRateLimitMiddleware_BlocksAfterMaxFailuresPerIP(t *testing.T) {
	t.Parallel()
	c := cache.NewInMemory()
	mw := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerIP(2),
		WithLoginRateLimitPerUsername(100),
		WithLoginRateLimitWindow(time.Minute),
	)
	handler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

	// Attempts vary the username so the per-username counter never trips —
	// the per-IP counter is what blocks them.
	for i, login := range []string{"alice", "bob"} {
		req := newLoginRequest(t, login, "wrong", "10.0.0.2:1234")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equalf(t, http.StatusUnauthorized, w.Code, "attempt %d (login=%s) reached inner", i, login)
	}

	req := newLoginRequest(t, "carol", "wrong", "10.0.0.2:1234")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "third IP attempt must be 429 even with a fresh username")
}

func TestLoginRateLimitMiddleware_SeparateIPBuckets(t *testing.T) {
	t.Parallel()
	c := cache.NewInMemory()
	mw := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerIP(1),
		WithLoginRateLimitPerUsername(100),
		WithLoginRateLimitWindow(time.Minute),
	)
	handler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

	// First IP exhausts its limit.
	req := newLoginRequest(t, "alice", "wrong", "10.0.0.10:1234")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	// Second IP must not inherit the first IP's failure count.
	req = newLoginRequest(t, "alice", "wrong", "10.0.0.11:1234")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "different IP must have independent counter")
}

func TestLoginRateLimitMiddleware_SuccessResetsUsernameCounter(t *testing.T) {
	t.Parallel()
	c := cache.NewInMemory()
	mw := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerUsername(2),
		WithLoginRateLimitPerIP(100),
		WithLoginRateLimitWindow(time.Minute),
	)
	failHandler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})
	okHandler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusOK})

	// Burn one failure for alice.
	req := newLoginRequest(t, "alice", "wrong", "10.0.0.20:1")
	w := httptest.NewRecorder()
	failHandler.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	// A successful login resets her counter.
	req = newLoginRequest(t, "alice", "right", "10.0.0.20:2")
	w = httptest.NewRecorder()
	okHandler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// She should be allowed two more failures before the per-username limit
	// trips, proving the previous one was reset.
	for i := range 2 {
		req = newLoginRequest(t, "alice", "wrong", "10.0.0.20:3")
		w = httptest.NewRecorder()
		failHandler.ServeHTTP(w, req)
		assert.Equalf(t, http.StatusUnauthorized, w.Code, "post-reset attempt %d should reach inner", i)
	}
}

func TestLoginRateLimitMiddleware_BodyForwardedToNextHandler(t *testing.T) {
	t.Parallel()
	// The middleware reads the request body to peek at the username; it must
	// then restore the body so the wrapped login handler can decode it again.
	c := cache.NewInMemory()
	mw := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerIP(100),
		WithLoginRateLimitPerUsername(100),
	)

	var seen []byte
	wrapped := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))

	req := newLoginRequest(t, "alice", "secret", "10.0.0.30:1")
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	assert.Contains(t, string(seen), `"login":"alice"`,
		"inner handler must still see the JSON body verbatim")
}

func TestLoginRateLimitMiddleware_EmailFieldUsedAsUsername(t *testing.T) {
	t.Parallel()
	c := cache.NewInMemory()
	mw := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerUsername(2),
		WithLoginRateLimitPerIP(100),
		WithLoginRateLimitWindow(time.Minute),
	)
	handler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

	for i := range 2 {
		body, err := json.Marshal(map[string]string{
			"email":    "Alice@Example.COM",
			"password": "wrong",
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		req.RemoteAddr = "10.0.0.40:1"
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equalf(t, http.StatusUnauthorized, w.Code, "attempt %d", i)
	}

	// Third attempt must be 429 — and case must not matter.
	body, err := json.Marshal(map[string]string{
		"email":    "ALICE@example.com",
		"password": "wrong",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.RemoteAddr = "10.0.0.40:2"
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "different-case email must hit the same bucket")
}

func TestLoginRateLimitMiddleware_HonoursClientIPHeader(t *testing.T) {
	t.Parallel()
	c := cache.NewInMemory()
	mw := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerIP(1),
		WithLoginRateLimitPerUsername(100),
		WithLoginRateLimitClientIPHeader("X-Real-IP"),
	)
	handler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

	// Same RemoteAddr but different X-Real-IP — second request must pass.
	for _, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		req := newLoginRequest(t, "bob", "wrong", "10.0.0.50:1234")
		req.Header.Set("X-Real-IP", ip)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equalf(t, http.StatusUnauthorized, w.Code, "X-Real-IP=%s reached inner", ip)
	}

	// A third request from the first IP would now exceed its quota.
	req := newLoginRequest(t, "bob", "wrong", "10.0.0.50:1234")
	req.Header.Set("X-Real-IP", "203.0.113.1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "second hit on same X-Real-IP must 429")
}

func TestLoginRateLimitMiddleware_NoBodyDoesNotPanic(t *testing.T) {
	t.Parallel()
	c := cache.NewInMemory()
	mw := NewLoginRateLimitMiddleware(c, api.NewResponder())
	handler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", http.NoBody)
	req.RemoteAddr = "10.0.0.60:1"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"empty body must not block the inner handler")
}

// Sanity: the limiter relies on the cache returning ErrNotFound for missing
// keys; if a buggy cache returned a non-numeric value, readCount must treat it
// as zero rather than panic or count nonsense.
func TestLoginRateLimitMiddleware_NonNumericCacheValueTreatedAsZero(t *testing.T) {
	t.Parallel()
	c := cache.NewInMemory()
	require.NoError(t, c.Set(context.Background(), "auth:login-fail:ip:10.0.0.70", "garbage"))

	mw := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerIP(2),
		WithLoginRateLimitPerUsername(100),
	)
	handler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

	req := newLoginRequest(t, "x", "wrong", "10.0.0.70:1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"corrupt cache value must not lock the user out before any failures are counted")
}

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — a brute-force-throttled login
//     endpoint must record both the throttled (blocked) attempts and the
//     individual failed attempts so an operator can detect a credential-
//     stuffing campaign (OWASP ASVS §7.1.3). The submitted identifier must be
//     recorded as attempted_login in Extra, NEVER as the actor (it is an
//     unauthenticated, attacker-controlled value — OWASP ASVS §7.1.4).
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// TestLoginRateLimitMiddleware_Audit_OverLimitEmitsBlocked covers OWASP
// API2:2023. Once a counter is over its limit the limiter must refuse the
// attempt (429) AND emit an auth.login.blocked event with outcome blocked,
// category ratelimit, the blocked-by reason ("ip" or "username"), and the
// submitted identifier carried only in Extra.attempted_login.
func TestLoginRateLimitMiddleware_Audit_OverLimitEmitsBlocked(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		opts       []LoginRateLimitOption
		priming    int
		wantReason string
	}{
		{
			name: "blocked_by_username_records_username_reason",
			opts: []LoginRateLimitOption{
				WithLoginRateLimitPerUsername(2),
				WithLoginRateLimitPerIP(100),
				WithLoginRateLimitWindow(time.Minute),
			},
			priming:    2,
			wantReason: "username",
		},
		{
			name: "blocked_by_ip_records_ip_reason",
			opts: []LoginRateLimitOption{
				WithLoginRateLimitPerIP(2),
				WithLoginRateLimitPerUsername(100),
				WithLoginRateLimitWindow(time.Minute),
			},
			priming:    2,
			wantReason: "ip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			recorder := &auditCapture{}
			c := cache.NewInMemory()
			opts := make([]LoginRateLimitOption, 0, len(tt.opts)+1)
			opts = append(opts, tt.opts...)
			opts = append(opts, WithLoginRateLimitAuditLogger(recorder))
			mw := NewLoginRateLimitMiddleware(c, api.NewResponder(), opts...)
			handler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

			// Burn enough failures to push the relevant counter to its limit.
			for range tt.priming {
				req := newLoginRequest(t, "alice", "wrong", "10.0.0.80:1")
				handler.ServeHTTP(httptest.NewRecorder(), req)
			}

			req := newLoginRequest(t, "alice", "wrong", "10.0.0.80:1")
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, http.StatusTooManyRequests, w.Code,
				"an over-limit attempt must be refused before credentials are checked")

			ev, ok := findEvent(recorder.snapshot(), audit.EventLoginBlocked)
			require.True(t, ok, "a blocked login attempt must leave an auth.login.blocked event")
			assert.Equal(t, audit.OutcomeBlocked, ev.Outcome, "a rate-limited attempt is blocked")
			assert.Equal(t, audit.CategoryRateLimit, ev.Category)
			assert.Equal(t, tt.wantReason, ev.Reason, "the blocked-by reason must be recorded")
			assert.Equal(t, audit.AuthMethodAnonymous, ev.AuthMethod,
				"a throttled attempt is unauthenticated")
			assert.Zero(t, ev.ActorID,
				"the attacker-controlled login must never be attributed as an actor id")
			assert.Empty(t, ev.ActorLogin,
				"the submitted identifier must never be recorded as actor_login")
			login, hasLogin := extraString(ev, "attempted_login")
			require.True(t, hasLogin, "the submitted identifier must be carried as attempted_login")
			assert.Equal(t, "alice", login,
				"attempted_login must hold the submitted (lowercased) identifier")
		})
	}
}

// TestLoginRateLimitMiddleware_Audit_DownstreamUnauthorizedEmitsFailure
// covers OWASP API2:2023. When the wrapped login handler answers 401 the
// limiter must record an auth.login.failure event with the submitted
// identifier in Extra.attempted_login (never as the actor) so individual
// failed attempts are auditable, not just the throttled ones.
func TestLoginRateLimitMiddleware_Audit_DownstreamUnauthorizedEmitsFailure(t *testing.T) {
	t.Parallel()
	// ARRANGE
	recorder := &auditCapture{}
	c := cache.NewInMemory()
	mw := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerIP(100),
		WithLoginRateLimitPerUsername(100),
		WithLoginRateLimitAuditLogger(recorder),
	)
	handler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

	req := newLoginRequest(t, "Bob@Example.com", "wrong", "10.0.0.81:1")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code,
		"the downstream 401 must be passed through; body=%s", w.Body.String())

	events := recorder.snapshot()
	assert.Equal(t, 0, countEvents(events, audit.EventLoginBlocked),
		"a first failed attempt under the limit must not be recorded as blocked")

	ev, ok := findEvent(events, audit.EventLoginFailure)
	require.True(t, ok, "a downstream 401 must leave an auth.login.failure event")
	assert.Equal(t, audit.OutcomeFailure, ev.Outcome, "a failed login is a failure")
	assert.Equal(t, audit.CategoryAuthentication, ev.Category)
	assert.Equal(t, audit.AuthMethodAnonymous, ev.AuthMethod)
	assert.Zero(t, ev.ActorID,
		"a failed credential check must not attribute an actor id")
	assert.Empty(t, ev.ActorLogin,
		"the submitted identifier must never be recorded as actor_login")
	login, hasLogin := extraString(ev, "attempted_login")
	require.True(t, hasLogin, "the submitted identifier must be carried as attempted_login")
	assert.Equal(t, "bob@example.com", login,
		"attempted_login must hold the submitted email, lowercased, never as the actor")
}

// TestLoginRateLimitMiddleware_Audit_SuccessIsNotAudited covers OWASP
// API2:2023. A successful login is recorded by the login handler itself
// (auth.login.success); the rate limiter must NOT additionally emit a
// failure or blocked event for a 200 response.
func TestLoginRateLimitMiddleware_Audit_SuccessIsNotAudited(t *testing.T) {
	t.Parallel()
	// ARRANGE
	recorder := &auditCapture{}
	c := cache.NewInMemory()
	mw := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerIP(100),
		WithLoginRateLimitPerUsername(100),
		WithLoginRateLimitAuditLogger(recorder),
	)
	handler := mw.Middleware(&loginAttemptHandler{outcome: http.StatusOK})

	req := newLoginRequest(t, "alice", "right", "10.0.0.82:1")
	w := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code)
	events := recorder.snapshot()
	assert.Equal(t, 0, countEvents(events, audit.EventLoginFailure),
		"the rate limiter must not record a failure for a successful login")
	assert.Equal(t, 0, countEvents(events, audit.EventLoginBlocked),
		"the rate limiter must not record a block for a successful login")
}

// TestLoginRateLimitMiddleware_KeyPrefixIsolatesCounters proves the S2 fix: two
// limiters sharing one cache but declaring different key prefixes keep
// independent counters, so a failed SSO ticket exchange never spends the
// customer's password-login budget (and vice versa).
func TestLoginRateLimitMiddleware_KeyPrefixIsolatesCounters(t *testing.T) {
	t.Parallel()
	c := cache.NewInMemory()

	const ip = "10.0.0.20:1234"

	loginMW := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerIP(1),
		WithLoginRateLimitPerUsername(100),
		WithLoginRateLimitWindow(time.Minute),
	).Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

	exchangeMW := NewLoginRateLimitMiddleware(c, api.NewResponder(),
		WithLoginRateLimitPerIP(1),
		WithLoginRateLimitPerUsername(100),
		WithLoginRateLimitWindow(time.Minute),
		WithLoginRateLimitKeyPrefix("auth:sso-exchange-fail:"),
	).Middleware(&loginAttemptHandler{outcome: http.StatusUnauthorized})

	// Exhaust the login limiter for this IP.
	req := newLoginRequest(t, "alice", "wrong", ip)
	w := httptest.NewRecorder()
	loginMW.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	req = newLoginRequest(t, "alice", "wrong", ip)
	w = httptest.NewRecorder()
	loginMW.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code, "login limiter must now block this IP")

	// Same cache, same IP, different prefix: the exchange limiter must not have
	// inherited the login failures.
	req = newLoginRequest(t, "alice", "wrong", ip)
	w = httptest.NewRecorder()
	exchangeMW.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "exchange limiter must keep its own IP counter")
}
