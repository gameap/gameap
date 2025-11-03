package getabilities

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	rbac      base.RBAC
	responder base.Responder
}

func NewHandler(
	rbac base.RBAC,
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

	isAdmin, err := h.rbac.Can(
		ctx, session.User.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check user permissions"))

		return
	}

	groupedAbilities := domain.GetGroupedAbilities(isAdmin)
	response := newAbilitiesResponse(groupedAbilities)

	h.responder.Write(ctx, rw, response)
}
