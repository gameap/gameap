package players

import "time"

type Player struct {
	ID    string
	Name  string
	Ping  string
	Score string
	Addr  string

	// Additional fields
	UniqID string
}

type PlayerManager interface {
	// ParsePlayers takes the raw response from the server and parses it into a slice of Player structs.
	ParsePlayers(data string) ([]Player, error)

	// PlayersCommand returns the command string to retrieve the list of players from the server via RCON.
	PlayersCommand() string

	// KickCommand returns the command string to kick a player with the given reason.
	KickCommand(player Player, reason string) string

	// BanCommand returns the command string to ban a player with the given reason.
	BanCommand(player Player, reason string, time time.Duration) string
}
