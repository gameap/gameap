package getusers

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
	usersRepo repositories.UserRepository
	responder base.Responder
}

func NewHandler(
	usersRepo repositories.UserRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		usersRepo: usersRepo,
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

	in, err := readInput(r)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	filter := buildFilter(in)
	pagination := buildPagination(in)

	var users []domain.User

	if filter == nil {
		users, err = h.usersRepo.FindAll(ctx, nil, pagination)
	} else {
		users, err = h.usersRepo.Find(ctx, filter, nil, pagination)
	}

	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find users"))

		return
	}

	usersResponse := newUsersResponseFromUsers(users)

	h.responder.Write(ctx, rw, usersResponse)
}
