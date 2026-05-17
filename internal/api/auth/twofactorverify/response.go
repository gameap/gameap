package twofactorverify

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type verifyResponse struct {
	Token     string   `json:"token"`
	ExpiresIn int64    `json:"expires_in"`
	User      userInfo `json:"user"`
}

type userInfo struct {
	Login string  `json:"login"`
	Email string  `json:"email"`
	Name  *string `json:"name"`
}

func newVerifyResponse(user *domain.User, token string, expiresIn time.Duration) verifyResponse {
	return verifyResponse{
		Token:     token,
		ExpiresIn: int64(expiresIn.Seconds()),
		User: userInfo{
			Login: user.Login,
			Email: user.Email,
			Name:  user.Name,
		},
	}
}
