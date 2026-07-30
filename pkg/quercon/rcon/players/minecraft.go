package players

import (
	"strings"
	"time"
)

type MinecraftPlayerManager struct{}

func NewMinecraftPlayers() PlayerManager {
	return &MinecraftPlayerManager{}
}

func (mgr *MinecraftPlayerManager) Capabilities() Capability {
	return Capability{List: true, Kick: true, Ban: true}
}

func (mgr *MinecraftPlayerManager) ParsePlayers(data string) ([]Player, error) {
	colonIdx := strings.LastIndex(data, ":")
	if colonIdx == -1 || colonIdx == len(data)-1 {
		return []Player{}, nil
	}

	playersPart := strings.TrimSpace(data[colonIdx+1:])
	if playersPart == "" {
		return []Player{}, nil
	}

	entries := strings.Split(playersPart, ",")
	players := make([]Player, 0, len(entries))

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		name, uuid := parseMinecraftPlayerEntry(entry)
		if name == "" {
			continue
		}

		players = append(players, Player{
			Name:   name,
			ID:     uuid,
			UniqID: uuid,
		})
	}

	return players, nil
}

func parseMinecraftPlayerEntry(entry string) (name, uuid string) {
	parenIdx := strings.LastIndex(entry, "(")
	if parenIdx == -1 {
		return entry, ""
	}

	name = strings.TrimSpace(entry[:parenIdx])
	uuidPart := entry[parenIdx+1:]
	if before, _, ok := strings.Cut(uuidPart, ")"); ok {
		uuid = before
	}

	return name, uuid
}

func (mgr *MinecraftPlayerManager) PlayersCommand() string {
	return "list uuids"
}

func (mgr *MinecraftPlayerManager) KickCommand(player Player, reason string) (string, error) {
	if err := player.ValidateName(); err != nil {
		return "", err
	}

	sb := strings.Builder{}
	sb.Grow(64)

	sb.WriteString("kick ")
	sb.WriteString(player.Name)

	if reason != "" {
		sb.WriteString(" ")
		sb.WriteString(reason)
	}

	return sb.String(), nil
}

func (mgr *MinecraftPlayerManager) BanCommand(player Player, reason string, _ time.Duration) (string, error) {
	if err := player.ValidateName(); err != nil {
		return "", err
	}

	sb := strings.Builder{}
	sb.Grow(64)

	sb.WriteString("ban ")
	sb.WriteString(player.Name)

	if reason != "" {
		sb.WriteString(" ")
		sb.WriteString(reason)
	}

	return sb.String(), nil
}
