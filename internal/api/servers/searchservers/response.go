package searchservers

import (
	"github.com/gameap/gameap/internal/domain"
)

type searchServerResponse struct {
	ID         uint                `json:"id"`
	Name       string              `json:"name"`
	ServerIP   string              `json:"server_ip"`
	ServerPort int                 `json:"server_port"`
	GameID     string              `json:"game_id"`
	GameModID  uint                `json:"game_mod_id"`
	Game       *searchGameResponse `json:"game,omitempty"`
}

type searchGameResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func newSearchServersResponseFromServers(
	servers []*domain.Server, games map[string]domain.Game,
) []searchServerResponse {
	response := make([]searchServerResponse, 0, len(servers))

	for _, s := range servers {
		g, ok := games[s.GameID]
		if !ok {
			continue
		}

		response = append(response, newSearchServerResponseFromServer(s, &g))
	}

	return response
}

func newSearchServerResponseFromServer(s *domain.Server, g *domain.Game) searchServerResponse {
	return searchServerResponse{
		ID:         s.ID,
		Name:       s.Name,
		ServerIP:   s.ServerIP,
		ServerPort: s.ServerPort,
		GameID:     s.GameID,
		GameModID:  s.GameModID,
		Game: &searchGameResponse{
			Code: g.Code,
			Name: g.Name,
		},
	}
}
