package middlewares

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
)

type DaemonAuthMiddleware struct {
	nodeRepo  repositories.NodeRepository
	responder base.Responder
	audit     audit.Logger
}

func NewDaemonAuthMiddleware(
	nodeRepo repositories.NodeRepository,
	responder base.Responder,
	auditLogger audit.Logger,
) *DaemonAuthMiddleware {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &DaemonAuthMiddleware{
		nodeRepo:  nodeRepo,
		responder: responder,
		audit:     auditLogger,
	}
}

func (m *DaemonAuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authToken := r.Header.Get("X-Auth-Token")
		if authToken == "" {
			audit.DaemonRejected(r.Context(), m.audit, "token_not_set")
			m.responder.WriteError(r.Context(), w, api.WrapHTTPError(
				errors.New("token not set"),
				http.StatusUnauthorized,
			))

			return
		}

		// Daemon tokens are stored as SHA-256 hashes; hash the presented token
		// before lookup so the database never has to hold the plaintext.
		hashedToken := pkgstrings.SHA256(authToken)
		nodes, err := m.nodeRepo.Find(
			r.Context(),
			&filters.FindNode{
				GDaemonAPIToken: &hashedToken,
				WithDeleted:     true,
			},
			nil,
			&filters.Pagination{Limit: 1},
		)
		if err != nil {
			m.responder.WriteError(r.Context(), w, api.WrapHTTPError(
				errors.WithMessage(err, "failed to find node"),
				http.StatusInternalServerError,
			))

			return
		}

		if len(nodes) == 0 {
			audit.DaemonRejected(r.Context(), m.audit, "invalid_api_token")
			m.responder.WriteError(r.Context(), w, api.WrapHTTPError(
				errors.New("invalid api token"),
				http.StatusUnauthorized,
			))

			return
		}

		node := &nodes[0]

		// Add daemon session to context
		ctx := auth.ContextWithDaemonSession(r.Context(), &auth.DaemonSession{
			Node: node,
		})

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
