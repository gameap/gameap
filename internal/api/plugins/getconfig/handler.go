package getconfig

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/plugin/pluginconfig"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

var errPluginNotInstalled = errors.New("plugin is not installed")

// Handler answers a plugin's configuration for the admin UI: the schema the
// manifest declares, the stored values and the names of the secrets that
// hold a value. Secret values never leave the panel.
type Handler struct {
	pluginRepo repositories.PluginRepository
	resolver   DBIDResolver
	responder  base.Responder
}

func NewHandler(
	pluginRepo repositories.PluginRepository,
	resolver DBIDResolver,
	responder base.Responder,
) *Handler {
	return &Handler{
		pluginRepo: pluginRepo,
		resolver:   resolver,
		responder:  responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := api.NewInputReader(r).ReadString("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read plugin ID"))

		return
	}

	if id == "" {
		h.responder.WriteError(ctx, rw, api.NewValidationError("plugin ID is required"))

		return
	}

	record, err := h.findPlugin(ctx, h.resolveDBID(id))
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, newConfigResponse(record))
}

func (h *Handler) findPlugin(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, error) {
	plugins, err := h.pluginRepo.Find(ctx, filters.FindPluginByIDs(dbID), nil, &filters.Pagination{Limit: 1})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find plugin")
	}

	if len(plugins) == 0 {
		return nil, api.WrapHTTPError(errPluginNotInstalled, http.StatusNotFound)
	}

	return &plugins[0], nil
}

func (h *Handler) resolveDBID(id string) domain.Uint64ID {
	if h.resolver != nil {
		if dbID, found := h.resolver.GetDBPluginID(id); found {
			return dbID
		}
	}

	return pkgplugin.ParsePluginID(id)
}

type configResponse struct {
	pluginconfig.View

	ID string `json:"id"`
}

func newConfigResponse(record *domain.Plugin) configResponse {
	return configResponse{
		ID:   pkgplugin.CompactPluginID(record.ID),
		View: pluginconfig.NewView(record),
	}
}
