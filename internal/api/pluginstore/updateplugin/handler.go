package updateplugin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

type Handler struct {
	storeService  *pluginstore.Service
	pluginRepo    repositories.PluginRepository
	fileManager   files.FileManager
	loader        *plugin.Loader
	subscriptions plugininstall.SubscriptionRefresher
	sync          plugininstall.SyncNotifier
	pluginsDir    string
	responder     base.Responder
}

func NewHandler(
	storeService *pluginstore.Service,
	pluginRepo repositories.PluginRepository,
	fileManager files.FileManager,
	loader *plugin.Loader,
	subscriptions plugininstall.SubscriptionRefresher,
	sync plugininstall.SyncNotifier,
	pluginsDir string,
	responder base.Responder,
) *Handler {
	return &Handler{
		storeService:  storeService,
		pluginRepo:    pluginRepo,
		fileManager:   fileManager,
		loader:        loader,
		subscriptions: subscriptions,
		sync:          sync,
		pluginsDir:    pluginsDir,
		responder:     responder,
	}
}

const extendedWriteDeadline = 5 * time.Minute

type input struct {
	Version string `json:"version"`
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rc := http.NewResponseController(rw)
	if err := rc.SetWriteDeadline(time.Now().Add(extendedWriteDeadline)); err != nil {
		slog.WarnContext(ctx, "failed to extend write deadline", slog.String("error", err.Error()))
	}

	storePluginID, err := api.NewInputReader(r).ReadString("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read plugin ID"))

		return
	}

	inp, err := h.parseInput(r)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	dbID := pkgplugin.ParsePluginID(storePluginID)

	pluginRecord, err := h.findInstalledPlugin(ctx, dbID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// Held while the module is down and the file is being replaced: the
	// reconciler must not rebuild the old module in between.
	if h.loader != nil {
		release := h.loader.Hold(dbID)
		defer release()
	}

	if err := h.unloadPlugin(ctx, dbID); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}
	defer plugininstall.RefreshSubscriptions(ctx, h.subscriptions)

	selectedVersion, err := h.selectVersion(ctx, storePluginID, inp.Version)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	wasmBytes, err := h.downloadAndVerify(ctx, storePluginID, selectedVersion)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	filename := storePluginID + ".wasm"
	pluginPath := path.Join(h.pluginsDir, filename)

	if err := h.fileManager.Write(ctx, pluginPath, wasmBytes); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save plugin file"))

		return
	}

	h.updatePluginRecord(pluginRecord, selectedVersion, filename, wasmBytes)

	if err := h.pluginRepo.Save(ctx, pluginRecord); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to update plugin record"))

		return
	}

	// The row is saved from here on: whatever the load does, peers learn
	// about the new version.
	defer plugininstall.Notify(ctx, h.sync, dbID, plugininstall.ActionUpdate)

	loaded, err := plugininstall.TryLoadPlugin(ctx, h.loader, pluginRecord)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "plugin installed but failed to load"),
			http.StatusUnprocessableEntity,
		))

		return
	}

	h.recordDeclaredPermissions(ctx, pluginRecord, loaded)

	h.responder.Write(ctx, rw, newUpdateResponse(pluginRecord))
}

// recordDeclaredPermissions keeps required_permissions in step with the new
// build. Grants are not widened: a version that starts needing more is
// denied those calls until an operator grants them, which the warning and
// the admin UI point out.
func (h *Handler) recordDeclaredPermissions(
	ctx context.Context,
	pluginRecord *domain.Plugin,
	loaded *pkgplugin.LoadedPlugin,
) {
	if loaded == nil || loaded.Info == nil {
		return
	}

	declared := domain.ParsePluginPermissions(loaded.Info.RequiredPermissions)
	if !slices.Equal(declared, pluginRecord.RequiredPermissions) {
		pluginRecord.RequiredPermissions = declared

		if err := h.pluginRepo.Save(ctx, pluginRecord); err != nil {
			slog.WarnContext(ctx, "failed to record the updated plugin's declared permissions",
				slog.Uint64("plugin_id", uint64(pluginRecord.ID)),
				slog.String("error", err.Error()))
		}
	}

	used := plugin.UsedPermissions(loaded.HostImports, loaded.SubscribedEvents)
	if missing := plugin.MissingPermissions(used, pluginRecord.AllowedPermissions); len(missing) > 0 {
		slog.WarnContext(ctx, "updated plugin uses permissions it is not granted; those calls will be refused",
			slog.Uint64("plugin_id", uint64(pluginRecord.ID)),
			slog.Any("missing_permissions", plugin.PermissionNames(missing)))
	}
}

func (h *Handler) parseInput(r *http.Request) (input, error) {
	var inp input
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&inp); err != nil {
			return inp, api.WrapHTTPError(
				errors.WithMessage(err, "invalid request body"),
				http.StatusBadRequest,
			)
		}
	}

	return inp, nil
}

func (h *Handler) findInstalledPlugin(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, error) {
	installedPlugins, err := h.pluginRepo.Find(ctx, filters.FindPluginByIDs(dbID), nil, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find installed plugin")
	}

	if len(installedPlugins) == 0 {
		return nil, api.WrapHTTPError(errors.New("plugin not installed"), http.StatusNotFound)
	}

	return &installedPlugins[0], nil
}

func (h *Handler) unloadPlugin(ctx context.Context, dbID domain.Uint64ID) error {
	if h.loader == nil {
		return nil
	}

	managerID, ok := h.loader.GetPluginManagerID(dbID)
	if !ok {
		managerID = pkgplugin.CompactPluginID(dbID)
	}

	if err := h.loader.Unload(ctx, managerID); err != nil {
		if errors.Is(err, pkgplugin.ErrPluginNotFound) {
			return nil
		}

		return errors.WithMessage(err, "failed to unload plugin")
	}

	return nil
}

func (h *Handler) selectVersion(
	ctx context.Context,
	storePluginID string,
	requestedVersion string,
) (*pluginstore.PluginVersion, error) {
	versions, err := h.storeService.GetPluginVersions(ctx, storePluginID, pluginstore.GetPluginVersionsParams{
		Page:    1,
		PerPage: 100,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get plugin versions")
	}

	if len(versions.Data) == 0 {
		return nil, api.WrapHTTPError(errors.New("no versions available for this plugin"), http.StatusNotFound)
	}

	return findVersion(versions.Data, requestedVersion)
}

func findVersion(versions []pluginstore.PluginVersion, requested string) (*pluginstore.PluginVersion, error) {
	if requested != "" {
		for i, v := range versions {
			if v.Version == requested {
				return &versions[i], nil
			}
		}

		return nil, api.WrapHTTPError(errors.New("specified version not found"), http.StatusNotFound)
	}

	for i, v := range versions {
		if v.IsStable {
			return &versions[i], nil
		}
	}

	return &versions[0], nil
}

func (h *Handler) downloadAndVerify(
	ctx context.Context,
	storePluginID string,
	version *pluginstore.PluginVersion,
) ([]byte, error) {
	wasmBytes, err := h.storeService.DownloadPlugin(ctx, storePluginID, version.Version)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to download plugin")
	}

	if !pluginstore.VerifyHash(wasmBytes, version.FileHash) {
		return nil, api.WrapHTTPError(errors.New("plugin file hash mismatch"), http.StatusUnprocessableEntity)
	}

	return wasmBytes, nil
}

func (h *Handler) updatePluginRecord(
	record *domain.Plugin,
	version *pluginstore.PluginVersion,
	filename string,
	wasmBytes []byte,
) {
	record.Version = version.Version
	record.Filename = new(filename)
	record.Checksum = new(plugin.FileChecksum(wasmBytes))
	record.Status = domain.PluginStatusActive
	record.UpdatedAt = new(time.Now())
}
