// Whitebox tests for buildGetCertificate. Lives in package application so it
// can drive the unexported function directly. The ACME branch reuses the
// container's real Service, but overrides its lego factory so the test never
// reaches a real CA.

package application

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/acme"
	"github.com/gameap/gameap/internal/config"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/registration"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildGetCertConfig wires a minimal config + temp legacy dir so that the
// container's FileManager initialiser does not panic.
func buildGetCertConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg := &config.Config{}
	cfg.Files.Driver = filesDriverLocal
	cfg.Files.Local.BasePath = t.TempDir()
	cfg.Cache.Driver = cacheDriverMemory
	cfg.ACME.StoragePath = "acme"

	return cfg
}

func writeTestCertFiles(t *testing.T) (certPath, keyPath string) {
	t.Helper()

	dir := t.TempDir()
	certPEM, keyPEM := buildSelfSignedPEM(t)

	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	require.NoError(t, os.WriteFile(certPath, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyPath, keyPEM, 0o600))

	return certPath, keyPath
}

func buildSelfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "buildgetcert-test"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(60 * 24 * time.Hour),
		DNSNames:     []string{"example.com"},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return certPEM, keyPEM
}

func acmeResource(t *testing.T) *certificate.Resource {
	t.Helper()

	certPEM, keyPEM := buildSelfSignedPEM(t)

	return &certificate.Resource{
		Domain:            "example.com",
		CertURL:           "https://test/cert",
		PrivateKey:        keyPEM,
		Certificate:       certPEM,
		IssuerCertificate: certPEM,
	}
}

func TestBuildGetCertificate_FileSource(t *testing.T) {
	t.Parallel()

	// ARRANGE
	certPath, keyPath := writeTestCertFiles(t)

	cfg := buildGetCertConfig(t)
	cfg.TLS.CertFile = certPath
	cfg.TLS.KeyFile = keyPath

	require.Equal(t, config.CertSourceFile, cfg.EffectiveCertSource(),
		"sanity: configured TLS files must yield CertSourceFile")

	container := NewContainer(cfg)

	// ACT
	getCert, err := buildGetCertificate(context.Background(), cfg, container)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, getCert)

	cert, err := getCert(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.NotEmpty(t, cert.Certificate, "callback must return a populated certificate")
}

func TestBuildGetCertificate_InlineSource(t *testing.T) {
	t.Parallel()

	// ARRANGE
	certPEM, keyPEM := buildSelfSignedPEM(t)

	cfg := buildGetCertConfig(t)
	cfg.TLS.Cert = string(certPEM)
	cfg.TLS.Key = string(keyPEM)

	require.Equal(t, config.CertSourceInline, cfg.EffectiveCertSource())

	container := NewContainer(cfg)

	// ACT
	getCert, err := buildGetCertificate(context.Background(), cfg, container)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, getCert)

	cert, err := getCert(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	require.NotNil(t, cert)
}

func TestBuildGetCertificate_FileSourceWithBadPathReturnsError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	cfg := buildGetCertConfig(t)
	cfg.TLS.CertFile = "/nonexistent/cert.pem"
	cfg.TLS.KeyFile = "/nonexistent/key.pem"

	container := NewContainer(cfg)

	// ACT
	getCert, err := buildGetCertificate(context.Background(), cfg, container)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load static TLS certificate",
		"file load error must be wrapped")
	assert.Nil(t, getCert)
}

func TestBuildGetCertificate_NoneSourceReturnsError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	cfg := buildGetCertConfig(t)
	require.Equal(t, config.CertSourceNone, cfg.EffectiveCertSource())

	container := NewContainer(cfg)

	// ACT
	getCert, err := buildGetCertificate(context.Background(), cfg, container)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no certificate source configured")
	assert.Nil(t, getCert)
}

func TestBuildGetCertificate_ACMESourceReturnsACMECallback(t *testing.T) {
	t.Parallel()

	// ARRANGE: full ACME config + lego factory override so Service.Start
	// completes against a fake CA and serves the resource we stub in.
	cfg := buildGetCertConfig(t)
	cfg.ACME.Enabled = true
	cfg.ACME.ChallengeType = config.ACMEChallengeDNS01
	cfg.ACME.Email = "ops@example.com"
	cfg.ACME.Domains = []string{"example.com"}
	cfg.ACME.DNSProvider = "cloudflare"
	cfg.ACME.RenewalThreshold = 30 * 24 * time.Hour
	cfg.ACME.RenewalCheckInterval = 1 * time.Hour
	cfg.ACME.PropagationTimeout = 2 * time.Minute

	require.Equal(t, config.CertSourceACME, cfg.EffectiveCertSource())

	resource := acmeResource(t)
	container := NewContainer(cfg)

	svc := container.ACMEService()
	require.NotNil(t, svc)

	// Override the lego factory so neither Register nor Obtain hit a real CA.
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return &noopLegoClient{resource: resource}, nil
	})

	// ACT
	getCert, err := buildGetCertificate(context.Background(), cfg, container)
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, getCert)

	cert, err := getCert(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	require.NotNil(t, cert, "ACME callback must return a populated certificate")
}

func TestBuildGetCertificate_ACMEStartFailureReturnsError(t *testing.T) {
	t.Parallel()

	// ARRANGE: ACME-enabled config, but the lego factory always errors so
	// Service.Start fails inside buildGetCertificate.
	cfg := buildGetCertConfig(t)
	cfg.ACME.Enabled = true
	cfg.ACME.ChallengeType = config.ACMEChallengeDNS01
	cfg.ACME.Email = "ops@example.com"
	cfg.ACME.Domains = []string{"example.com"}
	cfg.ACME.DNSProvider = "cloudflare"

	require.Equal(t, config.CertSourceACME, cfg.EffectiveCertSource())

	container := NewContainer(cfg)

	svc := container.ACMEService()
	svc.SetLegoFactory(func(_ registration.User, _ acme.ServiceConfig, _ acme.LegoFactoryDeps) (acme.LegoClient, error) {
		return nil, errors.New("lego factory simulated failure")
	})

	// ACT
	getCert, err := buildGetCertificate(context.Background(), cfg, container)

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ACME initialisation failed",
		"buildGetCertificate must wrap Service.Start errors")
	assert.Nil(t, getCert)
}

// noopLegoClient is the minimal acme.LegoClient that returns a pre-baked
// resource for every ObtainCertificate / RenewCertificate call.
type noopLegoClient struct {
	resource *certificate.Resource
}

func (c *noopLegoClient) ObtainCertificate(
	_ context.Context, _ certificate.ObtainRequest,
) (*certificate.Resource, error) {
	return c.resource, nil
}

func (c *noopLegoClient) RenewCertificate(
	_ context.Context, _ certificate.Resource,
) (*certificate.Resource, error) {
	return c.resource, nil
}

func (c *noopLegoClient) Register(_ context.Context, _ bool) (*registration.Resource, error) {
	return &registration.Resource{URI: "https://test/reg"}, nil
}
