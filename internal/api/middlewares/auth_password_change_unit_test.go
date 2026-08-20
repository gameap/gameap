// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — changing a user's password must
//     invalidate every credential issued before the change. These tests pin
//     the cutoff-policy helpers directly (the branches that a real, always-iat
//     JWT/PASETO cannot reach) and the PASETO session path through the
//     middleware.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package middlewares

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errStubIssuedAt is a stand-in for an unreadable `iat` claim.
var errStubIssuedAt = errors.New("iat unreadable")

// stubClaims is a minimal auth.Claims whose GetIssuedAt result is fully
// controllable, so the fail-open branches (absent / unreadable iat) — which a
// real token that always carries an iat cannot exercise — can be reached.
type stubClaims struct {
	iat    *time.Time
	iatErr error
}

func (s stubClaims) GetSubject() (string, error)            { return "user:login:alice", nil }
func (s stubClaims) GetExpirationTime() (*time.Time, error) { return nil, nil }
func (s stubClaims) GetScope() (string, error)              { return "", nil }
func (s stubClaims) GetIssuedAt() (*time.Time, error)       { return s.iat, s.iatErr }

// TestRejectPATIfCreatedBeforePasswordChange covers OWASP API2:2023. A PAT
// created before the user's last password change is rejected; a token or user
// missing the relevant timestamp passes (backward compatible with credentials
// predating the tracking).
func TestRejectPATIfCreatedBeforePasswordChange(t *testing.T) {
	t.Parallel()
	now := time.Now()
	before := now.Add(-time.Hour)
	after := now.Add(time.Hour)

	tests := []struct {
		name              string
		passwordChangedAt *time.Time
		tokenCreatedAt    *time.Time
		wantError         string
	}{
		{
			name:              "no_password_change_recorded_passes",
			passwordChangedAt: nil,
			tokenCreatedAt:    &before,
			wantError:         "",
		},
		{
			name:              "token_without_created_at_passes",
			passwordChangedAt: &now,
			tokenCreatedAt:    nil,
			wantError:         "",
		},
		{
			name:              "token_created_before_change_is_rejected",
			passwordChangedAt: &now,
			tokenCreatedAt:    &before,
			wantError:         "invalid personal access token",
		},
		{
			name:              "token_created_after_change_passes",
			passwordChangedAt: &now,
			tokenCreatedAt:    &after,
			wantError:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			user := &domain.User{ID: 1, Login: "alice"}
			user.SetPasswordChangedAt(tt.passwordChangedAt)
			token := &domain.PersonalAccessToken{CreatedAt: tt.tokenCreatedAt}

			// ACT
			err := rejectPATIfCreatedBeforePasswordChange(user, token)

			// ASSERT
			if tt.wantError == "" {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

// TestRejectIfIssuedBeforePasswordChange covers OWASP API2:2023. A session
// token whose iat precedes the password change is rejected; an absent change
// time or an absent/unreadable iat is fail-open (the token already passed
// signature validation, so a missing cutoff must not itself reject it).
func TestRejectIfIssuedBeforePasswordChange(t *testing.T) {
	t.Parallel()
	now := time.Now()
	before := now.Add(-time.Hour)
	after := now.Add(time.Hour)

	tests := []struct {
		name              string
		passwordChangedAt *time.Time
		claims            stubClaims
		wantError         string
	}{
		{
			name:              "no_password_change_recorded_passes",
			passwordChangedAt: nil,
			claims:            stubClaims{iat: &before},
			wantError:         "",
		},
		{
			name:              "absent_iat_does_not_reject",
			passwordChangedAt: &now,
			claims:            stubClaims{iat: nil},
			wantError:         "",
		},
		{
			name:              "unreadable_iat_does_not_reject",
			passwordChangedAt: &now,
			claims:            stubClaims{iat: nil, iatErr: errStubIssuedAt},
			wantError:         "",
		},
		{
			name:              "issued_before_change_is_rejected",
			passwordChangedAt: &now,
			claims:            stubClaims{iat: &before},
			wantError:         "invalid or expired token",
		},
		{
			name:              "issued_after_change_passes",
			passwordChangedAt: &now,
			claims:            stubClaims{iat: &after},
			wantError:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			user := &domain.User{ID: 1, Login: "alice"}
			user.SetPasswordChangedAt(tt.passwordChangedAt)

			// ACT
			err := rejectIfIssuedBeforePasswordChange(user, tt.claims)

			// ASSERT
			if tt.wantError == "" {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

// TestAuthMiddleware_PasswordChange_InvalidatesPASETOSessionTokens covers OWASP
// API2:2023 through the PASETO code path (v4.local tokens), exercising the real
// pasetoClaims.GetIssuedAt cutoff comparison end-to-end in the middleware.
func TestAuthMiddleware_PasswordChange_InvalidatesPASETOSessionTokens(t *testing.T) {
	t.Parallel()
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	tests := []struct {
		name              string
		passwordChangedAt *time.Time
		wantStatus        int
	}{
		{
			name:              "paseto_issued_before_password_change_is_rejected",
			passwordChangedAt: &future, // token minted now; iat (now) < future
			wantStatus:        http.StatusUnauthorized,
		},
		{
			name:              "paseto_issued_after_password_change_is_accepted",
			passwordChangedAt: &past, // token minted now; iat (now) > past
			wantStatus:        http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			userRepo := inmemory.NewUserRepository()
			user := &domain.User{ID: 1, Login: "alice", Email: "alice@example.com"}
			user.SetPasswordChangedAt(tt.passwordChangedAt)
			require.NoError(t, userRepo.Save(context.Background(), user))

			pasetoSvc, err := auth.NewPASETOService([]byte(testJWTSecret))
			require.NoError(t, err)

			token, err := pasetoSvc.GenerateTokenForUser(user, time.Hour)
			require.NoError(t, err)

			mw := NewAuthMiddleware(
				pasetoSvc, userRepo, inmemory.NewPersonalAccessTokenRepository(),
				auth.NoopRevocation{}, nil, api.NewResponder(), nil,
			)
			handler := mw.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/servers", http.NoBody)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			assert.Equal(t, tt.wantStatus, w.Code, "body=%s", w.Body.String())
		})
	}
}
