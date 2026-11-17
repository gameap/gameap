package players

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// RustPlayerManager handles Rust (Facepunch). The playerlist command answers with a JSON array,
// and players are addressed by their SteamID.
type RustPlayerManager struct{}

func NewRustPlayers() PlayerManager {
	return &RustPlayerManager{}
}

type rustPlayer struct {
	SteamID     json.Number `json:"SteamID"`
	DisplayName string      `json:"DisplayName"`
	Ping        json.Number `json:"Ping"`
	Address     string      `json:"Address"`
}

func (mgr *RustPlayerManager) Capabilities() Capability {
	return Capability{List: true, Kick: true, Ban: true}
}

func (mgr *RustPlayerManager) ParsePlayers(data string) ([]Player, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return []Player{}, nil
	}

	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.UseNumber()

	var list []rustPlayer
	if err := decoder.Decode(&list); err != nil {
		return nil, errors.Wrap(err, "invalid playerlist response")
	}

	players := make([]Player, 0, len(list))
	for _, p := range list {
		steamID := p.SteamID.String()

		players = append(players, Player{
			ID:     steamID,
			UniqID: steamID,
			Name:   p.DisplayName,
			Ping:   p.Ping.String(),
			Addr:   p.Address,
		})
	}

	return players, nil
}

func (mgr *RustPlayerManager) PlayersCommand() string {
	return "playerlist"
}

func (mgr *RustPlayerManager) KickCommand(player Player, reason string) (string, error) {
	if err := player.ValidateUniqID(); err != nil {
		return "", err
	}

	return "kick " + quoteRustArg(player.UniqID) + " " + quoteRustArg(reason), nil
}

// BanCommand bans by SteamID. Rust's banid takes the duration as an optional fourth argument
// with a unit suffix (a bare number means hours); without it the ban is permanent.
func (mgr *RustPlayerManager) BanCommand(player Player, reason string, duration time.Duration) (string, error) {
	if err := player.ValidateUniqID(); err != nil {
		return "", err
	}

	sb := strings.Builder{}
	sb.Grow(96)

	sb.WriteString("banid ")
	sb.WriteString(quoteRustArg(player.UniqID))
	sb.WriteString(" ")
	sb.WriteString(quoteRustArg(player.Name))
	sb.WriteString(" ")
	sb.WriteString(quoteRustArg(reason))

	if seconds := int64(duration.Seconds()); seconds > 0 {
		sb.WriteString(" ")
		sb.WriteString(strconv.FormatInt(seconds, 10))
		sb.WriteString("s")
	}

	return sb.String(), nil
}

// quoteRustArg wraps a console argument in double quotes. Rust's console splits arguments on
// spaces and has no escape for a quote inside a quoted argument, so inner quotes are replaced.
func quoteRustArg(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `'`) + `"`
}
