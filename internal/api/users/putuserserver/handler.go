package putuserserver

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
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
	responder        base.Responder
	audit            audit.Logger
	pluginDispatcher PluginDispatcher
}

func NewHandler(
	usersRepo repositories.UserRepository,
	serversRepo repositories.ServerRepository,
	rbac base.RBAC,
	responder base.Responder,
	auditLogger audit.Logger,
	pluginDispatcher PluginDispatcher,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		usersRepo:        usersRepo,
		serversRepo:      serversRepo,
		rbac:             rbac,
		responder:        responder,
		audit:            auditLogger,
		pluginDispatcher: pluginDispatcher,
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

	serverID, err := input.ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
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

	if err := base.EnsureTargetNotAdminForToken(ctx, h.rbac, user.ID); err != nil {
		if errors.Is(err, base.ErrTokenCannotModifyAdmin) {
			audit.AccessDenied(ctx, h.audit, "user",
				strconv.FormatUint(uint64(user.ID), 10), "token_target_is_admin")
		}
		h.responder.WriteError(ctx, rw, err)

		return
	}

	serverExists, err := h.serversRepo.Exists(ctx, &filters.FindServer{IDs: []uint{serverID}})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check server existence"))

		return
	}
	if !serverExists {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("server not found"))

		return
	}

	err = h.serversRepo.AttachUserServer(ctx, userID, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to attach server to user"))

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventUserServerAttach, audit.CategoryAdminOp,
		"user", strconv.FormatUint(uint64(user.ID), 10), "server_attach",
		slog.Uint64("server_id", uint64(serverID)))

	if h.pluginDispatcher != nil {
		h.pluginDispatcher.DispatchUserEventAsync(ctx, pluginproto.EventType_EVENT_TYPE_USER_UPDATED, user,
			map[string]string{"changed_fields": "servers"})
	}

	rw.WriteHeader(http.StatusNoContent)
}
