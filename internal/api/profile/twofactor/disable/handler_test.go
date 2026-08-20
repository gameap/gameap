// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — turning 2FA off must require BOTH the
//     account password and a valid second factor, so neither a hijacked
//     session alone nor a leaked password alone can strip the protection.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package disable

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type disableFixture struct {
	handler      *Handler
	repo         *inmemory.UserRepository
	recoveryCode string
}

func newDisableFixture(t *testing.T, enabled bool) *disableFixture {
	t.Helper()

	manager, err := twofactor.NewManager([]byte("test-app-key"))
	require.NoError(t, err)

	plain, encoded, err := manager.GenerateRecoveryCodes()
	require.NoError(t, err)
	secret, _, err := manager.GenerateSecret("alice")
	require.NoError(t, err)
	encSecret, err := manager.EncryptSecret(secret)
	require.NoError(t, err)
	hashedPassword, err := auth.HashPassword("password123")
	require.NoError(t, err)

	user := &domain.User{ID: 1, Login: "alice", Email: "alice@example.com", Password: hashedPassword}
	if enabled {
		user.TwoFactorEnabled = true
		user.TwoFactorSecret = &encSecret
		user.TwoFactorRecoveryCodes = &encoded
	}

	repo := inmemory.NewUserRepository()
	require.NoError(t, repo.Save(context.Background(), user))

	return &disableFixture{
		handler:      NewHandler(repo, manager, api.NewResponder(), nil),
		repo:         repo,
		recoveryCode: plain[0],
	}
}

func disableReq(t *testing.T, body string) *http.Request {
	t.Helper()

	session := &auth.Session{Login: "alice", User: &domain.User{ID: 1, Login: "alice"}}
	req := httptest.NewRequest(http.MethodDelete, "/api/profile/2fa", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	return req.WithContext(auth.ContextWithSession(context.Background(), session))
}

func TestDisable_PasswordPlusCodeClearsTwoFactor(t *testing.T) {
	t.Parallel()

	f := newDisableFixture(t, true)

	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, disableReq(t, `{"password":"password123","code":"`+f.recoveryCode+`"}`))

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	users, err := f.repo.Find(context.Background(), nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.False(t, users[0].TwoFactorEnabled)
	assert.Nil(t, users[0].TwoFactorSecret, "the stored secret must be wiped")
	assert.Nil(t, users[0].TwoFactorRecoveryCodes, "recovery codes must be wiped")
}

func TestDisable_Rejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		enabled        bool
		body           func(f *disableFixture) string
		expectedStatus int
		wantError      string
	}{
		{
			name:           "wrong_password",
			enabled:        true,
			body:           func(f *disableFixture) string { return `{"password":"nope","code":"` + f.recoveryCode + `"}` },
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid credentials",
		},
		{
			name:           "wrong_code",
			enabled:        true,
			body:           func(_ *disableFixture) string { return `{"password":"password123","code":"zzzzz-zzzzz"}` },
			expectedStatus: http.StatusUnauthorized,
			wantError:      "invalid verification code",
		},
		{
			name:           "not_enabled",
			enabled:        false,
			body:           func(f *disableFixture) string { return `{"password":"password123","code":"` + f.recoveryCode + `"}` },
			expectedStatus: http.StatusConflict,
			wantError:      "not enabled",
		},
		{
			name:           "missing_code",
			enabled:        true,
			body:           func(_ *disableFixture) string { return `{"password":"password123"}` },
			expectedStatus: http.StatusUnprocessableEntity,
			wantError:      "code field is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newDisableFixture(t, tt.enabled)

			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, disableReq(t, tt.body(f)))

			require.Equal(t, tt.expectedStatus, w.Code, "body=%s", w.Body.String())
			assert.Contains(t, w.Body.String(), tt.wantError)
		})
	}
}
