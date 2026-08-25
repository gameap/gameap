package postusers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
)

// PluginDispatcher publishes user events to plugins; satisfied by
// *plugin.Dispatcher.
type PluginDispatcher interface {
	DispatchUserEventAsync(
		ctx context.Context,
		eventType pluginproto.EventType,
		user *domain.User,
		extraData map[string]string,
	)
}

type Handler struct {
	usersRepo        repositories.UserRepository
	serversRepo      repositories.ServerRepository
	rbac             base.RBAC
	tm               base.TransactionManager
	pluginDispatcher PluginDispatcher
	responder        base.Responder
}

func NewHandler(
	usersRepo repositories.UserRepository,
	serversRepo repositories.ServerRepository,
	rbac base.RBAC,
	tm base.TransactionManager,
	pluginDispatcher PluginDispatcher,
	responder base.Responder,
) *Handler {
	return &Handler{
		usersRepo:        usersRepo,
		rbac:             rbac,
		serversRepo:      serversRepo,
		tm:               tm,
		pluginDispatcher: pluginDispatcher,
		responder:        responder,
	}
}

func (h *Handler) checkUserExistence(ctx context.Context, input *createUserInput) error {
	existingUsers, err := h.usersRepo.Find(ctx, &filters.FindUser{
		Logins: []string{input.Login},
	}, nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		return errors.WithMessage(err, "failed to check existing user")
	}

	if len(existingUsers) > 0 {
		return api.WrapHTTPError(
			errors.New("user with this login already exists"),
			http.StatusConflict,
		)
	}

	existingUsers, err = h.usersRepo.Find(ctx, &filters.FindUser{
		Emails: []string{input.Email},
	}, nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		return errors.WithMessage(err, "failed to check existing user")
	}

	if len(existingUsers) > 0 {
		return api.WrapHTTPError(
			errors.New("user with this email already exists"),
			http.StatusConflict,
		)
	}

	return nil
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

	input := &createUserInput{}

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

	if err := h.checkUserExistence(ctx, input); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	user, err := input.ToDomain()
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to create user domain model"))

		return
	}

	err = h.tm.Do(ctx, func(ctx context.Context) error {
		err = h.usersRepo.Save(ctx, user)
		if err != nil {
			return errors.WithMessage(err, "failed to save user")
		}

		err = h.serversRepo.SetUserServers(ctx, user.ID, input.ServerIDs())
		if err != nil {
			return errors.WithMessage(err, "failed to assign servers to user")
		}

		err = h.rbac.SetRolesToUser(ctx, user.ID, input.Roles)
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

	if h.pluginDispatcher != nil {
		h.pluginDispatcher.DispatchUserEventAsync(ctx, pluginproto.EventType_EVENT_TYPE_USER_CREATED, user, nil)
	}

	roles, err := h.rbac.GetRoles(ctx, user.ID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to get user roles"))

		return
	}

	rw.WriteHeader(http.StatusCreated)
	h.responder.Write(ctx, rw, newUserResponseFromUser(user, roles))
}
