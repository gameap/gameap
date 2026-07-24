package getrconfeatures

import (
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/quercon"
)

type featuresResponse struct {
	Rcon          bool `json:"rcon"`
	PlayersManage bool `json:"playersManage"`
}

func newFeaturesResponse(resolver *quercon.Resolver, game domain.Game) featuresResponse {
	rconSupported, playersSupported := resolver.RconFeatures(game)

	return featuresResponse{
		Rcon:          rconSupported,
		PlayersManage: playersSupported,
	}
}
