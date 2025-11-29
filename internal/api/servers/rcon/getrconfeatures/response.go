package getrconfeatures

import (
	"github.com/gameap/gameap/internal/api/servers/rcon/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/quercon/rcon"
)

type featuresResponse struct {
	Rcon          bool `json:"rcon"`
	PlayersManage bool `json:"playersManage"`
}

func newFeaturesResponse(game domain.Game) featuresResponse {
	protocol, err := base.DetermineProtocol(game)
	if err != nil {
		return featuresResponse{
			Rcon:          false,
			PlayersManage: false,
		}
	}

	return featuresResponse{
		Rcon:          rcon.IsProtocolSupported(protocol),
		PlayersManage: rcon.IsPlayerManagementSupported(game.Code),
	}
}
