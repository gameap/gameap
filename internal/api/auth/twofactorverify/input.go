package twofactorverify

import (
	"strings"

	"github.com/gameap/gameap/pkg/api"
)

var (
	ErrChallengeTokenRequired = api.NewValidationError("challenge_token field is required")
	ErrCodeRequired           = api.NewValidationError("code field is required")
)

type verifyInput struct {
	ChallengeToken string `json:"challenge_token"`
	Code           string `json:"code"`
}

func (i *verifyInput) Validate() error {
	i.ChallengeToken = strings.TrimSpace(i.ChallengeToken)
	i.Code = strings.TrimSpace(i.Code)

	if i.ChallengeToken == "" {
		return ErrChallengeTokenRequired
	}

	if i.Code == "" {
		return ErrCodeRequired
	}

	return nil
}
