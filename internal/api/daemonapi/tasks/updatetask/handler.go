package updatetask

import (
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	daemonTaskRepo repositories.DaemonTaskRepository
	responder      base.Responder
}

func NewHandler(
	daemonTaskRepo repositories.DaemonTaskRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		daemonTaskRepo: daemonTaskRepo,
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

	taskID, err := api.NewInputReader(r).ReadUint("gdaemon_task")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid task ID"),
			http.StatusBadRequest,
		))

		return
	}

	input := &updateTaskInput{}

	err = json.NewDecoder(r.Body).Decode(&input)
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

	filter := &filters.FindDaemonTask{
		IDs:                []uint{taskID},
		DedicatedServerIDs: []uint{node.ID},
	}

	tasks, err := h.daemonTaskRepo.Find(ctx, filter, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find daemon task"),
			http.StatusInternalServerError,
		))

		return
	}

	if len(tasks) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("daemon task not found"),
			http.StatusNotFound,
		))

		return
	}

	task := &tasks[0]

	task.Status = input.ToStatus()

	err = h.daemonTaskRepo.Save(ctx, task)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to update daemon task"),
			http.StatusInternalServerError,
		))

		return
	}

	h.responder.Write(ctx, rw, newUpdateTaskResponse())
}
