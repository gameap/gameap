package postnode

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
	"github.com/rs/xid"
)

const (
	apiKeyLength     = 32
	certificatesPath = "certs"
)

var (
	ErrFailedToGenerateAPIKey  = errors.New("failed to generate API key")
	ErrFailedToSaveCertificate = errors.New("failed to save certificate")
	ErrFailedToSaveNode        = errors.New("failed to save node")
)

type Handler struct {
	repo        repositories.NodeRepository
	fileManager files.FileManager
	responder   base.Responder
}

func NewHandler(
	repo repositories.NodeRepository,
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

	input := &createDedicatedServerInput{}

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "invalid request"))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "validation failed"))

		return
	}

	node, err := h.createNode(ctx, input)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to create dedicated server"))

		return
	}

	response := newDedicatedServerResponse(node)
	rw.WriteHeader(http.StatusCreated)
	h.responder.Write(ctx, rw, response)
}

func (h *Handler) createNode(ctx context.Context, input *createDedicatedServerInput) (*domain.Node, error) {
	if input.GdaemonServerCert == "" {
		return nil, ErrCertificateRequired
	}

	certXID := xid.New().String()
	certPath := filepath.Join(certificatesPath, certXID+".crt")

	if err := h.fileManager.Write(ctx, certPath, []byte(input.GdaemonServerCert)); err != nil {
		return nil, errors.WithMessage(ErrFailedToSaveCertificate, err.Error())
	}

	apiKey, err := strings.CryptoRandomString(apiKeyLength)
	if err != nil {
		_ = h.fileManager.Delete(ctx, certPath)

		return nil, errors.WithMessage(ErrFailedToGenerateAPIKey, err.Error())
	}

	node := input.ToDomain(apiKey, certPath)

	now := time.Now()
	node.CreatedAt = &now
	node.UpdatedAt = &now

	if err := h.repo.Save(ctx, node); err != nil {
		_ = h.fileManager.Delete(ctx, certPath)

		return nil, errors.WithMessage(ErrFailedToSaveNode, err.Error())
	}

	return node, nil
}
