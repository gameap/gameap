package importgameap

import (
	"context"
	"io"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/domain/gamesimport"
	"github.com/gameap/gameap/internal/services/gameapimporter"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

const maxBodySize = 1 << 20 // 1 MB

type GameAPImporter interface {
	Import(
		ctx context.Context,
		export *domain.GameExport,
		opts *gamesimport.Options,
	) (*gameapimporter.ImportResult, error)
}

type Handler struct {
	importer  GameAPImporter
	responder base.Responder
}

func NewHandler(importer GameAPImporter, responder base.Responder) *Handler {
	return &Handler{
		importer:  importer,
		responder: responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	inp, err := readInput(r)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	opts := inp.toImportOptions()
	if err := opts.Validate(); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	r.Body = http.MaxBytesReader(rw, r.Body, maxBodySize)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.responder.WriteError(ctx, rw, api.NewError(http.StatusBadRequest, "request body too large, maximum 1 MB allowed"))

			return
		}

		wrappedErr := errors.Wrap(err, "failed to read request body")
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(wrappedErr, http.StatusBadRequest))

		return
	}

	if len(body) == 0 {
		h.responder.WriteError(ctx, rw, api.NewError(http.StatusBadRequest, "request body is empty"))

		return
	}

	export, err := domain.ParseGameExport(body)
	if err != nil {
		wrappedErr := errors.WithMessage(err, "failed to parse GameAP YAML")
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(wrappedErr, http.StatusBadRequest))

		return
	}

	// Validating here rather than only inside the importer keeps a malformed file
	// a 422 instead of the 500 an unwrapped importer error would produce.
	if err := export.Validate(); err != nil {
		h.responder.WriteError(ctx, rw, api.NewValidationError(err.Error()))

		return
	}

	result, err := h.importer.Import(ctx, export, opts)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to import GameAP config"))

		return
	}

	h.responder.Write(ctx, rw, Response{
		GameCode:     result.Game.Code,
		GameName:     result.Game.Name,
		ModsImported: len(result.ModsCreated) + len(result.ModsUpdated),
		ModsCreated:  result.ModsCreated,
		ModsUpdated:  result.ModsUpdated,
	})
}

type Response struct {
	GameCode     string   `json:"game_code"`
	GameName     string   `json:"game_name"`
	ModsImported int      `json:"mods_imported"`
	ModsCreated  []string `json:"mods_created"`
	ModsUpdated  []string `json:"mods_updated"`
}
