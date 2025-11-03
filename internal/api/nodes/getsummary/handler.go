package getsummary

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

const (
	connectTimeout = 500 * time.Millisecond
)

type statusService interface {
	Version(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error)
}

type Handler struct {
	nodeRepo      repositories.NodeRepository
	statusService statusService
	responder     base.Responder
}

func NewHandler(
	nodeRepo repositories.NodeRepository,
	statusService statusService,
	responder base.Responder,
) *Handler {
	return &Handler{
		nodeRepo:      nodeRepo,
		statusService: statusService,
		responder:     responder,
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

	nodes, err := h.nodeRepo.FindAll(ctx, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find nodes"))

		return
	}

	summary := h.calculateSummary(ctx, nodes)

	h.responder.Write(ctx, rw, summary)
}

func (h *Handler) calculateSummary(ctx context.Context, nodes []domain.Node) summaryResponse {
	total := len(nodes)
	enabled := 0
	disabled := 0

	onlineNodes := make([]nodeSummary, 0)
	offlineNodes := make([]nodeSummary, 0)

	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := range nodes {
		node := nodes[i]

		if node.Enabled {
			enabled++
		} else {
			disabled++
		}

		wg.Add(1)
		go func(node domain.Node) {
			defer wg.Done()

			summary := nodeSummary{
				ID:       node.ID,
				Name:     node.Name,
				Location: node.Location,
				Enabled:  node.Enabled,
			}

			ctxWithTimeout, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()

			version, err := h.statusService.Version(ctxWithTimeout, &node)
			if err != nil {
				slog.Debug("failed to get node version", "node_id", node.ID, "error", err)
				summary.Online = false

				mu.Lock()
				offlineNodes = append(offlineNodes, summary)
				mu.Unlock()

				return
			}

			summary.Online = true
			summary.Version = version.Version
			summary.BuildDate = version.BuildDate

			mu.Lock()
			onlineNodes = append(onlineNodes, summary)
			mu.Unlock()
		}(node)
	}

	wg.Wait()

	online := len(onlineNodes)
	offline := total - online

	return summaryResponse{
		Total:        total,
		Enabled:      enabled,
		Disabled:     disabled,
		Online:       online,
		Offline:      offline,
		OnlineNodes:  onlineNodes,
		OfflineNodes: offlineNodes,
	}
}
