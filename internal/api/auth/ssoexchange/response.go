package ssoexchange

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/services/mfanudge"
)

// exchangeResponse mirrors the login response so the SPA can reuse the same
// "store this token" path, plus the redirect target recorded when the ticket
// was minted.
type exchangeResponse struct {
	Token      string   `json:"token"`
	ExpiresIn  int64    `json:"expires_in"`
	User       userInfo `json:"user"`
	RedirectTo string   `json:"redirect_to,omitempty"`

	// MFAEnrollmentRequired and MFANudge carry the admin-MFA policy verdict, in
	// the same shape the password login uses, so the SPA's enforcement modal
	// works identically whichever way the session was obtained.
	MFAEnrollmentRequired bool           `json:"mfa_enrollment_required,omitempty"`
	MFANudge              *mfanudge.View `json:"mfa_nudge,omitempty"`
}

type userInfo struct {
	Login string  `json:"login"`
	Email string  `json:"email"`
	Name  *string `json:"name"`
}

func newExchangeResponse(
	user *domain.User, token string, expiresIn time.Duration, redirectTo string,
) exchangeResponse {
	return exchangeResponse{
		Token:     token,
		ExpiresIn: int64(expiresIn.Seconds()),
		User: userInfo{
			Login: user.Login,
			Email: user.Email,
			Name:  user.Name,
		},
		RedirectTo: redirectTo,
	}
}

// twoFactorChallengeResponse is returned instead of a session when the target
// account has 2FA enabled. Single sign-on must not become a way around the
// second factor, so the ticket buys nothing more than the password would.
type twoFactorChallengeResponse struct {
	TwoFactorRequired bool   `json:"two_factor_required"`
	ChallengeToken    string `json:"challenge_token"`
	ExpiresIn         int64  `json:"expires_in"`
	RedirectTo        string `json:"redirect_to,omitempty"`
}
