package twofactor

import (
	"time"

	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

// DefaultIssuer is the label shown by authenticator apps next to the account.
const DefaultIssuer = "GameAP"

// Manager bundles the TOTP, secret-encryption and recovery-code primitives
// behind a single injectable dependency. The clock is overridable so handler
// tests can validate codes at a fixed time.
type Manager struct {
	cipher *Cipher
	issuer string
	now    func() time.Time
}

type Option func(*Manager)

// WithClock overrides the time source used for TOTP validation.
func WithClock(now func() time.Time) Option {
	return func(m *Manager) {
		if now != nil {
			m.now = now
		}
	}
}

// WithIssuer overrides the issuer label embedded in the otpauth URI.
func WithIssuer(issuer string) Option {
	return func(m *Manager) {
		if issuer != "" {
			m.issuer = issuer
		}
	}
}

// NewManager derives the at-rest encryption key from appKey. The app key may
// carry a "base64:"/"ascii85:" prefix (same convention as the auth secret); it
// is decoded here so callers can pass the raw config value.
func NewManager(appKey []byte, opts ...Option) (*Manager, error) {
	cipher, err := NewCipher(auth.DecodeWithPrefix(appKey))
	if err != nil {
		return nil, errors.WithMessage(err, "failed to initialise two-factor cipher")
	}

	m := &Manager{
		cipher: cipher,
		issuer: DefaultIssuer,
		now:    time.Now,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m, nil
}

// EncryptSecret encrypts a freshly generated base32 TOTP secret for storage.
func (m *Manager) EncryptSecret(secret string) (string, error) {
	return m.cipher.Encrypt(secret)
}
