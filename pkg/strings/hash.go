package strings

import (
	"crypto/sha256"
	"encoding/hex"
)

func SHA256(v string) string {
	hash := sha256.Sum256([]byte(v))

	return hex.EncodeToString(hash[:])
}
