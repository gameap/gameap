package auth

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type Service interface {
	ValidateToken(tokenString string) (Claims, error)
	GenerateTokenForUser(user *domain.User, tokenDuration time.Duration) (string, error)
	// GenerateMFAEnrollmentToken mints a session token scoped to the
	// 2FA-enrollment endpoints (ScopeMFAEnrollment). Issued at login to an
	// admin who must enrol 2FA before regaining full access.
	GenerateMFAEnrollmentToken(user *domain.User, tokenDuration time.Duration) (string, error)
}
