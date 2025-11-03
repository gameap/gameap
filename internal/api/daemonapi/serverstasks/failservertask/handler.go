package failservertask

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	serverTaskRepo     repositories.ServerTaskRepository
	serverTaskFailRepo repositories.ServerTaskFailRepository
	serverRepo         repositories.ServerRepository
	responder          base.Responder
}

func NewHandler(
	serverTaskRepo repositories.ServerTaskRepository,
	serverTaskFailRepo repositories.ServerTaskFailRepository,
	serverRepo repositories.ServerRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverTaskRepo:     serverTaskRepo,
		serverTaskFailRepo: serverTaskFailRepo,
		serverRepo:         serverRepo,
		responder:          responder,
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

	input := &failServerTaskInput{}

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

	now := time.Now()
	serverTaskFail := &domain.ServerTaskFail{
		ServerTaskID: taskID,
		Output:       input.Output,
		CreatedAt:    &now,
		UpdatedAt:    &now,
	}

	err = h.serverTaskFailRepo.Save(ctx, serverTaskFail)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to save server task fail"),
			http.StatusInternalServerError,
		))

		return
	}

	h.responder.Write(ctx, rw, newFailServerTaskResponse())
}
