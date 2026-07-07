package middlewares

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

var errPATNotAllowedOnAdminRoute = errors.New(
	"personal access tokens cannot access this administrative endpoint",
)

// TokenAdminGuardMiddleware rejects personal-access-token (PAT) sessions on
// administrative routes that do not declare explicit PAT abilities.
//
// Such routes never pass through PersonalAccessMiddleware, so without this guard
// a PAT would be gated only by the user's RBAC admin role (IsAdminMiddleware) —
// the token's declared ability scope is ignored entirely, letting a narrowly
// scoped PAT perform any admin action the owning user may. This guard fails
// closed: an admin action via PAT is only reachable on routes that explicitly
// check the token's abilities. Non-token (session) callers pass through
// untouched to the normal admin check.
type TokenAdminGuardMiddleware struct {
	responder base.Responder
	audit     audit.Logger
}

func NewTokenAdminGuardMiddleware(responder base.Responder, auditLogger audit.Logger) *TokenAdminGuardMiddleware {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &TokenAdminGuardMiddleware{
		responder: responder,
		audit:     auditLogger,
	}
}

func (m *TokenAdminGuardMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		session := auth.SessionFromContext(ctx)

		if session.IsTokenSession() {
			audit.AccessDenied(ctx, m.audit, "admin", "", "pat_not_allowed_on_admin_route")
			m.responder.WriteError(ctx, w, api.WrapHTTPError(
				errPATNotAllowedOnAdminRoute,
				http.StatusForbidden,
			))

			return
		}

		next.ServeHTTP(w, r)
	})
}
