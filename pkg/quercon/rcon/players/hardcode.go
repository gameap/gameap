package players

import "errors"

var (
	ErrPlayersManagementNotSupported = errors.New("players management is not supported for this game")

	// ErrPlayerActionNotSupported is returned by a manager that can list players but cannot
	// build a kick or ban command for its game.
	ErrPlayerActionNotSupported = errors.New("player action is not supported for this game")
)

var mapPlayerManagersByGameCode = map[string]func() PlayerManager{
	"cs":        NewValvePlayers,
	"cstrike":   NewValvePlayers,
	"tfc":       NewValvePlayers,
	"dod":       NewValvePlayers,
	"gearbox":   NewValvePlayers,
	"hl":        NewValvePlayers,
	"valve":     NewValvePlayers,
	"minecraft": NewMinecraftPlayers,
	"q2":        NewQuakePlayers,
	"q3":        NewQuakePlayers,
	"cod4":      NewQuakePlayers,
	"samp":      NewSAMPPlayers,
	"arma2":     NewBattlEyePlayers,
	"arma2oa":   NewBattlEyePlayers,
	"arma3":     NewBattlEyePlayers,
}

// mapPlayerManagersByEngine covers whole engine families, so a custom game with its own code
// still gets a parser. Only the families added alongside the UDP protocols are listed: mapping
// source and goldsource here would hand a players panel to a dozen games that do not have one
// today. Callers must lower-case the engine, and resolve any version-dependent alias first.
var mapPlayerManagersByEngine = map[string]func() PlayerManager{
	"q2":              NewQuakePlayers,
	"q3":              NewQuakePlayers,
	"cod4":            NewQuakePlayers,
	"samp":            NewSAMPPlayers,
	"arma":            NewBattlEyePlayers,
	"arma3":           NewBattlEyePlayers,
	"armedassault2":   NewBattlEyePlayers,
	"armedassault2oa": NewBattlEyePlayers,
	"armedassault3":   NewBattlEyePlayers,
}

func NewPlayerManagerByGameCode(gameCode string) (PlayerManager, error) {
	if constructor, ok := mapPlayerManagersByGameCode[gameCode]; ok {
		return constructor(), nil
	}

	return nil, ErrPlayersManagementNotSupported
}

// NewPlayerManagerByEngine resolves a manager for a whole engine family.
func NewPlayerManagerByEngine(engine string) (PlayerManager, error) {
	if constructor, ok := mapPlayerManagersByEngine[engine]; ok {
		return constructor(), nil
	}

	return nil, ErrPlayersManagementNotSupported
}

func IsPlayerManagementSupported(gameCode string) bool {
	_, ok := mapPlayerManagersByGameCode[gameCode]

	return ok
}
