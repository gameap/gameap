package getuser

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	usersRepo repositories.UserRepository
	rbacRepo  repositories.RBACRepository
	responder base.Responder
}

func NewHandler(
	usersRepo repositories.UserRepository,
	rbacRepo repositories.RBACRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		usersRepo: usersRepo,
		rbacRepo:  rbacRepo,
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

	input := api.NewInputReader(r)

	userID, err := input.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid user id"),
			http.StatusBadRequest,
		))

		return
	}

	filter := &filters.FindUser{
		IDs: []uint{userID},
	}

	users, err := h.usersRepo.Find(ctx, filter, nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user"))

		return
	}

	if len(users) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("user not found"))

		return
	}

	user := users[0]

	roles, err := h.rbacRepo.GetRolesForEntity(ctx, user.ID, domain.EntityTypeUser)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to get user roles"))

		return
	}

	h.responder.Write(ctx, rw, newUserResponseFromUser(&user, roles))
}
