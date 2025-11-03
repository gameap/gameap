package putserver

import (
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	serverRepo repositories.ServerRepository
	responder  base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverRepo: serverRepo,
		responder:  responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	daemonSession := auth.DaemonSessionFromContext(ctx)
	if daemonSession == nil || daemonSession.Node == nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("daemon session not found"),
			http.StatusUnauthorized,
		))

		return
	}

	node := daemonSession.Node

	serverID, err := api.NewInputReader(r).ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server ID"),
			http.StatusBadRequest,
		))

		return
	}

	input := &updateServerInput{}

	err = json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request"),
			http.StatusBadRequest,
		))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid input"),
			http.StatusBadRequest,
		))

		return
	}

	filter := &filters.FindServer{
		IDs:   []uint{serverID},
		DSIDs: []uint{node.ID},
	}

	servers, err := h.serverRepo.Find(ctx, filter, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find server"),
			http.StatusInternalServerError,
		))

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

	if input.Installed != nil {
		server.Installed = domain.ServerInstalledStatus(*input.Installed)
	}

	if input.ProcessActive != nil {
		server.ProcessActive = input.ProcessActive.Bool()
	}

	if input.LastProcessCheck != nil {
		server.LastProcessCheck = &input.LastProcessCheck.Time
	}

	err = h.serverRepo.Save(ctx, server)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to update server"),
			http.StatusInternalServerError,
		))

		return
	}

	h.responder.Write(ctx, rw, newUpdateServerResponse())
}
