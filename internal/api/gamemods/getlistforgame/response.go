package getlistforgame

import (
	"github.com/gameap/gameap/internal/domain"
)

type GameModResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func newGameModsResponseFromGameMods(gameMods []domain.GameMod) []GameModResponse {
	response := make([]GameModResponse, 0, len(gameMods))

	for _, gm := range gameMods {
		response = append(response, GameModResponse{
			ID:   gm.ID,
			Name: gm.Name,
		})
	}

	return response
}
