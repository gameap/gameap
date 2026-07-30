package kickplayer

import (
	"encoding/json"
	"strings"

	"github.com/gameap/gameap/pkg/flexible"
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
	"github.com/pkg/errors"
)

// commandSeparators are the characters a game console treats as a command
// boundary. Kick/ban commands are assembled by string concatenation from these
// request fields, so a separator would smuggle an extra console command into
// the RCON packet — turning the "rcon players" ability into full console
// access.
const commandSeparators = "\r\n;\x00"

// ErrCommandSeparator reports a request field that cannot be safely embedded
// in an RCON command.
var ErrCommandSeparator = errors.New("value must not contain line breaks, ';' or NUL")

func validateCommandArg(field, value string) error {
	if strings.ContainsAny(value, commandSeparators) {
		return errors.WithMessage(ErrCommandSeparator, field)
	}

	return nil
}

type playerInput struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Score  string `json:"score"`
	Ping   string `json:"ping"`
	IP     string `json:"ip"`
	UniqID string `json:"uniqid"`
}

type kickRequest struct {
	Player            json.RawMessage `json:"player"`
	Reason            string          `json:"reason"`
	DurationInMinutes flexible.Int    `json:"time"`
}

func (r *kickRequest) Validate() error {
	if len(r.Player) == 0 {
		return errors.New("player is required")
	}

	return validateCommandArg("reason", r.Reason)
}

func (r *kickRequest) ToPlayer() (players.Player, error) {
	player, err := r.decodePlayer()
	if err != nil {
		return players.Player{}, err
	}

	if err := validatePlayerFields(player); err != nil {
		return players.Player{}, err
	}

	return player, nil
}

func (r *kickRequest) decodePlayer() (players.Player, error) {
	var stringID string
	if err := json.Unmarshal(r.Player, &stringID); err == nil {
		return players.Player{
			ID:     stringID,
			UniqID: stringID,
		}, nil
	}

	var playerObj playerInput
	if err := json.Unmarshal(r.Player, &playerObj); err != nil {
		return players.Player{}, errors.New("player must be a string ID or player object")
	}

	player := players.Player{
		ID:     playerObj.ID,
		Name:   playerObj.Name,
		Score:  playerObj.Score,
		Ping:   playerObj.Ping,
		Addr:   playerObj.IP,
		UniqID: playerObj.UniqID,
	}

	if player.UniqID == "" {
		player.UniqID = player.ID
	}

	return player, nil
}

// validatePlayerFields guards every player field, not just the ones the
// built-in managers use: plugin-registered protocols render arbitrary command
// templates over {id} {name} {uniqid} {ping} {score} {addr}.
func validatePlayerFields(player players.Player) error {
	fields := []struct {
		name  string
		value string
	}{
		{"player id", player.ID},
		{"player name", player.Name},
		{"player uniqid", player.UniqID},
		{"player ping", player.Ping},
		{"player score", player.Score},
		{"player ip", player.Addr},
	}

	for _, f := range fields {
		if err := validateCommandArg(f.name, f.value); err != nil {
			return err
		}
	}

	return nil
}
