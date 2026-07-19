package middlewares

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

var errMFAEnrollmentScope = errors.New("session is restricted to two-factor enrollment")

// MFAEnrollmentScopeMiddleware enforces that a session established from an
// MFA-enrollment token — issued at login to an admin who crossed the MFA
// hard-fail threshold (see services/mfanudge) — is only honoured on endpoints
// that explicitly opted in: the 2FA setup/confirm/recovery routes, the profile
// read, logout and the public config. On any other endpoint such a session is
// rejected with 403, so a forced-enrollment user cannot reach the rest of the
// API until 2FA is enrolled.
//
// It must run after the auth middleware (the session is read from context) and
// before the handler. A request without an enrollment-scoped session passes
// through untouched, so it is safe to wrap every route with it.
//
// enforce mirrors AUTH_REQUIRE_MFA_FOR_ADMINS: with the feature disabled an
// enrollment-scoped token that is still in circulation (minted before the
// operator turned the flag off) is honoured as a full session — otherwise its
// bearer would stay locked out of the API with no enrollment modal to escape
// through.
type MFAEnrollmentScopeMiddleware struct {
	responder base.Responder
	audit     audit.Logger
	enforce   bool
}

func NewMFAEnrollmentScopeMiddleware(
	responder base.Responder,
	auditLogger audit.Logger,
	enforce bool,
) *MFAEnrollmentScopeMiddleware {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &MFAEnrollmentScopeMiddleware{
		responder: responder,
		audit:     auditLogger,
		enforce:   enforce,
	}
}

func (m *MFAEnrollmentScopeMiddleware) Middleware(next http.Handler, allowMFAEnrollment bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		session := auth.SessionFromContext(ctx)

		if m.enforce && session != nil && session.MFAEnrollmentOnly && !allowMFAEnrollment {
			audit.AccessDenied(ctx, m.audit, "endpoint", r.URL.Path, "mfa_enrollment_scope")
			m.responder.WriteError(ctx, w, api.WrapHTTPError(
				errMFAEnrollmentScope,
				http.StatusForbidden,
			))

			return
		}

		next.ServeHTTP(w, r)
	})
}
