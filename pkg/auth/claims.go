package auth

import "time"

// ScopeMFAEnrollment marks a session token whose bearer may only reach the
// 2FA-enrollment endpoints. It is set on the token issued at login when an
// admin without 2FA has crossed the MFA hard-fail threshold, forcing them to
// enrol before they regain full API access.
const ScopeMFAEnrollment = "mfa_enrollment"

type Claims interface {
	GetSubject() (string, error)
	// GetExpirationTime returns the token's `exp` claim if present, or nil
	// when the token has no expiration (e.g. some Personal Access Token
	// flows). Implementations may return an error if claims are unparseable.
	GetExpirationTime() (*time.Time, error)
	// GetScope returns the token's custom "scope" claim, or "" when the token
	// carries no scope (an ordinary, unrestricted session). An error is
	// returned only when the claims are structurally unreadable.
	GetScope() (string, error)
}
