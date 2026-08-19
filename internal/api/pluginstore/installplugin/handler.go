package installplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"path"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
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
	pluginsDir    string
	responder     base.Responder
}

func NewHandler(
	storeService *pluginstore.Service,
	pluginRepo repositories.PluginRepository,
	fileManager files.FileManager,
	loader *plugin.Loader,
	subscriptions plugininstall.SubscriptionRefresher,
	pluginsDir string,
	responder base.Responder,
) *Handler {
	return &Handler{
		storeService:  storeService,
		pluginRepo:    pluginRepo,
		fileManager:   fileManager,
		loader:        loader,
		subscriptions: subscriptions,
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

	if err := plugininstall.CheckNotInstalled(ctx, h.pluginRepo, dbID); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	lang := pluginstore.ExtractLanguage(r)

	pluginDetails, err := h.storeService.GetPlugin(ctx, storePluginID, lang)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to get plugin details"))

		return
	}

	if err := h.checkSubscription(ctx, pluginDetails); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

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

	pluginRecord := h.buildPluginRecord(dbID, pluginDetails, selectedVersion, filename, storePluginID)

	if err := h.pluginRepo.Save(ctx, pluginRecord); err != nil {
		_ = h.fileManager.Delete(ctx, pluginPath)
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save plugin record"))

		return
	}

	if err := h.loadAndGrant(ctx, pluginRecord, filename, wasmBytes); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	plugininstall.RefreshSubscriptions(ctx, h.subscriptions)

	h.responder.Write(ctx, rw, newInstallResponse(pluginRecord))
}

// loadAndGrant starts the installed module and records the grants its manifest
// asks for. Both failures leave the plugin installed but unusable, so the
// returned error is already HTTP-wrapped for the caller to report.
func (h *Handler) loadAndGrant(
	ctx context.Context,
	pluginRecord *domain.Plugin,
	filename string,
	wasmBytes []byte,
) error {
	loaded, err := plugininstall.TryLoadPlugin(ctx, h.loader, h.pluginRepo, pluginRecord, filename)
	if err != nil {
		wasmHash := sha256.Sum256(wasmBytes)

		slog.WarnContext(
			ctx,
			"failed to load wasm file",
			slog.String("wasm_hash", hex.EncodeToString(wasmHash[:])),
			slog.String("error", err.Error()),
		)

		return api.WrapHTTPError(
			errors.WithMessage(err, "plugin installed but failed to load"),
			http.StatusUnprocessableEntity,
		)
	}

	if err := h.recordPermissions(ctx, pluginRecord, loaded); err != nil {
		h.disableUngrantedPlugin(ctx, pluginRecord, loaded)

		return api.WrapHTTPError(
			errors.WithMessage(err, "plugin installed but its permissions were not recorded"),
			http.StatusUnprocessableEntity,
		)
	}

	return nil
}

// recordPermissions persists the grants the module asks for. The store API
// carries no manifest, so required_permissions is only known once the wasm is
// loaded — without this step a store-installed plugin holds no grants at all
// and every gated host module (secrets, files, manage_rbac) denies it.
// Installing is an admin action, so confirming the install is the grant, same
// rule as the upload path (plugininstall.BuildPluginRecord).
func (h *Handler) recordPermissions(
	ctx context.Context,
	pluginRecord *domain.Plugin,
	loaded *pkgplugin.LoadedPlugin,
) error {
	if loaded == nil || loaded.Info == nil {
		return nil
	}

	permissions := domain.ParsePluginPermissions(loaded.Info.RequiredPermissions)
	if len(permissions) == 0 {
		return nil
	}

	pluginRecord.RequiredPermissions = permissions
	pluginRecord.AllowedPermissions = permissions

	return h.pluginRepo.Save(ctx, pluginRecord)
}

// disableUngrantedPlugin stops a module whose grants could not be persisted.
// Left running it would hit a denial in every gated host library, and the
// still-active database record would load it that way again after a restart.
// Both steps are best effort: the install itself is already reported as failed.
func (h *Handler) disableUngrantedPlugin(
	ctx context.Context,
	pluginRecord *domain.Plugin,
	loaded *pkgplugin.LoadedPlugin,
) {
	if h.loader != nil && loaded != nil && loaded.Info != nil {
		if err := h.loader.Unload(ctx, loaded.Info.Id); err != nil {
			slog.WarnContext(ctx, "failed to unload plugin without recorded permissions",
				slog.String("plugin", loaded.Info.Id),
				slog.String("error", err.Error()))
		}
	}

	pluginRecord.Status = domain.PluginStatusError

	if err := h.pluginRepo.Save(ctx, pluginRecord); err != nil {
		slog.WarnContext(ctx, "failed to mark plugin as errored",
			slog.Uint64("plugin_id", uint64(pluginRecord.ID)),
			slog.String("error", err.Error()))
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

func (h *Handler) checkSubscription(ctx context.Context, details *pluginstore.PluginDetails) error {
	if !details.RequiresSubscription {
		return nil
	}

	if !h.storeService.HasLicenseKey() {
		return api.WrapHTTPError(
			errors.New("this plugin requires a subscription"),
			http.StatusPaymentRequired,
		)
	}

	licenseValidation, err := h.storeService.ValidateLicense(ctx)
	if err != nil {
		return api.WrapHTTPError(
			errors.New("this plugin requires a subscription"),
			http.StatusPaymentRequired,
		)
	}

	if licenseValidation == nil || !licenseValidation.Valid {
		return api.WrapHTTPError(
			errors.New("this plugin requires a subscription"),
			http.StatusPaymentRequired,
		)
	}

	for _, sub := range licenseValidation.Subscriptions {
		if sub.PluginID == details.ID {
			return nil
		}
	}

	return api.WrapHTTPError(
		errors.New("no active subscription for this plugin"),
		http.StatusPaymentRequired,
	)
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

func (h *Handler) buildPluginRecord(
	dbID domain.Uint64ID,
	details *pluginstore.PluginDetails,
	version *pluginstore.PluginVersion,
	filename string,
	storePluginID string,
) *domain.Plugin {
	source := h.storeService.BaseURL() + "/plugins/" + storePluginID

	return &domain.Plugin{
		ID:          dbID,
		Name:        details.Name,
		Version:     version.Version,
		Description: details.Description,
		Author:      details.Author.Username,
		Filename:    new(filename),
		Source:      new(source),
		Status:      domain.PluginStatusActive,
		InstalledAt: new(time.Now()),
	}
}
