package uninstall

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"strconv"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

type Handler struct {
	pluginRepo    repositories.PluginRepository
	fileManager   files.FileManager
	manager       PluginManager
	resolver      ManagerIDResolver
	subscriptions plugininstall.SubscriptionRefresher
	scheduler     TaskScheduler
	archiveEvents ArchiveEvents
	storage       PluginStorageCleaner
	secrets       PluginSecretCleaner
	sync          plugininstall.SyncNotifier
	guard         PluginPolicyCleaner
	pluginsDir    string
	responder     base.Responder
	audit         audit.Logger
}

func NewHandler(
	pluginRepo repositories.PluginRepository,
	fileManager files.FileManager,
	manager PluginManager,
	resolver ManagerIDResolver,
	subscriptions plugininstall.SubscriptionRefresher,
	scheduler TaskScheduler,
	archiveEvents ArchiveEvents,
	storage PluginStorageCleaner,
	secrets PluginSecretCleaner,
	sync plugininstall.SyncNotifier,
	guard PluginPolicyCleaner,
	pluginsDir string,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		pluginRepo:    pluginRepo,
		fileManager:   fileManager,
		manager:       manager,
		resolver:      resolver,
		subscriptions: subscriptions,
		scheduler:     scheduler,
		archiveEvents: archiveEvents,
		storage:       storage,
		secrets:       secrets,
		sync:          sync,
		guard:         guard,
		pluginsDir:    pluginsDir,
		responder:     responder,
		audit:         auditLogger,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	storePluginID, err := api.NewInputReader(r).ReadString("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read plugin ID"))

		return
	}

	dbID := pkgplugin.ParsePluginID(storePluginID)

	installedPlugins, err := h.pluginRepo.Find(ctx, filters.FindPluginByIDs(dbID), nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find installed plugin"))

		return
	}

	if len(installedPlugins) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("plugin not installed"),
			http.StatusNotFound,
		))

		return
	}

	pluginRecord := &installedPlugins[0]

	// Held until the row is gone: the reconciler must not revive the module
	// from the file this handler is about to delete.
	if unloader, ok := h.resolver.(RecordUnloader); ok {
		release := unloader.Hold(dbID)
		defer release()
	}

	if err := h.unloadPlugin(ctx, dbID); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to unload plugin"))

		return
	}

	// Plugin data goes first and a failure aborts the uninstall: the record
	// is still there, so the operator can retry, and a plugin reinstalled
	// under the same ID never inherits stale secrets.
	secretsRemoved, err := h.cleanupPluginData(ctx, dbID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	filename := storePluginID + ".wasm"
	if pluginRecord.Filename != nil && *pluginRecord.Filename != "" {
		filename = *pluginRecord.Filename
	}

	pluginPath := path.Join(h.pluginsDir, filename)

	if h.fileManager.Exists(ctx, pluginPath) {
		if err := h.fileManager.Delete(ctx, pluginPath); err != nil {
			h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete plugin file"))

			return
		}
	}

	if err := h.pluginRepo.Delete(ctx, dbID); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete plugin record"))

		return
	}

	// Task cleanup must not fail the uninstall: orphaned rows are harmless
	// (their fires skip with a log) and other instances converge on refresh.
	if h.scheduler != nil {
		if _, err := h.scheduler.RemovePluginTasks(ctx, dbID); err != nil {
			slog.WarnContext(ctx, "failed to remove plugin scheduled tasks",
				slog.Uint64("plugin_id", uint64(dbID)),
				slog.String("error", err.Error()))
		}
	}

	if h.archiveEvents != nil {
		h.archiveEvents.RemovePlugin(uint64(dbID))
	}

	// The record is gone, so nothing can grant the plugin anything again:
	// its rate limiter buckets and audit throttle state are dead weight.
	if h.guard != nil {
		h.guard.Forget(uint64(dbID))
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventPluginUninstall, audit.CategoryPluginOp,
		"plugin", strconv.FormatUint(uint64(dbID), 10), "uninstall",
		slog.Int("secrets_removed", secretsRemoved))

	plugininstall.Notify(ctx, h.sync, dbID, plugininstall.ActionUninstall)

	rw.WriteHeader(http.StatusNoContent)
}

// cleanupPluginData drops the plugin's storage entries and secrets and
// reports how many secrets went. Unlike the task cleanup it is not best
// effort: the caller runs it while the plugin record still exists, so a
// failure leaves a retryable uninstall instead of orphaned credentials.
func (h *Handler) cleanupPluginData(ctx context.Context, dbID domain.Uint64ID) (int, error) {
	if h.storage != nil {
		if err := h.storage.DeleteByPlugin(ctx, uint64(dbID)); err != nil {
			return 0, errors.WithMessage(err, "failed to remove plugin storage entries")
		}
	}

	if h.secrets == nil {
		return 0, nil
	}

	removed, err := h.secrets.DeleteByPlugin(ctx, dbID)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to remove plugin secrets")
	}

	return removed, nil
}

func (h *Handler) unloadPlugin(ctx context.Context, dbID domain.Uint64ID) error {
	// The loader owns the whole unload: pending recovery, subscriptions and
	// the PLUGIN_UNLOADED event.
	if unloader, ok := h.resolver.(RecordUnloader); ok {
		_, err := unloader.UnloadRecord(ctx, dbID, internalplugin.TriggerUninstall)

		return err
	}

	// A reload the recovery supervisor scheduled must not bring the plugin
	// back after the uninstall; cancel it whether or not the plugin is
	// currently loaded.
	if canceller, ok := h.resolver.(RecoveryCanceller); ok {
		canceller.Forget(dbID)
	}

	if h.manager == nil {
		return nil
	}

	managerID := ""
	if h.resolver != nil {
		managerID, _ = h.resolver.GetPluginManagerID(dbID)
	}
	if managerID == "" {
		managerID = pkgplugin.CompactPluginID(dbID)
	}

	if _, exists := h.manager.GetPlugin(managerID); !exists {
		return nil
	}

	if err := h.manager.Unload(ctx, managerID); err != nil {
		return err
	}

	plugininstall.RefreshSubscriptions(ctx, h.subscriptions)

	return nil
}
