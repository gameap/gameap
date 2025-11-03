package getnodes

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
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

	nodes, err := h.nodesRepo.FindAll(ctx, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find nodes"))

		return
	}

	nodesResponse := newNodesResponseFromNodes(nodes)

	h.responder.Write(ctx, rw, nodesResponse)
}
