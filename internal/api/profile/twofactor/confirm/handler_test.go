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
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	f := newConfirmFixture(t, false)

	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, confirmReq(t, `{"code":"`+f.validCode+`"}`))

	require.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "no pending two-factor enrollment")
}

func TestConfirm_MissingCodeIsValidationError(t *testing.T) {
	f := newConfirmFixture(t, true)

	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, confirmReq(t, `{}`))

	require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "code field is required")
}
