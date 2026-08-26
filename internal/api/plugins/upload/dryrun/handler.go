package dryrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
)

type LoaderManager interface {
	LoadTransient(
		ctx context.Context, wasmBytes []byte, config map[string]string, pluginID uint64,
	) (*pkgplugin.LoadedPlugin, error)
}

type Handler struct {
	manager    LoaderManager
	pluginRepo repositories.PluginRepository
	responder  base.Responder
}

func NewHandler(
	manager LoaderManager,
	pluginRepo repositories.PluginRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		manager:    manager,
		pluginRepo: pluginRepo,
		responder:  responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	wasmBytes, err := plugininstall.ReadWASMFromMultipart(rw, r)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if err := plugininstall.ValidateWASM(wasmBytes); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	loaded, err := h.manager.LoadTransient(ctx, wasmBytes, nil, 0)
	if err != nil {
		wasmHash := sha256.Sum256(wasmBytes)

		slog.WarnContext(
			ctx,
			"failed to load wasm file",
			slog.String("wasm_hash", hex.EncodeToString(wasmHash[:])),
			slog.String("error", err.Error()),
		)

		h.responder.WriteError(ctx, rw, api.WrapHTTPErrorWithTitle(
			pkgplugin.SanitizeLoadError(err),
			http.StatusBadRequest,
			"plugins.validation_failed_title",
		))

		return
	}
	defer func() {
		if err := loaded.Close(ctx); err != nil {
			slog.ErrorContext(
				ctx,
				"failed to close transient plugin",
				slog.String("error", err.Error()),
				slog.String("plugin_id", loaded.Info.Id),
			)
		}
	}()

	subscribedEvents := h.getSubscribedEvents(ctx, loaded)

	h.responder.Write(ctx, rw, newDryRunResponse(loaded, subscribedEvents, h.findInstalled(ctx, loaded)))
}

// findInstalled reports the plugin already installed under the uploaded
// module's id, so the caller knows before confirming that the file replaces a
// running plugin rather than adding one. A repository failure is not fatal
// for a validation request: the file is still reported as valid, and the
// install endpoint refuses the conflict on its own.
func (h *Handler) findInstalled(ctx context.Context, loaded *pkgplugin.LoadedPlugin) *domain.Plugin {
	if h.pluginRepo == nil {
		return nil
	}

	dbID := pkgplugin.ParsePluginID(loaded.Info.Id)

	installed, err := h.pluginRepo.Find(ctx, filters.FindPluginByIDs(dbID), nil, nil)
	if err != nil {
		slog.WarnContext(ctx, "failed to check whether the plugin is already installed",
			slog.String("plugin_id", loaded.Info.Id),
			slog.String("error", err.Error()))

		return nil
	}

	if len(installed) == 0 {
		return nil
	}

	return &installed[0]
}

func (h *Handler) getSubscribedEvents(ctx context.Context, loaded *pkgplugin.LoadedPlugin) []proto.EventType {
	resp, err := loaded.Instance.GetSubscribedEvents(ctx, &proto.GetSubscribedEventsRequest{})
	if err != nil {
		return nil
	}

	return resp.Events
}
