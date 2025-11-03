package getnode

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	nodesRepo repositories.NodeRepository
	responder base.Responder
}

func NewHandler(
	nodesRepo repositories.NodeRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		nodesRepo: nodesRepo,
		responder: responder,
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

	nodes, err := h.nodesRepo.Find(ctx, filter, nil, &filters.Pagination{
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

	h.responder.Write(ctx, rw, newNodeResponseFromNode(&nodes[0]))
}
