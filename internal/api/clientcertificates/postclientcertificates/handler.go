package postclientcertificates

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
	"github.com/rs/xid"
)

const (
	maxFileSize         = 10 * 1024 * 1024
	certificatesPath    = certificates.ClientCertificatesPath
	fieldCertificate    = "certificate"
	fieldPrivateKey     = "private_key"
	fieldPrivateKeyPass = "private_key_pass"
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

	err := r.ParseMultipartForm(maxFileSize)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to parse multipart form"))

		return
	}

	input := &clientCertificateInput{
		CertificateFile: r.MultipartForm.File[fieldCertificate],
		PrivateKeyFile:  r.MultipartForm.File[fieldPrivateKey],
		PrivateKeyPass:  r.FormValue(fieldPrivateKeyPass),
	}

	if err := input.Validate(); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "validation failed"))

		return
	}

	cert, err := h.saveCertificate(ctx, input)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save certificate"))

		return
	}

	response := newClientCertificateResponseFromClientCertificate(cert)
	rw.WriteHeader(http.StatusCreated)
	h.responder.Write(ctx, rw, response)
}

func (h *Handler) saveCertificate(
	ctx context.Context,
	input *clientCertificateInput,
) (*domain.ClientCertificate, error) {
	certFile, err := input.CertificateFile[0].Open()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to open certificate file")
	}
	defer func(certFile multipart.File) {
		err := certFile.Close()
		if err != nil {
			slog.Warn(fmt.Sprintf("failed to close certificate file: %v", err))
		}
	}(certFile)

	certData, err := io.ReadAll(certFile)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read certificate file")
	}

	keyFile, err := input.PrivateKeyFile[0].Open()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to open private key file")
	}
	defer func(keyFile multipart.File) {
		err := keyFile.Close()
		if err != nil {
			slog.Warn(fmt.Sprintf("failed to close private key file: %v", err))
		}
	}(keyFile)

	keyData, err := io.ReadAll(keyFile)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read private key file")
	}

	if err := h.validateCertificateAndKey(certData, keyData); err != nil {
		return nil, api.WrapHTTPError(
			errors.WithMessage(err, "certificate and private key validation failed"),
			http.StatusBadRequest,
		)
	}

	certInfo, err := h.parseCertificate(certData)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse certificate")
	}

	fileXID := xid.New().String()

	certPath := filepath.Join(certificatesPath, fileXID+".crt")
	keyPath := filepath.Join(certificatesPath, fileXID+".key")

	if err := h.fileManager.Write(ctx, certPath, certData); err != nil {
		return nil, errors.WithMessage(err, "failed to write certificate file")
	}

	if err := h.fileManager.Write(ctx, keyPath, keyData); err != nil {
		deleteErr := h.fileManager.Delete(ctx, certPath)
		if deleteErr != nil {
			slog.Warn(fmt.Sprintf("failed to delete certificate file after private key write failure: %v", deleteErr))
		}

		return nil, errors.WithMessage(err, "failed to write private key file")
	}

	fingerprint := h.calculateFingerprint(certData)

	cert := &domain.ClientCertificate{
		Fingerprint: fingerprint,
		Expires:     certInfo.NotAfter,
		Certificate: certPath,
		PrivateKey:  keyPath,
	}

	if err := h.repo.Save(ctx, cert); err != nil {
		_ = h.fileManager.Delete(ctx, certPath)
		_ = h.fileManager.Delete(ctx, keyPath)

		return nil, errors.WithMessage(err, "failed to save certificate to repository")
	}

	return cert, nil
}

func (h *Handler) validateCertificateAndKey(certData, keyData []byte) error {
	certBlock, _ := pem.Decode(certData)
	if certBlock == nil {
		return errors.New("failed to decode certificate PEM")
	}

	_, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return errors.WithMessage(err, "failed to parse certificate")
	}

	keyBlock, _ := pem.Decode(keyData)
	if keyBlock == nil {
		return errors.New("failed to decode private key PEM")
	}

	if keyBlock.Type != "PRIVATE KEY" &&
		keyBlock.Type != "RSA PRIVATE KEY" &&
		keyBlock.Type != "EC PRIVATE KEY" {
		return errors.Errorf(
			"invalid PEM block type: %s, expected PRIVATE KEY, RSA PRIVATE KEY, or EC PRIVATE KEY",
			keyBlock.Type,
		)
	}

	return nil
}

func (h *Handler) parseCertificate(certData []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certData)
	if block == nil {
		return nil, errors.New("failed to decode PEM block containing the certificate")
	}
	if block.Type != "CERTIFICATE" {
		return nil, errors.Errorf("invalid PEM block type: %s, expected CERTIFICATE", block.Type)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse certificate")
	}

	return cert, nil
}

func (h *Handler) calculateFingerprint(certData []byte) string {
	block, _ := pem.Decode(certData)
	hash := sha256.Sum256(block.Bytes)
	hexStr := hex.EncodeToString(hash[:])

	parts := make([]string, 0, 32)
	for i := 0; i < len(hexStr); i += 2 {
		parts = append(parts, hexStr[i:i+2])
	}

	return strings.ToUpper(strings.Join(parts, ":"))
}
