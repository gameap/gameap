package getrconfeatures

import (
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/quercon"
)

type featuresResponse struct {
	Rcon bool `json:"rcon"`
	// PlayersManage predates the split between listing and moderating players. It mirrors
	// PlayersList so clients released before the split keep working.
	PlayersManage bool `json:"playersManage"`
	PlayersList   bool `json:"playersList"`
	PlayersKick   bool `json:"playersKick"`
	PlayersBan    bool `json:"playersBan"`
}

func newFeaturesResponse(resolver *quercon.Resolver, game domain.Game) featuresResponse {
	features := resolver.RconFeatures(game)

	return featuresResponse{
		Rcon:          features.Rcon,
		PlayersManage: features.PlayersList,
		PlayersList:   features.PlayersList,
		PlayersKick:   features.PlayersKick,
		PlayersBan:    features.PlayersBan,
	}
}
