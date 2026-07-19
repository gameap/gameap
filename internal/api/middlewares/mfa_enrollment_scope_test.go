// Security tests.
//
// OWASP API Security Top 10:2023:
//   - API2:2023 Broken Authentication — a session minted from an
//     MFA-enrollment token must only be honoured on the 2FA-enrollment
//     endpoints that explicitly opted in. Everywhere else it is refused with
//     403 and the denial is audited, so a forced-enrollment admin cannot reach
//     the wider API until a second factor is enrolled.
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

func TestMFAEnrollmentScopeMiddleware(t *testing.T) {
	authedUser := &domain.User{ID: 1, Login: "admin"}

	tests := []struct {
		name               string
		session            *auth.Session
		allowMFAEnrollment bool
		enforceDisabled    bool
		wantStatus         int
		wantDenied         bool
	}{
		{
			name:               "enrollment_session_on_opted_in_route_passes",
			session:            &auth.Session{User: authedUser, MFAEnrollmentOnly: true},
			allowMFAEnrollment: true,
			wantStatus:         http.StatusOK,
		},
		{
			name:               "enrollment_session_on_other_route_is_denied",
			session:            &auth.Session{User: authedUser, MFAEnrollmentOnly: true},
			allowMFAEnrollment: false,
			wantStatus:         http.StatusForbidden,
			wantDenied:         true,
		},
		{
			name:               "normal_session_is_unaffected",
			session:            &auth.Session{User: authedUser},
			allowMFAEnrollment: false,
			wantStatus:         http.StatusOK,
		},
		{
			name:               "no_session_passes_through",
			session:            nil,
			allowMFAEnrollment: false,
			wantStatus:         http.StatusOK,
		},
		{
			// AUTH_REQUIRE_MFA_FOR_ADMINS=false: a leftover enrollment-scoped
			// token must be honoured as a full session, otherwise its bearer is
			// locked out with no enrollment flow to escape through.
			name:               "enrollment_session_honoured_when_enforcement_disabled",
			session:            &auth.Session{User: authedUser, MFAEnrollmentOnly: true},
			allowMFAEnrollment: false,
			enforceDisabled:    true,
			wantStatus:         http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &auditCapture{}
			mw := NewMFAEnrollmentScopeMiddleware(api.NewResponder(), recorder, !tt.enforceDisabled)
			handler := mw.Middleware(noopNextHandler(), tt.allowMFAEnrollment)

			req := httptest.NewRequest(http.MethodGet, "/api/servers", http.NoBody)
			if tt.session != nil {
				req = req.WithContext(auth.ContextWithSession(req.Context(), tt.session))
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code, "body=%s", w.Body.String())

			ev, found := findEvent(recorder.snapshot(), audit.EventAccessDenied)
			if tt.wantDenied {
				assert.Contains(t, w.Body.String(), "restricted to two-factor enrollment")
				require.True(t, found, "a scope denial must be audited")
				assert.Equal(t, audit.OutcomeDenied, ev.Outcome)
				assert.Equal(t, "mfa_enrollment_scope", ev.Reason)
			} else {
				assert.False(t, found, "no denial event when the request is allowed")
			}
		})
	}
}

// TestMFAEnrollmentScopeMiddleware_DeniedBeforeHandler covers OWASP API2:2023:
// the denied request must never reach the wrapped handler.
func TestMFAEnrollmentScopeMiddleware_DeniedBeforeHandler(t *testing.T) {
	reached := false
	mw := NewMFAEnrollmentScopeMiddleware(api.NewResponder(), &auditCapture{}, true)
	handler := mw.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached = true
	}), false)

	req := httptest.NewRequest(http.MethodGet, "/api/servers", http.NoBody)
	req = req.WithContext(auth.ContextWithSession(
		context.Background(), &auth.Session{User: &domain.User{ID: 1}, MFAEnrollmentOnly: true},
	))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.False(t, reached, "a scope-denied request must not reach the handler")
}
