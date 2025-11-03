package posttoken

import (
	"fmt"
	"strings"

	"github.com/gameap/gameap/internal/domain"
)

type tokenResponse struct {
	Token string `json:"token"`
}

func newTokenResponse(token *domain.PersonalAccessToken, plainToken string) *tokenResponse {
	b := strings.Builder{}
	b.Grow(len(plainToken) + 21)

	b.WriteString(plainToken)

	return &tokenResponse{
		Token: fmt.Sprintf("%d|%s", token.ID, plainToken),
	}
}
