package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
)

func TestTokenAdminGuardMiddleware(t *testing.T) {
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
			req := httptest.NewRequest(http.MethodDelete, "/api/users/5", nil)
			req = req.WithContext(auth.ContextWithSession(req.Context(), tt.session))

			rr := httptest.NewRecorder()
			middleware.Middleware(nextHandler).ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Equal(t, tt.wantReach, rr.Body.String() == "reached")
		})
	}
}
