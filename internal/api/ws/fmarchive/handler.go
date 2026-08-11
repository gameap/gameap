package fmarchive

import (
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/ws"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

// Handler streams archive create/extract progress of one server's file
// manager: `archive.progress` and `archive.complete` frames bridged from the
// server-scoped realtime channel. No initial state is sent — the client
// correlates frames by the operation_id it received from the 202 response
// and must connect before starting the operation.
type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	hub            *ws.Hub
	originPatterns []string
	responder      base.Responder
	logger         *slog.Logger
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	rbac base.RBAC,
	hub *ws.Hub,
	originPatterns []string,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		hub:            hub,
		originPatterns: originPatterns,
		responder:      responder,
		logger:         slog.Default(),
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := auth.SessionFromContext(ctx)
	if !session.IsAuthenticated() {
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

	server, err := h.serverFinder.FindUserServer(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	err = h.abilityChecker.CheckOrError(
		ctx,
		session.User.ID,
		server.ID,
		[]domain.AbilityName{domain.AbilityNameGameServerFiles},
	)
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

	client := ws.NewClient(ctx, conn, h.hub, nil, h.logger)

	topic := ws.ChannelToTopic(channels.BuildRealtimeFMArchiveChannel(uint64(server.ID)))
	h.hub.Register(client, topic)

	h.logger.Debug("file-manager archive websocket connected", "server_id", server.ID)

	client.Run()
}
