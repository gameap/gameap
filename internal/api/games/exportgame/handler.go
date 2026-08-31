package exportgame

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

type GameExporter interface {
	Export(ctx context.Context, gameCode string) ([]byte, error)
}

type Handler struct {
	exporter  GameExporter
	responder base.Responder
}

func NewHandler(exporter GameExporter, responder base.Responder) *Handler {
	return &Handler{
		exporter:  exporter,
		responder: responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	reader := api.NewInputReader(r)
	gameCode, err := reader.ReadString("code")
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read game code"))

		return
	}

	if gameCode == "" {
		h.responder.WriteError(ctx, rw, api.NewError(http.StatusBadRequest, "game code is required"))

		return
	}

	yamlData, err := h.exporter.Export(ctx, gameCode)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to export game"))

		return
	}

	filename := gameCode + ".gameap.yaml"

	rw.Header().Set("Content-Type", "application/x-yaml")
	rw.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	rw.WriteHeader(http.StatusOK)

	if _, err := rw.Write(yamlData); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to write response"))
	}
}
