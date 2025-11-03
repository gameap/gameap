package getfastrcon

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	serverFinder *serversbase.ServerFinder
	gameModRepo  repositories.GameModRepository
	responder    base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	gameModRepo repositories.GameModRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder: serversbase.NewServerFinder(serverRepo, rbac),
		gameModRepo:  gameModRepo,
		responder:    responder,
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

	serverID, err := input.ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	server, err := h.serverFinder.FindUserServer(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	gameMods, err := h.gameModRepo.Find(ctx, &filters.FindGameMod{
		IDs: []uint{server.GameModID},
	}, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find game mod for server"),
			http.StatusInternalServerError,
		))

		return
	}
	if len(gameMods) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("game mod for server not found"),
			http.StatusNotFound,
		))

		return
	}

	gameMod := gameMods[0]

	h.responder.Write(ctx, rw, newFastRconResponse(gameMod.FastRcon))
}
