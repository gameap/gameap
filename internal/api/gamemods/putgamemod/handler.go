package putgamemod

import (
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	gmBase "github.com/gameap/gameap/internal/api/gamemods/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

var ErrGameModNotFound = api.NewNotFoundError("game mod not found")

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

	id, err := api.NewInputReader(r).ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid game mod id"),
			http.StatusBadRequest,
		))

		return
	}

	input := &updateGameModInput{}

	err = json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "invalid request"))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "invalid input"))

		return
	}

	gameMods, err := h.repo.Find(ctx, &filters.FindGameMod{
		IDs: []uint{id},
	}, nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find game mod"))

		return
	}

	if len(gameMods) == 0 {
		h.responder.WriteError(ctx, rw, ErrGameModNotFound)

		return
	}

	gameMod := &gameMods[0]

	input.Apply(gameMod)

	err = h.repo.Save(ctx, gameMod)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to update game mod"))

		return
	}

	h.responder.Write(ctx, rw, gmBase.NewGameModResponseFromGameMod(gameMod))
}
