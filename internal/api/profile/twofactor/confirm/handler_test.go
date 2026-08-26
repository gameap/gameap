// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — 2FA only activates after the user
//     proves possession of the authenticator (a correct TOTP code for the
//     pending secret); a wrong code must not enable it, and recovery codes are
//     surfaced exactly once on activation.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package confirm

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/pkg/errors"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errStorageUnavailable = errors.New("storage unavailable")
	errValidationBackend  = errors.New("cipher unavailable")
	errRecoveryGeneration = errors.New("entropy source unavailable")
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

// stubTwoFactor accepts any code and lets a single manager step fail.
type stubTwoFactor struct {
	validateErr error
	recoveryErr error
}

func (s *stubTwoFactor) ValidateTOTP(_, _ string, _ *int64) (bool, int64, error) {
	if s.validateErr != nil {
		return false, 0, s.validateErr
	}

	return true, 1, nil
}

func (s *stubTwoFactor) GenerateRecoveryCodes() ([]string, string, error) {
	if s.recoveryErr != nil {
		return nil, "", s.recoveryErr
	}

	return []string{"code-1"}, "encoded", nil
}

type confirmFixture struct {
	handler   *Handler
	repo      *inmemory.UserRepository
	validCode string
}

func newConfirmFixture(t *testing.T, pending bool) *confirmFixture {
	t.Helper()

	now := time.Unix(1_700_000_000, 0)
	manager, err := twofactor.NewManager([]byte("test-app-key"), twofactor.WithClock(func() time.Time {
		return now
	}))
	require.NoError(t, err)

	secret, _, err := manager.GenerateSecret("alice")
	require.NoError(t, err)
	code, err := totp.GenerateCode(secret, now)
	require.NoError(t, err)

	user := &domain.User{ID: 1, Login: "alice", Email: "alice@example.com"}
	if pending {
		enc, encErr := manager.EncryptSecret(secret)
		require.NoError(t, encErr)
		user.TwoFactorSecret = &enc
	}

	repo := inmemory.NewUserRepository()
	require.NoError(t, repo.Save(context.Background(), user))

	return &confirmFixture{
		handler:   NewHandler(repo, manager, api.NewResponder(), nil),
		repo:      repo,
		validCode: code,
	}
}

func confirmReq(t *testing.T, body string) *http.Request {
	t.Helper()

	session := &auth.Session{Login: "alice", User: &domain.User{ID: 1, Login: "alice"}}
	req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/confirm", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	return req.WithContext(auth.ContextWithSession(context.Background(), session))
}

func TestConfirm_ValidCodeActivatesAndReturnsRecoveryCodes(t *testing.T) {
	t.Parallel()

	f := newConfirmFixture(t, true)

	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, confirmReq(t, `{"code":"`+f.validCode+`"}`))

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "recovery_codes")

	users, err := f.repo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.True(t, users[0].TwoFactorEnabled, "a confirmed code must activate 2FA")
	require.NotNil(t, users[0].TwoFactorRecoveryCodes)
	require.NotNil(t, users[0].TwoFactorLastUsedStep,
		"the confirming step must be recorded to block its immediate replay")
}

func TestConfirm_WrongCodeDoesNotActivate(t *testing.T) {
	t.Parallel()

	f := newConfirmFixture(t, true)

	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, confirmReq(t, `{"code":"000000"}`))

	require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid verification code")

	users, err := f.repo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	assert.False(t, users[0].TwoFactorEnabled, "a wrong code must not enable 2FA")
}

func TestConfirm_RejectsWithoutPendingSecret(t *testing.T) {
	t.Parallel()

	f := newConfirmFixture(t, false)

	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, confirmReq(t, `{"code":"`+f.validCode+`"}`))

	require.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "no pending two-factor enrollment")
}

func TestConfirm_MissingCodeIsValidationError(t *testing.T) {
	t.Parallel()

	f := newConfirmFixture(t, true)

	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, confirmReq(t, `{}`))

	require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "code field is required")
}

// Activation must never be reachable without a session.
func TestConfirm_RejectsUnauthenticated(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newConfirmFixture(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/confirm",
		bytes.NewBufferString(`{"code":"`+f.validCode+`"}`))

	// ACT
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
}

func TestConfirm_MalformedBodyIsBadRequest(t *testing.T) {
	t.Parallel()

	// ARRANGE
	f := newConfirmFixture(t, true)

	// ACT
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, confirmReq(t, `{"code":`))

	// ASSERT
	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid request body")
}

// Confirming twice must not re-issue recovery codes: the second call has to be
// rejected before any code is minted.
func TestConfirm_RejectsWhenAlreadyEnabled(t *testing.T) {
	t.Parallel()

	// ARRANGE
	repo := inmemory.NewUserRepository()
	secret := "enc:already"
	require.NoError(t, repo.Save(context.Background(), &domain.User{
		ID: 1, Login: "alice", TwoFactorEnabled: true, TwoFactorSecret: &secret,
	}))
	handler := NewHandler(repo, &stubTwoFactor{}, api.NewResponder(), nil)

	// ACT
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, confirmReq(t, `{"code":"123456"}`))

	// ASSERT
	require.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "already enabled")
	assert.NotContains(t, w.Body.String(), "recovery_codes",
		"a rejected confirmation must not mint recovery codes")
}

func TestConfirm_RejectsWhenSessionUserIsGone(t *testing.T) {
	t.Parallel()

	// ARRANGE
	handler := NewHandler(inmemory.NewUserRepository(), &stubTwoFactor{}, api.NewResponder(), nil)

	// ACT
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, confirmReq(t, `{"code":"123456"}`))

	// ASSERT
	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "user not found")
}

// Whatever fails on the activation path, 2FA must stay off and no recovery
// codes may leak into the response.
func TestConfirm_StorageAndCryptoErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		repo        func(t *testing.T) *errUserRepository
		twoFactor   twoFactorManager
		mustNotLeak string
	}{
		{
			name: "user_lookup_error_aborts_activation",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: pendingRepo(t), findErr: errStorageUnavailable}
			},
			twoFactor:   &stubTwoFactor{},
			mustNotLeak: "storage unavailable",
		},
		{
			name: "code_validation_error_aborts_activation",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: pendingRepo(t)}
			},
			twoFactor:   &stubTwoFactor{validateErr: errValidationBackend},
			mustNotLeak: "cipher unavailable",
		},
		{
			name: "recovery_code_generation_error_aborts_activation",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: pendingRepo(t)}
			},
			twoFactor:   &stubTwoFactor{recoveryErr: errRecoveryGeneration},
			mustNotLeak: "entropy source unavailable",
		},
		{
			name: "activation_store_error_leaves_2fa_off",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: pendingRepo(t), saveErr: errStorageUnavailable}
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
			handler := NewHandler(repo, tt.twoFactor, api.NewResponder(), nil)

			// ACT
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, confirmReq(t, `{"code":"123456"}`))

			// ASSERT
			require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
			assert.NotContains(t, w.Body.String(), tt.mustNotLeak,
				"the internal cause must not reach the client")
			assert.NotContains(t, w.Body.String(), "recovery_codes",
				"a failed activation must not hand out recovery codes")

			users, err := repo.UserRepository.Find(context.Background(), nil, nil, nil)
			require.NoError(t, err)
			require.Len(t, users, 1)
			assert.False(t, users[0].TwoFactorEnabled, "a failed activation must leave 2FA off")
		})
	}
}

func pendingRepo(t *testing.T) *inmemory.UserRepository {
	t.Helper()

	repo := inmemory.NewUserRepository()
	secret := "enc:pending"
	require.NoError(t, repo.Save(context.Background(), &domain.User{
		ID: 1, Login: "alice", Email: "alice@example.com", TwoFactorSecret: &secret,
	}))

	return repo
}
