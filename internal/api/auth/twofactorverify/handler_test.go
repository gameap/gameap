// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — the 2FA verification endpoint must
//     issue a session only for a live single-use challenge plus a correct
//     second factor, must consume the challenge on success, must not leak a
//     token on any failure, and must bound wrong-code guessing per challenge.
//   - API4:2023 Unrestricted Resource Consumption — repeated wrong codes
//     against one challenge must exhaust a fixed budget and destroy it.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package twofactorverify

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/api/auth/login"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/pkg/errors"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJWTSecret = "test-jwt-secret-for-2fa-verify-handler"

type verifyFixture struct {
	handler   *Handler
	cache     cache.Cache
	manager   *twofactor.Manager
	userRepo  *inmemory.UserRepository
	token     string
	plainCode string
	// secret is the decrypted TOTP secret, so tests can compute a live code.
	secret string
	// now is the fixed clock the manager validates TOTP against.
	now time.Time
}

// newVerifyFixture builds a fully real handler: a 2FA-enabled user with an
// encrypted TOTP secret and one set of recovery codes, plus a live challenge
// in the cache. The first recovery code is returned as a known-good factor so
// the success path can be exercised without computing a TOTP value.
func newVerifyFixture(t *testing.T) *verifyFixture {
	t.Helper()

	now := time.Unix(1_700_000_000, 0)
	manager, err := twofactor.NewManager([]byte("test-app-key"), twofactor.WithClock(func() time.Time {
		return now
	}))
	require.NoError(t, err)

	secret, _, err := manager.GenerateSecret("alice")
	require.NoError(t, err)
	encSecret, err := manager.EncryptSecret(secret)
	require.NoError(t, err)
	plainCodes, encodedCodes, err := manager.GenerateRecoveryCodes()
	require.NoError(t, err)

	userRepo := inmemory.NewUserRepository()
	user := &domain.User{
		Login:                  "alice",
		Email:                  "alice@example.com",
		TwoFactorEnabled:       true,
		TwoFactorSecret:        &encSecret,
		TwoFactorRecoveryCodes: &encodedCodes,
	}
	require.NoError(t, userRepo.Save(context.Background(), user))

	c := cache.NewInMemory()
	token := twofactor.ChallengeTokenPrefix + "fixture-challenge-secret-abcdef0123456789"
	encoded, err := twofactor.MarshalChallengePayload(twofactor.ChallengePayload{
		UserID:    user.ID,
		Login:     user.Login,
		Email:     user.Email,
		ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
	})
	require.NoError(t, err)
	require.NoError(t, c.Set(context.Background(), twofactor.ChallengeCacheKey(token), encoded))

	handler := NewHandler(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		manager,
		c,
		api.NewResponder(),
		nil,
	)

	return &verifyFixture{
		handler:   handler,
		cache:     c,
		manager:   manager,
		userRepo:  userRepo,
		token:     token,
		plainCode: plainCodes[0],
		secret:    secret,
		now:       now,
	}
}

var (
	errCacheBackend       = errors.New("cache backend down")
	errStorageUnavailable = errors.New("storage unavailable")
)

// errCache makes a single cache operation fail while the rest keep working.
type errCache struct {
	cache.Cache

	getErr    error
	deleteErr error
}

func (c *errCache) Get(ctx context.Context, key string) (any, error) {
	if c.getErr != nil {
		return nil, c.getErr
	}

	return c.Cache.Get(ctx, key)
}

func (c *errCache) Delete(ctx context.Context, key string) error {
	if c.deleteErr != nil {
		return c.deleteErr
	}

	return c.Cache.Delete(ctx, key)
}

// errUserRepository makes the user lookup or the write fail.
type errUserRepository struct {
	*inmemory.UserRepository

	findErr error
	saveErr error
}

func (r *errUserRepository) Find(
	ctx context.Context,
	filter *filters.FindUser,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.User, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}

	return r.UserRepository.Find(ctx, filter, order, pagination)
}

func (r *errUserRepository) Save(ctx context.Context, user *domain.User) error {
	if r.saveErr != nil {
		return r.saveErr
	}

	return r.UserRepository.Save(ctx, user)
}

// handlerWith rebuilds the handler around a substituted cache or repository,
// keeping every other collaborator real.
func (f *verifyFixture) handlerWith(c cache.Cache, repo repositories.UserRepository) *Handler {
	return NewHandler(
		auth.NewJWTService([]byte(testJWTSecret)),
		repo,
		f.manager,
		c,
		api.NewResponder(),
		nil,
	)
}

// seedChallenge writes a challenge with the given expiry for the fixture user.
func (f *verifyFixture) seedChallenge(t *testing.T, token string, expiresAt time.Time) {
	t.Helper()

	users, err := f.userRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, users, 1)

	encoded, err := twofactor.MarshalChallengePayload(twofactor.ChallengePayload{
		UserID:    users[0].ID,
		Login:     users[0].Login,
		Email:     users[0].Email,
		ExpiresAt: expiresAt.Unix(),
	})
	require.NoError(t, err)
	require.NoError(t, f.cache.Set(context.Background(), twofactor.ChallengeCacheKey(token), encoded))
}

func doVerify(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/2fa/verify", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	return w
}

func bodyJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m))

	return m
}

// TestVerify_ValidRecoveryCode_IssuesToken covers OWASP API2:2023: a live
// challenge plus a correct recovery code yields a session token and consumes
// both the challenge (single-use) and the recovery code (single-use).
func TestVerify_ValidRecoveryCode_IssuesToken(t *testing.T) {
	f := newVerifyFixture(t)

	w := doVerify(t, f.handler, `{"challenge_token":"`+f.token+`","code":"`+f.plainCode+`"}`)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	resp := bodyJSON(t, w)
	assert.NotEmpty(t, resp["token"])
	user, ok := resp["user"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "alice", user["login"])

	_, err := f.cache.Get(context.Background(), twofactor.ChallengeCacheKey(f.token))
	assert.ErrorIs(t, err, cache.ErrNotFound, "a consumed challenge must be gone")
}

// TestVerify_ChallengeIsSingleUse covers OWASP API2:2023: once a challenge has
// produced a token it cannot be reused, even with a still-valid second factor.
func TestVerify_ChallengeIsSingleUse(t *testing.T) {
	f := newVerifyFixture(t)

	first := doVerify(t, f.handler, `{"challenge_token":"`+f.token+`","code":"`+f.plainCode+`"}`)
	require.Equal(t, http.StatusOK, first.Code)

	second := doVerify(t, f.handler, `{"challenge_token":"`+f.token+`","code":"`+f.plainCode+`"}`)
	require.Equal(t, http.StatusUnauthorized, second.Code)
	assert.Contains(t, second.Body.String(), "invalid or expired challenge")
}

// TestVerify_RecoveryCodeIsSingleUse covers OWASP API2:2023: a recovery code
// spent on one challenge must not authenticate a fresh challenge.
func TestVerify_RecoveryCodeIsSingleUse(t *testing.T) {
	f := newVerifyFixture(t)

	require.Equal(t, http.StatusOK,
		doVerify(t, f.handler, `{"challenge_token":"`+f.token+`","code":"`+f.plainCode+`"}`).Code)

	// A brand-new challenge for the same user; the already-spent code must fail.
	fresh := twofactor.ChallengeTokenPrefix + "second-challenge-secret-9876543210fedcba"
	users, err := f.userRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	encoded, err := twofactor.MarshalChallengePayload(twofactor.ChallengePayload{
		UserID:    users[0].ID,
		Login:     users[0].Login,
		Email:     users[0].Email,
		ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
	})
	require.NoError(t, err)
	require.NoError(t, f.cache.Set(
		context.Background(), twofactor.ChallengeCacheKey(fresh), encoded,
	))

	w := doVerify(t, f.handler, `{"challenge_token":"`+fresh+`","code":"`+f.plainCode+`"}`)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "invalid verification code")
}

// TestVerify_BruteForceExhaustsChallenge covers OWASP API4:2023 / API2:2023:
// after maxVerifyAttempts wrong codes the challenge is destroyed, so even the
// correct code afterwards cannot complete it.
func TestVerify_BruteForceExhaustsChallenge(t *testing.T) {
	f := newVerifyFixture(t)

	for i := range maxVerifyAttempts {
		w := doVerify(t, f.handler, `{"challenge_token":"`+f.token+`","code":"000000"}`)
		require.Equal(t, http.StatusUnauthorized, w.Code, "attempt %d", i)
	}

	_, err := f.cache.Get(context.Background(), twofactor.ChallengeCacheKey(f.token))
	require.ErrorIs(t, err, cache.ErrNotFound, "the challenge must be destroyed after the budget")

	w := doVerify(t, f.handler, `{"challenge_token":"`+f.token+`","code":"`+f.plainCode+`"}`)
	require.Equal(t, http.StatusUnauthorized, w.Code,
		"a correct code must not rescue an exhausted challenge")
	assert.Contains(t, w.Body.String(), "invalid or expired challenge")
}

// TestVerify_InputAndChallengeRejections covers OWASP API2:2023: malformed
// input, non-challenge-shaped tokens and unknown challenges are all refused
// without issuing a token.
func TestVerify_InputAndChallengeRejections(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		expectedStatus int
		wantError      string
	}{
		{
			name:           "missing_code",
			body:           `{"challenge_token":"g2fa_x"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "code field is required",
		},
		{
			name:           "missing_challenge_token",
			body:           `{"code":"123456"}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "challenge_token field is required",
		},
		{
			name:           "non_challenge_shaped_token",
			body:           `{"challenge_token":"eyJhbGciOiJIUzM4NCJ9.x.y","code":"123456"}`,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid or expired challenge",
		},
		{
			name:           "short_lived_prefix_not_accepted",
			body:           `{"challenge_token":"glst_somesecret","code":"123456"}`,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid or expired challenge",
		},
		{
			name:           "unknown_challenge",
			body:           `{"challenge_token":"g2fa_never-issued","code":"123456"}`,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid or expired challenge",
		},
		{
			name:           "invalid_json",
			body:           `{"challenge_token": }`,
			expectedStatus: http.StatusBadRequest,
			wantError:      "invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newVerifyFixture(t)

			w := doVerify(t, f.handler, tt.body)

			require.Equal(t, tt.expectedStatus, w.Code, "body=%s", w.Body.String())
			assert.Contains(t, w.Body.String(), tt.wantError)
			assert.NotContains(t, w.Body.String(), `"token"`,
				"no session token may be issued on a failed verification")
		})
	}
}

// TestVerify_UserNoLongerEligible covers OWASP API2:2023: if 2FA was disabled
// for the account after the challenge was issued, the stale challenge must be
// refused and destroyed rather than minting a session.
func TestVerify_UserNoLongerEligible(t *testing.T) {
	f := newVerifyFixture(t)

	users, err := f.userRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	users[0].TwoFactorEnabled = false
	require.NoError(t, f.userRepo.Save(context.Background(), &users[0]))

	w := doVerify(t, f.handler, `{"challenge_token":"`+f.token+`","code":"`+f.plainCode+`"}`)

	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	_, getErr := f.cache.Get(context.Background(), twofactor.ChallengeCacheKey(f.token))
	assert.ErrorIs(t, getErr, cache.ErrNotFound, "a stale challenge must be destroyed")
}

// TestVerify_ValidTOTPCode_IssuesToken covers OWASP API2:2023: the primary
// second factor (the authenticator code) completes the challenge and records
// the consumed step so it cannot be replayed.
func TestVerify_ValidTOTPCode_IssuesToken(t *testing.T) {
	// ARRANGE
	f := newVerifyFixture(t)
	code, err := totp.GenerateCode(f.secret, f.now)
	require.NoError(t, err)

	// ACT
	w := doVerify(t, f.handler, `{"challenge_token":"`+f.token+`","code":"`+code+`"}`)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.NotEmpty(t, bodyJSON(t, w)["token"])

	users, err := f.userRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.NotNil(t, users[0].TwoFactorLastUsedStep,
		"the consumed TOTP step must be persisted to block its replay")

	require.NotNil(t, users[0].TwoFactorRecoveryCodes)
	assert.NotEmpty(t, *users[0].TwoFactorRecoveryCodes,
		"a TOTP login must not spend a recovery code")
}

// TestVerify_TOTPCodeIsSingleUse covers OWASP API2:2023: a TOTP code captured
// inside its validity window must not authenticate a second challenge.
func TestVerify_TOTPCodeIsSingleUse(t *testing.T) {
	// ARRANGE
	f := newVerifyFixture(t)
	code, err := totp.GenerateCode(f.secret, f.now)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK,
		doVerify(t, f.handler, `{"challenge_token":"`+f.token+`","code":"`+code+`"}`).Code)

	replayToken := twofactor.ChallengeTokenPrefix + "replay-challenge-secret-0123456789abcdef"
	f.seedChallenge(t, replayToken, time.Now().Add(5*time.Minute))

	// ACT
	w := doVerify(t, f.handler, `{"challenge_token":"`+replayToken+`","code":"`+code+`"}`)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid verification code")
	assert.NotContains(t, w.Body.String(), `"token"`)
}

// TestVerify_CorruptChallengeIsRefusedAndDestroyed covers OWASP API2:2023: a
// cache entry that does not decode must never be trusted, and must be dropped
// rather than left around for another attempt.
func TestVerify_CorruptChallengeIsRefusedAndDestroyed(t *testing.T) {
	// ARRANGE
	f := newVerifyFixture(t)
	key := twofactor.ChallengeCacheKey(f.token)
	require.NoError(t, f.cache.Set(context.Background(), key, "not-a-valid-payload"))

	// ACT
	w := doVerify(t, f.handler, `{"challenge_token":"`+f.token+`","code":"`+f.plainCode+`"}`)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid or expired challenge")
	assert.NotContains(t, w.Body.String(), `"token"`)

	_, err := f.cache.Get(context.Background(), key)
	assert.ErrorIs(t, err, cache.ErrNotFound, "a corrupt challenge must be destroyed")
}

// TestVerify_ExpiredChallengeIsDestroyedOnWrongCode covers OWASP API2:2023: a
// challenge past its deadline must be dropped on the next attempt instead of
// having its TTL rewritten.
func TestVerify_ExpiredChallengeIsDestroyedOnWrongCode(t *testing.T) {
	// ARRANGE
	f := newVerifyFixture(t)
	expired := twofactor.ChallengeTokenPrefix + "expired-challenge-secret-abcdef9876543210"
	f.seedChallenge(t, expired, time.Now().Add(-time.Minute))

	// ACT
	w := doVerify(t, f.handler, `{"challenge_token":"`+expired+`","code":"000000"}`)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())

	_, err := f.cache.Get(context.Background(), twofactor.ChallengeCacheKey(expired))
	assert.ErrorIs(t, err, cache.ErrNotFound, "an expired challenge must not survive an attempt")
}

// TestVerify_BackendErrors covers OWASP API2:2023: no infrastructure failure
// may be turned into a session, and none may leak its cause to the client.
func TestVerify_BackendErrors(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*verifyFixture) *Handler
		mustNotLeak string
	}{
		{
			name: "challenge_cache_error_denies_login",
			mutate: func(f *verifyFixture) *Handler {
				return f.handlerWith(&errCache{Cache: f.cache, getErr: errCacheBackend}, f.userRepo)
			},
			mustNotLeak: "cache backend down",
		},
		{
			name: "challenge_consume_error_denies_login",
			mutate: func(f *verifyFixture) *Handler {
				return f.handlerWith(&errCache{Cache: f.cache, deleteErr: errCacheBackend}, f.userRepo)
			},
			mustNotLeak: "cache backend down",
		},
		{
			name: "user_lookup_error_denies_login",
			mutate: func(f *verifyFixture) *Handler {
				return f.handlerWith(f.cache, &errUserRepository{
					UserRepository: f.userRepo, findErr: errStorageUnavailable,
				})
			},
			mustNotLeak: "storage unavailable",
		},
		{
			name: "factor_state_persist_error_denies_login",
			mutate: func(f *verifyFixture) *Handler {
				return f.handlerWith(f.cache, &errUserRepository{
					UserRepository: f.userRepo, saveErr: errStorageUnavailable,
				})
			},
			mustNotLeak: "storage unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			f := newVerifyFixture(t)
			handler := tt.mutate(f)

			// ACT
			w := doVerify(t, handler, `{"challenge_token":"`+f.token+`","code":"`+f.plainCode+`"}`)

			// ASSERT
			require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
			assert.NotContains(t, w.Body.String(), tt.mustNotLeak,
				"the internal cause must not reach the client")
			assert.NotContains(t, w.Body.String(), `"token"`,
				"no session token may be issued when the backend fails")
		})
	}
}

// TestVerify_RememberMeCarriesThroughChallenge covers OWASP API2:2023: the
// "remember me" choice made at password time must survive the 2FA step and
// govern the lifetime of the issued session, not be silently downgraded.
func TestVerify_RememberMeCarriesThroughChallenge(t *testing.T) {
	// ARRANGE
	f := newVerifyFixture(t)
	rememberToken := twofactor.ChallengeTokenPrefix + "remember-challenge-secret-fedcba9876543210"

	users, err := f.userRepo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, users, 1)
	encoded, err := twofactor.MarshalChallengePayload(twofactor.ChallengePayload{
		UserID:    users[0].ID,
		Login:     users[0].Login,
		Email:     users[0].Email,
		ExpiresAt: time.Now().Add(5 * time.Minute).Unix(),
		Remember:  true,
	})
	require.NoError(t, err)
	require.NoError(t, f.cache.Set(
		context.Background(), twofactor.ChallengeCacheKey(rememberToken), encoded,
	))

	// ACT
	w := doVerify(t, f.handler, `{"challenge_token":"`+rememberToken+`","code":"`+f.plainCode+`"}`)

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.InDelta(t, login.RememberMeDuration.Seconds(), bodyJSON(t, w)["expires_in"], 0,
		"a remembered challenge must issue the long-lived session")
}
