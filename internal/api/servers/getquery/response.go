package getquery

import (
	"fmt"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/quercon/query"
)

type queryResponse struct {
	Status   string  `json:"status"`
	Hostname *string `json:"hostname,omitempty"`
	Map      *string `json:"map,omitempty"`
	Players  *string `json:"players,omitempty"`
	Version  *string `json:"version,omitempty"`
	Password *string `json:"password,omitempty"`
	JoinLink *string `json:"joinlink,omitempty"`
}

func newQueryResponse(result *query.Result, _ *domain.Server) queryResponse {
	if result == nil || !result.Online {
		return queryResponse{
			Status: "offline",
		}
	}

	players := fmt.Sprintf("%d/%d", result.PlayersNum, result.MaxPlayersNum)

	return queryResponse{
		Status:   "online",
		Hostname: &result.Name,
		Map:      &result.Map,
		Players:  &players,
	}
}
