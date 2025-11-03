package gettask

import (
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

	filter := parseFilters(r)
	filter.DedicatedServerIDs = []uint{node.ID}

	tasks, err := h.daemonTaskRepo.Find(ctx, filter, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find daemon tasks"),
			http.StatusInternalServerError,
		))

		return
	}

	response := make([]TaskResponse, 0, len(tasks))
	for i := range tasks {
		response = append(response, newTaskResponse(&tasks[i]))
	}

	h.responder.Write(ctx, rw, response)
}

func parseFilters(r *http.Request) *filters.FindDaemonTask {
	query := r.URL.Query()

	filter := &filters.FindDaemonTask{}

	if statusFilter := query.Get("filter[status]"); statusFilter != "" {
		filter.Statuses = []domain.DaemonTaskStatus{domain.DaemonTaskStatus(statusFilter)}
	}

	return filter
}
