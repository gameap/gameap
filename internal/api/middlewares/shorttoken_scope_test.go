// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API1:2023 Broken Object Level Authorization / API2:2023 Broken
//     Authentication — a single-use short-lived token must only be honoured on
//     endpoints that explicitly opted in (WebSocket upgrades, file downloads).
//     Everywhere else it is refused with 403 and the denial is audited, so a
//     token captured from a URL cannot be replayed against the wider API.
//
// Reference: https://owasp.org/API-Security/editions/2023/
package middlewares

import (
	"context"
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

func TestShortLivedScopeMiddleware(t *testing.T) {
	t.Parallel()
	authedUser := &domain.User{ID: 1, Login: "alice"}

	tests := []struct {
		name            string
		session         *auth.Session
		allowShortLived bool
		wantStatus      int
		wantDenied      bool
	}{
		{
			name:            "short_lived_session_on_opted_in_route_passes",
			session:         &auth.Session{User: authedUser, ShortLived: true},
			allowShortLived: true,
			wantStatus:      http.StatusOK,
		},
		{
			name:            "short_lived_session_on_other_route_is_denied",
			session:         &auth.Session{User: authedUser, ShortLived: true},
			allowShortLived: false,
			wantStatus:      http.StatusForbidden,
			wantDenied:      true,
		},
		{
			// The guard must key off ShortLived, not the presence of a Token:
			// a PAT-derived short-lived session is still URL-borne and must be
			// confined to opted-in routes even though it carries PAT abilities.
			name: "pat_derived_short_lived_session_on_other_route_is_denied",
			session: &auth.Session{
				User:       authedUser,
				ShortLived: true,
				Token: &domain.PersonalAccessToken{
					ID:        7,
					Abilities: &[]domain.PATAbility{domain.PATAbilityServerList},
				},
			},
			allowShortLived: false,
			wantStatus:      http.StatusForbidden,
			wantDenied:      true,
		},
		{
			name:            "normal_session_is_unaffected",
			session:         &auth.Session{User: authedUser},
			allowShortLived: false,
			wantStatus:      http.StatusOK,
		},
		{
			name:            "no_session_passes_through",
			session:         nil,
			allowShortLived: false,
			wantStatus:      http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			recorder := &auditCapture{}
			mw := NewShortLivedScopeMiddleware(api.NewResponder(), recorder)
			handler := mw.Middleware(noopNextHandler(), tt.allowShortLived)

			req := httptest.NewRequest(http.MethodGet, "/api/servers", http.NoBody)
			if tt.session != nil {
				req = req.WithContext(auth.ContextWithSession(req.Context(), tt.session))
			}
			w := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(w, req)

			// ASSERT
			require.Equal(t, tt.wantStatus, w.Code, "body=%s", w.Body.String())

			ev, found := findEvent(recorder.snapshot(), audit.EventAccessDenied)
			if tt.wantDenied {
				assert.Contains(t, w.Body.String(), "short-lived token is not accepted")
				require.True(t, found, "a scope denial must be audited")
				assert.Equal(t, audit.OutcomeDenied, ev.Outcome)
				assert.Equal(t, "shorttoken_scope", ev.Reason)
			} else {
				assert.False(t, found, "no denial event when the request is allowed")
			}
		})
	}
}

// TestShortLivedScopeMiddleware_DeniedBeforeHandler covers OWASP API2:2023:
// the denied request must never reach the wrapped handler.
func TestShortLivedScopeMiddleware_DeniedBeforeHandler(t *testing.T) {
	t.Parallel()
	reached := false
	mw := NewShortLivedScopeMiddleware(api.NewResponder(), &auditCapture{})
	handler := mw.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}), false)

	req := httptest.NewRequest(http.MethodGet, "/api/servers", http.NoBody)
	req = req.WithContext(auth.ContextWithSession(
		context.Background(), &auth.Session{User: &domain.User{ID: 1}, ShortLived: true},
	))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.False(t, reached, "a scope-denied request must not reach the handler")
}
