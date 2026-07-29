package languages

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/gameap/gameap/internal/i18n"
)

// Handler serves the list of available UI locales (with display labels) read
// from the i18n filesystem, so the frontend can build its language switcher
// without a hardcoded list.
type Handler struct {
	fsys fs.FS
}

func NewHandler(fsys fs.FS) *Handler {
	return &Handler{fsys: fsys}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	languages, err := i18n.ListLanguages(h.fsys)
	if err != nil {
		slog.Error("languages: failed to list", slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	if err := json.NewEncoder(w).Encode(languages); err != nil {
		slog.Error("languages: failed to encode response", slog.String("error", err.Error()))
	}
}
