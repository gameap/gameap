package deleteserver

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

type Handler struct {
	serverRepo repositories.ServerRepository
	rbac       base.RBAC
	responder  base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverRepo: serverRepo,
		rbac:       rbac,
		responder:  responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	serverID, err := api.NewInputReader(r).ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	servers, err := h.serverRepo.Find(ctx, filters.FindServerByIDs(serverID), nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find server"))

		return
	}

	if len(servers) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("server not found"),
			http.StatusNotFound,
		))

		return
	}

	server := &servers[0]

	if err := h.serverRepo.Delete(ctx, server.ID); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete server"))

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
