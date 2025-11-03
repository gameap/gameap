package getdaemontask

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
	daemonTasksRepo repositories.DaemonTaskRepository
	responder       base.Responder
	withOutput      bool
}

func NewHandler(
	daemonTasksRepo repositories.DaemonTaskRepository,
	responder base.Responder,
	withOutput bool,
) *Handler {
	return &Handler{
		daemonTasksRepo: daemonTasksRepo,
		responder:       responder,
		withOutput:      withOutput,
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

	taskID, err := inputReader.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid task id"),
			http.StatusBadRequest,
		))

		return
	}

	filter := &filters.FindDaemonTask{
		IDs: []uint{taskID},
	}

	var tasks []domain.DaemonTask

	if h.withOutput {
		tasks, err = h.daemonTasksRepo.FindWithOutput(ctx, filter, nil, &filters.Pagination{
			Limit:  1,
			Offset: 0,
		})
	} else {
		tasks, err = h.daemonTasksRepo.Find(ctx, filter, nil, &filters.Pagination{
			Limit:  1,
			Offset: 0,
		})
	}
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find daemon task"))

		return
	}

	if len(tasks) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("daemon task not found"))

		return
	}

	response := newDaemonTaskOutputResponseFromDaemonTask(&tasks[0])

	h.responder.Write(ctx, rw, response)
}
