package attach

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
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
	"github.com/pkg/errors"
)

const (
	typeAttachInput  = "attach.input"
	typeAttachDetach = "attach.detach"
)

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	nodeRepo       repositories.NodeRepository
	hub            *ws.Hub
	originPatterns []string
	registry       *session.Registry
	attachHandler  *handlers.AttachHandler
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

	if !h.registry.IsConnectedAnywhere(nodeID) {
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

	canSend := h.canSendCommands(ctx, sess.User, server)

	sessionID := idgen.New()
	h.runAttachSession(ctx, conn, server, node, sessionID, sess.User, canSend)
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
