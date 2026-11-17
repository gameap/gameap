package completesession

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/api/filemanager/uploadsession"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	resolver  *uploadsession.Resolver
	service   uploadsession.Service
	responder base.Responder
}

func NewHandler(
	resolver *uploadsession.Resolver,
	service uploadsession.Service,
	responder base.Responder,
) *Handler {
	return &Handler{
		resolver:  resolver,
		service:   service,
		responder: responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := auth.SessionFromContext(ctx)
	if !session.IsAuthenticated() {
		h.responder.WriteError(ctx, rw, uploadsession.ErrUserNotAuthenticated)

		return
	}

	reader := api.NewInputReader(r)
	serverID, err := reader.ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}
	uploadID, _ := reader.ReadString("uploadID")
	if uploadID == "" {
		h.responder.WriteError(ctx, rw, uploadsession.ErrUploadIDRequired)

		return
	}

	resolved, err := h.resolver.Resolve(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// The session carries the path it was created for. Confining the dispatch to
	// the server of this request also refuses a session started for another one.
	ctx = daemon.ConfineToServer(ctx, resolved.Node, resolved.Server)

	if completeErr := h.service.Complete(
		ctx, uploadID, session.User.ID, resolved.Node,
	); completeErr != nil {
		h.responder.WriteError(ctx, rw, completeErr)

		return
	}

	h.responder.Write(ctx, rw, &Response{
		UploadID:  uploadID,
		Completed: true,
	})
}
