package players

import (
	"regexp"
	"strings"
	"time"
)

// The `status` output differs slightly across the id Tech family, so the row is matched
// right-anchored on its fixed tail instead of by column positions — the name column is padded
// and can itself contain spaces, and Quake 3 appends a colour reset to every nickname.
//
// Quake 3 / Wolfenstein: ET:
//
//	map: q3dm17
//	num score ping name            lastmsg address               qport rate
//	--- ----- ---- --------------- ------- --------------------- ----- -----
//	  0    15   45 Player^7              50 192.0.2.10:27960      12345 25000
//
// Call of Duty adds a 32-character GUID column between ping and name:
//
//	num score ping guid                             name            lastmsg address               qport rate
//	  0     0   58 0123456789abcdef0123456789abcdef Player^7              0 192.0.2.10:28960      12345 25000
//
// Quake 2 has no rate column, and prints CNCT or ZMBI in place of the ping for clients that are
// still connecting or already gone:
//
//	num score ping name            lastmsg address               qport
//	  0    12 CNCT Player                0 192.0.2.10:27901       1234
var (
	quakeRowWithGUID = regexp.MustCompile(
		`^\s*(\d+)\s+(-?\d+)\s+(\S+)\s+([0-9a-fA-F]{32})\s+(.*?)\s+(-?\d+)\s+(\S+)\s+(\d+)\s+(\d+)\s*$`)
	quakeRowWithRate = regexp.MustCompile(
		`^\s*(\d+)\s+(-?\d+)\s+(\S+)\s+(.*?)\s+(-?\d+)\s+(\S+)\s+(\d+)\s+(\d+)\s*$`)
	quakeRow = regexp.MustCompile(
		`^\s*(\d+)\s+(-?\d+)\s+(\S+)\s+(.*?)\s+(-?\d+)\s+(\S+)\s+(\d+)\s*$`)

	quakeColorCode = regexp.MustCompile(`\^[0-9]`)
)

// QuakePlayerManager reads the player list of an id Tech 2 or id Tech 3 server. Kicking and
// banning are not implemented: the commands differ between Quake 3, the Call of Duty titles and
// Wolfenstein: Enemy Territory, so there is no single template that would be correct.
type QuakePlayerManager struct{}

func NewQuakePlayers() PlayerManager {
	return &QuakePlayerManager{}
}

func (mgr *QuakePlayerManager) Capabilities() Capability {
	return Capability{List: true}
}

func (mgr *QuakePlayerManager) PlayersCommand() string {
	return "status"
}

func (mgr *QuakePlayerManager) ParsePlayers(data string) ([]Player, error) {
	lines := strings.Split(data, "\n")
	players := make([]Player, 0, len(lines))

	for _, line := range lines {
		player, ok := parseQuakePlayerLine(line)
		if !ok {
			continue
		}

		players = append(players, player)
	}

	return players, nil
}

func (mgr *QuakePlayerManager) KickCommand(_ Player, _ string) (string, error) {
	return "", ErrPlayerActionNotSupported
}

func (mgr *QuakePlayerManager) BanCommand(_ Player, _ string, _ time.Duration) (string, error) {
	return "", ErrPlayerActionNotSupported
}

// parseQuakePlayerLine tries the row layouts from the most specific to the least, so a Call of
// Duty row is not mistaken for a Quake 3 one whose nickname happens to start with a GUID-shaped
// token. A line that matches none of them is a header, a separator or a message, and is skipped.
func parseQuakePlayerLine(line string) (Player, bool) {
	if match := quakeRowWithGUID.FindStringSubmatch(line); match != nil {
		return buildQuakePlayer(match[1], match[2], match[3], match[5], match[7], match[4]), true
	}

	if match := quakeRowWithRate.FindStringSubmatch(line); match != nil {
		return buildQuakePlayer(match[1], match[2], match[3], match[4], match[6], ""), true
	}

	if match := quakeRow.FindStringSubmatch(line); match != nil {
		return buildQuakePlayer(match[1], match[2], match[3], match[4], match[6], ""), true
	}

	return Player{}, false
}

func buildQuakePlayer(id, score, ping, name, address, guid string) Player {
	player := Player{
		ID:     id,
		Name:   decodeLatin1IfInvalidUTF8(strings.TrimSpace(quakeColorCode.ReplaceAllString(name, ""))),
		Score:  score,
		Ping:   ping,
		Addr:   stripQuakePort(address),
		UniqID: guid,
	}

	// The client slot is the only stable identity these engines expose when a game has no GUID.
	if player.UniqID == "" {
		player.UniqID = id
	}

	return player
}

// stripQuakePort drops the port from an address. Bots and local clients are reported with a
// literal placeholder instead of an address, which is passed through unchanged.
func stripQuakePort(address string) string {
	if address == "bot" || address == "loopback" {
		return address
	}

	if before, _, ok := strings.Cut(address, ":"); ok {
		return before
	}

	return address
}
