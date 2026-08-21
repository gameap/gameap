package install

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"strconv"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

// update replaces an installed plugin with the uploaded build. The plugin's
// storage, secrets and scheduled tasks are keyed by plugin ID and outlive the
// swap untouched, which is the whole point: uninstalling and installing again
// was the only way to move a file-installed plugin to a new version.
func (h *Handler) update(
	ctx context.Context,
	rw http.ResponseWriter,
	installed *domain.Plugin,
	loaded *pkgplugin.LoadedPlugin,
	wasmBytes []byte,
	dbID domain.Uint64ID,
) {
	filename := plugininstall.ResolvePluginFilename(installed, dbID)
	pluginPath := path.Join(h.pluginsDir, filename)

	previous := *installed
	previousVersion := installed.Version
	previousBytes, hasPrevious := h.readInstalledFile(ctx, pluginPath)

	// The manager keys plugins by ID and refuses a second Load of one that is
	// already running, so the old module has to go before the new file lands.
	if err := plugininstall.UnloadPlugin(ctx, h.loader, dbID); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if err := h.fileManager.Write(ctx, pluginPath, wasmBytes); err != nil {
		h.restorePrevious(ctx, &previous, pluginPath, previousBytes, hasPrevious)
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save plugin file"))

		return
	}

	plugininstall.ApplyManifest(installed, loaded, filename)

	if err := h.pluginRepo.Save(ctx, installed); err != nil {
		h.restorePrevious(ctx, &previous, pluginPath, previousBytes, hasPrevious)
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save plugin record"))

		return
	}

	if _, err := plugininstall.TryLoadPlugin(ctx, h.loader, h.pluginRepo, installed, filename); err != nil {
		restored := h.restorePrevious(ctx, &previous, pluginPath, previousBytes, hasPrevious)

		h.responder.WriteError(ctx, rw, api.WrapHTTPErrorWithTitle(
			errors.WithMessage(err, updateFailureMessage(restored)),
			http.StatusUnprocessableEntity,
			installFailedTitle,
		))

		return
	}

	// Audited only once the new build is running: a failed update is rolled
	// back, so recording it earlier would log a change that did not happen.
	audit.SensitiveOp(ctx, h.audit, audit.EventPluginUpdate, audit.CategoryPluginOp,
		"plugin", strconv.FormatUint(uint64(dbID), 10), "update",
		slog.String("plugin", loaded.Info.Id),
		slog.String("previous_version", previousVersion),
		slog.String("version", installed.Version))

	plugininstall.RefreshSubscriptions(ctx, h.subscriptions)

	h.responder.Write(ctx, rw, newUpdateResponse(installed, previousVersion))
}

func updateFailureMessage(restored bool) string {
	if restored {
		return "plugin update failed, previous version restored"
	}

	return "plugin updated but failed to load"
}

// readInstalledFile keeps the bytes currently on disk so a failed update can
// be undone. A plugin whose file is already gone cannot be restored; the
// update then goes ahead as the only way forward.
func (h *Handler) readInstalledFile(ctx context.Context, pluginPath string) ([]byte, bool) {
	if !h.fileManager.Exists(ctx, pluginPath) {
		return nil, false
	}

	data, err := h.fileManager.Read(ctx, pluginPath)
	if err != nil {
		slog.WarnContext(ctx, "failed to read installed plugin file, update will not be reversible",
			slog.String("path", pluginPath),
			slog.String("error", err.Error()))

		return nil, false
	}

	return data, true
}

// restorePrevious puts the plugin back the way it was and reports whether it
// succeeded. Every step is best effort — the update is already being reported
// as failed, and a panel left with no module at all is worse than a warning.
func (h *Handler) restorePrevious(
	ctx context.Context,
	previous *domain.Plugin,
	pluginPath string,
	previousBytes []byte,
	hasPrevious bool,
) bool {
	if !hasPrevious {
		return false
	}

	if err := h.fileManager.Write(ctx, pluginPath, previousBytes); err != nil {
		slog.ErrorContext(ctx, "failed to restore previous plugin file",
			slog.String("path", pluginPath),
			slog.String("error", err.Error()))

		return false
	}

	restored := *previous

	if err := h.pluginRepo.Save(ctx, &restored); err != nil {
		slog.ErrorContext(ctx, "failed to restore previous plugin record",
			slog.Uint64("plugin_id", uint64(restored.ID)),
			slog.String("error", err.Error()))

		return false
	}

	filename := plugininstall.ResolvePluginFilename(&restored, restored.ID)

	if _, err := plugininstall.TryLoadPlugin(ctx, h.loader, h.pluginRepo, &restored, filename); err != nil {
		slog.ErrorContext(ctx, "failed to load previous plugin version back",
			slog.Uint64("plugin_id", uint64(restored.ID)),
			slog.String("error", err.Error()))

		return false
	}

	plugininstall.RefreshSubscriptions(ctx, h.subscriptions)

	return true
}
