// White-box tests for the gRPC and multiplexer TLS builders on the DI Container.
//
// These are transport-security tests. Relevant standards:
//   - OWASP API Security Top 10:2023 — API8:2023 Security Misconfiguration
//     (transport security: TLS must be enabled, hardened, and mTLS enforced when
//     requested).
//   - OWASP ASVS 4.0.3 §9.1 — TLS configuration: a minimum TLS version of 1.2,
//     a server certificate that parses, and verified client certificates when
//     mutual TLS is required.
//
// They run entirely on CPU: certificates are generated into the per-test temp
// FileManager (no network, no real CA).

package application

import (
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGRPCTLSConfig covers the top-level gRPC TLS toggle.
// API8:2023 Security Misconfiguration / ASVS §9.1: when GRPC_TLS_ENABLED is off
// the builder must explicitly opt out (nil config, plaintext), and when on it
// must delegate to the hardened builder.
func TestGRPCTLSConfig(t *testing.T) {
	t.Parallel()

	t.Run("tls_disabled_returns_nil_config_and_no_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.GRPC.TLSEnabled = false
		})

		// ACT
		tlsCfg, err := c.grpcTLSConfig()

		// ASSERT
		require.NoError(t, err)
		assert.Nil(t, tlsCfg, "TLS disabled must yield a nil config so the server runs plaintext")
	})

	t.Run("tls_enabled_returns_non_nil_config", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.GRPC.TLSEnabled = true
		})

		// ACT
		tlsCfg, err := c.grpcTLSConfig()

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, tlsCfg, "TLS enabled must delegate to buildGRPCTLSConfig and return a config")
	})
}

// TestBuildGRPCTLSConfig covers the hardened gRPC server TLS config.
// API8:2023 Security Misconfiguration / ASVS §9.1: the config must enforce a
// modern minimum protocol version, carry exactly one parseable server
// certificate, and only require/verify client certificates when mTLS is enabled.
func TestBuildGRPCTLSConfig(t *testing.T) {
	t.Parallel()

	t.Run("generates_hardened_config_without_client_auth_by_default", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.GRPC.TLSEnabled = true
			cfg.GRPC.RequireMTLS = false
		})

		// ACT
		tlsCfg, err := c.buildGRPCTLSConfig()

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, tlsCfg)

		assert.GreaterOrEqual(t, tlsCfg.MinVersion, uint16(tls.VersionTLS12),
			"HardenServerConfig must enforce at least TLS 1.2")

		require.Len(t, tlsCfg.Certificates, 1, "exactly one server certificate must be loaded")
		leaf, parseErr := x509.ParseCertificate(tlsCfg.Certificates[0].Certificate[0])
		require.NoError(t, parseErr, "the generated server certificate must parse")
		assert.Equal(t, "GameAP API Server", leaf.Subject.CommonName,
			"the server certificate must carry the configured common name")

		assert.Equal(t, tls.NoClientCert, tlsCfg.ClientAuth,
			"without mTLS the server must not request a client certificate")
		assert.Nil(t, tlsCfg.ClientCAs, "without mTLS no client CA pool must be configured")
	})

	t.Run("require_mtls_enables_client_cert_verification", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.GRPC.TLSEnabled = true
			cfg.GRPC.RequireMTLS = true
		})

		// ACT
		tlsCfg, err := c.buildGRPCTLSConfig()

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, tlsCfg)

		assert.Equal(t, tls.RequireAndVerifyClientCert, tlsCfg.ClientAuth,
			"GRPC_REQUIRE_MTLS must require and verify a client certificate")
		require.NotNil(t, tlsCfg.ClientCAs,
			"mTLS must populate the client CA pool from the root certificate")
	})
}

// TestGRPCCertSignOptions covers the SAN/identity inputs used to mint the gRPC
// server certificate. ASVS §9.1: the certificate must always be valid for the
// loopback identities the local multiplexer presents, so an operator who never
// configures an external host still gets a working, verifiable certificate.
func TestGRPCCertSignOptions(t *testing.T) {
	t.Parallel()

	t.Run("always_includes_loopback_and_localhost_sans", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := newWiredContainer(t)

		// ACT
		opts := c.grpcCertSignOptions()

		// ASSERT
		require.NotNil(t, opts)
		assert.Equal(t, "GameAP API Server", opts.CommonName,
			"the gRPC server certificate common name is fixed")

		assert.Contains(t, ipAddressesToStrings(opts.IPAddresses), loopbackBindIP,
			"the loopback IP SAN must always be present via the resolver fallback")
		assert.Contains(t, opts.DNSNames, "localhost",
			"the localhost DNS SAN must always be present via the resolver fallback")
	})
}

// TestBuildMultiplexerTLSConfig covers the HTTPS/mux TLS config.
// API8:2023 Security Misconfiguration / ASVS §9.1: when no certificate source is
// configured the mux must run plaintext (nil config), and when an inline cert is
// configured the mux must serve a hardened TLS config carrying that certificate.
func TestBuildMultiplexerTLSConfig(t *testing.T) {
	t.Parallel()

	t.Run("tls_disabled_returns_nil_config_and_no_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := newWiredContainer(t)
		require.False(t, c.config.TLSEnabled(), "sanity: no cert source must disable TLS")

		// ACT
		tlsCfg, err := c.buildMultiplexerTLSConfig()

		// ASSERT
		require.NoError(t, err)
		assert.Nil(t, tlsCfg, "no cert source must yield a nil config so the mux runs plaintext")
	})

	t.Run("inline_certificate_yields_hardened_config", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		certPEM, keyPEM := buildSelfSignedPEM(t)
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.TLS.Cert = string(certPEM)
			cfg.TLS.Key = string(keyPEM)
		})
		require.Equal(t, config.CertSourceInline, c.config.EffectiveCertSource(),
			"sanity: inline cert + key must select the inline source")

		// ACT
		tlsCfg, err := c.buildMultiplexerTLSConfig()

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, tlsCfg)
		assert.GreaterOrEqual(t, tlsCfg.MinVersion, uint16(tls.VersionTLS12),
			"the mux TLS config must be hardened to at least TLS 1.2")
		require.Len(t, tlsCfg.Certificates, 1,
			"the inline certificate must be loaded into the config")
	})
}

// TestMultiplexedServer is a construction smoke test for the lazily-built
// multiplexer. It binds an ephemeral loopback port (no fixed port, no external
// reachability) and verifies the server is built once. Serve() is intentionally
// not driven here — the full Serve loop is covered by multiplexer_test.go.
func TestMultiplexedServer(t *testing.T) {
	t.Parallel()

	t.Run("builds_once_on_ephemeral_loopback_port", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		// MultiplexedServer pulls in the full HTTP router (HTTPServer -> Router),
		// which wires the auth-backed handlers; AuthService panics on an empty
		// service value, so pin paseto (the production default) here.
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.AuthService = authServicePaseto
			cfg.HTTPBindIP = loopbackBindIP
			cfg.HTTPPort = 0
		})

		// ACT
		first, err := c.MultiplexedServer()

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, first)
		t.Cleanup(func() { _ = first.Close() })

		second, err := c.MultiplexedServer()
		require.NoError(t, err)
		assert.Same(t, first, second, "MultiplexedServer must be a lazy singleton")
	})
}
