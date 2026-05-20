package auth

import (
	"crypto/sha256"
	"encoding/base64"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultBcryptCost = bcrypt.DefaultCost

	// legacyBcryptMaxBytes is bcrypt's documented input limit. Raw passwords
	// longer than this would error in the legacy verification path, so the
	// fallback is skipped above this threshold.
	legacyBcryptMaxBytes = 72
)

// preHash applies SHA-256 + base64 so the bcrypt step always receives a
// fixed 44-character ASCII pre-image, neutralising bcrypt's 72-byte input
// limit without truncating user input. See OWASP ASVS 4.0.3 §2.1.2 / §2.1.3.
//
// SHA-256 produces 32 bytes, std-base64 encodes that as 44 ASCII characters,
// well below 72 and free of NUL bytes that would otherwise trigger bcrypt's
// historical C-string truncation behaviour.
func preHash(password string) []byte {
	sum := sha256.Sum256([]byte(password))

	return []byte(base64.StdEncoding.EncodeToString(sum[:]))
}

// HashPassword hashes a plain-text password using bcrypt over a SHA-256
// pre-hash, so passwords of any length up to MaxPasswordLength can be stored
// without truncation (OWASP ASVS 4.0.3 §2.1.2 / §2.1.3).
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword(preHash(password), DefaultBcryptCost)
	if err != nil {
		return "", errors.Wrap(err, "failed to hash password")
	}

	return string(bytes), nil
}

// VerifyPassword compares a stored bcrypt hash against a candidate password.
//
// It accepts both the current scheme (bcrypt over a SHA-256 pre-hash) and the
// legacy scheme (bcrypt over the raw password) so accounts created before the
// §2.1.2 pre-hash migration continue to authenticate.
//
// When the stored hash matches under the legacy scheme, needsRehash is true
// and the caller MUST re-hash the password with HashPassword and persist the
// upgraded hash to migrate the user in-place.
func VerifyPassword(hashedPassword, password string) (needsRehash bool, err error) {
	if err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), preHash(password)); err == nil {
		return false, nil
	}

	if len(password) <= legacyBcryptMaxBytes {
		if legacyErr := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)); legacyErr == nil {
			return true, nil
		}
	}

	return false, errors.Wrap(err, "password verification failed")
}
