package twofactor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"

	"github.com/pkg/errors"
)

// secretEncryptionInfo is the HKDF "info" label binding the derived key to this
// specific use. It is versioned: bumping the suffix rotates the encryption key
// for every stored TOTP secret (existing secrets become undecryptable and
// affected users must re-enroll).
const secretEncryptionInfo = "gameap-2fa-secret-v1" //nolint:gosec // G101: HKDF label, not a secret

const derivedKeyLength = 32 // AES-256

// Cipher encrypts and decrypts TOTP secrets at rest with AES-256-GCM. The key
// is HKDF-SHA256-derived from the application key so the raw app secret is
// never used directly and is independent of the auth-token signing key.
type Cipher struct {
	gcm cipher.AEAD
}

func NewCipher(appKey []byte) (*Cipher, error) {
	if len(appKey) == 0 {
		return nil, errors.New("two-factor encryption key is empty")
	}

	key, err := hkdf.Key(sha256.New, appKey, nil, secretEncryptionInfo, derivedKeyLength)
	if err != nil {
		return nil, errors.Wrap(err, "failed to derive two-factor encryption key")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create aes cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create gcm")
	}

	return &Cipher{gcm: gcm}, nil
}

// Encrypt returns the base64 of nonce||ciphertext. A fresh random nonce is
// prepended on every call so identical secrets never produce identical output.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", errors.Wrap(err, "failed to generate nonce")
	}

	sealed := c.gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (c *Cipher) Decrypt(ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode ciphertext")
	}

	nonceSize := c.gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, sealed := raw[:nonceSize], raw[nonceSize:]

	plaintext, err := c.gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to decrypt two-factor secret")
	}

	return string(plaintext), nil
}
