package console

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	wsbase "github.com/gameap/gameap/internal/api/ws/base"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/grpc/handlers"
	"github.com/gameap/gameap/internal/grpc/session"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/secretmask"
	"github.com/pkg/errors"
)

const (
	typeConsoleHistory = "console.history"
	typeConsoleCommand = "console.command"
)

type daemonCommands interface {
	ExecuteCommand(
		ctx context.Context,
		node *domain.Node,
		command string,
		opts ...daemon.CommandServiceOption,
	) (*daemon.CommandResult, error)
}

type consoleLogService interface {
	GetConsoleLog(ctx context.Context, nodeID uint64, serverID uint64, maxBytes int64) (string, error)
}

type Handler struct {
	serverFinder      *serversbase.ServerFinder
	abilityChecker    *serversbase.AbilityChecker
	nodeRepo          repositories.NodeRepository
	hub               *ws.Hub
	originPatterns    []string
	registry          *session.Registry
	commandHandler    *handlers.CommandHandler
	daemonCommands    daemonCommands
	consoleLogService consoleLogService
	responder         base.Responder
	logger            *slog.Logger
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	hub *ws.Hub,
	originPatterns []string,
	registry *session.Registry,
	commandHandler *handlers.CommandHandler,
	daemonCommands daemonCommands,
	cls consoleLogService,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder:      serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker:    serversbase.NewAbilityChecker(rbac),
		nodeRepo:          nodeRepo,
		hub:               hub,
		originPatterns:    originPatterns,
		registry:          registry,
		commandHandler:    commandHandler,
		daemonCommands:    daemonCommands,
		consoleLogService: cls,
		responder:         responder,
		logger:            slog.Default(),
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	s := auth.SessionFromContext(ctx)
	if !s.IsAuthenticated() {
		h.responder.WriteError(ctx, rw, api.NewError(http.StatusUnauthorized, "user not authenticated"))

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

	server, err := h.serverFinder.FindUserServer(ctx, s.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if err = h.abilityChecker.CheckOrError(
		ctx,
		s.User.ID,
		server.ID,
		[]domain.AbilityName{domain.AbilityNameGameServerConsoleView},
	); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	node, err := h.findNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if !h.registry.IsConnected(uint64(server.DSID)) {
		h.responder.WriteError(ctx, rw, api.NewError(
			http.StatusServiceUnavailable,
			"daemon is not connected via grpc",
		))

		return
	}

	conn, err := ws.Accept(rw, r, &websocket.AcceptOptions{
		OriginPatterns: h.originPatterns,
	})
	if err != nil {
		h.logger.Warn("websocket accept failed", "error", err)

		return
	}

	consoleTopic := ws.ChannelToTopic(channels.BuildRealtimeConsoleOutputChannel(uint64(serverID)))

	canSend := h.canSendCommands(ctx, s.User, server)

	h.runGRPCMode(ctx, conn, server, node, consoleTopic, s.User, canSend)
}

func (h *Handler) runGRPCMode(
	ctx context.Context,
	conn *websocket.Conn,
	server *domain.Server,
	node *domain.Node,
	consoleTopic string,
	user *domain.User,
	canSend bool,
) {
	client := ws.NewClient(ctx, conn, h.hub, nil, h.logger)
	msgHandler, cleanup := h.newGRPCMessageHandler(ctx, client, server, node, user, canSend)
	client.SetMessageHandler(msgHandler)

	// The game server start command embeds the RCON password, so it shows up in the console
	// stream. Installed before Register so no broadcast can reach the peer unfiltered.
	masker := secretmask.New(server.RconPassword())
	client.SetOutboundFilter(wsbase.NewOutboundMaskFilter(masker))

	defer cleanup()

	h.hub.Register(client, consoleTopic)

	h.sendConsoleHistory(ctx, client, server, node, masker)

	client.Run()
}

func (h *Handler) sendConsoleHistory(
	ctx context.Context,
	client *ws.Client,
	server *domain.Server,
	node *domain.Node,
	masker *secretmask.Masker,
) {
	output, err := h.getConsoleLog(ctx, server, node)
	if err != nil {
		h.logger.Warn("failed to load console history", "server_id", server.ID, "error", err)

		return
	}

	// Masked here rather than left to the outbound filter: the filter works on the encoded
	// frame, where a password carrying JSON-escaped characters would no longer match.
	output = masker.String(output)

	if output != "" {
		client.SendMessage(ws.NewOutboundMessage(typeConsoleHistory, consoleHistoryPayload{
			Output: output,
		}))
	}
}

func (h *Handler) getConsoleLog(ctx context.Context, server *domain.Server, node *domain.Node) (string, error) {
	if h.consoleLogService != nil {
		output, err := h.consoleLogService.GetConsoleLog(ctx, uint64(node.ID), uint64(server.ID), 0)
		if err == nil {
			return output, nil
		}

		h.logger.Debug("console log service unavailable, falling back",
			"server_id", server.ID, "error", err,
		)
	}

	if node.ScriptGetConsole != nil && *node.ScriptGetConsole != "" {
		cmd := server.ReplaceServerShortcodes(node, *node.ScriptGetConsole, nil)

		result, err := h.daemonCommands.ExecuteCommand(ctx, node, cmd)
		if err != nil {
			return "", errors.WithMessage(err, "failed to execute get console script")
		}

		return result.Output, nil
	}

	return "", nil
}

func (h *Handler) findNode(ctx context.Context, nodeID uint) (*domain.Node, error) {
	nodes, err := h.nodeRepo.Find(ctx, &filters.FindNode{
		IDs: []uint{nodeID},
	}, nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find node")
	}

	if len(nodes) == 0 {
		return nil, api.NewNotFoundError("node not found")
	}

	return &nodes[0], nil
}

func (h *Handler) canSendCommands(ctx context.Context, user *domain.User, server *domain.Server) bool {
	err := h.abilityChecker.CheckOrError(
		ctx,
		user.ID,
		server.ID,
		[]domain.AbilityName{domain.AbilityNameGameServerConsoleSend},
	)

	return err == nil
}

type consoleHistoryPayload struct {
	Output string `json:"output"`
}

type consoleCommandPayload struct {
	Command string `json:"command"`
}
