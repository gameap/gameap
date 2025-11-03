package deleteuser

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	userService *services.UserService
	responder   base.Responder
}

func NewHandler(
	userService *services.UserService,
	responder base.Responder,
) *Handler {
	return &Handler{
		userService: userService,
		responder:   responder,
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

	if userID == session.User.ID {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("cannot delete yourself"),
			http.StatusBadRequest,
		))

		return
	}

	users, err := h.userService.Find(ctx, &filters.FindUser{
		IDs: []uint{userID},
	}, nil, &filters.Pagination{
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

	err = h.userService.Delete(ctx, userID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete user"))

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
