package players

import (
	"errors"
	"time"
)

var (
	ErrPlayerNameRequired   = errors.New("player name is required")
	ErrPlayerUniqIDRequired = errors.New("player unique ID is required")
)

type Player struct {
	ID    string
	Name  string
	Ping  string
	Score string
	Addr  string

	// Additional fields
	UniqID string
}

func (p Player) ValidateName() error {
	if p.Name == "" {
		return ErrPlayerNameRequired
	}

	return nil
}

func (p Player) ValidateUniqID() error {
	if p.UniqID == "" {
		return ErrPlayerUniqIDRequired
	}

	return nil
}

// Capability reports which player operations a manager can actually perform. Listing players
// and moderating them are separate: several engines expose a readable player list but have no
// kick or ban command that is uniform across the games of that family.
type Capability struct {
	List bool
	Kick bool
	Ban  bool
}

type PlayerManager interface {
	// Capabilities reports what this manager supports, so the panel can advertise only the
	// actions that will work instead of offering ones that fail on use.
	Capabilities() Capability

	// ParsePlayers takes the raw response from the server and parses it into a slice of Player structs.
	ParsePlayers(data string) ([]Player, error)

	// PlayersCommand returns the command string to retrieve the list of players from the server via RCON.
	PlayersCommand() string

	// KickCommand returns the command string to kick a player with the given reason.
	KickCommand(player Player, reason string) (string, error)

	// BanCommand returns the command string to ban a player with the given reason.
	BanCommand(player Player, reason string, time time.Duration) (string, error)
}
