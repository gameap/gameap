package console

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
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
	"github.com/pkg/errors"
)

const (
	typeConsoleOutput  = "console.output"
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

type fileService interface {
	Download(ctx context.Context, node *domain.Node, filePath string) ([]byte, error)
	Upload(
		ctx context.Context, node *domain.Node, filePath string,
		content []byte, perms os.FileMode, owner daemon.OwnerOptions,
	) error
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
	fileService       fileService
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
	fileService fileService,
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
		fileService:       fileService,
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

	conn, err := ws.Accept(rw, r, &websocket.AcceptOptions{
		OriginPatterns: h.originPatterns,
	})
	if err != nil {
		h.logger.Warn("websocket accept failed", "error", err)

		return
	}

	consoleTopic := ws.ChannelToTopic(channels.BuildRealtimeConsoleOutputChannel(uint64(serverID)))

	canSend := h.canSendCommands(ctx, s.User, server)

	if h.registry.IsConnected(uint64(server.DSID)) {
		h.runGRPCMode(ctx, conn, server, node, consoleTopic, s.User, canSend)
	} else {
		h.runLegacyMode(ctx, conn, server, node, consoleTopic, s.User, canSend)
	}
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
	defer cleanup()

	h.hub.Register(client, consoleTopic)

	h.sendConsoleHistory(ctx, client, server, node)

	client.Run()
}

func (h *Handler) runLegacyMode(
	ctx context.Context,
	conn *websocket.Conn,
	server *domain.Server,
	node *domain.Node,
	consoleTopic string,
	user *domain.User,
	canSend bool,
) {
	client := ws.NewClient(ctx, conn, h.hub, nil, h.logger)
	msgHandler := h.newLegacyMessageHandler(ctx, client, server, node, user, canSend)
	client.SetMessageHandler(msgHandler)
	h.hub.Register(client, consoleTopic)

	h.sendConsoleHistory(ctx, client, server, node)

	poller := newLegacyPoller(client, h.fileService, node, server.Dir, h.logger)
	go poller.run(ctx)

	client.Run()
}

func (h *Handler) sendConsoleHistory(ctx context.Context, client *ws.Client, server *domain.Server, node *domain.Node) {
	output, err := h.getConsoleLog(ctx, server, node)
	if err != nil {
		h.logger.Warn("failed to load console history", "server_id", server.ID, "error", err)

		return
	}

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

	if h.registry.IsConnected(uint64(node.ID)) {
		return "", nil
	}

	return h.downloadOutputFile(ctx, node, server.Dir)
}

func (h *Handler) downloadOutputFile(ctx context.Context, node *domain.Node, serverDir string) (string, error) {
	outputPath := serverDir + "/output.txt"

	content, err := h.fileService.Download(ctx, node, outputPath)
	if err != nil {
		return "", errors.WithMessage(err, "failed to download console log")
	}

	result := string(content)

	const maxSymbols = 65536
	if len(result) > maxSymbols {
		result = result[len(result)-maxSymbols:]
	}

	return result, nil
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

type consoleOutputPayload struct {
	Chunk string `json:"chunk"`
}

type consoleCommandPayload struct {
	Command string `json:"command"`
}
