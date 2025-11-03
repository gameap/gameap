package getgames

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

type Handler struct {
	repo      repositories.GameRepository
	responder base.Responder
}

func NewHandler(repo repositories.GameRepository, responder base.Responder) *Handler {
	return &Handler{
		repo:      repo,
		responder: responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	games, err := h.repo.FindAll(ctx, []filters.Sorting{
		{
			Field:     "name",
			Direction: filters.SortDirectionAsc,
		},
	}, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find all games"))

		return
	}

	gamesResponse := newGamesResponseFromGames(games)

	h.responder.Write(ctx, rw, gamesResponse)
}
