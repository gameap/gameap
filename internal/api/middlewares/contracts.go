package middlewares

import "github.com/gameap/gameap/pkg/auth"

type authService interface {
	ValidateToken(tokenString string) (auth.Claims, error)
}
