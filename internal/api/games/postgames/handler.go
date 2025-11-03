package postgames

import (
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

var ErrGameAlreadyExists = api.NewError(http.StatusConflict, "game with this code already exists")

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

	input := &createGameInput{}

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "invalid request"))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "invalid input"))

		return
	}

	existingGames, err := h.repo.Find(ctx, filters.FindGameByCodes(input.Code), nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check game existence"))

		return
	}

	if len(existingGames) > 0 {
		h.responder.WriteError(ctx, rw, ErrGameAlreadyExists)

		return
	}

	game := input.ToDomain()

	err = h.repo.Save(ctx, game)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save game"))

		return
	}

	h.responder.Write(ctx, rw, base.Success)
}
