package middlewares

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type DaemonAuthMiddleware struct {
	nodeRepo  repositories.NodeRepository
	responder base.Responder
}

func NewDaemonAuthMiddleware(
	nodeRepo repositories.NodeRepository,
	responder base.Responder,
) *DaemonAuthMiddleware {
	return &DaemonAuthMiddleware{
		nodeRepo:  nodeRepo,
		responder: responder,
	}
}

func (m *DaemonAuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authToken := r.Header.Get("X-Auth-Token")
		if authToken == "" {
			m.responder.WriteError(r.Context(), w, api.WrapHTTPError(
				errors.New("token not set"),
				http.StatusUnauthorized,
			))

			return
		}

		// Find node by gdaemon_api_token
		nodes, err := m.nodeRepo.Find(
			r.Context(),
			&filters.FindNode{
				GDaemonAPIToken: &authToken,
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
