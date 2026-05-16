package deletenode

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

var (
	ErrNodeHasServers = errors.New("cannot delete node with existing game servers")
)

type Handler struct {
	nodesRepo   repositories.NodeRepository
	serversRepo repositories.ServerRepository
	responder   base.Responder
	audit       audit.Logger
}

func NewHandler(
	nodesRepo repositories.NodeRepository,
	serversRepo repositories.ServerRepository,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		nodesRepo:   nodesRepo,
		serversRepo: serversRepo,
		responder:   responder,
		audit:       auditLogger,
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
	now := time.Now()
	node.DeletedAt = &now

	err = h.nodesRepo.Save(ctx, &node)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete node"))

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventNodeDelete, audit.CategoryNodeOp,
		"node", strconv.FormatUint(uint64(nodeID), 10), "delete")

	rw.WriteHeader(http.StatusNoContent)
}
