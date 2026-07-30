package players

import (
	"strconv"
	"strings"
	"time"
)

// SAMPPlayerManager reads the player list of a SA-MP or open.mp server. The console prints a
// tab-separated table:
//
//	ID	Name	Ping	IP
//	0	Alice	42	192.0.2.10
//	1	Bob	53	192.0.2.11
//
// An empty server prints nothing at all, which is indistinguishable from a command that produced
// no output — the transport handles that by probing the server during Open.
//
// Kicking and banning are not implemented.
type SAMPPlayerManager struct{}

func NewSAMPPlayers() PlayerManager {
	return &SAMPPlayerManager{}
}

func (mgr *SAMPPlayerManager) Capabilities() Capability {
	return Capability{List: true}
}

func (mgr *SAMPPlayerManager) PlayersCommand() string {
	return "players"
}

func (mgr *SAMPPlayerManager) ParsePlayers(data string) ([]Player, error) {
	lines := strings.Split(data, "\n")
	players := make([]Player, 0, len(lines))

	for _, line := range lines {
		// Fields are tab-separated, but fall back to any whitespace: SA-MP nicknames cannot
		// contain spaces, so splitting on them is safe.
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}

		// Skips the header row and any console message that is not a player line.
		if _, err := strconv.Atoi(fields[0]); err != nil {
			continue
		}

		player := Player{
			ID:     fields[0],
			Name:   fields[1],
			UniqID: fields[0],
		}

		if len(fields) >= 3 {
			player.Ping = fields[2]
		}

		if len(fields) >= 4 {
			player.Addr = fields[3]
		}

		players = append(players, player)
	}

	return players, nil
}

func (mgr *SAMPPlayerManager) KickCommand(_ Player, _ string) (string, error) {
	return "", ErrPlayerActionNotSupported
}

func (mgr *SAMPPlayerManager) BanCommand(_ Player, _ string, _ time.Duration) (string, error) {
	return "", ErrPlayerActionNotSupported
}
