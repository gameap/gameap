package deleteservertask

import (
	"context"
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
	serverTasksRepo repositories.ServerTaskRepository
	serversRepo     repositories.ServerRepository
	rbac            base.RBAC
	responder       base.Responder
}

func NewHandler(
	serverTasksRepo repositories.ServerTaskRepository,
	serversRepo repositories.ServerRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverTasksRepo: serverTasksRepo,
		serversRepo:     serversRepo,
		rbac:            rbac,
		responder:       responder,
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

	inputReader := api.NewInputReader(r)

	serverID, err := inputReader.ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	taskID, err := inputReader.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid task id"),
			http.StatusBadRequest,
		))

		return
	}

	hasAccess, err := h.hasServerAccess(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check server access"))

		return
	}
	if !hasAccess {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("access to server denied"),
			http.StatusForbidden,
		))

		return
	}

	// Verify the task belongs to the server
	tasks, err := h.serverTasksRepo.Find(
		ctx,
		&filters.FindServerTask{
			IDs:        []uint{taskID},
			ServersIDs: []uint{serverID},
		},
		nil,
		nil,
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find server task"))

		return
	}

	if len(tasks) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("server task not found"),
			http.StatusNotFound,
		))

		return
	}

	err = h.serverTasksRepo.Delete(ctx, taskID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete server task"))

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (h *Handler) hasServerAccess(ctx context.Context, user *domain.User, serverID uint) (bool, error) {
	isAdmin, err := h.rbac.Can(
		ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions},
	)
	if err != nil {
		return false, errors.WithMessage(err, "failed to check user permissions")
	}

	if isAdmin {
		return true, nil
	}

	servers, err := h.serversRepo.FindUserServers(ctx, user.ID, filters.FindServerByIDs(serverID), nil, nil)
	if err != nil {
		return false, errors.WithMessage(err, "failed to check server access")
	}

	if len(servers) == 0 {
		return false, nil
	}

	canControlTasks, err := h.rbac.CanForEntity(
		ctx,
		user.ID,
		domain.EntityTypeServer,
		serverID,
		[]domain.AbilityName{domain.AbilityNameGameServerTasks},
	)
	if err != nil {
		return false, errors.WithMessage(err, "failed to check user permissions")
	}

	return canControlTasks, nil
}
