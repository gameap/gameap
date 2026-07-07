package putprofile

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

// tokenIssuer is the subset of auth.Service the handler needs to mint a fresh
// session token after a password change, which invalidates every
// previously-issued token — including the one that made the request.
type tokenIssuer interface {
	GenerateTokenForUser(user *domain.User, tokenDuration time.Duration) (string, error)
}
