// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — beginning enrollment must require an
//     authenticated session, must store the new secret only in encrypted form,
//     and must not flip the account to 2FA-enabled until it is confirmed.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package setup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errStorageUnavailable = errors.New("storage unavailable")
	errSecretGeneration   = errors.New("entropy source unavailable")
	errSecretEncryption   = errors.New("cipher unavailable")
)

// errUserRepository makes the user lookup or the write fail, so the handler's
// storage-error branches become reachable.
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

// stubTwoFactor lets a single manager step fail while the others behave.
type stubTwoFactor struct {
	generateErr error
	encryptErr  error
}

// stubSecret is the secret stubTwoFactor mints, so tests can assert it never
// reaches the client on a failed enrollment.
const stubSecret = "STUBSECRET234567"

func (s *stubTwoFactor) GenerateSecret(_ string) (string, string, error) {
	if s.generateErr != nil {
		return "", "", s.generateErr
	}

	return stubSecret, "otpauth://totp/gameap:alice?secret=" + stubSecret, nil
}

func (s *stubTwoFactor) EncryptSecret(secret string) (string, error) {
	if s.encryptErr != nil {
		return "", s.encryptErr
	}

	return "enc:" + secret, nil
}

func seededRepo(t *testing.T) *inmemory.UserRepository {
	t.Helper()

	repo := inmemory.NewUserRepository()
	require.NoError(t, repo.Save(context.Background(), &domain.User{
		ID: 1, Login: "alice", Email: "alice@example.com",
	}))

	return repo
}

func newManager(t *testing.T) *twofactor.Manager {
	t.Helper()

	m, err := twofactor.NewManager([]byte("test-app-key"))
	require.NoError(t, err)

	return m
}

func authedRequest(login string) *http.Request {
	session := &auth.Session{
		Login: login,
		User:  &domain.User{ID: 1, Login: login},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/setup", http.NoBody)

	return req.WithContext(auth.ContextWithSession(context.Background(), session))
}

func TestSetup_StartsEnrollmentEncryptedAndInactive(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewUserRepository()
	require.NoError(t, repo.Save(context.Background(), &domain.User{
		ID: 1, Login: "alice", Email: "alice@example.com",
	}))
	manager := newManager(t)
	handler := NewHandler(repo, manager, api.NewResponder())

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authedRequest("alice"))

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "otpauth://totp/")
	assert.Contains(t, w.Body.String(), `"secret"`)

	users, err := repo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, users, 1)
	stored := users[0]
	require.NotNil(t, stored.TwoFactorSecret)
	assert.False(t, stored.TwoFactorEnabled, "2FA must stay inactive until confirmed")

	ok, _, err := manager.ValidateTOTP(*stored.TwoFactorSecret, "000000", nil)
	assert.NoError(t, err, "the stored secret must be valid ciphertext the manager can open")
	assert.False(t, ok, "an arbitrary code must not validate")
}

func TestSetup_RejectsWhenAlreadyEnabled(t *testing.T) {
	t.Parallel()

	repo := inmemory.NewUserRepository()
	secret := "already-encrypted"
	require.NoError(t, repo.Save(context.Background(), &domain.User{
		ID: 1, Login: "alice", TwoFactorEnabled: true, TwoFactorSecret: &secret,
	}))
	handler := NewHandler(repo, newManager(t), api.NewResponder())

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authedRequest("alice"))

	require.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "already enabled")
}

// A personal access token lives in a third-party system. Letting one enrol or
// replace a second factor would let whoever steals it re-anchor the owner's
// account to an authenticator they control, so the whole 2FA surface refuses a
// token session outright. See base.EnsureSecondFactorChangeAllowedForSession.
func TestSetup_RefusesTokenSession(t *testing.T) {
	t.Parallel()

	// ARRANGE
	handler := NewHandler(seededRepo(t), newManager(t), api.NewResponder())

	req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/setup", http.NoBody)
	req = req.WithContext(auth.ContextWithSession(context.Background(), tokenSession()))

	// ACT
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusForbidden, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "cannot manage two-factor authentication")
	assert.NotContains(t, w.Body.String(), stubSecret)
}

// tokenSession authenticates as a user the way a personal access token does.
func tokenSession() *auth.Session {
	return &auth.Session{
		Login: "alice",
		User:  &domain.User{ID: 1, Login: "alice"},
		Token: &domain.PersonalAccessToken{ID: 7},
	}
}

func TestSetup_RejectsUnauthenticated(t *testing.T) {
	t.Parallel()

	handler := NewHandler(inmemory.NewUserRepository(), newManager(t), api.NewResponder())

	req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/setup", http.NoBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// A session whose user no longer exists must not enroll anything.
func TestSetup_RejectsWhenSessionUserIsGone(t *testing.T) {
	t.Parallel()

	// ARRANGE
	handler := NewHandler(inmemory.NewUserRepository(), newManager(t), api.NewResponder())

	// ACT
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authedRequest("ghost"))

	// ASSERT
	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "user not found")
}

// Any failure along the enrollment path must abort without leaving a usable
// pending secret behind, and must not leak the internal cause to the client.
func TestSetup_StorageAndCryptoErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		repo func(t *testing.T) *errUserRepository
		// twoFactor is the manager slice the handler depends on.
		twoFactor twoFactorManager
		// mustNotLeak is internal detail that may never reach the response body.
		mustNotLeak string
	}{
		{
			name: "user_lookup_error_aborts_enrollment",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: seededRepo(t), findErr: errStorageUnavailable}
			},
			twoFactor:   &stubTwoFactor{},
			mustNotLeak: "storage unavailable",
		},
		{
			name: "secret_generation_error_aborts_enrollment",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: seededRepo(t)}
			},
			twoFactor:   &stubTwoFactor{generateErr: errSecretGeneration},
			mustNotLeak: "entropy source unavailable",
		},
		{
			name: "secret_encryption_error_aborts_enrollment",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: seededRepo(t)}
			},
			twoFactor:   &stubTwoFactor{encryptErr: errSecretEncryption},
			mustNotLeak: "cipher unavailable",
		},
		{
			name: "pending_secret_store_error_aborts_enrollment",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: seededRepo(t), saveErr: errStorageUnavailable}
			},
			twoFactor:   &stubTwoFactor{},
			mustNotLeak: "storage unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			repo := tt.repo(t)
			handler := NewHandler(repo, tt.twoFactor, api.NewResponder())

			// ACT
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, authedRequest("alice"))

			// ASSERT
			require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
			assert.NotContains(t, w.Body.String(), tt.mustNotLeak,
				"the internal cause must not reach the client")
			assert.NotContains(t, w.Body.String(), stubSecret,
				"a failed enrollment must not echo the generated secret")
			assert.NotContains(t, w.Body.String(), `"secret"`,
				"a failed enrollment must not emit a setup response at all")

			users, err := repo.UserRepository.Find(context.Background(), nil, nil, nil)
			require.NoError(t, err)
			require.Len(t, users, 1)
			assert.False(t, users[0].TwoFactorEnabled, "a failed enrollment must never enable 2FA")
		})
	}
}

// Restarting enrollment must invalidate whatever the previous attempt left
// behind, so a half-finished attempt cannot be resumed with stale state.
func TestSetup_RestartClearsPreviousEnrollmentState(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := inmemory.NewUserRepository()
	staleSecret := "enc:stale"
	staleCodes := "stale-codes"
	staleStep := int64(42)
	require.NoError(t, repo.Save(context.Background(), &domain.User{
		ID:                     1,
		Login:                  "alice",
		TwoFactorSecret:        &staleSecret,
		TwoFactorRecoveryCodes: &staleCodes,
		TwoFactorLastUsedStep:  &staleStep,
	}))
	handler := NewHandler(repo, newManager(t), api.NewResponder())

	// ACT
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authedRequest("alice"))

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	users, err := repo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.NotNil(t, users[0].TwoFactorSecret)
	assert.NotEqual(t, staleSecret, *users[0].TwoFactorSecret, "a restart must mint a fresh secret")
	assert.Nil(t, users[0].TwoFactorRecoveryCodes, "stale recovery codes must be discarded")
	assert.Nil(t, users[0].TwoFactorLastUsedStep, "the stale replay guard must be reset")
}
