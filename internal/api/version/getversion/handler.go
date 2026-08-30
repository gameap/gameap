package getversion

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/application/defaults"
	"github.com/gameap/gameap/internal/services/releases"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	releases  releasesService
	responder base.Responder
}

func NewHandler(releasesService releasesService, responder base.Responder) *Handler {
	return &Handler{
		releases:  releasesService,
		responder: responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := auth.SessionFromContext(ctx)
	if !session.IsAuthenticated() {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user not authenticated"),
			http.StatusUnauthorized,
		))

		return
	}

	h.responder.Write(ctx, rw, newVersionResponse(
		defaults.Version,
		defaults.BuildDate,
		h.latest(ctx, releases.ComponentPanel),
		h.latest(ctx, releases.ComponentDaemon),
		h.releases.Enabled(),
	))
}

// latest never fails the request: an unreachable release source only means the
// dashboard shows the installed versions without an update notice.
func (h *Handler) latest(ctx context.Context, component releases.Component) releases.Info {
	info, err := h.releases.Latest(ctx, component)
	if err != nil {
		slog.WarnContext(ctx, "failed to resolve latest release",
			"component", component, "error", err,
		)

		return releases.Info{}
	}

	return info
}
