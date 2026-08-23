package deletenode

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
)

var (
	ErrNodeHasServers = errors.New("cannot delete node with existing game servers")
)

// PluginDispatcher lets plugins veto a deletion (NODE_PRE_DELETE) and learn
// about it afterwards (NODE_DELETED); satisfied by *plugin.Dispatcher.
type PluginDispatcher interface {
	DispatchNodeEvent(
		ctx context.Context,
		eventType pluginproto.EventType,
		node *domain.Node,
		extraData map[string]string,
	) *pkgplugin.EventDispatchResult
	DispatchNodeEventAsync(
		ctx context.Context,
		eventType pluginproto.EventType,
		node *domain.Node,
		extraData map[string]string,
	)
}

type Handler struct {
	nodesRepo        repositories.NodeRepository
	serversRepo      repositories.ServerRepository
	responder        base.Responder
	audit            audit.Logger
	pluginDispatcher PluginDispatcher
}

func NewHandler(
	nodesRepo repositories.NodeRepository,
	serversRepo repositories.ServerRepository,
	responder base.Responder,
	auditLogger audit.Logger,
	pluginDispatcher PluginDispatcher,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		nodesRepo:        nodesRepo,
		serversRepo:      serversRepo,
		responder:        responder,
		audit:            auditLogger,
		pluginDispatcher: pluginDispatcher,
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

	nodeID, err := input.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid node id"),
			http.StatusBadRequest,
		))

		return
	}

	nodes, err := h.nodesRepo.Find(ctx, &filters.FindNode{
		IDs: []uint{nodeID},
	}, nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find node"))

		return
	}

	if len(nodes) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("node not found"))

		return
	}

	hasServers, err := h.serversRepo.Exists(ctx, &filters.FindServer{
		DSIDs: []uint{nodeID},
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check for associated servers"))

		return
	}

	if hasServers {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			ErrNodeHasServers,
			http.StatusConflict,
		))

		return
	}

	node := nodes[0]

	if err := h.dispatchPreDelete(ctx, &node); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	now := time.Now()
	node.DeletedAt = &now

	err = h.nodesRepo.Save(ctx, &node)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete node"))

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventNodeDelete, audit.CategoryNodeOp,
		"node", strconv.FormatUint(uint64(nodeID), 10), "delete")

	if h.pluginDispatcher != nil {
		h.pluginDispatcher.DispatchNodeEventAsync(ctx, pluginproto.EventType_EVENT_TYPE_NODE_DELETED, &node, nil)
	}

	rw.WriteHeader(http.StatusNoContent)
}

// dispatchPreDelete gives plugins a chance to cancel the deletion.
func (h *Handler) dispatchPreDelete(ctx context.Context, node *domain.Node) error {
	if h.pluginDispatcher == nil {
		return nil
	}

	result := h.pluginDispatcher.DispatchNodeEvent(ctx, pluginproto.EventType_EVENT_TYPE_NODE_PRE_DELETE, node, nil)
	if result == nil || !result.Cancelled {
		return nil
	}

	msg := result.CancelMessage
	if msg == "" {
		msg = result.CancelledBy
	}

	return api.WrapHTTPError(
		errors.Wrapf(servercontrol.ErrCancelledByPlugin, "cancelled by %s: %s", result.CancelledBy, msg),
		http.StatusConflict,
	)
}
