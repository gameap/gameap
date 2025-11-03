package getrconfeatures

import (
	"strings"
)

type featuresResponse struct {
	Rcon          bool `json:"rcon"`
	PlayersManage bool `json:"playersManage"`
}

func newFeaturesResponse(engine string) featuresResponse {
	return featuresResponse{
		Rcon:          true,
		PlayersManage: isGoldSourceEngine(engine),
	}
}

func isGoldSourceEngine(engine string) bool {
	return strings.ToLower(engine) == "goldsource"
}
