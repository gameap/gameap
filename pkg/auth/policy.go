package auth

import (
	"fmt"

	"github.com/gameap/gameap/pkg/api"
)

// Password policy enforced per OWASP ASVS 4.0.3:
//
//	§2.1.1 — user-set passwords are at least 12 characters in length.
//	§2.1.2 — passwords of at least 64 characters are permitted, and passwords
//	         of more than 128 characters are denied. No truncation is
//	         performed; the storage layer pre-hashes (see HashPassword in
//	         password.go) so the full input contributes to the digest.
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
)

// ValidatePassword enforces the OWASP ASVS 4.0.3 §2.1.1 / §2.1.2 length policy
// on a user-supplied password. The returned error is a typed *api.Error so
// handlers can surface it directly through their responder.
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

	return nil
}
