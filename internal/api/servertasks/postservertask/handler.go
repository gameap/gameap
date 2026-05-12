package postservertask

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

// Dispatcher signals the daemon when a task is created. The HTTP handler
// is happy with a write-only contract; the concrete implementation lives
// in `internal/services/servertaskdispatcher`.
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

	serverTask, err := input.ToDomain(serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		))

		return
	}

	nodeID := server.DSID
	serverTask.NodeID = &nodeID
	uid := session.User.ID
	serverTask.CreatedByUserID = &uid

	err = h.serverTasksRepo.Save(ctx, serverTask)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save server task"))

		return
	}

	if h.dispatcher != nil {
		if dispatchErr := h.dispatcher.DispatchUpsert(ctx, serverTask); dispatchErr != nil {
			// Persisted successfully; dispatch failures are logged inside
			// the dispatcher and recovered via daemon resync. Do not
			// surface them to the HTTP caller.
			_ = dispatchErr
		}
	}

	response := newServerTaskResponseFromServerTask(serverTask)

	rw.WriteHeader(http.StatusCreated)
	h.responder.Write(ctx, rw, response)
}
