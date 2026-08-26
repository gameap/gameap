package update

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

const extendedWriteDeadline = 5 * time.Minute

type LoaderManager interface {
	LoadTransient(
		ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64,
	) (*pkgplugin.LoadedPlugin, error)
}

// Handler replaces the code of an installed plugin with an uploaded build.
// Everything an uninstall would destroy — gameap-storage entries, secrets,
// configuration and the granted permissions — survives, which is what makes
// this the way to iterate on a plugin instead of uninstall + install.
type Handler struct {
	manager       LoaderManager
	pluginRepo    repositories.PluginRepository
	fileManager   files.FileManager
	loader        *plugin.Loader
	subscriptions plugininstall.SubscriptionRefresher
	sync          plugininstall.SyncNotifier
	pluginsDir    string
	responder     base.Responder
	audit         audit.Logger
}

func NewHandler(
	manager LoaderManager,
	pluginRepo repositories.PluginRepository,
	fileManager files.FileManager,
	loader *plugin.Loader,
	subscriptions plugininstall.SubscriptionRefresher,
	sync plugininstall.SyncNotifier,
	pluginsDir string,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		manager:       manager,
		pluginRepo:    pluginRepo,
		fileManager:   fileManager,
		loader:        loader,
		subscriptions: subscriptions,
		sync:          sync,
		pluginsDir:    pluginsDir,
		responder:     responder,
		audit:         auditLogger,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rc := http.NewResponseController(rw)
	if err := rc.SetWriteDeadline(time.Now().Add(extendedWriteDeadline)); err != nil {
		slog.WarnContext(ctx, "failed to extend write deadline", slog.String("error", err.Error()))
	}

	requestedID, err := api.NewInputReader(r).ReadString("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read plugin ID"))

		return
	}

	dbID := pkgplugin.ParsePluginID(requestedID)

	wasmBytes, err := plugininstall.ReadWASMFromMultipart(rw, r)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if err := plugininstall.ValidateWASM(wasmBytes); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// Looked up before the module is built: a 404 must not cost the caller a
	// full wasm compilation.
	pluginRecord, err := h.findInstalledPlugin(ctx, dbID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	loaded, err := h.loadUploaded(ctx, wasmBytes, dbID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}
	defer func() {
		if err := loaded.Close(ctx); err != nil {
			slog.WarnContext(ctx, "failed to close transient plugin",
				slog.String("plugin_id", loaded.Info.Id),
				slog.String("error", err.Error()))
		}
	}()

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

	// The row's own file name is reused, so a plugin never ends up with two
	// wasm files and a store plugin replaced by hand leaves nothing behind.
	filename := plugin.ResolveFilename(pluginRecord)
	pluginPath := path.Join(h.pluginsDir, filename)

	// The build that runs today, held until the row is saved. The file
	// manager has no atomic replace, so a failure between overwriting the
	// file and saving the row would otherwise leave the new bytes under a row
	// that still describes the old ones — and no one recovers from that on
	// their own: pluginsync sees a file that does not match the recorded
	// checksum and refuses to refetch an uploaded plugin, because no instance
	// has its file. Deleting the file (how the install path undoes its own
	// write) leads to the same dead end, so the previous bytes are what gets
	// put back.
	previous, hadPrevious := h.readInstalledBuild(ctx, pluginPath)

	if err := h.fileManager.Write(ctx, pluginPath, wasmBytes); err != nil {
		h.restoreInstalledBuild(ctx, pluginPath, previous, hadPrevious)
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save plugin file"))

		return
	}

	updatePluginRecord(pluginRecord, loaded, filename, wasmBytes)

	if err := h.pluginRepo.Save(ctx, pluginRecord); err != nil {
		h.restoreInstalledBuild(ctx, pluginPath, previous, hadPrevious)
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to update plugin record"))

		return
	}

	// The row is saved from here on: whatever the load does, peers learn
	// about the new build.
	defer plugininstall.Notify(ctx, h.sync, dbID, plugininstall.ActionUpdate)

	audit.SensitiveOp(ctx, h.audit, audit.EventPluginUpdate, audit.CategoryPluginOp,
		"plugin", strconv.FormatUint(uint64(dbID), 10), "update",
		slog.String("plugin", loaded.Info.Id),
		slog.String("version", pluginRecord.Version))

	// A successful load reports the permissions the new build exercises
	// without holding the grant (Loader.warnMissingPermissions), so the
	// operator is told what to grant without this handler repeating it.
	if _, err := plugininstall.TryLoadPlugin(ctx, h.loader, pluginRecord); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "plugin updated but failed to load"),
			http.StatusUnprocessableEntity,
		))

		return
	}

	h.responder.Write(ctx, rw, newUpdateResponse(pluginRecord))
}

// loadUploaded builds the uploaded module to read its manifest and refuses a
// file that describes a different plugin than the one being updated: the
// module the caller uploaded is the one that would run under the target's id,
// grants and stored data.
func (h *Handler) loadUploaded(
	ctx context.Context,
	wasmBytes []byte,
	dbID domain.Uint64ID,
) (*pkgplugin.LoadedPlugin, error) {
	loaded, err := h.manager.LoadTransient(ctx, wasmBytes, nil, 0)
	if err != nil {
		return nil, api.WrapHTTPErrorWithTitle(
			pkgplugin.SanitizeLoadError(err),
			http.StatusBadRequest,
			"plugins.validation_failed_title",
		)
	}

	if uploadedID := pkgplugin.ParsePluginID(loaded.Info.Id); uploadedID != dbID {
		slog.WarnContext(ctx, "uploaded plugin does not match the plugin being updated",
			slog.String("uploaded_plugin_id", loaded.Info.Id),
			slog.Uint64("plugin_id", uint64(dbID)))

		if err := loaded.Close(ctx); err != nil {
			slog.WarnContext(ctx, "failed to close transient plugin",
				slog.String("plugin_id", loaded.Info.Id),
				slog.String("error", err.Error()))
		}

		return nil, api.WrapHTTPError(
			errors.New("the uploaded file is a different plugin than the one being updated"),
			http.StatusBadRequest,
		)
	}

	return loaded, nil
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

// updatePluginRecord rewrites the row from the uploaded build's manifest. The
// manifest is the only metadata a file plugin has, so it also replaces the
// store-provided name and description of a plugin being taken over by hand.
//
// AllowedPermissions is deliberately untouched: grants are not widened by
// uploading a build that asks for more. The new declaration is recorded in
// RequiredPermissions, the calls behind an ungranted permission stay refused,
// and the admin UI points out what is missing.
func updatePluginRecord(
	record *domain.Plugin,
	loaded *pkgplugin.LoadedPlugin,
	filename string,
	wasmBytes []byte,
) {
	source := plugininstall.FileSourcePrefix + filename

	record.Name = loaded.Info.Name
	record.Version = loaded.Info.Version
	record.Description = loaded.Info.Description
	record.Author = loaded.Info.Author
	record.APIVersion = loaded.Info.ApiVersion
	record.Filename = new(filename)
	record.Source = new(source)
	record.Checksum = new(plugin.FileChecksum(wasmBytes))
	record.RequiredPermissions = domain.ParsePluginPermissions(loaded.Info.RequiredPermissions)

	// The previous build's failure no longer describes what is installed:
	// re-uploading is how an operator fixes a plugin stuck in status error.
	record.Status = domain.PluginStatusActive
	record.LastError = nil
	record.LastErrorAt = nil
}

// readInstalledBuild snapshots the file the plugin runs today so a failed
// replacement can put it back. A row whose file is missing or unreadable
// reports no previous build: there is nothing to restore, and repairing
// exactly that is one of the reasons to upload again.
func (h *Handler) readInstalledBuild(ctx context.Context, pluginPath string) ([]byte, bool) {
	if !h.fileManager.Exists(ctx, pluginPath) {
		return nil, false
	}

	previous, err := h.fileManager.Read(ctx, pluginPath)
	if err != nil {
		slog.WarnContext(ctx, "failed to read the installed plugin file; a failed update will not restore it",
			slog.String("path", pluginPath),
			slog.String("error", err.Error()))

		return nil, false
	}

	return previous, true
}

// restoreInstalledBuild undoes the file write of an update that could not be
// completed. Best effort: the request is already reported as failed, and a
// restore that fails leaves a file that does not match its row — which the
// operator repairs by uploading again.
func (h *Handler) restoreInstalledBuild(ctx context.Context, pluginPath string, previous []byte, hadPrevious bool) {
	var err error

	switch {
	case hadPrevious:
		err = h.fileManager.Write(ctx, pluginPath, previous)
	case h.fileManager.Exists(ctx, pluginPath):
		err = h.fileManager.Delete(ctx, pluginPath)
	}

	if err != nil {
		slog.ErrorContext(ctx, "failed to restore the plugin file after a failed update; "+
			"the file no longer matches the plugin record, upload the plugin again",
			slog.String("path", pluginPath),
			slog.String("error", err.Error()))
	}
}
