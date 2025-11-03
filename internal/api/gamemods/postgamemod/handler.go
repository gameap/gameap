package postgamemod

import (
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
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

	input := &gameModInput{}

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

	gameMod := input.ToDomain()

	err = h.repo.Save(ctx, gameMod)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save game mod"))

		return
	}

	h.responder.Write(ctx, rw, base.Success)
}
