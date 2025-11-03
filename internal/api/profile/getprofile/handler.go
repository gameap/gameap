package getprofile

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	rbac      repositories.RBACRepository
	responder base.Responder
}

func NewHandler(
	rbac repositories.RBACRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		rbac:      rbac,
		responder: responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := auth.SessionFromContext(ctx)
	if !session.IsAuthenticated() {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user not authenticated"),
			http.StatusUnauthorized,
		))

		return
	}

	roles, err := h.rbac.GetRolesForEntity(ctx, session.User.ID, domain.EntityTypeUser)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to get user roles"))

		return
	}

	h.responder.Write(ctx, rw, newProfileResponseFromUser(session.User, roles))
}
