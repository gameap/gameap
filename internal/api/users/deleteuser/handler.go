package deleteuser

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
)

type Handler struct {
	userService      *services.UserService
	pluginDispatcher PluginDispatcher
	responder        base.Responder
}

// PluginDispatcher lets plugins veto a deletion (USER_PRE_DELETE) and learn
// about it afterwards (USER_DELETED); satisfied by *plugin.Dispatcher.
type PluginDispatcher interface {
	DispatchUserEvent(
		ctx context.Context,
		eventType pluginproto.EventType,
		user *domain.User,
		extraData map[string]string,
	) *pkgplugin.EventDispatchResult
	DispatchUserEventAsync(
		ctx context.Context,
		eventType pluginproto.EventType,
		user *domain.User,
		extraData map[string]string,
	)
}

func NewHandler(
	userService *services.UserService,
	pluginDispatcher PluginDispatcher,
	responder base.Responder,
) *Handler {
	return &Handler{
		userService:      userService,
		pluginDispatcher: pluginDispatcher,
		responder:        responder,
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

	user := &users[0]

	if err := h.dispatchPreDelete(ctx, user); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	err = h.userService.Delete(ctx, userID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete user"))

		return
	}

	if h.pluginDispatcher != nil {
		h.pluginDispatcher.DispatchUserEventAsync(ctx, pluginproto.EventType_EVENT_TYPE_USER_DELETED, user, nil)
	}

	rw.WriteHeader(http.StatusNoContent)
}

// dispatchPreDelete gives plugins a chance to cancel the deletion.
func (h *Handler) dispatchPreDelete(ctx context.Context, user *domain.User) error {
	if h.pluginDispatcher == nil {
		return nil
	}

	result := h.pluginDispatcher.DispatchUserEvent(ctx, pluginproto.EventType_EVENT_TYPE_USER_PRE_DELETE, user, nil)
	if result == nil || !result.Cancelled {
		return nil
	}

	msg := result.CancelMessage
	if msg == "" {
		msg = result.CancelledBy
	}

	return api.WrapHTTPError(
		errors.Wrapf(servercontrol.ErrCancelledByPlugin, "cancelled by %s: %s", result.CancelledBy, msg),
		http.StatusConflict,
	)
}
