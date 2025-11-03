package putprofile

import (
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	userRepo  repositories.UserRepository
	responder base.Responder
}

func NewHandler(
	userRepo repositories.UserRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		userRepo:  userRepo,
		responder: responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := auth.SessionFromContext(ctx)
	if session == nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user not authenticated"),
			http.StatusUnauthorized,
		))

		return
	}

	input := &updateProfileInput{}

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request"),
			http.StatusBadRequest,
		))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid input"),
			http.StatusBadRequest,
		))

		return
	}

	users, err := h.userRepo.Find(ctx, &filters.FindUser{
		Logins: []string{session.Login},
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

	user := &users[0]

	// Verify current password if password change is requested
	if input.Password != nil {
		if input.CurrentPassword == nil {
			h.responder.WriteError(ctx, rw, api.WrapHTTPError(
				errors.New("current password is required for password change"),
				http.StatusBadRequest,
			))

			return
		}

		err = auth.VerifyPassword(user.Password, *input.CurrentPassword)
		if err != nil {
			h.responder.WriteError(ctx, rw, api.WrapHTTPError(
				errors.New("current password is incorrect"),
				http.StatusBadRequest,
			))

			return
		}

		hashedPassword, err := auth.HashPassword(*input.Password)
		if err != nil {
			h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to hash password"))

			return
		}

		user.Password = hashedPassword
	}

	if input.Name != nil {
		user.Name = input.Name
	}

	err = h.userRepo.Save(ctx, user)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to update profile"))

		return
	}

	h.responder.Write(ctx, rw, newUpdateProfileResponse())
}
