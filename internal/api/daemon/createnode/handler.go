package createnode

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	daemonbase "github.com/gameap/gameap/internal/api/daemon/base"
	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
	"github.com/rs/xid"
)

const (
	maxFileSize  = 10 * 1024 * 1024
	apiKeyLength = 64
)

var (
	ErrInvalidToken               = errors.New("invalid token")
	ErrGdaemonServerCertRequired  = errors.New("gdaemon_server_cert is required")
	ErrFailedToGetRootCertificate = errors.New("failed to get root certificate")
)

type Handler struct {
	cache                 cache.Cache
	nodesRepo             repositories.NodeRepository
	clientCertificateRepo repositories.ClientCertificateRepository
	certificatesSvc       *certificates.Service
	responder             base.Responder
}

func NewHandler(
	cache cache.Cache,
	nodesRepo repositories.NodeRepository,
	clientCertificateRepo repositories.ClientCertificateRepository,
	certificatesSvc *certificates.Service,
	responder base.Responder,
) *Handler {
	return &Handler{
		cache:                 cache,
		nodesRepo:             nodesRepo,
		clientCertificateRepo: clientCertificateRepo,
		certificatesSvc:       certificatesSvc,
		responder:             responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	inputReader := api.NewInputReader(r)
	token, err := inputReader.ReadString("token")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid token"),
			http.StatusBadRequest,
		))

		return
	}

	if err := h.verifyCreateToken(ctx, token); err != nil {
		if errors.Is(err, ErrInvalidToken) || errors.Is(err, cache.ErrNotFound) {
			h.responder.WriteError(ctx, rw, api.WrapHTTPError(
				errors.WithMessage(err, "invalid create token"),
				http.StatusUnauthorized,
			))

			return
		}

		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to verify create token"))

		return
	}

	err = r.ParseMultipartForm(maxFileSize)
	if err != nil {
		slog.ErrorContext(
			ctx,
			"failed to parse multipart form",
			slog.String("error", err.Error()),
			slog.String("content_type", r.Header.Get("Content-Type")),
			slog.Int64("content_length", r.ContentLength),
		)
		h.responder.WriteError(ctx, rw,
			api.WrapHTTPError(
				errors.WithMessage(err, "failed to parse multipart form"),
				http.StatusBadRequest,
			),
		)

		return
	}

	input := newNodeInputFromRequest(r)

	if err := input.Validate(); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "validation failed"))

		return
	}

	node, signedCert, err := h.createNode(ctx, input)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to create node"))

		return
	}

	if err := h.cache.Delete(ctx, daemonbase.AutoCreateTokenCacheKey); err != nil {
		slog.Warn(fmt.Sprintf("failed to delete create token from cache: %v", err))
	}

	rootCert, err := h.certificatesSvc.Root(ctx)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(ErrFailedToGetRootCertificate, err.Error()))

		return
	}

	response := buildCreateResponse(node.ID, node.GdaemonAPIKey, rootCert, signedCert)

	rw.Header().Set("Content-Type", "text/plain")
	_, _ = rw.Write([]byte(response))
}

func (h *Handler) verifyCreateToken(ctx context.Context, token string) error {
	val, err := h.cache.Get(ctx, daemonbase.AutoCreateTokenCacheKey)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			return ErrInvalidToken
		}

		return errors.WithMessage(err, "failed to get create token from cache")
	}

	if val == nil {
		return ErrInvalidToken
	}

	storedToken, ok := val.(string)
	if !ok {
		return errors.New("invalid create token type in cache")
	}

	if token != storedToken {
		return ErrInvalidToken
	}

	return nil
}

func (h *Handler) createNode(ctx context.Context, input *nodeInput) (*domain.Node, string, error) {
	csr, err := h.readCSR(input.GdaemonServerCert)
	if err != nil {
		return nil, "", errors.WithMessage(err, "failed to read CSR")
	}

	signedCert, err := h.certificatesSvc.Sign(ctx, csr, nil)
	if err != nil {
		return nil, "", errors.WithMessage(err, "failed to sign certificate")
	}

	apiKey, err := strings.CryptoRandomString(apiKeyLength)
	if err != nil {
		return nil, "", errors.WithMessage(err, "failed to generate api key")
	}

	node := input.ToDomain(apiKey, certificates.RootCACert)

	node.ClientCertificateID, err = h.getClientCertificateID(ctx)
	if err != nil {
		return nil, "", errors.WithMessage(err, "failed to get client certificate ID")
	}

	now := time.Now()
	node.CreatedAt = &now
	node.UpdatedAt = &now

	if err := h.nodesRepo.Save(ctx, node); err != nil {
		return nil, "", errors.WithMessage(err, "failed to save node")
	}

	return node, signedCert, nil
}

func (h *Handler) getClientCertificateID(ctx context.Context) (uint, error) {
	certs, err := h.clientCertificateRepo.Find(
		ctx,
		nil,
		nil,
		&filters.Pagination{
			Limit: 1,
		},
	)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to find client certificates")
	}

	if len(certs) > 0 {
		return certs[0].ID, nil
	}

	certName := xid.New().String()

	certPath := filepath.Join(certificates.ClientCertificatesPath, certName+".crt")
	keyPath := filepath.Join(certificates.ClientCertificatesPath, certName+".key")

	// Create a new client certificate if none exist
	clientCert, _, err := h.certificatesSvc.Generate(ctx, certPath, keyPath, nil)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to generate client certificate")
	}

	// Fingerprint the certificate
	fingerprint, err := h.certificatesSvc.Fingerprint(clientCert)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to fingerprint client certificate")
	}

	clientCertificate := domain.ClientCertificate{
		Certificate: certPath,
		PrivateKey:  keyPath,
		Fingerprint: fingerprint,
		Expires:     time.Now().Add(certificates.CertYears * 365 * 24 * time.Hour),
	}

	if err := h.clientCertificateRepo.Save(ctx, &clientCertificate); err != nil {
		return 0, errors.WithMessage(err, "failed to save client certificate")
	}

	return clientCertificate.ID, nil
}

func (h *Handler) readCSR(fileHeaders []*multipart.FileHeader) (string, error) {
	if len(fileHeaders) == 0 {
		return "", ErrGdaemonServerCertRequired
	}

	file, err := fileHeaders[0].Open()
	if err != nil {
		return "", errors.WithMessage(err, "failed to open certificate file")
	}
	defer func(f multipart.File) {
		if closeErr := f.Close(); closeErr != nil {
			slog.Warn(fmt.Sprintf("failed to close certificate file: %v", closeErr))
		}
	}(file)

	data, err := io.ReadAll(file)
	if err != nil {
		return "", errors.WithMessage(err, "failed to read certificate file")
	}

	return string(data), nil
}
