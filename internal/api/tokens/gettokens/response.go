package gettokens

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type tokenResponse struct {
	ID         uint                 `json:"id"`
	Name       string               `json:"name"`
	Abilities  *[]domain.PATAbility `json:"abilities"`
	LastUsedAt *time.Time           `json:"last_used_at"`
	CreatedAt  *time.Time           `json:"created_at"`
}

func newTokensResponseFromTokens(tokens []domain.PersonalAccessToken) []tokenResponse {
	response := make([]tokenResponse, 0, len(tokens))

	for _, token := range tokens {
		response = append(response, newTokenResponseFromToken(&token))
	}

	return response
}

func newTokenResponseFromToken(token *domain.PersonalAccessToken) tokenResponse {
	return tokenResponse{
		ID:         token.ID,
		Name:       token.Name,
		Abilities:  token.Abilities,
		LastUsedAt: token.LastUsedAt,
		CreatedAt:  token.CreatedAt,
	}
}
