package install

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

const extendedWriteDeadline = 5 * time.Minute

type LoaderManager interface {
	Load(ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64) (*pkgplugin.LoadedPlugin, error)
	Unload(ctx context.Context, pluginID string) error
}

type Handler struct {
	manager     LoaderManager
	pluginRepo  repositories.PluginRepository
	fileManager files.FileManager
	loader      *plugin.Loader
	pluginsDir  string
	responder   base.Responder
	audit       audit.Logger
}

func NewHandler(
	manager LoaderManager,
	pluginRepo repositories.PluginRepository,
	fileManager files.FileManager,
	loader *plugin.Loader,
	pluginsDir string,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		manager:     manager,
		pluginRepo:  pluginRepo,
		fileManager: fileManager,
		loader:      loader,
		pluginsDir:  pluginsDir,
		responder:   responder,
		audit:       auditLogger,
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

	loaded, err := h.manager.Load(ctx, wasmBytes, nil, 0)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to load plugin for validation"))

		return
	}

	pluginID := pkgplugin.CompactPluginID(pkgplugin.ParsePluginID(loaded.Info.Id))
	dbID := pkgplugin.ParsePluginID(loaded.Info.Id)

	if err := h.manager.Unload(ctx, pluginID); err != nil {
		slog.WarnContext(ctx, "failed to unload temporary plugin",
			slog.String("plugin_id", pluginID),
			slog.String("error", err.Error()))
	}

	if err := plugininstall.CheckNotInstalled(ctx, h.pluginRepo, dbID); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	filename := strconv.FormatUint(uint64(dbID), 10) + ".wasm"
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

	if err := plugininstall.TryLoadPlugin(ctx, h.loader, h.pluginRepo, pluginRecord, filename); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "plugin installed but failed to load"),
			http.StatusUnprocessableEntity,
		))

		return
	}

	h.responder.Write(ctx, rw, newInstallResponse(pluginRecord))
}
