package putuser

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	usersRepo   repositories.UserRepository
	serversRepo repositories.ServerRepository
	rbac        base.RBAC
	tm          base.TransactionManager
	responder   base.Responder
}

func NewHandler(
	usersRepo repositories.UserRepository,
	serversRepo repositories.ServerRepository,
	rbac base.RBAC,
	tm base.TransactionManager,
	responder base.Responder,
) *Handler {
	return &Handler{
		usersRepo:   usersRepo,
		serversRepo: serversRepo,
		rbac:        rbac,
		tm:          tm,
		responder:   responder,
	}
}

//nolint:funlen
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

	updateInput := &updateUserInput{}

	err = json.NewDecoder(r.Body).Decode(&updateInput)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request"),
			http.StatusBadRequest,
		))

		return
	}

	err = updateInput.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid input"),
			http.StatusBadRequest,
		))

		return
	}

	users, err := h.usersRepo.Find(ctx, &filters.FindUser{
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

	user := &users[0]

	err = updateInput.Apply(user)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to apply input"))

		return
	}

	err = h.tm.Do(ctx, func(ctx context.Context) error {
		err = h.usersRepo.Save(ctx, user)
		if err != nil {
			return errors.WithMessage(err, "failed to save user")
		}

		err = h.serversRepo.SetUserServers(ctx, user.ID, updateInput.ServerIDs())
		if err != nil {
			return errors.WithMessage(err, "failed to assign servers to user")
		}

		err = h.rbac.SetRolesToUser(ctx, user.ID, updateInput.Roles)
		if err != nil {
			var errInvalidRole rbac.InvalidRoleNameError
			if errors.As(err, &errInvalidRole) {
				return api.WrapHTTPError(err, http.StatusUnprocessableEntity)
			}

			return errors.WithMessage(err, "failed to assign roles to user")
		}

		return nil
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	roleNames, err := h.rbac.GetRoles(ctx, user.ID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to get user roles"))

		return
	}

	h.responder.Write(ctx, rw, newUserResponseFromUser(user, roleNames))
}
