package updateservertask

import (
	"context"
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

	taskID, err := api.NewInputReader(r).ReadUint("server_task")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid task ID"),
			http.StatusBadRequest,
		))

		return
	}

	input, err := h.parseAndValidateInput(r)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	task, err := h.findServerTask(ctx, taskID, daemonSession.Node.ID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.updateTask(task, input)

	err = h.serverTaskRepo.Save(ctx, task)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to update server task"),
			http.StatusInternalServerError,
		))

		return
	}

	h.responder.Write(ctx, rw, newUpdateServerTaskResponse())
}

func (h *Handler) parseAndValidateInput(r *http.Request) (*updateServerTaskInput, error) {
	input := &updateServerTaskInput{}

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		return nil, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request"),
			http.StatusBadRequest,
		)
	}

	err = input.Validate()
	if err != nil {
		return nil, api.WrapHTTPError(
			errors.WithMessage(err, "invalid input"),
			http.StatusBadRequest,
		)
	}

	return input, nil
}

func (h *Handler) findServerTask(ctx context.Context, taskID, nodeID uint) (*domain.ServerTask, error) {
	tasks, err := h.serverTaskRepo.Find(
		ctx,
		&filters.FindServerTask{
			IDs:     []uint{taskID},
			NodeIDs: []uint{nodeID},
		},
		nil,
		nil,
	)
	if err != nil {
		return nil, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find server task"),
			http.StatusInternalServerError,
		)
	}

	if len(tasks) == 0 {
		return nil, api.WrapHTTPError(
			errors.New("server task not found"),
			http.StatusNotFound,
		)
	}

	return &tasks[0], nil
}

func (h *Handler) updateTask(task *domain.ServerTask, input *updateServerTaskInput) {
	if input.Counter != nil {
		task.Counter = *input.Counter
	} else {
		task.Counter++
	}

	if input.Repeat != nil {
		task.Repeat = *input.Repeat
	}

	if input.RepeatPeriod != nil {
		task.RepeatPeriod = time.Duration(*input.RepeatPeriod) * time.Second
	}
	if input.ExecuteDate != nil {
		task.ExecuteDate = input.ExecuteDate.Time
	}

	now := time.Now()
	task.UpdatedAt = &now
}
