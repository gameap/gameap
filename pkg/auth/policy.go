package auth

import (
	"fmt"
	"strings"

	"github.com/gameap/gameap/pkg/api"
)

// Password policy enforced per OWASP ASVS 4.0.3:
//
//	§2.1.1 — user-set passwords are at least 12 characters in length.
//	§2.1.2 — passwords of at least 64 characters are permitted, and passwords
//	         of more than 128 characters are denied. No truncation is
//	         performed; the storage layer pre-hashes (see HashPassword in
//	         password.go) so the full input contributes to the digest.
//	§2.1.7 — passwords are checked against a common-password blocklist
//	         (see blocklist.go); the operator can opt out with
//	         AUTH_ALLOW_WEAK_PASSWORDS=true.
const (
	MinPasswordLength = 12
	MaxPasswordLength = 128
)

var (
	ErrPasswordRequired = api.NewValidationError("password is required")
	ErrPasswordTooShort = api.NewValidationError(
		fmt.Sprintf("password must be at least %d characters long", MinPasswordLength),
	)
	ErrPasswordTooLong = api.NewValidationError(
		fmt.Sprintf("password must not exceed %d characters", MaxPasswordLength),
	)
	ErrPasswordBlocked = api.NewValidationError("password is too common, please choose another")
)

// ValidatePassword enforces the OWASP ASVS 4.0.3 §2.1.1 / §2.1.2 / §2.1.7
// policy on a user-supplied password. The returned error is a typed
// *api.Error so handlers can surface it directly through their responder.
//
// The blocklist lookup is skipped when the operator sets
// AUTH_ALLOW_WEAK_PASSWORDS=true (see SetAllowWeakPasswords) or when no
// blocklist has been installed (default test/dev state — NoopBlocklist).
func ValidatePassword(password string) error {
	if password == "" {
		return ErrPasswordRequired
	}

	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}

	if len(password) > MaxPasswordLength {
		return ErrPasswordTooLong
	}

	policyMu.RLock()
	allowWeak := allowWeakPasswords
	blocklist := passwordBlocklist
	policyMu.RUnlock()

	if !allowWeak && blocklist != nil && blocklist.Contains(strings.ToLower(password)) {
		return ErrPasswordBlocked
	}

	return nil
}
