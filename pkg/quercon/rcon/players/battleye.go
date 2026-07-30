package players

import (
	"regexp"
	"strings"
	"time"
)

// BattlEye answers `players` with a fixed table. The name is last and may contain spaces, so the
// trailing group is greedy:
//
//	Players on server:
//	[#] [IP Address]:[Port] [Ping] [GUID] [Name]
//	--------------------------------------------------
//	0   192.0.2.10:2304       45   80c1b8dbf9d9d3f9e1a2b3c4d5e6f708(OK) Nickname
//	1   192.0.2.11:2304       0    -                                    Other Name (Lobby)
//	(2 players in total)
var battlEyeRow = regexp.MustCompile(`^\s*(\d+)\s+(\S+?):(\d+)\s+(\d+)\s+(\S+)\s+(.+?)\s*$`)

// BattlEyePlayerManager reads the player list of a BattlEye-protected server (Arma 2/3, DayZ,
// SCUM). Kicking and banning are not implemented.
type BattlEyePlayerManager struct{}

func NewBattlEyePlayers() PlayerManager {
	return &BattlEyePlayerManager{}
}

func (mgr *BattlEyePlayerManager) Capabilities() Capability {
	return Capability{List: true}
}

func (mgr *BattlEyePlayerManager) PlayersCommand() string {
	return "players"
}

func (mgr *BattlEyePlayerManager) ParsePlayers(data string) ([]Player, error) {
	lines := strings.Split(data, "\n")
	players := make([]Player, 0, len(lines))

	for _, line := range lines {
		match := battlEyeRow.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		players = append(players, Player{
			ID:     match[1],
			Name:   decodeLatin1IfInvalidUTF8(strings.TrimSpace(match[6])),
			Ping:   match[4],
			Addr:   match[2],
			UniqID: battlEyeGUID(match[5]),
		})
	}

	return players, nil
}

func (mgr *BattlEyePlayerManager) KickCommand(_ Player, _ string) (string, error) {
	return "", ErrPlayerActionNotSupported
}

func (mgr *BattlEyePlayerManager) BanCommand(_ Player, _ string, _ time.Duration) (string, error) {
	return "", ErrPlayerActionNotSupported
}

// battlEyeGUID drops the verification status the server appends to the GUID, and turns the
// placeholder used for a player whose GUID is not known yet into an empty value.
func battlEyeGUID(field string) string {
	if field == "-" {
		return ""
	}

	if before, _, ok := strings.Cut(field, "("); ok {
		return before
	}

	return field
}
