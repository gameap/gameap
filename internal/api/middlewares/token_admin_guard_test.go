package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenAdminGuardMiddleware(t *testing.T) {
	t.Parallel()
	middleware := NewTokenAdminGuardMiddleware(api.NewResponder(), nil)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reached"))
	})

	tests := []struct {
		name       string
		session    *auth.Session
		wantStatus int
		wantReach  bool
	}{
		{
			name:       "user_session_passes_through_to_admin_check",
			session:    &auth.Session{User: &domain.User{ID: 1, Login: "admin"}},
			wantStatus: http.StatusOK,
			wantReach:  true,
		},
		{
			// A nil session (unauthenticated / optional-auth request) is not a
			// token session, so the guard is transparent and the downstream
			// admin check decides. IsTokenSession() must be nil-safe.
			name:       "nil_session_passes_through",
			session:    nil,
			wantStatus: http.StatusOK,
			wantReach:  true,
		},
		{
			// An authenticated non-token (interactive) session without a token
			// also passes straight through to the normal admin check.
			name:       "authenticated_non_token_session_passes_through",
			session:    &auth.Session{User: &domain.User{ID: 1, Login: "admin"}, Token: nil},
			wantStatus: http.StatusOK,
			wantReach:  true,
		},
		{
			name: "pat_session_is_forbidden_on_admin_route_without_ability_check",
			session: &auth.Session{
				User:  &domain.User{ID: 1, Login: "admin"},
				Token: &domain.PersonalAccessToken{ID: 7, Abilities: &[]domain.PATAbility{"server-list"}},
			},
			wantStatus: http.StatusForbidden,
			wantReach:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodDelete, "/api/users/5", nil)
			req = req.WithContext(auth.ContextWithSession(req.Context(), tt.session))

			rr := httptest.NewRecorder()
			middleware.Middleware(nextHandler).ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Equal(t, tt.wantReach, rr.Body.String() == "reached")
		})
	}
}

// TestTokenAdminGuardMiddleware_Audit pins the detective control.
//
// OWASP API Security Top 10:2023:
//   - API5:2023 Broken Function Level Authorization — a PAT denied on an
//     administrative route must leave an access-denied audit event (OWASP ASVS
//     §7.2.1) so the attempt is visible; a permitted (non-token) caller must
//     NOT produce a denial record (no false authorization claim).
//
// Reference: https://owasp.org/API-Security/editions/2023/
func TestTokenAdminGuardMiddleware_Audit(t *testing.T) {
	t.Parallel()
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("pat_denial_emits_access_denied_event", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		recorder := &auditCapture{}
		middleware := NewTokenAdminGuardMiddleware(api.NewResponder(), recorder)
		session := &auth.Session{
			User:  &domain.User{ID: 1, Login: "admin"},
			Token: &domain.PersonalAccessToken{ID: 7, Abilities: &[]domain.PATAbility{"server-list"}},
		}

		req := httptest.NewRequest(http.MethodDelete, "/api/users/5", nil)
		req = req.WithContext(auth.ContextWithSession(req.Context(), session))
		rr := httptest.NewRecorder()

		// ACT
		middleware.Middleware(nextHandler).ServeHTTP(rr, req)

		// ASSERT
		require.Equal(t, http.StatusForbidden, rr.Code)

		ev, found := findEvent(recorder.snapshot(), audit.EventAccessDenied)
		require.True(t, found, "a PAT denied on an admin route must leave an access-denied event")
		assert.Equal(t, audit.CategoryAuthorization, ev.Category)
		assert.Equal(t, audit.OutcomeDenied, ev.Outcome)
		assert.Equal(t, "admin", ev.ResourceType)
		assert.Equal(t, "pat_not_allowed_on_admin_route", ev.Reason)
		assert.Equal(t, audit.AuthMethodPAT, ev.AuthMethod,
			"the denied caller was authenticated by a personal access token")
		assert.Equal(t, uint(1), ev.ActorID, "the denial must be attributed to the token's owning user")
	})

	t.Run("non_token_session_emits_no_denial", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		recorder := &auditCapture{}
		middleware := NewTokenAdminGuardMiddleware(api.NewResponder(), recorder)
		session := &auth.Session{User: &domain.User{ID: 1, Login: "admin"}}

		req := httptest.NewRequest(http.MethodDelete, "/api/users/5", nil)
		req = req.WithContext(auth.ContextWithSession(req.Context(), session))
		rr := httptest.NewRecorder()

		// ACT
		middleware.Middleware(nextHandler).ServeHTTP(rr, req)

		// ASSERT
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventAccessDenied),
			"an interactive session that passes the guard must not record a denial")
	})
}
