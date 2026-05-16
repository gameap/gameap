package middlewares

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

var errShortLivedTokenScope = errors.New("short-lived token is not accepted on this endpoint")

// ShortLivedScopeMiddleware enforces that single-use short-lived tokens are
// only honoured on endpoints that explicitly opted in — the WebSocket
// upgrades and file downloads, the only places a token must travel in the
// URL. On any other endpoint a short-lived session is rejected with 403,
// shrinking the blast radius if such a token leaks via logs/history/proxies.
//
// It must run after the auth middleware (the session is read from context)
// and before the handler. A request without a short-lived session passes
// through untouched, so it is safe to wrap every route with it.
type ShortLivedScopeMiddleware struct {
	responder base.Responder
	audit     audit.Logger
}

func NewShortLivedScopeMiddleware(
	responder base.Responder,
	auditLogger audit.Logger,
) *ShortLivedScopeMiddleware {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &ShortLivedScopeMiddleware{
		responder: responder,
		audit:     auditLogger,
	}
}

func (m *ShortLivedScopeMiddleware) Middleware(next http.Handler, allowShortLived bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		session := auth.SessionFromContext(ctx)

		if session != nil && session.ShortLived && !allowShortLived {
			audit.AccessDenied(ctx, m.audit, "endpoint", r.URL.Path, "shorttoken_scope")
			m.responder.WriteError(ctx, w, api.WrapHTTPError(
				errShortLivedTokenScope,
				http.StatusForbidden,
			))

			return
		}

		next.ServeHTTP(w, r)
	})
}
