package getplugin

import (
	"log/slog"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

type Handler struct {
	storeService *pluginstore.Service
	pluginRepo   repositories.PluginRepository
	responder    base.Responder
}

func NewHandler(
	storeService *pluginstore.Service,
	pluginRepo repositories.PluginRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		storeService: storeService,
		pluginRepo:   pluginRepo,
		responder:    responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pluginID, err := api.NewInputReader(r).ReadString("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read plugin ID"))

		return
	}

	lang := pluginstore.ExtractLanguage(r)

	plugin, err := h.storeService.GetPlugin(ctx, pluginID, lang)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to get plugin from store"))

		return
	}

	var installedVersion *string
	dbID := pkgplugin.ParsePluginID(pluginID)
	installedPlugins, err := h.pluginRepo.Find(ctx, nil, nil, nil)
	if err == nil {
		for _, p := range installedPlugins {
			if p.ID == dbID {
				installedVersion = &p.Version

				break
			}
		}
	}

	var licenseValidation *pluginstore.LicenseValidation
	if h.storeService.HasLicenseKey() {
		licenseValidation, err = h.storeService.ValidateLicense(ctx)
		if err != nil {
			slog.Warn(
				"failed to validate license",
				slog.String("error", err.Error()),
				slog.String("plugin_id", pluginID),
			)
		}
	}

	h.responder.Write(ctx, rw, newPluginResponse(plugin, installedVersion, licenseValidation))
}
