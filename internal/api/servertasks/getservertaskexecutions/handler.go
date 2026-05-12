package getservertaskexecutions

import (
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

// Handler returns the audit log of executions for one scheduled task,
// ordered by started_at DESC. Optional `status` query filters the set.
type Handler struct {
	serverTaskRepo repositories.ServerTaskRepository
	executionRepo  repositories.ServerTaskExecutionRepository
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	rbac           base.RBAC
	responder      base.Responder
}

func NewHandler(
	serverTaskRepo repositories.ServerTaskRepository,
	executionRepo repositories.ServerTaskExecutionRepository,
	serversRepo repositories.ServerRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverTaskRepo: serverTaskRepo,
		executionRepo:  executionRepo,
		serverFinder:   serversbase.NewServerFinder(serversRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		rbac:           rbac,
		responder:      responder,
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

	if err = h.abilityChecker.CheckOrError(
		ctx, session.User.ID, server.ID,
		[]domain.AbilityName{domain.AbilityNameGameServerTasks},
	); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	isAdmin, err := h.rbac.Can(
		ctx, session.User.ID,
		[]domain.AbilityName{domain.AbilityNameAdminRolesPermissions},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check admin permissions"))

		return
	}

	tasks, err := h.serverTaskRepo.Find(ctx, &filters.FindServerTask{
		IDs:        []uint{taskID},
		ServersIDs: []uint{serverID},
		// allow listing executions for soft-deleted tasks so the audit
		// trail does not vanish the moment the task is removed.
		IncludeDeleted: true,
	}, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "find task"))

		return
	}
	if len(tasks) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("server task not found"), http.StatusNotFound,
		))

		return
	}

	filter := &filters.FindServerTaskExecution{
		ServerTaskIDs: []uint{taskID},
	}

	if statusParam := r.URL.Query().Get("status"); statusParam != "" {
		filter.Statuses = []domain.ServerTaskExecutionStatus{
			domain.ServerTaskExecutionStatus(statusParam),
		}
	}

	executions, err := h.executionRepo.Find(ctx, filter, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "load executions"))

		return
	}

	h.responder.Write(ctx, rw, newExecutionsResponse(executions, isAdmin))
}
