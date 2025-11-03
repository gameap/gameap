package gethealth

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
)

type Responder interface {
	WriteError(ctx context.Context, rw http.ResponseWriter, err error)
	Write(ctx context.Context, rw http.ResponseWriter, result any)
}

type Handler struct {
	db        *sql.DB
	responder Responder
}

func NewGetHealthHandler(db *sql.DB, responder Responder) *Handler {
	return &Handler{
		db:        db,
		responder: responder,
	}
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	ctx := request.Context()

	err := h.db.PingContext(ctx)
	if err != nil {
		h.responder.WriteError(ctx, writer, err)
	}

	h.responder.Write(ctx, writer, base.Success)
}
