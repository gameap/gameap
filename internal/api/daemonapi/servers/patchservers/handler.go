package patchservers

import (
	"context"
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

	node, err := h.validateDaemonSession(ctx)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusUnauthorized,
		))

		return
	}

	inputs, err := h.parseAndValidateInputs(r)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		))

		return
	}

	if len(inputs) == 0 {
		h.responder.Write(ctx, rw, newBulkUpdateServerResponse())

		return
	}

	servers, err := h.fetchServers(ctx, node, inputs)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusInternalServerError,
		))

		return
	}

	h.updateServers(servers, inputs)

	err = h.saveServers(ctx, servers)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusInternalServerError,
		))

		return
	}

	h.responder.Write(ctx, rw, newBulkUpdateServerResponse())
}

func (h *Handler) validateDaemonSession(ctx context.Context) (*domain.Node, error) {
	daemonSession := auth.DaemonSessionFromContext(ctx)
	if daemonSession == nil || daemonSession.Node == nil {
		return nil, errors.New("daemon session not found")
	}

	return daemonSession.Node, nil
}

func (h *Handler) parseAndValidateInputs(r *http.Request) ([]bulkUpdateServerInput, error) {
	var inputs []bulkUpdateServerInput

	err := json.NewDecoder(r.Body).Decode(&inputs)
	if err != nil {
		return nil, errors.WithMessage(err, "invalid request")
	}

	for _, input := range inputs {
		err = input.Validate()
		if err != nil {
			return nil, errors.WithMessage(err, "invalid input")
		}
	}

	return inputs, nil
}

func (h *Handler) fetchServers(
	ctx context.Context,
	node *domain.Node,
	inputs []bulkUpdateServerInput,
) (map[uint]*domain.Server, error) {
	serverIDs := make([]uint, 0, len(inputs))
	for _, input := range inputs {
		serverIDs = append(serverIDs, input.ID)
	}

	filter := &filters.FindServer{
		IDs:   serverIDs,
		DSIDs: []uint{node.ID},
	}

	servers, err := h.serverRepo.Find(ctx, filter, nil, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find servers")
	}

	serverMap := make(map[uint]*domain.Server, len(servers))
	for i := range servers {
		serverMap[servers[i].ID] = &servers[i]
	}

	return serverMap, nil
}

func (h *Handler) updateServers(
	serverMap map[uint]*domain.Server,
	inputs []bulkUpdateServerInput,
) {
	for _, input := range inputs {
		server, exists := serverMap[input.ID]
		if !exists {
			continue
		}

		if input.Installed != nil {
			server.Installed = domain.ServerInstalledStatus(*input.Installed)
		}

		if input.ProcessActive != nil {
			server.ProcessActive = input.ProcessActive.Bool()
		}

		if input.LastProcessCheck != nil {
			server.LastProcessCheck = &input.LastProcessCheck.Time
		}
	}
}

func (h *Handler) saveServers(
	ctx context.Context,
	servers map[uint]*domain.Server,
) error {
	serverList := make([]*domain.Server, 0, len(servers))
	for _, server := range servers {
		serverList = append(serverList, server)
	}

	err := h.serverRepo.SaveBulk(ctx, serverList)
	if err != nil {
		return errors.WithMessage(err, "failed to update servers")
	}

	return nil
}
