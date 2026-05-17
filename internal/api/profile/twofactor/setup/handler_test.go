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
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestSetup_RejectsUnauthenticated(t *testing.T) {
	handler := NewHandler(inmemory.NewUserRepository(), newManager(t), api.NewResponder())

	req := httptest.NewRequest(http.MethodPost, "/api/profile/2fa/setup", http.NoBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}
