package base

import (
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/quercon/rcon"
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
	"github.com/pkg/errors"
)

var mapProtocolByGameCode = map[string]rcon.Protocol{
	"bms":       rcon.ProtocolSource,  // Black Mesa: Source
	"cs":        rcon.ProtocolGoldSrc, // Counter-Strike 1.6
	"cs2":       rcon.ProtocolSource,  // Counter-Strike 2
	"csgo":      rcon.ProtocolSource,  // Counter-Strike: Global Offensive
	"cssource":  rcon.ProtocolSource,  // Counter-Strike: Source
	"cssv34":    rcon.ProtocolSource,  // Counter-Strike: Source v34
	"cstrike":   rcon.ProtocolGoldSrc, // Counter-Strike 1.6
	"czero":     rcon.ProtocolSource,  // Counter-Strike: Condition Zero
	"dmc":       rcon.ProtocolSource,  // Deathmatch Classic
	"dod":       rcon.ProtocolGoldSrc, // Day of Defeat
	"dods":      rcon.ProtocolSource,  // Day of Defeat: Source
	"garrysmod": rcon.ProtocolSource,  // Garry's Mod
	"gearbox":   rcon.ProtocolGoldSrc, // Half-Life: Opposing Force
	"hl":        rcon.ProtocolGoldSrc, // Half-Life
	"hl2mp":     rcon.ProtocolSource,  // Half-Life 2: Deathmatch
	"l4d":       rcon.ProtocolSource,  // Left 4 Dead
	"l4d2":      rcon.ProtocolSource,  // Left 4 Dead 2
	"minecraft": rcon.ProtocolSource,  // Minecraft
	"op4":       rcon.ProtocolGoldSrc, // Half-Life: Opposing Force
	"ricochet":  rcon.ProtocolGoldSrc, // Ricochet
	"svencoop":  rcon.ProtocolGoldSrc, // Sven Co-op
	"tf2":       rcon.ProtocolSource,  // Team Fortress 2
	"tfc":       rcon.ProtocolGoldSrc, // Team Fortress Classic
	"valve":     rcon.ProtocolGoldSrc, // Half-Life
	"q2":        rcon.ProtocolQuake2,  // Quake 2
	"q3":        rcon.ProtocolQuake3,  // Quake 3
	"cod4":      rcon.ProtocolQuake3,  // Call of Duty 4
	"samp":      rcon.ProtocolSAMP,    // GTA: San Andreas Multiplayer
	"arma2":     rcon.ProtocolBattlEye,
	"arma2oa":   rcon.ProtocolBattlEye,
	"arma3":     rcon.ProtocolBattlEye,
}

var mapProtocolByEngine = map[string]rcon.Protocol{
	"goldsource": rcon.ProtocolGoldSrc,
	"goldsrc":    rcon.ProtocolGoldSrc,
	"source":     rcon.ProtocolSource,
	"minecraft":  rcon.ProtocolSource,
	"factorio":   rcon.ProtocolSource, // Factorio headless speaks Valve RCON
	"q2":         rcon.ProtocolQuake2,
	"q3":         rcon.ProtocolQuake3,
	"cod4":       rcon.ProtocolQuake3,
	"samp":       rcon.ProtocolSAMP,
	"arma":       rcon.ProtocolBattlEye,
	"arma3":      rcon.ProtocolBattlEye,

	// Engine names used by panels seeded from an older catalogue snapshot. The current
	// catalogue calls these arma/arma3, and a games upgrade rewrites them, but an installation
	// that never ran one still carries the old spelling.
	"armedassault2":   rcon.ProtocolBattlEye,
	"armedassault2oa": rcon.ProtocolBattlEye,
	"armedassault3":   rcon.ProtocolBattlEye,
}

// legacyIDTechEngine is the id Tech family name an older catalogue snapshot used for both
// Quake 2 and Quake 3; only the engine version tells them apart. The current catalogue uses
// q2 and q3 directly.
const legacyIDTechEngine = "idtech"

var mapIDTechVersionToEngine = map[string]string{
	"2": "q2",
	"3": "q3",
}

// canonicalEngine lower-cases the engine and resolves the one alias that cannot be expressed as
// a plain lookup key, because it depends on the engine version as well.
func canonicalEngine(game domain.Game) string {
	engine := strings.ToLower(game.Engine)
	if engine != legacyIDTechEngine {
		return engine
	}

	if resolved, ok := mapIDTechVersionToEngine[strings.TrimSpace(game.EngineVersion)]; ok {
		return resolved
	}

	return engine
}

func DetermineProtocol(game domain.Game) (rcon.Protocol, error) {
	protocol, err := DetermineProtocolByEngine(canonicalEngine(game))
	if err == nil {
		return protocol, nil
	}

	return DetermineProtocolByGameCode(game.Code)
}

func DetermineProtocolByEngine(engine string) (rcon.Protocol, error) {
	engine = strings.ToLower(engine)

	if protocol, ok := mapProtocolByEngine[engine]; ok {
		return protocol, nil
	}

	return "", errors.Errorf("unable to determine RCON protocol for engine: %s", engine)
}

func DetermineProtocolByGameCode(gameCode string) (rcon.Protocol, error) {
	if protocol, ok := mapProtocolByGameCode[gameCode]; ok {
		return protocol, nil
	}

	return "", errors.Errorf("unable to determine RCON protocol for game code: %s", gameCode)
}

// DeterminePlayerManager picks the players-list parser for a game. The engine family is tried
// first so a custom game with its own code still resolves, then the game-code table, which is
// where the Valve and Minecraft parsers live.
func DeterminePlayerManager(game domain.Game) (players.PlayerManager, error) {
	if manager, err := players.NewPlayerManagerByEngine(canonicalEngine(game)); err == nil {
		return manager, nil
	}

	return players.NewPlayerManagerByGameCode(game.Code)
}
