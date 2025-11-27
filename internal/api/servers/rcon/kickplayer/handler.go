package kickplayer

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
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
	"github.com/pkg/errors"
)

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	gameRepo       repositories.GameRepository
	responder      base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	gameRepo repositories.GameRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		gameRepo:       gameRepo,
		responder:      responder,
	}
}

//nolint:funlen
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

	inputReader := api.NewInputReader(r)

	command, err := inputReader.ReadString("command")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid command"),
			http.StatusBadRequest,
		))

		return
	}

	if command != "kick" && command != "ban" {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid command: must be 'kick' or 'ban'"),
			http.StatusNotFound,
		))

		return
	}

	serverID, err := inputReader.ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	server, err := h.serverFinder.FindUserServer(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find server"))

		return
	}

	if err = h.abilityChecker.CheckOrError(
		ctx, session.User.ID, server.ID, []domain.AbilityName{domain.AbilityNameGameServerRconPlayers},
	); err != nil {
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

	kickInput, err := h.readKickInput(r)
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

	playerManager, err := players.NewPlayerManagerByGameCode(server.GameID)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "player management not supported for this game"),
			http.StatusNotImplemented,
		))

		return
	}

	player, err := kickInput.ToPlayer()
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		))

		return
	}

	var rconCommand string
	if command == "kick" {
		rconCommand = playerManager.KickCommand(player, kickInput.Reason)
	} else {
		rconCommand = playerManager.BanCommand(
			player,
			kickInput.Reason,
			time.Duration(int64(kickInput.DurationInMinutes.Int())*int64(time.Minute)),
		)
	}

	slog.DebugContext(
		ctx,
		"Executing RCON command",
		slog.String("command", rconCommand),
		slog.String("protocol", string(protocol)),
	)

	output, err := h.executeRconCommand(ctx, server, protocol, rconCommand)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, newKickResponse(output))
}

func (h *Handler) readKickInput(r *http.Request) (*kickRequest, error) {
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

	var kickInput kickRequest
	if err := json.Unmarshal(body, &kickInput); err != nil {
		return nil, api.WrapHTTPError(
			errors.WithMessage(err, "failed to parse request body"),
			http.StatusBadRequest,
		)
	}

	if err := kickInput.Validate(); err != nil {
		return nil, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		)
	}

	return &kickInput, nil
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
	defer func(client rcon.Client) {
		err := client.Close()
		if err != nil {
			slog.Warn(
				"failed to close rcon client",
				slog.String("error", err.Error()),
			)
		}
	}(client)

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
