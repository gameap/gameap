package upgradegames

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
)

type GameUpgrader interface {
	UpgradeGames(ctx context.Context) error
}

type Handler struct {
	upgrader  GameUpgrader
	responder base.Responder
}

func NewHandler(
	upgrader GameUpgrader,
	responder base.Responder,
) *Handler {
	return &Handler{
		upgrader:  upgrader,
		responder: responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	err := h.upgrader.UpgradeGames(ctx)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, base.Success)
}
