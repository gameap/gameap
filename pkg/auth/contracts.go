package auth

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type Service interface {
	ValidateToken(tokenString string) (Claims, error)
	GenerateTokenForUser(user *domain.User, tokenDuration time.Duration) (string, error)
}
