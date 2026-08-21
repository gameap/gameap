package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

const extendedWriteDeadline = 5 * time.Minute

// installFailedTitle is a translation key the frontend resolves; without it a
// rejected upload is reported under the raw HTTP status text.
const installFailedTitle = "plugins.installation_failed_title"

type LoaderManager interface {
	LoadTransient(
		ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64,
	) (*pkgplugin.LoadedPlugin, error)
}

type Handler struct {
	manager       LoaderManager
	pluginRepo    repositories.PluginRepository
	fileManager   files.FileManager
	loader        *plugin.Loader
	subscriptions plugininstall.SubscriptionRefresher
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

	wasmBytes, err := plugininstall.ReadWASMFromMultipart(rw, r)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if err := plugininstall.ValidateWASM(wasmBytes); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// Read after ReadWASMFromMultipart: it is what parses the form.
	updateRequested, err := api.NewFormReader(r).ReadBool("update")
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read update flag"))

		return
	}

	// The transient load compiles and initializes the uploaded build before
	// anything on disk or in the database is touched, so a broken upload can
	// never take down the running plugin.
	loaded, err := h.manager.LoadTransient(ctx, wasmBytes, nil, 0)
	if err != nil {
		wasmHash := sha256.Sum256(wasmBytes)

		slog.WarnContext(ctx, "failed to load uploaded wasm file",
			slog.String("wasm_hash", hex.EncodeToString(wasmHash[:])),
			slog.String("error", err.Error()))

		// A build that does not compile is the caller's problem, reported the
		// same way the dry-run endpoint reports it — a bare 500 hides both the
		// status and the reason from whoever uploaded it.
		h.responder.WriteError(ctx, rw, api.WrapHTTPErrorWithTitle(
			pkgplugin.SanitizeLoadError(err),
			http.StatusBadRequest,
			"plugins.validation_failed_title",
		))

		return
	}
	defer func() {
		if err := loaded.Close(ctx); err != nil {
			slog.WarnContext(ctx, "failed to close transient plugin",
				slog.String("plugin_id", loaded.Info.Id),
				slog.String("error", err.Error()))
		}
	}()

	dbID := pkgplugin.ParsePluginID(loaded.Info.Id)

	installed, err := plugininstall.FindInstalled(ctx, h.pluginRepo, dbID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if installed != nil && !updateRequested {
		h.responder.WriteError(ctx, rw, api.WrapHTTPErrorWithTitle(
			plugininstall.ErrPluginAlreadyInstalled,
			http.StatusConflict,
			installFailedTitle,
		))

		return
	}

	if err := plugininstall.CheckNameAvailable(ctx, h.pluginRepo, dbID, loaded.Info.Name); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if installed != nil {
		h.update(ctx, rw, installed, loaded, wasmBytes, dbID)

		return
	}

	h.install(ctx, rw, loaded, wasmBytes, dbID)
}

func (h *Handler) install(
	ctx context.Context,
	rw http.ResponseWriter,
	loaded *pkgplugin.LoadedPlugin,
	wasmBytes []byte,
	dbID domain.Uint64ID,
) {
	filename := plugininstall.ResolvePluginFilename(nil, dbID)
	pluginPath := path.Join(h.pluginsDir, filename)

	if err := h.fileManager.Write(ctx, pluginPath, wasmBytes); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save plugin file"))

		return
	}

	pluginRecord := plugininstall.BuildPluginRecord(dbID, loaded, filename, "file://"+filename)

	if err := h.pluginRepo.Save(ctx, pluginRecord); err != nil {
		_ = h.fileManager.Delete(ctx, pluginPath)
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save plugin record"))

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventPluginInstall, audit.CategoryPluginOp,
		"plugin", strconv.FormatUint(uint64(dbID), 10), "install",
		slog.String("plugin", loaded.Info.Id))

	if _, err := plugininstall.TryLoadPlugin(ctx, h.loader, h.pluginRepo, pluginRecord, filename); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPErrorWithTitle(
			errors.WithMessage(err, "plugin installed but failed to load"),
			http.StatusUnprocessableEntity,
			installFailedTitle,
		))

		return
	}

	plugininstall.RefreshSubscriptions(ctx, h.subscriptions)

	h.responder.Write(ctx, rw, newInstallResponse(pluginRecord))
}
