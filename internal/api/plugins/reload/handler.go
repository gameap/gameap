package reload

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

// pluginLoadFailedTitle is the i18n key the frontend resolves for the
// error dialog, as the upload handlers do for validation failures.
const pluginLoadFailedTitle = "plugins.reload_failed_title"

var (
	errPluginLoadFailed    = errors.New("plugin failed to load")
	errReloadWithoutRecord = errors.New("plugin reloaded but its record is missing")
)

type Handler struct {
	reloader  Reloader
	sync      plugininstall.SyncNotifier
	responder base.Responder
	audit     audit.Logger
}

func NewHandler(
	reloader Reloader,
	sync plugininstall.SyncNotifier,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		reloader:  reloader,
		sync:      sync,
		responder: responder,
		audit:     auditLogger,
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

	dbID := h.resolveDBID(id)
	resourceID := strconv.FormatUint(uint64(dbID), 10)

	plugin, loaded, err := h.reloader.Reload(ctx, dbID)
	if plugin != nil {
		// The row was reached and its generation bumped: the other instances
		// restart the plugin too, whatever the outcome here.
		plugininstall.Notify(ctx, h.sync, dbID, plugininstall.ActionReload)
	}

	if err != nil {
		h.responder.WriteError(ctx, rw, h.reloadError(ctx, resourceID, err))

		return
	}

	if plugin == nil {
		h.responder.WriteError(ctx, rw, errReloadWithoutRecord)

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventPluginReloaded, audit.CategoryPluginOp,
		"plugin", resourceID, "reload", slog.String("trigger", "manual"))

	h.responder.Write(ctx, rw, newReloadResponse(plugin, loaded != nil))
}

// reloadError maps loader failures to HTTP statuses. A load failure is
// audited: the operator asked for a restart and the plugin is now in status
// "error".
func (h *Handler) reloadError(ctx context.Context, resourceID string, err error) error {
	switch {
	case errors.Is(err, internalplugin.ErrPluginNotInstalled):
		return api.WrapHTTPError(err, http.StatusNotFound)
	case errors.Is(err, internalplugin.ErrPluginDisabled), errors.Is(err, internalplugin.ErrPluginUpdating):
		return api.WrapHTTPError(err, http.StatusConflict)
	}

	slog.ErrorContext(ctx, "plugin reload failed",
		slog.String("plugin_id", resourceID),
		slog.String("error", err.Error()))

	audit.SensitiveOpFailed(ctx, h.audit, audit.EventPluginReloaded, audit.CategoryPluginOp,
		"plugin", resourceID, "reload", "load_failed", slog.String("trigger", "manual"))

	// The same text the plugin row now carries as last_error: wazero stack
	// traces stripped, length capped.
	return api.WrapHTTPErrorWithTitle(
		errors.Wrap(errPluginLoadFailed, internalplugin.LoadErrorText(err)),
		http.StatusUnprocessableEntity,
		pluginLoadFailedTitle,
	)
}

// resolveDBID prefers the loader's mapping for plugins loaded on this
// instance and falls back to decoding the ID, which covers plugins that are
// installed but not loaded.
func (h *Handler) resolveDBID(id string) domain.Uint64ID {
	if resolver, ok := h.reloader.(DBIDResolver); ok {
		if dbID, found := resolver.GetDBPluginID(id); found {
			return dbID
		}
	}

	return pkgplugin.ParsePluginID(id)
}
