package createsession

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/api/filemanager/uploadsession"
	"github.com/gameap/gameap/internal/upload"
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

	serverID, err := api.NewInputReader(r).ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	input := &Input{}
	if decodeErr := json.NewDecoder(r.Body).Decode(input); decodeErr != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(decodeErr, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	if validateErr := input.Validate(); validateErr != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(validateErr, http.StatusBadRequest))

		return
	}

	resolved, err := h.resolver.Resolve(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	fullPath := filepath.Join(resolved.Node.WorkPath, resolved.Server.Dir, input.Path, input.Filename)

	var suUser string
	if resolved.Server.SuUser != nil {
		suUser = *resolved.Server.SuUser
	}

	sess, err := h.service.Create(ctx, upload.CreateParams{
		ServerID:         resolved.Server.ID,
		NodeID:           resolved.Node.ID,
		UserID:           session.User.ID,
		FullPath:         fullPath,
		SuUser:           suUser,
		TotalSize:        input.TotalSize,
		ExpectedChecksum: input.ExpectedChecksum,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	rw.WriteHeader(http.StatusCreated)
	h.responder.Write(ctx, rw, newResponse(sess))
}
