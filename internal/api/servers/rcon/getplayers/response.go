package getplayers

import (
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
)

type playerResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Score string `json:"score"`
	Ping  string `json:"ping"`
	IP    string `json:"ip"`
}

func newPlayersResponse(playersList []players.Player) []playerResponse {
	response := make([]playerResponse, 0, len(playersList))

	for _, player := range playersList {
		response = append(response, playerResponse{
			ID:    player.ID,
			Name:  player.Name,
			Score: player.Score,
			Ping:  player.Ping,
			IP:    player.Addr,
		})
	}

	return response
}
