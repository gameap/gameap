package searchservers

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

type Handler struct {
	serverRepo repositories.ServerRepository
	gameRepo   repositories.GameRepository
	responder  base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	gameRepo repositories.GameRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverRepo: serverRepo,
		gameRepo:   gameRepo,
		responder:  responder,
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

	q, err := readInput(r)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to read input"),
			http.StatusBadRequest,
		))

		return
	}

	servers, err := h.serverRepo.Search(ctx, q)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to search servers"))

		return
	}

	gameCodes := lo.Uniq(lo.Map(servers, func(s *domain.Server, _ int) string {
		return s.GameID
	}))

	gameList, err := h.gameRepo.Find(ctx, &filters.FindGame{
		Codes: gameCodes,
	}, nil, nil)

	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find games"))

		return
	}

	games := make(map[string]domain.Game, len(gameList))
	for _, game := range gameList {
		games[game.Code] = game
	}

	serversResponse := newSearchServersResponseFromServers(servers, games)

	h.responder.Write(ctx, rw, serversResponse)
}
