package getgamemod

import (
	"net/http"
	"strconv"

	"github.com/gameap/gameap/internal/api/base"
	gmBase "github.com/gameap/gameap/internal/api/gamemods/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type Handler struct {
	repo      repositories.GameModRepository
	responder base.Responder
}

func NewHandler(repo repositories.GameModRepository, responder base.Responder) *Handler {
	return &Handler{
		repo:      repo,
		responder: responder,
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
		h.responder.WriteError(ctx, rw, api.NewError(http.StatusBadRequest, "invalid game mod id"))

		return
	}
	if id <= 0 {
		h.responder.WriteError(ctx, rw, api.NewValidationError("game mod id must be greater than zero"))
	}

	gameMods, err := h.repo.Find(ctx, &filters.FindGameMod{
		IDs: []uint{uint(id)},
	}, nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find game mod"))

		return
	}

	if len(gameMods) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("game mod not found"))

		return
	}

	h.responder.Write(ctx, rw, gmBase.NewGameModResponseFromGameMod(&gameMods[0]))
}
