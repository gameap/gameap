package getclientcertificates

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
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

	certificates, err := h.repo.FindAll(ctx, []filters.Sorting{
		{
			Field:     "id",
			Direction: filters.SortDirectionAsc,
		},
	}, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find client certificates"))

		return
	}

	certificatesResponse := make([]clientCertificateResponse, 0, len(certificates))

	for _, cert := range certificates {
		certInfo, err := h.parseCertificateContent(ctx, cert.Certificate)
		if err != nil {
			h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to parse certificate content"))

			return
		}

		certificatesResponse = append(
			certificatesResponse,
			newClientCertificateResponseFromClientCertificate(
				cert,
				certInfo,
			))
	}

	h.responder.Write(ctx, rw, certificatesResponse)
}

func (h *Handler) parseCertificateContent(
	ctx context.Context,
	certPath string,
) (clientCertificateInfo, error) {
	certPEM, err := h.fileManager.Read(ctx, certPath)
	if err != nil {
		return clientCertificateInfo{}, errors.WithMessage(err, "failed to read certificate file")
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return clientCertificateInfo{}, errors.New("failed to decode PEM block containing the certificate")
	}
	if block.Type != "CERTIFICATE" {
		return clientCertificateInfo{}, errors.Errorf("invalid PEM block type: %s, expected CERTIFICATE", block.Type)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return clientCertificateInfo{}, errors.WithMessage(err, "failed to parse certificate")
	}

	info := clientCertificateInfo{
		Expires:       cert.NotAfter,
		SignatureType: cert.SignatureAlgorithm.String(),

		CommonName: cert.Subject.CommonName,

		IssuerCommonName: cert.Issuer.CommonName,
	}

	if len(cert.Subject.Country) > 0 {
		info.Country = cert.Subject.Country[0]
	}
	if len(cert.Subject.Province) > 0 {
		info.State = cert.Subject.Province[0]
	}
	if len(cert.Subject.Locality) > 0 {
		info.Locality = cert.Subject.Locality[0]
	}
	if len(cert.Subject.Organization) > 0 {
		info.Organization = cert.Subject.Organization[0]
	}
	if len(cert.Subject.OrganizationalUnit) > 0 {
		info.OrganizationalUnit = cert.Subject.OrganizationalUnit[0]
	}
	if len(cert.EmailAddresses) > 0 {
		info.Email = cert.EmailAddresses[0]
	}

	if len(cert.Issuer.Country) > 0 {
		info.IssuerCountry = cert.Issuer.Country[0]
	}
	if len(cert.Issuer.Province) > 0 {
		info.IssuerState = cert.Issuer.Province[0]
	}
	if len(cert.Issuer.Locality) > 0 {
		info.IssuerLocality = cert.Issuer.Locality[0]
	}
	if len(cert.Issuer.Organization) > 0 {
		info.IssuerOrganization = cert.Issuer.Organization[0]
	}
	if len(cert.Issuer.OrganizationalUnit) > 0 {
		info.IssuerOrganizationalUnit = cert.Issuer.OrganizationalUnit[0]
	}

	return info, nil
}
