package getdaemonstatus

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type daemonStatusService interface {
	Status(ctx context.Context, node *domain.Node) (*daemon.NodeStatus, error)
}

type Handler struct {
	nodeRepo     repositories.NodeRepository
	daemonStatus daemonStatusService
	responder    base.Responder
}

func NewHandler(
	nodeRepo repositories.NodeRepository,
	daemonStatus daemonStatusService,
	responder base.Responder,
) *Handler {
	return &Handler{
		nodeRepo:     nodeRepo,
		daemonStatus: daemonStatus,
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

	nodeID, err := input.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid node id"),
			http.StatusBadRequest,
		))

		return
	}

	filter := &filters.FindNode{
		IDs: []uint{nodeID},
	}

	nodes, err := h.nodeRepo.Find(ctx, filter, nil, &filters.Pagination{
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

	node := &nodes[0]

	status, err := h.daemonStatus.Status(ctx, node)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to get daemon status"),
			http.StatusInternalServerError,
		))

		return
	}

	h.responder.Write(ctx, rw, newDaemonStatusResponse(node, status))
}
