package deleteservertask

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Dispatcher interface {
	DispatchDelete(ctx context.Context, taskID uint64, nodeID uint64, version uint64) error
}

var requiredAbilities = []domain.AbilityName{domain.AbilityNameGameServerTasks}

type Handler struct {
	serverTasksRepo repositories.ServerTaskRepository
	serverFinder    *serversbase.ServerFinder
	abilityChecker  *serversbase.AbilityChecker
	dispatcher      Dispatcher
	responder       base.Responder
}

func NewHandler(
	serverTasksRepo repositories.ServerTaskRepository,
	serversRepo repositories.ServerRepository,
	rbac base.RBAC,
	dispatcher Dispatcher,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverTasksRepo: serverTasksRepo,
		serverFinder:    serversbase.NewServerFinder(serversRepo, rbac),
		abilityChecker:  serversbase.NewAbilityChecker(rbac),
		dispatcher:      dispatcher,
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

	server, err := h.serverFinder.FindUserServer(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	err = h.abilityChecker.CheckOrError(
		ctx,
		session.User.ID,
		server.ID,
		requiredAbilities,
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

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

	existing := &tasks[0]

	err = h.serverTasksRepo.SoftDelete(ctx, taskID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete server task"))

		return
	}

	if h.dispatcher != nil && existing.NodeID != nil {
		_ = h.dispatcher.DispatchDelete(
			ctx, uint64(taskID), uint64(*existing.NodeID), existing.Version+1,
		)
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (h *Handler) AuthorizeIdempotencyReplay(r *http.Request) error {
	return serversbase.AuthorizeIdempotencyReplay(r, h.serverFinder, h.abilityChecker, requiredAbilities)
}
