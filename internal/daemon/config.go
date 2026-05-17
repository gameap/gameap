package daemon

import (
	"context"

	"github.com/gameap/gameap/internal/daemon/binnapi"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

type configMaker struct {
	certRepo    repositories.ClientCertificateRepository
	fileManager files.FileManager
	cipher      *secret.Cipher
}

func newConfigMaker(
	certRepo repositories.ClientCertificateRepository,
	fileManager files.FileManager,
) *configMaker {
	return &configMaker{
		certRepo:    certRepo,
		fileManager: fileManager,
		cipher:      secret.Disabled(),
	}
}

// LegacyOption configures the legacy (BINN) daemon services.
type LegacyOption func(*configMaker)

// WithSecretCipher injects the cipher used to decrypt gdaemon_password at rest.
func WithSecretCipher(c *secret.Cipher) LegacyOption {
	return func(m *configMaker) {
		if c != nil {
			m.cipher = c
		}
	}
}

func (s *configMaker) Make(ctx context.Context, node *domain.Node) (config, error) {
	return s.MakeWithMode(ctx, node, binnapi.ModeStatus)
}

func (s *configMaker) MakeWithMode(ctx context.Context, node *domain.Node, mode binnapi.Mode) (config, error) {
	if node == nil {
		return config{}, errors.New("node not found")
	}

	serverCert, err := s.fileManager.Read(ctx, node.GdaemonServerCert)
	if err != nil {
		return config{}, errors.WithMessage(err, "failed to read server certificate")
	}

	certs, err := s.certRepo.Find(
		ctx,
		filters.FindClientCertificateByIDs(node.ClientCertificateID),
		nil,
		nil,
	)
	if err != nil {
		return config{}, errors.WithMessage(err, "failed to find client certificate")
	}
	if len(certs) == 0 {
		return config{}, errors.New("client certificate not found")
	}

	cert := certs[0]

	clientCert, err := s.fileManager.Read(ctx, cert.Certificate)
	if err != nil {
		return config{}, errors.WithMessage(err, "failed to read client certificate")
	}

	privateKey, err := s.fileManager.Read(ctx, cert.PrivateKey)
	if err != nil {
		return config{}, errors.WithMessage(err, "failed to read private key")
	}

	password, err := s.cipher.Decrypt(lo.FromPtr(node.GdaemonPassword))
	if err != nil {
		return config{}, errors.WithMessage(err, "failed to decrypt gdaemon password")
	}

	return config{
		Host:              node.GdaemonHost,
		Port:              node.GdaemonPort,
		Username:          lo.FromPtr(node.GdaemonLogin),
		Password:          password,
		ServerCertificate: serverCert,
		ClientCertificate: clientCert,
		PrivateKey:        privateKey,
		Timeout:           defaultTimeout,
		Mode:              mode,
	}, nil
}
