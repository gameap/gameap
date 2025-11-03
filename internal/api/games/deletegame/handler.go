package deletegame

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

type Handler struct {
	repo       repositories.GameRepository
	serverRepo repositories.ServerRepository
	responder  base.Responder
}

func NewHandler(
	repo repositories.GameRepository,
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

	gameCode, err := api.NewInputReader(r).ReadString("code")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.NewValidationError("invalid game code"))

		return
	}

	if gameCode == "" {
		h.responder.WriteError(ctx, rw, api.NewValidationError("game code is required"))

		return
	}

	exists, err := h.serverRepo.Exists(ctx, &filters.FindServer{
		GameIDs: []string{gameCode},
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check server existence"))

		return
	}

	if exists {
		h.responder.WriteError(ctx, rw, api.NewValidationError("cannot delete game: servers are using this game"))

		return
	}

	err = h.repo.Delete(ctx, gameCode)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete game"))

		return
	}

	h.responder.Write(ctx, rw, base.Success)
}
