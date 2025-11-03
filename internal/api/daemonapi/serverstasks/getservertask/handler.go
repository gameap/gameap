package getservertask

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	serverTaskRepo repositories.ServerTaskRepository
	serverRepo     repositories.ServerRepository
	responder      base.Responder
}

func NewHandler(
	serverTaskRepo repositories.ServerTaskRepository,
	serverRepo repositories.ServerRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverTaskRepo: serverTaskRepo,
		serverRepo:     serverRepo,
		responder:      responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	daemonSession := auth.DaemonSessionFromContext(ctx)
	if daemonSession == nil || daemonSession.Node == nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("daemon session not found"),
			http.StatusUnauthorized,
		))

		return
	}

	node := daemonSession.Node

	taskID, err := api.NewInputReader(r).ReadUint("server_task")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid task ID"),
			http.StatusBadRequest,
		))

		return
	}

	tasks, err := h.serverTaskRepo.Find(
		ctx,
		&filters.FindServerTask{
			IDs:     []uint{taskID},
			NodeIDs: []uint{node.ID},
		},
		nil,
		nil,
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find server task"),
			http.StatusInternalServerError,
		))

		return
	}

	if len(tasks) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("server task not found"),
			http.StatusNotFound,
		))

		return
	}

	task := &tasks[0]

	response := newServerTaskResponse(task)

	h.responder.Write(ctx, rw, response)
}
