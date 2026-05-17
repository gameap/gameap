package strings

import (
	"crypto/sha256"
	"encoding/hex"
)

func SHA256(v string) string {
	hash := sha256.Sum256([]byte(v))

	return hex.EncodeToString(hash[:])
}

// IsSHA256Hex reports whether s is already a SHA-256 digest in lowercase hex
// (64 hex characters). Used to keep hashing idempotent so an already-hashed
// value is not hashed again.
func IsSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}

	return true
}

// SHA256IfNeeded returns SHA256(v) unless v already looks like a SHA-256 hex
// digest, in which case it is returned unchanged. Use this on externally
// supplied secrets that must be stored hashed without double-hashing a value
// that was already a hash.
func SHA256IfNeeded(v string) string {
	if IsSHA256Hex(v) {
		return v
	}

	return SHA256(v)
}
