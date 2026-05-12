package putservertask

import (
	"context"
	"encoding/json"
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
	DispatchUpsert(ctx context.Context, task *domain.ServerTask) error
}

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
		[]domain.AbilityName{domain.AbilityNameGameServerTasks},
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

	existingTask := &tasks[0]

	input := &serverTaskInput{}
	err = json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "validation failed"))

		return
	}

	updatedTask, err := input.ToDomain(serverID, existingTask)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		))

		return
	}

	updatedTask.ID = taskID

	err = h.serverTasksRepo.Save(ctx, updatedTask)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to update server task"))

		return
	}

	newVersion, err := h.serverTasksRepo.BumpVersion(ctx, updatedTask.ID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to bump task version"))

		return
	}
	updatedTask.Version = newVersion

	if h.dispatcher != nil {
		_ = h.dispatcher.DispatchUpsert(ctx, updatedTask)
	}

	response := newServerTaskResponseFromServerTask(updatedTask)

	h.responder.Write(ctx, rw, response)
}
