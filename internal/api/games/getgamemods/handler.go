package getgamemods

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
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

	gameCode, err := api.NewInputReader(r).ReadString("code")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid game code"),
			http.StatusBadRequest,
		))

		return
	}
	if gameCode == "" {
		h.responder.WriteError(ctx, rw, api.NewValidationError("game code is required"))

		return
	}

	gameMods, err := h.repo.Find(ctx, filters.FindGameModByGameCodes(gameCode), []filters.Sorting{
		{
			Field:     "name",
			Direction: filters.SortDirectionAsc,
		},
	}, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find game mods"))

		return
	}

	h.responder.Write(ctx, rw, newGameModsResponseFromGameMods(gameMods))
}
