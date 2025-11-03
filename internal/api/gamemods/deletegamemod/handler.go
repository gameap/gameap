package deletegamemod

import (
	"net/http"
	"strconv"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type Handler struct {
	repo       repositories.GameModRepository
	serverRepo repositories.ServerRepository
	responder  base.Responder
}

func NewHandler(
	repo repositories.GameModRepository,
	serverRepo repositories.ServerRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		repo:       repo,
		serverRepo: serverRepo,
		responder:  responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	idStr := vars["id"]

	if idStr == "" {
		h.responder.WriteError(ctx, rw, api.NewValidationError("game mod id is required"))

		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.NewValidationError("invalid game mod id"))

		return
	}

	if id <= 0 {
		h.responder.WriteError(ctx, rw, api.NewValidationError("invalid game mod id"))

		return
	}

	exists, err := h.serverRepo.Exists(ctx, &filters.FindServer{
		GameModIDs: []uint{uint(id)},
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check server existence"))

		return
	}

	if exists {
		h.responder.WriteError(ctx, rw, api.NewValidationError("cannot delete game mod: servers are using this game mod"))

		return
	}

	err = h.repo.Delete(ctx, uint(id))
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete game mod"))

		return
	}

	h.responder.Write(ctx, rw, base.Success)
}
