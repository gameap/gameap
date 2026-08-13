package getloaded

import (
	"context"
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

// SyncStatusProvider reports what the reconciler did on this instance. The view
// is deliberately local: there is no cross-instance aggregation, so an operator
// reads it from whichever instance answered. Satisfied by *pluginsync.Service,
// which tolerates a nil receiver.
type SyncStatusProvider interface {
	Snapshot() map[domain.Uint64ID]pluginsync.Status
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
	sync SyncStatusProvider,
	responder base.Responder,
) *Handler {
	return &Handler{
		manager:    manager,
		loader:     loader,
		pluginRepo: pluginRepo,
		sync:       sync,
		responder:  responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	loadedPlugins := h.manager.GetPlugins()
	dbPlugins := h.fetchDBPlugins(ctx)

	var syncStatuses map[domain.Uint64ID]pluginsync.Status
	if h.sync != nil {
		syncStatuses = h.sync.Snapshot()
	}

	response := &listResponse{
		Data: make([]*loadedPluginResponse, 0, len(loadedPlugins)),
	}

	covered := make(map[domain.Uint64ID]struct{}, len(loadedPlugins))

	for _, loaded := range loadedPlugins {
		dbID := h.resolveDBID(loaded)
		covered[dbID] = struct{}{}

		response.Data = append(response.Data,
			newLoadedPluginResponse(loaded, dbPlugins[dbID], syncStatuses[dbID]))
	}

	// A plugin the database wants active but this instance could not load is
	// the case an operator most needs to see; leaving it out would make a
	// broken instance look healthy.
	for id, dbPlugin := range dbPlugins {
		if _, ok := covered[id]; ok {
			continue
		}

		if dbPlugin.Status != domain.PluginStatusActive {
			continue
		}

		response.Data = append(response.Data, newUnloadedPluginResponse(dbPlugin, syncStatuses[id]))
	}

	h.responder.Write(ctx, rw, response)
}

// resolveDBID prefers the loader's mapping: a store plugin's own manifest ID
// need not match the store ID the row is keyed on, and deriving the ID from the
// manifest alone would point at a row that does not exist.
func (h *Handler) resolveDBID(loaded *pkgplugin.LoadedPlugin) domain.Uint64ID {
	if h.loader != nil {
		if dbID, ok := h.loader.GetDBPluginID(loaded.Info.Id); ok {
			return dbID
		}
	}

	return pkgplugin.ParsePluginID(loaded.Info.Id)
}

func (h *Handler) fetchDBPlugins(ctx context.Context) map[domain.Uint64ID]*domain.Plugin {
	plugins, err := h.pluginRepo.FindAll(ctx, nil, nil)
	if err != nil {
		return nil
	}

	result := make(map[domain.Uint64ID]*domain.Plugin, len(plugins))
	for i := range plugins {
		result[plugins[i].ID] = &plugins[i]
	}

	return result
}
