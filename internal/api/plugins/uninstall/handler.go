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
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

type PluginManager interface {
	GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool)
	Unload(ctx context.Context, pluginID string) error
}

// ManagerIDResolver maps a plugin DB ID to the ID it is registered under in
// the manager (they differ when the wasm's own info ID is not the store ID).
type ManagerIDResolver interface {
	GetPluginManagerID(dbID domain.Uint64ID) (string, bool)
}

// TaskScheduler drops the plugin's scheduled task registrations on uninstall.
type TaskScheduler interface {
	RemovePluginTasks(ctx context.Context, pluginID domain.Uint64ID) (int, error)
}

// ArchiveEvents drops the plugin's archive event registrations on uninstall
// so stale deliveries cannot reach a freshly reinstalled instance.
type ArchiveEvents interface {
	RemovePlugin(pluginID uint64)
}

type Handler struct {
	pluginRepo    repositories.PluginRepository
	fileManager   files.FileManager
	manager       PluginManager
	resolver      ManagerIDResolver
	subscriptions plugininstall.SubscriptionRefresher
	scheduler     TaskScheduler
	archiveEvents ArchiveEvents
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

	if err := h.unloadPlugin(ctx, dbID); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to unload plugin"))

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

	audit.SensitiveOp(ctx, h.audit, audit.EventPluginUninstall, audit.CategoryPluginOp,
		"plugin", strconv.FormatUint(uint64(dbID), 10), "uninstall")

	rw.WriteHeader(http.StatusNoContent)
}

func (h *Handler) unloadPlugin(ctx context.Context, dbID domain.Uint64ID) error {
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
