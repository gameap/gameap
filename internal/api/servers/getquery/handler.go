package getquery

import (
	"net/http"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/quercon/query"
	"github.com/pkg/errors"
)

type Handler struct {
	serverFinder *serversbase.ServerFinder
	gameRepo     repositories.GameRepository
	responder    base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	gameRepo repositories.GameRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder: serversbase.NewServerFinder(serverRepo, rbac),
		gameRepo:     gameRepo,
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

	port := server.ServerPort
	if server.QueryPort != nil {
		port = *server.QueryPort
	}

	games, err := h.gameRepo.Find(ctx, filters.FindGameByCodes(server.GameID), nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find game for server"),
			http.StatusInternalServerError,
		))

		return
	}
	if len(games) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("game for server not found"),
			http.StatusInternalServerError,
		))

		return
	}

	game := games[0]

	queryProtocol, ok := getQueryProtocolByEngine(strings.ToLower(game.Engine))
	if !ok {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("unsupported game engine for query"),
			http.StatusBadRequest,
		))

		return
	}

	result, err := query.Query(ctx, server.ServerIP, port, queryProtocol)
	if err != nil && (result == nil || !result.Online) {
		h.responder.Write(ctx, rw, newQueryResponse(nil, server))

		return
	}

	h.responder.Write(ctx, rw, newQueryResponse(result, server))
}
