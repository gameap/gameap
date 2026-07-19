package getpluginicon

import (
	"net/http"
	"strconv"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagerhttp"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

type Handler struct {
	storeService *pluginstore.Service
	responder    base.Responder
}

func NewHandler(storeService *pluginstore.Service, responder base.Responder) *Handler {
	return &Handler{
		storeService: storeService,
		responder:    responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pluginID, err := api.NewInputReader(r).ReadString("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read plugin ID"))

		return
	}

	icon, err := h.storeService.GetPluginIcon(ctx, pluginID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to get plugin icon from store"))

		return
	}

	filemanagerhttp.SafeContentHeaders(rw.Header(), pluginID+"-icon", icon.ContentType)
	rw.Header().Set("Cache-Control", "private, max-age=3600")
	rw.Header().Set("Content-Length", strconv.Itoa(len(icon.Data)))
	rw.WriteHeader(http.StatusOK)
	//nolint:gosec // G705: SafeContentHeaders restricts Content-Type to inert
	// types (or forces an attachment) and sets nosniff + CSP sandbox, so the
	// proxied bytes cannot execute in the panel origin.
	_, _ = rw.Write(icon.Data)
}
