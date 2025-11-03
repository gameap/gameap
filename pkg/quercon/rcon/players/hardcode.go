package players

import "errors"

var (
	ErrPlayersManagementNotSupported = errors.New("players management is not supported for this game")
)

var mapPlayerManagersByGameCode = map[string]func() PlayerManager{
	"cs":      NewValvePlayers,
	"cstrike": NewValvePlayers,
	"tfc":     NewValvePlayers,
	"dod":     NewValvePlayers,
	"gearbox": NewValvePlayers,
	"hl":      NewValvePlayers,
	"valve":   NewValvePlayers,
}

func NewPlayerManagerByGameCode(gameCode string) (PlayerManager, error) {
	if constructor, ok := mapPlayerManagersByGameCode[gameCode]; ok {
		return constructor(), nil
	}

	return nil, ErrPlayersManagementNotSupported
}
