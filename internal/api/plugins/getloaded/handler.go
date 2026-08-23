package getloaded

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/pluginsync"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

type LoaderManager interface {
	GetPlugins() []*pkgplugin.LoadedPlugin
}

// SyncStatusProvider reports what the multi-instance reconciler did for
// each plugin on this instance; satisfied by *pluginsync.Service.
type SyncStatusProvider interface {
	Snapshot() map[domain.Uint64ID]pluginsync.Status
}

// Option tunes a Handler.
type Option func(*Handler)

// WithSyncStatus attaches the reconciler's per-plugin state to the listing.
func WithSyncStatus(provider SyncStatusProvider) Option {
	return func(h *Handler) {
		h.sync = provider
	}
}

type Handler struct {
	manager    LoaderManager
	loader     *plugin.Loader
	pluginRepo repositories.PluginRepository
	sync       SyncStatusProvider
	responder  base.Responder
}

func NewHandler(
	manager LoaderManager,
	loader *plugin.Loader,
	pluginRepo repositories.PluginRepository,
	responder base.Responder,
	opts ...Option,
) *Handler {
	h := &Handler{
		manager:    manager,
		loader:     loader,
		pluginRepo: pluginRepo,
		responder:  responder,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// ServeHTTP lists every installed plugin merged with this instance's runtime
// state: loaded plugins first, in manager order, then the installed plugins
// that are not loaded here (failed to load, disabled, updating) in
// repository order, so an operator can see and reload a broken plugin.
func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	loadedPlugins := h.manager.GetPlugins()
	installed := h.fetchInstalled(ctx)

	response := &listResponse{
		Data: make([]*loadedPluginResponse, 0, len(loadedPlugins)+len(installed)),
	}

	byID := make(map[domain.Uint64ID]*domain.Plugin, len(installed))
	for i := range installed {
		byID[installed[i].ID] = &installed[i]
	}

	seen := make(map[domain.Uint64ID]struct{}, len(loadedPlugins))
	syncStatus := h.syncSnapshot()

	for _, loaded := range loadedPlugins {
		dbID := h.databaseID(loaded)
		seen[dbID] = struct{}{}

		item := newLoadedPluginResponse(loaded, byID[dbID])
		item.Sync = newSyncResponse(syncStatus, dbID)

		response.Data = append(response.Data, item)
	}

	for i := range installed {
		if _, ok := seen[installed[i].ID]; ok {
			continue
		}

		item := newInstalledPluginResponse(&installed[i])
		item.Sync = newSyncResponse(syncStatus, installed[i].ID)
		item.applyLocalSyncFailure()

		response.Data = append(response.Data, item)
	}

	h.responder.Write(ctx, rw, response)
}

func (h *Handler) syncSnapshot() map[domain.Uint64ID]pluginsync.Status {
	if h.sync == nil {
		return nil
	}

	return h.sync.Snapshot()
}

// databaseID resolves the record a loaded plugin belongs to: the ID it was
// loaded for, else the loader's mapping, else its own declared ID.
func (h *Handler) databaseID(loaded *pkgplugin.LoadedPlugin) domain.Uint64ID {
	if loaded.DBID != 0 {
		return domain.Uint64ID(loaded.DBID)
	}

	if h.loader != nil {
		if dbID, ok := h.loader.GetDBPluginID(loaded.Info.Id); ok {
			return dbID
		}
	}

	return pkgplugin.ParsePluginID(loaded.Info.Id)
}

// fetchInstalled reads every plugin record; on a repository failure the
// list degrades to the loaded plugins only.
func (h *Handler) fetchInstalled(ctx context.Context) []domain.Plugin {
	plugins, err := h.pluginRepo.FindAll(ctx, nil, nil)
	if err != nil {
		slog.WarnContext(ctx, "failed to list installed plugins, reporting loaded plugins only",
			slog.String("error", err.Error()))

		return nil
	}

	return plugins
}
