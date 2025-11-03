package getgamemods

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	gmBase "github.com/gameap/gameap/internal/api/gamemods/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
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

	gameMods, err := h.repo.FindAll(ctx, []filters.Sorting{
		{
			Field:     "name",
			Direction: filters.SortDirectionAsc,
		},
	}, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find all game mods"))

		return
	}

	gameModsResponse := gmBase.NewGameModsResponseFromGameMods(gameMods)

	h.responder.Write(ctx, rw, gameModsResponse)
}
