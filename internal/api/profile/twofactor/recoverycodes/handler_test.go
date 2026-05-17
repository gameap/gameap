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
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/twofactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
