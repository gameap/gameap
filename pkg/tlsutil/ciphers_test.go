// API Security Tests for OWASP API Security Top 10:2023.
// Category: API8:2023 — Security Misconfiguration.
//
// Pins the explicit TLS cipher list and curve preferences used by every
// project listener (HTTP, gRPC, multiplexer) so a regression here cannot
// silently re-enable CBC suites, static RSA key exchange, or NIST curves
// that Go's defaults may broaden in the future.
package tlsutil_test

import (
	"crypto/tls"
	"testing"

	"github.com/gameap/gameap/pkg/tlsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModernCipherSuites_OnlyAEAD — OWASP API8:2023 — every returned suite is
// authenticated encryption (AES-GCM or ChaCha20-Poly1305). A regression that
// re-introduces CBC suites would break Lucky13 / BEAST resistance.
func TestModernCipherSuites_OnlyAEAD(t *testing.T) {
	suites := tlsutil.ModernCipherSuites()
	require.NotEmpty(t, suites)

	allowed := map[uint16]struct{}{
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256: {},
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256:   {},
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256:       {},
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:         {},
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384:       {},
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:         {},
	}

	for _, s := range suites {
		_, ok := allowed[s]
		assert.Truef(t, ok, "cipher 0x%04x is not in the audited AEAD allow-list", s)
	}
}

// TestModernCipherSuites_NoLegacy — OWASP API8:2023 — explicit denylist of
// suites that must never appear: static RSA, CBC-MAC, RC4, 3DES.
func TestModernCipherSuites_NoLegacy(t *testing.T) {
	forbidden := []uint16{
		tls.TLS_RSA_WITH_RC4_128_SHA,
		tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	}

	suites := tlsutil.ModernCipherSuites()

	for _, bad := range forbidden {
		assert.NotContainsf(t, suites, bad,
			"forbidden cipher 0x%04x must not appear in ModernCipherSuites", bad)
	}
}

// TestPreferredCurves_X25519First — OWASP API8:2023 — X25519 leads so handshake
// performance is optimal and we get curve-validation safety for free; P-256
// trails for legacy clients; nothing else is offered.
func TestPreferredCurves_X25519First(t *testing.T) {
	curves := tlsutil.PreferredCurves()
	require.Len(t, curves, 2)

	assert.Equal(t, tls.X25519, curves[0])
	assert.Equal(t, tls.CurveP256, curves[1])
}

// TestHardenServerConfig_AppliesDefaultsOnZeroValue — OWASP API8:2023 — a
// freshly-allocated tls.Config receives every project-wide hardening default.
func TestHardenServerConfig_AppliesDefaultsOnZeroValue(t *testing.T) {
	got := tlsutil.HardenServerConfig(&tls.Config{})

	assert.Equal(t, uint16(tls.VersionTLS12), got.MinVersion)
	assert.Equal(t, tlsutil.ModernCipherSuites(), got.CipherSuites)
	assert.Equal(t, tlsutil.PreferredCurves(), got.CurvePreferences)
}

// TestHardenServerConfig_DoesNotOverrideExplicitValues — OWASP API8:2023 —
// callers that intentionally set a stricter MinVersion, a custom cipher list,
// or curve preferences keep their choice. The helper only fills zero-valued
// fields.
func TestHardenServerConfig_DoesNotOverrideExplicitValues(t *testing.T) {
	customCiphers := []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384}
	customCurves := []tls.CurveID{tls.CurveP384}

	got := tlsutil.HardenServerConfig(&tls.Config{
		MinVersion:       tls.VersionTLS13,
		CipherSuites:     customCiphers,
		CurvePreferences: customCurves,
	})

	assert.Equal(t, uint16(tls.VersionTLS13), got.MinVersion,
		"explicit MinVersion must not be downgraded")
	assert.Equal(t, customCiphers, got.CipherSuites,
		"explicit CipherSuites must not be replaced")
	assert.Equal(t, customCurves, got.CurvePreferences,
		"explicit CurvePreferences must not be replaced")
}

// TestHardenServerConfig_NilInput — OWASP API8:2023 — passing nil yields a
// usable defaulted config rather than panicking; this keeps caller code
// (tlsutil.HardenServerConfig(nil)) ergonomic in tests.
func TestHardenServerConfig_NilInput(t *testing.T) {
	got := tlsutil.HardenServerConfig(nil)
	require.NotNil(t, got)

	assert.Equal(t, uint16(tls.VersionTLS12), got.MinVersion)
	assert.NotEmpty(t, got.CipherSuites)
	assert.NotEmpty(t, got.CurvePreferences)
}
