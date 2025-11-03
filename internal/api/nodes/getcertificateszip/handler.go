package getcertificateszip

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/certificates"
	"github.com/pkg/errors"
)

const (
	readmeContent = `* Move this files to certs directory (For linux: /etc/gameap-daemon/certs/)
* Edit gameap-daemon configuration, set ` + "`ca_certificate_file`, `certificate_chain_file` and `private_key_file`"
	privateKeyBits = 2048
)

type Handler struct {
	certificatesSvc *certificates.Service
	responder       base.Responder
}

func NewHandler(
	certificatesSvc *certificates.Service,
	responder base.Responder,
) *Handler {
	return &Handler{
		certificatesSvc: certificatesSvc,
		responder:       responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	zipData, err := h.generateCertificatesZip(ctx)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate certificates zip"))

		return
	}

	rw.Header().Set("Content-Type", "application/zip")
	rw.Header().Set("Content-Disposition", "attachment; filename=\"certificates.zip\"")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(zipData)
}

func (h *Handler) generateCertificatesZip(ctx context.Context) ([]byte, error) {
	rootCert, err := h.certificatesSvc.Root(ctx)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get root certificate")
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, privateKeyBits)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate private key")
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "GameAP",
			Organization: []string{"GameAP"},
		},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, privateKey)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create CSR")
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	serverCert, err := h.certificatesSvc.Sign(ctx, string(csrPEM), nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to sign certificate")
	}

	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	files := map[string]string{
		"ca.crt":     rootCert,
		"server.key": string(privateKeyPEM),
		"server.crt": serverCert,
		"README.md":  readmeContent,
	}

	for filename, content := range files {
		fileWriter, err := zipWriter.Create(filename)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to create zip file entry")
		}

		_, err = fileWriter.Write([]byte(content))
		if err != nil {
			return nil, errors.WithMessage(err, "failed to write to zip file")
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, errors.WithMessage(err, "failed to close zip writer")
	}

	return buf.Bytes(), nil
}
