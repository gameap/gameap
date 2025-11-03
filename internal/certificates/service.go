package certificates

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"time"

	"github.com/gameap/gameap/internal/files"
	"github.com/pkg/errors"
)

const (
	RootCACert             = "certs/root.crt"
	RootCAKey              = "certs/root.key"
	ClientCertificatesPath = "certs/client"
	ServerCertificatesPath = "certs/server"
	PrivateKeyBits         = 2048
	CertYears              = 10
)

var (
	ErrFailedToParseCSRPEM      = errors.New("failed to parse CSR PEM")
	ErrFailedToParseRootCertPEM = errors.New("failed to parse root certificate PEM")
	ErrFailedToParseRootKeyPEM  = errors.New("failed to parse root key PEM")
)

type SignOptions struct {
	CommonName         string
	Email              string
	Organization       string
	Country            string
	State              string
	Locality           string
	OrganizationalUnit string
}

type Service struct {
	fileManager files.FileManager
}

func NewService(fileManager files.FileManager) *Service {
	return &Service{
		fileManager: fileManager,
	}
}

// Root returns the root certificate.
func (s *Service) Root(ctx context.Context) (string, error) {
	if !s.fileManager.Exists(ctx, RootCACert) {
		if err := s.generateRoot(ctx); err != nil {
			return "", errors.Wrap(err, "failed to generate root certificate")
		}
	}

	cert, err := s.fileManager.Read(ctx, RootCACert)
	if err != nil {
		return "", errors.Wrap(err, "failed to read root certificate")
	}

	return string(cert), nil
}

// RootKey returns the root private key.
func (s *Service) RootKey(ctx context.Context) (string, error) {
	if !s.fileManager.Exists(ctx, RootCAKey) {
		if err := s.generateRoot(ctx); err != nil {
			return "", errors.Wrap(err, "failed to generate root key")
		}
	}

	key, err := s.fileManager.Read(ctx, RootCAKey)
	if err != nil {
		return "", errors.Wrap(err, "failed to read root key")
	}

	return string(key), nil
}

// Sign signs a CSR with the root certificate.
//

func (s *Service) Sign(ctx context.Context, csrPEM string, opts *SignOptions) (string, error) {
	// Parse CSR
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		return "", ErrFailedToParseCSRPEM
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse CSR")
	}

	// Verify CSR signature
	if err := csr.CheckSignature(); err != nil {
		return "", errors.Wrap(err, "failed to verify CSR signature")
	}

	// Load root certificate
	rootCertPEM, err := s.Root(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to load root certificate")
	}

	rootCertBlock, _ := pem.Decode([]byte(rootCertPEM))
	if rootCertBlock == nil {
		return "", ErrFailedToParseRootCertPEM
	}

	rootCert, err := x509.ParseCertificate(rootCertBlock.Bytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse root certificate")
	}

	// Load root key
	rootKeyPEM, err := s.RootKey(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to load root key")
	}

	rootKeyBlock, _ := pem.Decode([]byte(rootKeyPEM))
	if rootKeyBlock == nil {
		return "", ErrFailedToParseRootKeyPEM
	}

	rootKey, err := x509.ParsePKCS8PrivateKey(rootKeyBlock.Bytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse root key")
	}

	// Prepare certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", errors.Wrap(err, "failed to generate serial number")
	}

	subject := csr.Subject

	//nolint:nestif
	if opts != nil {
		if opts.CommonName != "" {
			subject.CommonName = opts.CommonName
		}
		if opts.Organization != "" {
			subject.Organization = []string{opts.Organization}
		}
		if opts.Country != "" {
			subject.Country = []string{opts.Country}
		}
		if opts.State != "" {
			subject.Province = []string{opts.State}
		}
		if opts.Locality != "" {
			subject.Locality = []string{opts.Locality}
		}
		if opts.OrganizationalUnit != "" {
			subject.OrganizationalUnit = []string{opts.OrganizationalUnit}
		}
		if opts.Email != "" {
			subject.ExtraNames = []pkix.AttributeTypeAndValue{
				{
					Type:  []int{1, 2, 840, 113549, 1, 9, 1}, // emailAddress OID
					Value: opts.Email,
				},
			}
		}
	}

	template := &x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               subject,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(CertYears * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Sign certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, rootCert, csr.PublicKey, rootKey)
	if err != nil {
		return "", errors.Wrap(err, "failed to create certificate")
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return string(certPEM), nil
}

// Generate generates a new certificate signed with the root certificate.
// Returns the certificate PEM and private key PEM.
func (s *Service) Generate(
	ctx context.Context,
	certificatePath, keyPath string,
	opts *SignOptions,
) (string, string, error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, PrivateKeyBits)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to generate private key")
	}

	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to marshal private key to PKCS8")
	}

	// Encode private key to PEM
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8,
	})

	// Prepare CSR subject
	subject := pkix.Name{
		CommonName:   "GameAP",
		Organization: []string{"GameAP"},
	}

	//nolint:nestif
	if opts != nil {
		if opts.CommonName != "" {
			subject.CommonName = opts.CommonName
		}
		if opts.Organization != "" {
			subject.Organization = []string{opts.Organization}
		}
		if opts.Country != "" {
			subject.Country = []string{opts.Country}
		}
		if opts.State != "" {
			subject.Province = []string{opts.State}
		}
		if opts.Locality != "" {
			subject.Locality = []string{opts.Locality}
		}
		if opts.OrganizationalUnit != "" {
			subject.OrganizationalUnit = []string{opts.OrganizationalUnit}
		}
		if opts.Email != "" {
			subject.ExtraNames = []pkix.AttributeTypeAndValue{
				{
					Type:  []int{1, 2, 840, 113549, 1, 9, 1}, // emailAddress OID
					Value: opts.Email,
				},
			}
		}
	}

	// Create CSR template
	csrTemplate := &x509.CertificateRequest{
		Subject:            subject,
		SignatureAlgorithm: x509.SHA256WithRSA,
	}

	// Create CSR
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, privateKey)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to create CSR")
	}

	// Encode CSR to PEM
	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	// Sign CSR with root certificate
	certPEM, err := s.Sign(ctx, string(csrPEM), opts)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to sign CSR")
	}

	// Write certificate to file
	if err := s.fileManager.Write(ctx, certificatePath, []byte(certPEM)); err != nil {
		return "", "", errors.Wrap(err, "failed to write certificate")
	}

	// Write private key to file
	if err := s.fileManager.Write(ctx, keyPath, privateKeyPEM); err != nil {
		return "", "", errors.Wrap(err, "failed to write private key")
	}

	return certPEM, string(privateKeyPEM), nil
}

func (s *Service) Fingerprint(certPEM string) (string, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return "", ErrFailedToParseCSRPEM
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse certificate")
	}

	hash := sha256.Sum256(cert.Raw)

	return hex.EncodeToString(hash[:]), nil
}

// generateRoot generates the root CA certificate and key.
func (s *Service) generateRoot(ctx context.Context) error {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, PrivateKeyBits)
	if err != nil {
		return errors.Wrap(err, "failed to generate private key")
	}

	// Prepare certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return errors.Wrap(err, "failed to generate serial number")
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "GameAP CA",
			Organization: []string{"GameAP"},
			Country:      []string{"RU"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(CertYears * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Create subject key identifier
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return errors.Wrap(err, "failed to marshal public key")
	}
	hash := sha256.Sum256(pubKeyBytes)
	template.SubjectKeyId = hash[:]

	// Self-sign certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return errors.Wrap(err, "failed to create certificate")
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return errors.Wrap(err, "failed to marshal private key to PKCS8")
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8,
	})

	// Write certificate to file
	if err := s.fileManager.Write(ctx, RootCACert, certPEM); err != nil {
		return errors.Wrap(err, "failed to write certificate")
	}

	// Write private key to file
	if err := s.fileManager.Write(ctx, RootCAKey, privateKeyPEM); err != nil {
		return errors.Wrap(err, "failed to write private key")
	}

	return nil
}
