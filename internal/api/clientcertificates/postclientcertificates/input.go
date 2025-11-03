package postclientcertificates

import (
	"mime/multipart"

	"github.com/gameap/gameap/pkg/api"
)

var (
	ErrCertificateRequired = api.NewValidationError("certificate file is required")
	ErrPrivateKeyRequired  = api.NewValidationError("private_key file is required")
	ErrInvalidCertificate  = api.NewValidationError("invalid certificate file")
	ErrInvalidPrivateKey   = api.NewValidationError("invalid private key file")
)

type clientCertificateInput struct {
	CertificateFile []*multipart.FileHeader
	PrivateKeyFile  []*multipart.FileHeader
	PrivateKeyPass  string
}

func (i *clientCertificateInput) Validate() error {
	if len(i.CertificateFile) == 0 {
		return ErrCertificateRequired
	}

	if len(i.PrivateKeyFile) == 0 {
		return ErrPrivateKeyRequired
	}

	if i.CertificateFile[0].Size == 0 {
		return ErrInvalidCertificate
	}

	if i.PrivateKeyFile[0].Size == 0 {
		return ErrInvalidPrivateKey
	}

	return nil
}
