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

// twoFactorChallengeResponse is returned instead of a token when the account
// has 2FA enabled: the password was correct, but the caller must still prove
// the second factor at /api/auth/2fa/verify using challengeToken.
type twoFactorChallengeResponse struct {
	TwoFactorRequired bool   `json:"two_factor_required"`
	ChallengeToken    string `json:"challenge_token"`
	ExpiresIn         int64  `json:"expires_in"`
}

func newTwoFactorChallengeResponse(challengeToken string, expiresIn time.Duration) twoFactorChallengeResponse {
	return twoFactorChallengeResponse{
		TwoFactorRequired: true,
		ChallengeToken:    challengeToken,
		ExpiresIn:         int64(expiresIn.Seconds()),
	}
}
