package enrollment

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/gameap/gameap/internal/certificates"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
	"github.com/rs/xid"
)

const (
	apiKeyLength        = 64
	defaultPort         = 31717
	defaultWorkPath     = "/srv/gameap"
	defaultSteamCMDPath = "/srv/gameap/steamcmd"
)

type EnrollResult struct {
	NodeID            uint
	APIKey            string
	RootCertificate   string
	ServerCertificate string
	ServerPrivateKey  string
}

type EnrollInput struct {
	Host         string
	Port         int32
	OS           string
	Version      string
	Capabilities []string
}

type Service struct {
	setupKeyManager *SetupKeyManager
	tickets         *TicketStore
	nodesRepo       repositories.NodeRepository
	clientCertRepo  repositories.ClientCertificateRepository
	certificatesSvc *certificates.Service
}

func NewService(
	setupKeyManager *SetupKeyManager,
	nodesRepo repositories.NodeRepository,
	clientCertRepo repositories.ClientCertificateRepository,
	certificatesSvc *certificates.Service,
) *Service {
	return &Service{
		setupKeyManager: setupKeyManager,
		tickets:         NewTicketStore(setupKeyManager.cache),
		nodesRepo:       nodesRepo,
		clientCertRepo:  clientCertRepo,
		certificatesSvc: certificatesSvc,
	}
}

func (s *Service) SetupKeyManager() *SetupKeyManager {
	return s.setupKeyManager
}

// Tickets exposes the enrollment ticket store used by the plugin host library.
func (s *Service) Tickets() *TicketStore {
	return s.tickets
}

// ValidateSetupKey accepts either the global admin setup key or a pending
// enrollment ticket key, without consuming anything. Used by the public setup
// script endpoint, which must serve a script for both kinds of key.
func (s *Service) ValidateSetupKey(ctx context.Context, key string) error {
	_, err := s.resolveSetupKey(ctx, key)

	return err
}

func (s *Service) Enroll(ctx context.Context, setupKey string, input *EnrollInput) (*EnrollResult, error) {
	ticket, err := s.resolveSetupKey(ctx, setupKey)
	if err != nil {
		return nil, err
	}

	apiKey, err := strings.CryptoRandomString(apiKeyLength)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate API key")
	}

	serverCert, serverKey, err := s.certificatesSvc.GenerateInMemory(ctx, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate server certificate")
	}

	rootCert, err := s.certificatesSvc.Root(ctx)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get root certificate")
	}

	clientCertID, err := s.getOrCreateClientCertificateID(ctx)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get client certificate ID")
	}

	port := int(input.Port)
	if port == 0 {
		port = defaultPort
	}

	node := &domain.Node{
		Enabled:             true,
		Name:                input.Host,
		OS:                  domain.ParseNodeOS(input.OS),
		Location:            "Unknown",
		Provider:            new("Unknown"),
		IPs:                 domain.IPList{input.Host},
		WorkPath:            defaultWorkPath,
		SteamcmdPath:        new(defaultSteamCMDPath),
		GdaemonHost:         input.Host,
		GdaemonPort:         port,
		GdaemonAPIKey:       strings.SHA256(apiKey),
		GdaemonServerCert:   certificates.RootCACert,
		ClientCertificateID: clientCertID,
		PreferInstallMethod: domain.NodePreferInstallMethodAuto,
	}

	now := time.Now()
	node.CreatedAt = &now
	node.UpdatedAt = &now

	if ticket != nil {
		ticket.Presets.ApplyTo(node)
	}

	if err := s.nodesRepo.Save(ctx, node); err != nil {
		return nil, errors.WithMessage(err, "failed to save node")
	}

	s.consumeSetupKey(ctx, ticket, node.ID)

	return &EnrollResult{
		NodeID:            node.ID,
		APIKey:            apiKey,
		RootCertificate:   rootCert,
		ServerCertificate: serverCert,
		ServerPrivateKey:  serverKey,
	}, nil
}

func (s *Service) getOrCreateClientCertificateID(ctx context.Context) (uint, error) {
	certs, err := s.clientCertRepo.Find(ctx, nil, nil, &filters.Pagination{Limit: 1})
	if err != nil {
		return 0, errors.WithMessage(err, "failed to find client certificates")
	}

	if len(certs) > 0 {
		return certs[0].ID, nil
	}

	certName := xid.New().String()
	certPath := filepath.Join(certificates.ClientCertificatesPath, certName+".crt")
	keyPath := filepath.Join(certificates.ClientCertificatesPath, certName+".key")

	clientCert, _, err := s.certificatesSvc.Generate(ctx, certPath, keyPath, nil)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to generate client certificate")
	}

	fingerprint, err := s.certificatesSvc.Fingerprint(clientCert)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to fingerprint client certificate")
	}

	clientCertificate := domain.ClientCertificate{
		Certificate: certPath,
		PrivateKey:  keyPath,
		Fingerprint: fingerprint,
		Expires:     time.Now().Add(certificates.CertYears * 365 * 24 * time.Hour),
	}

	if err := s.clientCertRepo.Save(ctx, &clientCertificate); err != nil {
		return 0, errors.WithMessage(err, "failed to save client certificate")
	}

	return clientCertificate.ID, nil
}

// resolveSetupKey authenticates an enrollment credential. The global admin key
// is checked first so its behaviour is untouched; only a key shaped like a
// ticket is looked up in the ticket store. A ticket miss reports the original
// global-key error, keeping the gateway's status mapping and the "no setup key
// configured" message intact.
func (s *Service) resolveSetupKey(ctx context.Context, setupKey string) (*Ticket, error) {
	globalErr := s.setupKeyManager.Validate(ctx, setupKey)
	if globalErr == nil {
		return nil, nil //nolint:nilnil // no ticket means the global key was used
	}

	if !errors.Is(globalErr, ErrInvalidSetupKey) && !errors.Is(globalErr, ErrSetupKeyNotConfigured) {
		return nil, globalErr
	}

	if !IsTicketKey(setupKey) {
		return nil, globalErr
	}

	ticket, err := s.tickets.Resolve(ctx, setupKey)
	if err != nil {
		if errors.Is(err, ErrTicketNotFound) {
			return nil, globalErr
		}

		return nil, err
	}

	return ticket, nil
}

// consumeSetupKey retires the credential that was just used: the global key is
// invalidated as before, a ticket is marked consumed and records its node.
func (s *Service) consumeSetupKey(ctx context.Context, ticket *Ticket, nodeID uint) {
	if ticket == nil {
		if err := s.setupKeyManager.Invalidate(ctx); err != nil {
			slog.WarnContext(ctx, "failed to invalidate setup key", slog.String("error", err.Error()))
		}

		return
	}

	if err := s.tickets.Consume(ctx, ticket, nodeID); err != nil {
		slog.WarnContext(ctx, "failed to consume enrollment ticket",
			slog.String("ticket_id", ticket.ID),
			slog.String("error", err.Error()))
	}
}
