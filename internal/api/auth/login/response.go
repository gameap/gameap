package login

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type loginResponse struct {
	Token     string   `json:"token"`
	ExpiresIn int64    `json:"expires_in"` // Token expiration in seconds
	User      userInfo `json:"user"`
}

type userInfo struct {
	Login string  `json:"login"`
	Email string  `json:"email"`
	Name  *string `json:"name"`
}

func newLoginResponseFromUser(user *domain.User, token string, expiresIn time.Duration) loginResponse {
	return loginResponse{
		Token:     token,
		ExpiresIn: int64(expiresIn.Seconds()),
		User: userInfo{
			Login: user.Login,
			Email: user.Email,
			Name:  user.Name,
		},
	}
}
