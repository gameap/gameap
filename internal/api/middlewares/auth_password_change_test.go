// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — changing a user's password must
//     invalidate every credential issued before the change: session tokens
//     whose `iat` predates the change, and personal access tokens created
//     before it. A credential minted after the change (or for a user with no
//     recorded change) must keep working.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package middlewares

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func passwordChangeMiddleware(
	userRepo *inmemory.UserRepository, tokenRepo *inmemory.PersonalAccessTokenRepository,
) *AuthMiddleware {
	return NewAuthMiddleware(
		auth.NewJWTService([]byte(testJWTSecret)),
		userRepo,
		tokenRepo,
		auth.NoopRevocation{},
		nil,
		api.NewResponder(),
		nil,
	)
}

func TestAuthMiddleware_PasswordChange_InvalidatesSessionTokens(t *testing.T) {
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
			name:              "token_issued_before_password_change_is_rejected",
			passwordChangedAt: &future, // token minted now; iat (now) < future
			wantStatus:        http.StatusUnauthorized,
		},
		{
			name:              "token_issued_after_password_change_is_accepted",
			passwordChangedAt: &past, // token minted now; iat (now) > past
			wantStatus:        http.StatusOK,
		},
		{
			name:              "no_recorded_password_change_is_accepted",
			passwordChangedAt: nil,
			wantStatus:        http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			userRepo := inmemory.NewUserRepository()
			user := &domain.User{ID: 1, Login: "alice", Email: "alice@example.com"}
			user.SetPasswordChangedAt(tt.passwordChangedAt)
			require.NoError(t, userRepo.Save(context.Background(), user))

			token, err := auth.NewJWTService([]byte(testJWTSecret)).GenerateTokenForUser(user, time.Hour)
			require.NoError(t, err)

			handler := passwordChangeMiddleware(userRepo, inmemory.NewPersonalAccessTokenRepository()).
				Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))

			req := httptest.NewRequest(http.MethodGet, "/api/servers", http.NoBody)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code, "body=%s", w.Body.String())
		})
	}
}

func TestAuthMiddleware_PasswordChange_InvalidatesPersonalAccessTokens(t *testing.T) {
	t.Parallel()
	now := time.Now()
	beforeChange := now.Add(-2 * time.Hour)
	afterChange := now.Add(-30 * time.Minute)
	changedAt := now.Add(-time.Hour)

	tests := []struct {
		name         string
		tokenCreated time.Time
		wantStatus   int
	}{
		{
			name:         "pat_created_before_password_change_is_rejected",
			tokenCreated: beforeChange,
			wantStatus:   http.StatusUnauthorized,
		},
		{
			name:         "pat_created_after_password_change_is_accepted",
			tokenCreated: afterChange,
			wantStatus:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			userRepo := inmemory.NewUserRepository()
			user := &domain.User{ID: 1, Login: "alice", Email: "alice@example.com"}
			user.SetPasswordChangedAt(&changedAt)
			require.NoError(t, userRepo.Save(context.Background(), user))

			created := tt.tokenCreated
			tokenRepo := inmemory.NewPersonalAccessTokenRepository()
			pat := &domain.PersonalAccessToken{
				TokenableType: domain.EntityTypeUser,
				TokenableID:   user.ID,
				Name:          "ci",
				Token:         pkgstrings.SHA256("s3cret"),
				Abilities:     &[]domain.PATAbility{"server-list"},
				CreatedAt:     &created,
			}
			require.NoError(t, tokenRepo.Save(context.Background(), pat))

			handler := passwordChangeMiddleware(userRepo, tokenRepo).
				Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))

			req := httptest.NewRequest(http.MethodGet, "/api/servers", http.NoBody)
			req.Header.Set("Authorization", "Bearer "+strconv.FormatUint(uint64(pat.ID), 10)+"|s3cret")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code, "body=%s", w.Body.String())
		})
	}
}
