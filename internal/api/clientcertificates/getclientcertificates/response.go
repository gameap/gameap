package getclientcertificates

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type clientCertificateResponse struct {
	ID          uint                  `json:"id"`
	Fingerprint string                `json:"fingerprint"`
	Expires     time.Time             `json:"expires"`
	Info        clientCertificateInfo `json:"info"`
}

type clientCertificateInfo struct {
	Expires       time.Time `json:"expires"`
	SignatureType string    `json:"signature_type"`

	Country            string `json:"country"`
	State              string `json:"state"`
	Locality           string `json:"locality"`
	Organization       string `json:"organization"`
	OrganizationalUnit string `json:"organizational_unit"`
	CommonName         string `json:"common_name"`
	Email              string `json:"email"`

	IssuerCountry            string `json:"issuer_country"`
	IssuerState              string `json:"issuer_state"`
	IssuerLocality           string `json:"issuer_locality"`
	IssuerOrganization       string `json:"issuer_organization"`
	IssuerOrganizationalUnit string `json:"issuer_organizational_unit"`
	IssuerCommonName         string `json:"issuer_common_name"`
	IssuerEmail              string `json:"issuer_email"`
}

func newClientCertificateResponseFromClientCertificate(
	cert domain.ClientCertificate,
	info clientCertificateInfo,
) clientCertificateResponse {
	return clientCertificateResponse{
		ID:          cert.ID,
		Fingerprint: cert.Fingerprint,
		Expires:     cert.Expires,
		Info:        info,
	}
}
