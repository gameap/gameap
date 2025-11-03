package getserverstasks

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

	tasks, err := h.serverTaskRepo.Find(
		ctx,
		&filters.FindServerTask{
			NodeIDs: []uint{node.ID},
		},
		nil,
		nil,
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find server tasks"),
			http.StatusInternalServerError,
		))

		return
	}

	response := make([]ServerTaskResponse, 0, len(tasks))
	for i := range tasks {
		response = append(response, newServerTaskResponse(&tasks[i]))
	}

	h.responder.Write(ctx, rw, response)
}
