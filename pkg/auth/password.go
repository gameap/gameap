package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"sync/atomic"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultBcryptCost is the cost factor every new hash uses unless
	// SetDefaultBcryptCost has been called at boot to install the
	// operator-chosen value. The constant is retained at bcrypt.DefaultCost
	// so existing code paths and tests that call HashPassword(...) without
	// any configuration coupling keep producing the same hashes.
	DefaultBcryptCost = bcrypt.DefaultCost

	// MinBcryptCost / MaxBcryptCost gate operator configuration. Below 10 is
	// below the bcrypt library's own documented minimum-for-production; above
	// 14 starts to cost a full second per hash on commodity CPUs, which
	// would itself become an availability problem.
	MinBcryptCost = 10
	MaxBcryptCost = 14

	// legacyBcryptMaxBytes is bcrypt's documented input limit. Raw passwords
	// longer than this would error in the legacy verification path, so the
	// fallback is skipped above this threshold.
	legacyBcryptMaxBytes = 72
)

// activeCost is the cost HashPassword uses for new hashes. atomic.Int32 lets
// SetDefaultBcryptCost be called once at boot and then read concurrently from
// every login / password-change handler without locking. The default matches
// the package constant so callers that never touch the config (tests,
// seeders) keep the historical behaviour.
var activeCost atomic.Int32

func init() {
	activeCost.Store(int32(DefaultBcryptCost))
}

// SetDefaultBcryptCost installs the operator-chosen cost factor for every
// subsequent HashPassword call. It returns an error and leaves the active
// cost untouched when the value is outside [MinBcryptCost, MaxBcryptCost]
// so a misconfigured AUTH_BCRYPT_COST cannot ship a weaker default than the
// project floor.
//
// Safe for concurrent use; intended to be invoked exactly once during
// application boot.
func SetDefaultBcryptCost(cost int) error {
	if cost < MinBcryptCost || cost > MaxBcryptCost {
		return errors.Errorf("bcrypt cost %d outside [%d,%d]", cost, MinBcryptCost, MaxBcryptCost)
	}

	activeCost.Store(int32(cost))

	return nil
}

// ActiveBcryptCost returns the cost factor HashPassword currently uses.
// Callers (login handler) compare this to a hash's stored cost via
// HashCost to decide whether to rehash on the way through.
func ActiveBcryptCost() int {
	return int(activeCost.Load())
}

// HashCost returns the bcrypt cost embedded in a stored hash. It is a thin
// wrapper around bcrypt.Cost so callers do not have to import golang.org/x/
// crypto/bcrypt just to drive the rehash-on-login flow.
func HashCost(hash string) (int, error) {
	return bcrypt.Cost([]byte(hash))
}

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
// without truncation (OWASP ASVS 4.0.3 §2.1.2 / §2.1.3). The cost is the
// one installed by SetDefaultBcryptCost (defaults to bcrypt.DefaultCost = 10
// so existing tests / seeders see no change).
func HashPassword(password string) (string, error) {
	return HashPasswordWithCost(password, ActiveBcryptCost())
}

// HashPasswordWithCost is the explicit form used by code paths that need to
// pin a specific cost (typically the boot-time hash precomputation for the
// timing-oracle dummy).
func HashPasswordWithCost(password string, cost int) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword(preHash(password), cost)
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
