package postcommand

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	rconbase "github.com/gameap/gameap/internal/api/servers/rcon/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/quercon/rcon"
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

	server, err := h.getServer(ctx, r, session.User)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if !server.IsOnline() {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("server is offline"),
			http.StatusServiceUnavailable,
		))

		return
	}

	commandInput, err := h.readCommandInput(r)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	game, err := h.findGame(ctx, server.GameID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	protocol, err := rconbase.DetermineProtocolByEngine(game.Engine)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "unsupported game engine"),
			http.StatusBadRequest,
		))

		return
	}

	if server.Rcon == nil || *server.Rcon == "" {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("rcon password not configured for server"),
			http.StatusPreconditionFailed,
		))

		return
	}

	output, err := h.executeRconCommand(ctx, server, protocol, commandInput.Command)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, newCommandResponse(output))
}

func (h *Handler) getServer(ctx context.Context, r *http.Request, user *domain.User) (*domain.Server, error) {
	input := api.NewInputReader(r)

	serverID, err := input.ReadUint("server")
	if err != nil {
		return nil, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		)
	}

	return h.serverFinder.FindUserServer(ctx, user, serverID)
}

func (h *Handler) readCommandInput(r *http.Request) (*commandRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, api.WrapHTTPError(
			errors.WithMessage(err, "failed to read request body"),
			http.StatusBadRequest,
		)
	}
	defer func() {
		err := r.Body.Close()
		if err != nil {
			slog.Warn("failed to close request body", "error", err)
		}
	}()

	var commandInput commandRequest
	if err := json.Unmarshal(body, &commandInput); err != nil {
		return nil, api.WrapHTTPError(
			errors.WithMessage(err, "failed to parse request body"),
			http.StatusBadRequest,
		)
	}

	if err := commandInput.Validate(); err != nil {
		return nil, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		)
	}

	return &commandInput, nil
}

func (h *Handler) findGame(ctx context.Context, gameID string) (*domain.Game, error) {
	games, err := h.gameRepo.Find(ctx, filters.FindGameByCodes(gameID), nil, nil)
	if err != nil {
		return nil, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find game for server"),
			http.StatusInternalServerError,
		)
	}
	if len(games) == 0 {
		return nil, api.WrapHTTPError(
			errors.New("game for server not found"),
			http.StatusInternalServerError,
		)
	}

	return &games[0], nil
}

func (h *Handler) executeRconCommand(
	ctx context.Context,
	server *domain.Server,
	protocol rcon.Protocol,
	command string,
) (string, error) {
	rconAddress := fmt.Sprintf("%s:%d", server.ServerIP, getRconPort(server))

	rconConfig := rcon.Config{
		Address:  rconAddress,
		Password: *server.Rcon,
		Protocol: protocol,
		Timeout:  10 * time.Second,
	}

	client, err := rcon.NewClient(rconConfig)
	if err != nil {
		return "", api.WrapHTTPError(
			errors.WithMessage(err, "failed to create rcon client"),
			http.StatusInternalServerError,
		)
	}

	if err := client.Open(ctx); err != nil {
		return "", api.WrapHTTPError(
			errors.WithMessage(err, "failed to connect to rcon"),
			http.StatusServiceUnavailable,
		)
	}
	defer client.Close()

	output, err := client.Execute(ctx, command)
	if err != nil {
		return "", api.WrapHTTPError(
			errors.WithMessage(err, "failed to execute rcon command"),
			http.StatusInternalServerError,
		)
	}

	return output, nil
}

func getRconPort(server *domain.Server) int {
	if server.RconPort != nil {
		return *server.RconPort
	}

	return server.ServerPort
}
