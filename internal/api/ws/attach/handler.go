package attach

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
	"github.com/gameap/gameap/pkg/idgen"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/gameap/gameap/pkg/shellescape"
	"github.com/pkg/errors"
)

const (
	typeAttachInput   = "attach.input"
	typeAttachOutput  = "attach.output"
	typeAttachDetach  = "attach.detach"
	typeAttachStarted = "attach.started"

	legacyPollInterval = 500 * time.Millisecond
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

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	nodeRepo       repositories.NodeRepository
	hub            *ws.Hub
	originPatterns []string
	registry       *session.Registry
	attachHandler  *handlers.AttachHandler
	daemonCommands daemonCommands
	fileService    fileService
	responder      base.Responder
	logger         *slog.Logger
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	hub *ws.Hub,
	originPatterns []string,
	registry *session.Registry,
	attachHandler *handlers.AttachHandler,
	daemonCommands daemonCommands,
	fileService fileService,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		nodeRepo:       nodeRepo,
		hub:            hub,
		originPatterns: originPatterns,
		registry:       registry,
		attachHandler:  attachHandler,
		daemonCommands: daemonCommands,
		fileService:    fileService,
		responder:      responder,
		logger:         slog.Default(),
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sess := auth.SessionFromContext(ctx)
	if !sess.IsAuthenticated() {
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

	server, err := h.serverFinder.FindUserServer(ctx, sess.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if err = h.abilityChecker.CheckOrError(
		ctx,
		sess.User.ID,
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

	nodeID := uint64(server.DSID)

	conn, err := ws.Accept(rw, r, &websocket.AcceptOptions{
		OriginPatterns: h.originPatterns,
	})
	if err != nil {
		h.logger.Warn("websocket accept failed", "error", err)

		return
	}

	canSend := h.canSendCommands(ctx, sess.User, server)

	if h.registry.IsConnectedAnywhere(nodeID) {
		sessionID := idgen.New()
		h.runAttachSession(ctx, conn, server, node, sessionID, sess.User, canSend)
	} else {
		h.runLegacyMode(ctx, conn, server, node, sess.User, canSend)
	}
}

func (h *Handler) runAttachSession(
	ctx context.Context,
	conn *websocket.Conn,
	server *domain.Server,
	node *domain.Node,
	sessionID string,
	user *domain.User,
	canSend bool,
) {
	client := ws.NewClient(ctx, conn, h.hub, nil, h.logger)

	startedTopic := ws.ChannelToTopic(channels.BuildRealtimeAttachStartedChannel(sessionID))
	outputTopic := ws.ChannelToTopic(channels.BuildRealtimeAttachOutputChannel(sessionID))
	closedTopic := ws.ChannelToTopic(channels.BuildRealtimeAttachClosedChannel(sessionID))

	h.hub.Register(client, startedTopic, outputTopic, closedTopic)
	h.attachHandler.TrackAttachSession(sessionID, uint64(server.ID))

	nodeID := uint64(node.ID)

	msgHandler := h.newMessageHandler(ctx, client, server, node, user, sessionID, canSend)
	client.SetMessageHandler(msgHandler)

	if err := h.registry.SendAttachRequest(ctx, nodeID, &proto.AttachRequest{
		SessionId: sessionID,
		ServerId:  uint64(server.ID),
	}); err != nil {
		h.logger.Warn("failed to send attach request",
			"server_id", server.ID,
			"session_id", sessionID,
			"error", err,
		)
		client.SendMessage(ws.NewErrorMessage("failed to attach to server console"))
	}

	client.Run()

	_ = h.registry.SendAttachDetach(context.Background(), nodeID, &proto.AttachDetach{
		SessionId: sessionID,
		Reason:    "client disconnected",
	})
	h.attachHandler.UntrackAttachSession(sessionID)
}

func (h *Handler) newMessageHandler(
	ctx context.Context,
	client *ws.Client,
	server *domain.Server,
	node *domain.Node,
	user *domain.User,
	sessionID string,
	canSend bool,
) ws.MessageHandler {
	nodeID := uint64(node.ID)

	return func(_ context.Context, msg *ws.InboundMessage) {
		switch msg.Type {
		case typeAttachInput:
			if !canSend {
				client.SendMessage(ws.NewErrorMessage("permission denied: cannot send input"))

				return
			}

			if err := h.abilityChecker.CheckOrError(
				ctx,
				user.ID,
				server.ID,
				[]domain.AbilityName{domain.AbilityNameGameServerConsoleSend},
			); err != nil {
				client.SendMessage(ws.NewErrorMessage("permission denied: cannot send input"))

				return
			}

			var payload attachInputPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				return
			}

			if err := h.registry.SendAttachInput(ctx, nodeID, &proto.AttachInput{
				SessionId: sessionID,
				Data:      payload.Data,
			}); err != nil {
				h.logger.Warn("failed to send attach input",
					"session_id", sessionID,
					"error", err,
				)
			}

		case typeAttachDetach:
			_ = h.registry.SendAttachDetach(ctx, nodeID, &proto.AttachDetach{
				SessionId: sessionID,
				Reason:    "user detached",
			})
			client.Close()
		}
	}
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

type attachInputPayload struct {
	Data []byte `json:"data"`
}

type attachStartedPayload struct {
	SessionID string `json:"session_id"`
	ServerID  uint64 `json:"server_id"`
}

type attachOutputPayload struct {
	Data []byte `json:"data"`
}

// Legacy mode for daemons not connected via gRPC.

func (h *Handler) runLegacyMode(
	ctx context.Context,
	conn *websocket.Conn,
	server *domain.Server,
	node *domain.Node,
	user *domain.User,
	canSend bool,
) {
	client := ws.NewClient(ctx, conn, h.hub, nil, h.logger)

	msgHandler := h.newLegacyMessageHandler(ctx, client, server, node, user, canSend)
	client.SetMessageHandler(msgHandler)

	client.SendMessage(ws.NewOutboundMessage(typeAttachStarted, attachStartedPayload{
		SessionID: idgen.New(),
		ServerID:  uint64(server.ID),
	}))

	lastContent := h.sendLegacyHistory(ctx, client, server, node)

	poller := &legacyAttachPoller{
		client:      client,
		fileService: h.fileService,
		node:        node,
		serverDir:   server.Dir,
		logger:      h.logger,
		lastContent: lastContent,
	}
	go poller.run(ctx)

	client.Run()
}

func (h *Handler) newLegacyMessageHandler(
	ctx context.Context,
	client *ws.Client,
	server *domain.Server,
	node *domain.Node,
	user *domain.User,
	canSend bool,
) ws.MessageHandler {
	return func(_ context.Context, msg *ws.InboundMessage) {
		switch msg.Type {
		case typeAttachInput:
			if !canSend {
				client.SendMessage(ws.NewErrorMessage("permission denied: cannot send input"))

				return
			}

			if err := h.abilityChecker.CheckOrError(
				ctx, user.ID, server.ID,
				[]domain.AbilityName{domain.AbilityNameGameServerConsoleSend},
			); err != nil {
				client.SendMessage(ws.NewErrorMessage("permission denied: cannot send input"))

				return
			}

			var payload attachInputPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				return
			}

			h.sendLegacyInput(ctx, client, server, node, string(payload.Data))

		case typeAttachDetach:
			client.Close()
		}
	}
}

func (h *Handler) sendLegacyInput(
	ctx context.Context, client *ws.Client, server *domain.Server, node *domain.Node, input string,
) {
	if node.ScriptSendCommand != nil && *node.ScriptSendCommand != "" {
		cmd := server.ReplaceServerShortcodes(node, *node.ScriptSendCommand, map[string]string{
			"command": shellescape.Quote(input),
		})

		if _, err := h.daemonCommands.ExecuteCommand(ctx, node, cmd); err != nil {
			h.logger.Warn("failed to execute send command script",
				"server_id", server.ID, "error", err)
			client.SendMessage(ws.NewErrorMessage("failed to send input"))
		}

		return
	}

	inputPath := filepath.Join(server.Dir, "input.txt")

	err := h.fileService.Upload(
		ctx, node, inputPath, []byte(input), 0o644, daemon.OwnerFromServer(server),
	)
	if err != nil {
		h.logger.Warn("failed to upload input", "server_id", server.ID, "error", err)
		client.SendMessage(ws.NewErrorMessage("failed to send input"))
	}
}

func (h *Handler) sendLegacyHistory(
	ctx context.Context, client *ws.Client, server *domain.Server, node *domain.Node,
) string {
	if node.ScriptGetConsole != nil && *node.ScriptGetConsole != "" {
		cmd := server.ReplaceServerShortcodes(node, *node.ScriptGetConsole, nil)

		result, err := h.daemonCommands.ExecuteCommand(ctx, node, cmd)
		if err == nil && result.Output != "" {
			client.SendMessage(ws.NewOutboundMessage(typeAttachOutput, attachOutputPayload{
				Data: []byte(result.Output),
			}))

			return result.Output
		}
	}

	outputPath := filepath.Join(server.Dir, "output.txt")

	content, err := h.fileService.Download(ctx, node, outputPath)
	if err != nil {
		return ""
	}

	const maxBytes = 65536
	if len(content) > maxBytes {
		content = content[len(content)-maxBytes:]
	}

	if len(content) > 0 {
		client.SendMessage(ws.NewOutboundMessage(typeAttachOutput, attachOutputPayload{
			Data: content,
		}))
	}

	return string(content)
}

type legacyAttachPoller struct {
	client      *ws.Client
	fileService fileService
	node        *domain.Node
	serverDir   string
	logger      *slog.Logger
	lastContent string
}

func (p *legacyAttachPoller) run(ctx context.Context) {
	ticker := time.NewTicker(legacyPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.client.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *legacyAttachPoller) poll(ctx context.Context) {
	outputPath := filepath.Join(p.serverDir, "output.txt")

	content, err := p.fileService.Download(ctx, p.node, outputPath)
	if err != nil {
		p.logger.Debug("legacy attach poll: download failed",
			"path", outputPath,
			"error", err,
		)

		return
	}

	currentContent := string(content)
	if currentContent == p.lastContent {
		return
	}

	var diff string
	if len(currentContent) > len(p.lastContent) && currentContent[:len(p.lastContent)] == p.lastContent {
		diff = currentContent[len(p.lastContent):]
	} else {
		diff = currentContent
	}

	p.lastContent = currentContent

	if diff != "" {
		p.client.SendMessage(ws.NewOutboundMessage(typeAttachOutput, attachOutputPayload{
			Data: []byte(diff),
		}))
	}
}
