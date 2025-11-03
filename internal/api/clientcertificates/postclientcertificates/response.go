package postclientcertificates

import (
	"github.com/gameap/gameap/internal/domain"
)

type clientCertificateResponse struct {
	ID uint `json:"id"`
}

func newClientCertificateResponseFromClientCertificate(cert *domain.ClientCertificate) clientCertificateResponse {
	return clientCertificateResponse{
		ID: cert.ID,
	}
}
