package getbusyports

import (
	"net/http"
	"sort"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

type Handler struct {
	serversRepo repositories.ServerRepository
	responder   base.Responder
}

func NewHandler(
	serversRepo repositories.ServerRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		serversRepo: serversRepo,
		responder:   responder,
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

	nodeID, err := input.ReadUint("node")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid node id"),
			http.StatusBadRequest,
		))

		return
	}

	servers, err := h.serversRepo.Find(ctx, &filters.FindServer{
		DSIDs: []uint{nodeID},
	}, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find servers"))

		return
	}

	busyPorts := h.collectBusyPorts(servers)

	h.responder.Write(ctx, rw, newBusyPortsResponse(busyPorts))
}

func (h *Handler) collectBusyPorts(servers []domain.Server) map[string][]int {
	result := make(map[string][]int, len(servers))

	for _, server := range servers {
		if _, exists := result[server.ServerIP]; !exists {
			result[server.ServerIP] = make([]int, 0, len(servers)*3)
		}

		result[server.ServerIP] = append(result[server.ServerIP], server.ServerPort)

		if server.QueryPort != nil {
			result[server.ServerIP] = append(result[server.ServerIP], *server.QueryPort)
		}

		if server.RconPort != nil {
			result[server.ServerIP] = append(result[server.ServerIP], *server.RconPort)
		}
	}

	for ip := range result {
		result[ip] = lo.Uniq(result[ip])
		sort.Ints(result[ip])
	}

	return result
}
