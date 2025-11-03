package getuserservers

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	serverRepo  repositories.ServerRepository
	gameRepo    repositories.GameRepository
	gameModRepo repositories.GameModRepository
	responder   base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	gameRepo repositories.GameRepository,
	gameModRepo repositories.GameModRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverRepo:  serverRepo,
		gameRepo:    gameRepo,
		gameModRepo: gameModRepo,
		responder:   responder,
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

	input := api.NewInputReader(r)

	userID, err := input.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid user id"),
			http.StatusBadRequest,
		))

		return
	}

	servers, err := h.serverRepo.FindUserServers(ctx, userID, nil, []filters.Sorting{
		{
			Field:     "name",
			Direction: filters.SortDirectionAsc,
		},
	}, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user servers"))

		return
	}

	gameIDs := make([]string, 0, len(servers))
	gameModIDs := make([]uint, 0, len(servers))
	for _, s := range servers {
		gameIDs = append(gameIDs, s.GameID)
		gameModIDs = append(gameModIDs, s.GameModID)
	}

	games, err := h.gameRepo.Find(ctx, &filters.FindGame{Codes: gameIDs}, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find games"))

		return
	}

	gameMods, err := h.gameModRepo.Find(ctx, &filters.FindGameMod{IDs: gameModIDs}, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find game mods"))

		return
	}

	serversResponse := newServersResponseFromServers(servers, games, gameMods)

	h.responder.Write(ctx, rw, serversResponse)
}
