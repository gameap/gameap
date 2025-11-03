package deleteclientcertificates

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type Handler struct {
	repo        repositories.ClientCertificateRepository
	fileManager files.FileManager
	responder   base.Responder
}

func NewHandler(
	repo repositories.ClientCertificateRepository,
	fileManager files.FileManager,
	responder base.Responder,
) *Handler {
	return &Handler{
		repo:        repo,
		fileManager: fileManager,
		responder:   responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	idStr := vars["id"]

	if idStr == "" {
		h.responder.WriteError(ctx, rw, api.NewValidationError("client certificate id is required"))

		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.NewValidationError("invalid client certificate id"))

		return
	}

	if id <= 0 {
		h.responder.WriteError(ctx, rw, api.NewValidationError("invalid client certificate id"))

		return
	}

	cert, err := h.findCertificate(ctx, uint(id))
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find client certificate"))

		return
	}

	if cert != nil {
		h.deleteFiles(ctx, cert)
	}

	err = h.repo.Delete(ctx, uint(id))
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete client certificate"))

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func (h *Handler) findCertificate(ctx context.Context, id uint) (*domain.ClientCertificate, error) {
	certs, err := h.repo.Find(ctx, &filters.FindClientCertificate{
		IDs: []uint{id},
	}, nil, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to query certificate")
	}

	if len(certs) == 0 {
		return nil, nil
	}

	return &certs[0], nil
}

func (h *Handler) deleteFiles(ctx context.Context, cert *domain.ClientCertificate) {
	if cert.Certificate != "" {
		if err := h.fileManager.Delete(ctx, cert.Certificate); err != nil {
			slog.WarnContext(ctx, "failed to delete certificate file",
				"path", cert.Certificate,
				"error", err)
		}
	}

	if cert.PrivateKey != "" {
		if err := h.fileManager.Delete(ctx, cert.PrivateKey); err != nil {
			slog.WarnContext(ctx, "failed to delete private key file",
				"path", cert.PrivateKey,
				"error", err)
		}
	}
}
