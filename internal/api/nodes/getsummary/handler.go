package getsummary

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/releases"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	versionpkg "github.com/gameap/gameap/pkg/version"
	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
)

const (
	connectTimeout           = 500 * time.Millisecond
	releasesLookupTimeout    = 3 * time.Second
	defaultCacheTTL          = 30 * time.Second
	backgroundRefreshTimeout = 10 * time.Second
	cacheKey                 = "nodes:summary"
)

type statusService interface {
	Version(ctx context.Context, node *domain.Node) (*daemon.NodeVersion, error)
}

// releasesService resolves the latest published gameap-daemon release so that
// nodes running an older daemon can be flagged.
type releasesService interface {
	Latest(ctx context.Context, component releases.Component) (releases.Info, error)
}

type Handler struct {
	nodeRepo        repositories.NodeRepository
	statusService   statusService
	releasesService releasesService
	responder       base.Responder
	cache           cache.Cache

	cacheTTL                 time.Duration
	backgroundRefreshTimeout time.Duration

	mu               sync.Mutex
	refreshScheduled bool

	sf singleflight.Group
}

func NewHandler(
	nodeRepo repositories.NodeRepository,
	statusService statusService,
	releasesService releasesService,
	responder base.Responder,
	c cache.Cache,
) *Handler {
	return &Handler{
		nodeRepo:                 nodeRepo,
		statusService:            statusService,
		releasesService:          releasesService,
		responder:                responder,
		cache:                    c,
		cacheTTL:                 defaultCacheTTL,
		backgroundRefreshTimeout: backgroundRefreshTimeout,
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

	summary, err := h.getOrCompute(ctx)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, summary)
	h.scheduleRefresh()
}

func (h *Handler) getOrCompute(ctx context.Context) (summaryResponse, error) {
	if cached, ok := h.tryGet(ctx); ok {
		return cached, nil
	}

	return h.computeAndCache(ctx)
}

func (h *Handler) tryGet(ctx context.Context) (summaryResponse, bool) {
	summary, err := cache.GetTyped[summaryResponse](ctx, h.cache, cacheKey)
	if err != nil {
		if !errors.Is(err, cache.ErrNotFound) {
			slog.WarnContext(ctx, "failed to read summary from cache", "error", err)
		}

		return summaryResponse{}, false
	}

	return summary, true
}

func (h *Handler) computeAndCache(ctx context.Context) (summaryResponse, error) {
	result, err, _ := h.sf.Do(cacheKey, func() (any, error) {
		return h.computeAndStore(ctx)
	})
	if err != nil {
		return summaryResponse{}, err
	}

	summary, ok := result.(summaryResponse)
	if !ok {
		return summaryResponse{}, errors.New("unexpected singleflight result type")
	}

	return summary, nil
}

func (h *Handler) computeAndStore(ctx context.Context) (summaryResponse, error) {
	nodes, err := h.nodeRepo.FindAll(ctx, nil, nil)
	if err != nil {
		return summaryResponse{}, errors.WithMessage(err, "failed to find nodes")
	}

	summary := h.calculateSummary(ctx, nodes)

	if err := h.cache.Set(ctx, cacheKey, summary, cache.WithExpiration(h.cacheTTL)); err != nil {
		slog.WarnContext(ctx, "failed to write summary to cache", "error", err)
	}

	return summary, nil
}

func (h *Handler) scheduleRefresh() {
	h.mu.Lock()
	if h.refreshScheduled {
		h.mu.Unlock()

		return
	}
	h.refreshScheduled = true
	h.mu.Unlock()

	time.AfterFunc(h.refreshDelay(), h.runScheduledRefresh)
}

func (h *Handler) refreshDelay() time.Duration {
	gap := h.cacheTTL - h.backgroundRefreshTimeout
	if gap <= 0 {
		return h.cacheTTL / 2
	}

	return gap * 9 / 10
}

func (h *Handler) runScheduledRefresh() {
	defer func() {
		h.mu.Lock()
		h.refreshScheduled = false
		h.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), h.backgroundRefreshTimeout)
	defer cancel()

	if _, err := h.computeAndCache(ctx); err != nil {
		slog.WarnContext(ctx, "scheduled summary refresh failed", "error", err)
	}
}

// latestDaemonVersion resolves the newest stable gameap-daemon release. A
// failure only costs the outdated marks, so the error is deliberately dropped.
func (h *Handler) latestDaemonVersion(ctx context.Context) string {
	if h.releasesService == nil {
		return ""
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, releasesLookupTimeout)
	defer cancel()

	info, err := h.releasesService.Latest(ctxWithTimeout, releases.ComponentDaemon)
	if err != nil {
		slog.DebugContext(ctx, "failed to resolve latest daemon release", "error", err)

		return ""
	}

	return info.LatestStable
}

func (h *Handler) calculateSummary(ctx context.Context, nodes []domain.Node) summaryResponse {
	total := len(nodes)
	enabled := 0
	disabled := 0

	onlineNodes := make([]nodeSummary, 0)
	offlineNodes := make([]nodeSummary, 0)

	latestDaemonVersion := h.latestDaemonVersion(ctx)

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
			summary.Outdated = versionpkg.IsNewer(version.Version, latestDaemonVersion)

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
