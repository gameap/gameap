// Package tlsutil centralises TLS configuration helpers used across HTTP, gRPC
// and the TLS multiplexer so a single audit point governs cipher selection,
// elliptic-curve preferences and other policy knobs that would otherwise drift.
package tlsutil

import "crypto/tls"

// ModernCipherSuites returns the explicit list of TLS 1.2 cipher suites the
// project uses on every listener. Documented choices, in priority order:
//
//   - AEAD-only: every suite below uses an authenticated-encryption mode
//     (AES-GCM or ChaCha20-Poly1305). CBC-MAC and RC4 are deliberately
//     excluded — Lucky13 / BEAST / RC4-bias still bite Go's older defaults
//     when an operator forces TLS 1.2 in a downgrade-tolerant network.
//   - ECDHE for forward secrecy: static RSA key exchange and DHE_RSA are
//     excluded.
//   - ChaCha20-Poly1305 first when the client lacks an AES-NI accelerator
//     (mobile, ARM SBCs running game-server nodes); Go's runtime decides this
//     dynamically via the cipher-suite ordering published here.
//   - ECDSA preferred over RSA (ECDSA-P256 is ~2× faster and matches the
//     panel's enrollment certificate flow which mints ECDSA leaves).
//
// TLS 1.3 suites are NOT returned: tls.Config.CipherSuites is ignored for
// TLS 1.3 by the Go stdlib, which picks from a fixed AEAD-only set
// (TLS_AES_128_GCM_SHA256 / TLS_AES_256_GCM_SHA384 /
// TLS_CHACHA20_POLY1305_SHA256). Listing them here would be misleading.
//
// The slice is intentionally returned by value so callers cannot mutate the
// package-level state. The constant cost (one alloc per listener init) is
// dwarfed by the TLS handshake itself.
func ModernCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	}
}

// PreferredCurves returns the elliptic curves the project prefers, in priority
// order. X25519 is fastest on every modern CPU and resistant to invalid-curve
// attacks; P-256 is the lowest-common-denominator for legacy clients;
// P-384/P-521 are omitted (slower than X25519 with no real security upside on
// the wire and not required by any target client).
func PreferredCurves() []tls.CurveID {
	return []tls.CurveID{
		tls.X25519,
		tls.CurveP256,
	}
}

// HardenServerConfig stamps the project-wide TLS defaults onto a server-side
// *tls.Config: TLS 1.2 floor, explicit cipher list, preferred curves. It does
// not overwrite Certificates, GetCertificate, ClientAuth or any other field —
// callers compose this with their own listener configuration.
//
// HardenServerConfig is safe to call on an already-hardened config: it only
// sets fields that are zero-valued.
func HardenServerConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		cfg = &tls.Config{}
	}

	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS12
	}

	if cfg.CipherSuites == nil {
		cfg.CipherSuites = ModernCipherSuites()
	}

	if cfg.CurvePreferences == nil {
		cfg.CurvePreferences = PreferredCurves()
	}

	return cfg
}
