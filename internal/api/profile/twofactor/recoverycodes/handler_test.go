// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — regenerating recovery codes must
//     require the account password (a hijacked session alone must not be able
//     to silently rotate the fallback and lock the owner out), only works when
//     2FA is enabled, and replaces the stored set.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package recoverycodes

import (
	"bytes"
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
	"golang.org/x/crypto/bcrypt"
)

var (
	errStorageUnavailable = errors.New("storage unavailable")
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

// stubTwoFactor lets recovery-code generation fail on demand.
type stubTwoFactor struct {
	recoveryErr error
}

func (s *stubTwoFactor) GenerateRecoveryCodes() ([]string, string, error) {
	if s.recoveryErr != nil {
		return nil, "", s.recoveryErr
	}

	return []string{"code-1"}, "encoded", nil
}

func newFixture(t *testing.T, enabled bool) (*Handler, *inmemory.UserRepository) {
	t.Helper()

	manager, err := twofactor.NewManager([]byte("test-app-key"))
	require.NoError(t, err)
	_, encoded, err := manager.GenerateRecoveryCodes()
	require.NoError(t, err)
	hashedPassword, err := auth.HashPassword("password123")
	require.NoError(t, err)

	user := &domain.User{ID: 1, Login: "alice", Email: "alice@example.com", Password: hashedPassword}
	if enabled {
		user.TwoFactorEnabled = true
		user.TwoFactorRecoveryCodes = &encoded
	}

	repo := inmemory.NewUserRepository()
	require.NoError(t, repo.Save(context.Background(), user))

	return NewHandler(repo, manager, api.NewResponder(), nil), repo
}

func recoveryReq(t *testing.T, body string) *http.Request {
	t.Helper()

	session := &auth.Session{Login: "alice", User: &domain.User{ID: 1, Login: "alice"}}
	req := httptest.NewRequest(
		http.MethodPost, "/api/profile/2fa/recovery-codes", bytes.NewBufferString(body),
	)
	req.Header.Set("Content-Type", "application/json")

	return req.WithContext(auth.ContextWithSession(context.Background(), session))
}

func TestRegenerate_ReplacesCodesWithPassword(t *testing.T) {
	handler, repo := newFixture(t, true)

	before, err := repo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	originalCodes := *before[0].TwoFactorRecoveryCodes

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, recoveryReq(t, `{"password":"password123"}`))

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "recovery_codes")

	after, err := repo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, after[0].TwoFactorRecoveryCodes)
	assert.NotEqual(t, originalCodes, *after[0].TwoFactorRecoveryCodes,
		"the stored recovery-code set must be replaced")
}

func TestRegenerate_Rejections(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		body           string
		expectedStatus int
		wantError      string
	}{
		{
			name:           "wrong_password",
			enabled:        true,
			body:           `{"password":"nope"}`,
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid credentials",
		},
		{
			name:           "not_enabled",
			enabled:        false,
			body:           `{"password":"password123"}`,
			expectedStatus: http.StatusConflict,
			wantError:      "not enabled",
		},
		{
			name:           "missing_password",
			enabled:        true,
			body:           `{}`,
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "password field is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _ := newFixture(t, tt.enabled)

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, recoveryReq(t, tt.body))

			require.Equal(t, tt.expectedStatus, w.Code, "body=%s", w.Body.String())
			assert.Contains(t, w.Body.String(), tt.wantError)
		})
	}
}

func TestRegenerate_RejectsUnauthenticated(t *testing.T) {
	// ARRANGE
	handler, _ := newFixture(t, true)
	req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/recovery-codes",
		bytes.NewBufferString(`{"password":"password123"}`))

	// ACT
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// ASSERT
	require.Equal(t, http.StatusUnauthorized, w.Code, "body=%s", w.Body.String())
}

func TestRegenerate_MalformedBodyIsBadRequest(t *testing.T) {
	// ARRANGE
	handler, _ := newFixture(t, true)

	// ACT
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, recoveryReq(t, `{"password":`))

	// ASSERT
	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "invalid request body")
}

func TestRegenerate_RejectsWhenSessionUserIsGone(t *testing.T) {
	// ARRANGE
	handler := NewHandler(inmemory.NewUserRepository(), &stubTwoFactor{}, api.NewResponder(), nil)

	// ACT
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, recoveryReq(t, `{"password":"password123"}`))

	// ASSERT
	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "user not found")
}

// A failed rotation must keep the previously issued set usable — the owner
// must not be locked out of their fallback by a transient backend error.
func TestRegenerate_StorageAndCryptoErrors(t *testing.T) {
	tests := []struct {
		name        string
		repo        func(t *testing.T) *errUserRepository
		twoFactor   twoFactorManager
		mustNotLeak string
	}{
		{
			name: "user_lookup_error_aborts_rotation",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: enabledRepo(t), findErr: errStorageUnavailable}
			},
			twoFactor:   &stubTwoFactor{},
			mustNotLeak: "storage unavailable",
		},
		{
			name: "recovery_code_generation_error_aborts_rotation",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: enabledRepo(t)}
			},
			twoFactor:   &stubTwoFactor{recoveryErr: errRecoveryGeneration},
			mustNotLeak: "entropy source unavailable",
		},
		{
			name: "store_error_keeps_previous_codes",
			repo: func(t *testing.T) *errUserRepository {
				t.Helper()

				return &errUserRepository{UserRepository: enabledRepo(t), saveErr: errStorageUnavailable}
			},
			twoFactor:   &stubTwoFactor{},
			mustNotLeak: "storage unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			repo := tt.repo(t)
			handler := NewHandler(repo, tt.twoFactor, api.NewResponder(), nil)

			// ACT
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, recoveryReq(t, `{"password":"password123"}`))

			// ASSERT
			require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
			assert.NotContains(t, w.Body.String(), tt.mustNotLeak,
				"the internal cause must not reach the client")
			assert.NotContains(t, w.Body.String(), "recovery_codes",
				"a failed rotation must not hand out codes it did not persist")

			users, err := repo.UserRepository.Find(context.Background(), nil, nil, nil)
			require.NoError(t, err)
			require.Len(t, users, 1)
			require.NotNil(t, users[0].TwoFactorRecoveryCodes)
			assert.Equal(t, "original-codes", *users[0].TwoFactorRecoveryCodes,
				"the previously issued set must stay usable")
		})
	}
}

// A pre-§2.1.2 password hash (bare bcrypt, no SHA-256 pre-hash) still
// authenticates, and the rotation piggybacks the hash upgrade onto its Save.
func TestRegenerate_UpgradesLegacyPasswordHash(t *testing.T) {
	// ARRANGE
	legacyHash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	require.NoError(t, err)

	repo := inmemory.NewUserRepository()
	codes := "original-codes"
	require.NoError(t, repo.Save(context.Background(), &domain.User{
		ID:                     1,
		Login:                  "alice",
		Email:                  "alice@example.com",
		Password:               string(legacyHash),
		TwoFactorEnabled:       true,
		TwoFactorRecoveryCodes: &codes,
	}))
	handler := NewHandler(repo, &stubTwoFactor{}, api.NewResponder(), nil)

	// ACT
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, recoveryReq(t, `{"password":"password123"}`))

	// ASSERT
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	users, findErr := repo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, findErr)
	require.Len(t, users, 1)
	assert.NotEqual(t, string(legacyHash), users[0].Password,
		"the legacy hash must be upgraded in place")

	needsRehash, verifyErr := auth.VerifyPassword(users[0].Password, "password123")
	require.NoError(t, verifyErr, "the upgraded hash must still verify the same password")
	assert.False(t, needsRehash, "the upgraded hash must not ask for another rehash")
}

func enabledRepo(t *testing.T) *inmemory.UserRepository {
	t.Helper()

	hashedPassword, err := auth.HashPassword("password123")
	require.NoError(t, err)

	repo := inmemory.NewUserRepository()
	codes := "original-codes"
	require.NoError(t, repo.Save(context.Background(), &domain.User{
		ID:                     1,
		Login:                  "alice",
		Email:                  "alice@example.com",
		Password:               hashedPassword,
		TwoFactorEnabled:       true,
		TwoFactorRecoveryCodes: &codes,
	}))

	return repo
}
