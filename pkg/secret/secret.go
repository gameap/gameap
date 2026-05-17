// Package secret provides reversible AES-256-GCM encryption for credentials
// that must be stored at rest but read back in plaintext (e.g. the legacy
// gdaemon SSH password). Values are tagged with an "enc:" prefix so plaintext
// written before encryption was enabled keeps working (backward compatible).
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"

	"github.com/pkg/errors"
)

// EncPrefix marks an encrypted value. A stored value without this prefix is
// treated as legacy plaintext and returned unchanged by Decrypt.
const EncPrefix = "enc:"

// Cipher encrypts/decrypts short secrets. A Cipher built from an empty key is
// "disabled": Encrypt is a no-op and Decrypt only passes through legacy
// plaintext (an encrypted value without a key is a hard error).
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher derives a 256-bit key from key via SHA-256 and returns an
// AES-256-GCM cipher. An empty key yields a disabled cipher.
//
// The derivation is a single, unsalted SHA-256 (intentionally not a slow KDF —
// it runs on every request). It therefore preserves, but does not amplify, the
// entropy of the configured value. ENCRYPTION_KEY must be a high-entropy random
// secret (e.g. 32 bytes from a CSPRNG), not a human-chosen passphrase: a
// low-entropy key is feasible to brute-force offline if the ciphertext leaks.
func NewCipher(key string) (*Cipher, error) {
	if key == "" {
		return &Cipher{}, nil
	}

	sum := sha256.Sum256([]byte(key))

	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AES cipher")
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create GCM")
	}

	return &Cipher{aead: aead}, nil
}

// Disabled returns a no-op cipher (passthrough). Used as a safe default when
// no ENCRYPTION_KEY is configured.
func Disabled() *Cipher {
	return &Cipher{}
}

// Enabled reports whether a key is configured.
func (c *Cipher) Enabled() bool {
	return c != nil && c.aead != nil
}

// Encrypt returns an "enc:"-prefixed, base64 nonce||ciphertext string. It is a
// no-op for empty input, an already-encrypted value, or a disabled cipher.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if !c.Enabled() || plaintext == "" || strings.HasPrefix(plaintext, EncPrefix) {
		return plaintext, nil
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.Wrap(err, "failed to generate nonce")
	}

	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)

	return EncPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. A value without the "enc:" prefix is returned
// unchanged (legacy plaintext). An encrypted value with no key configured is
// an error so a misconfiguration is loud rather than silently leaking nothing.
func (c *Cipher) Decrypt(stored string) (string, error) {
	if !strings.HasPrefix(stored, EncPrefix) {
		return stored, nil
	}

	if !c.Enabled() {
		return "", errors.New("value is encrypted but ENCRYPTION_KEY is not configured")
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, EncPrefix))
	if err != nil {
		return "", errors.Wrap(err, "failed to decode encrypted value")
	}

	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("encrypted value is too short")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]

	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to decrypt value")
	}

	return string(plaintext), nil
}
