package players

import (
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

var (
	ErrLineIsEmpty      = errors.New("line is empty or does not start with #")
	ErrNicknameNotFound = errors.New("nickname not found or improperly quoted")
	ErrLineDoesNotMatch = errors.New("line does not match the expected format")
)

type ValvePlayerManager struct{}

// NewValvePlayers creates a new instance of ValvePlayerManager parser.
func NewValvePlayers() PlayerManager {
	return &ValvePlayerManager{}
}

func (mgr *ValvePlayerManager) ParsePlayers(data string) ([]Player, error) {
	lines := strings.Split(data, "\n")
	players := make([]Player, 0, 32)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !mgr.isPlayerLine(line) {
			continue
		}

		player, err := mgr.parsePlayer(line)
		if err != nil {
			continue
		}

		players = append(players, *player)
	}

	return players, nil
}

func (mgr *ValvePlayerManager) isPlayerLine(line string) bool {
	if strings.HasPrefix(line, "#") && !strings.Contains(line, `"`) {
		return false
	}

	return strings.Contains(line, `"`)
}

func (mgr *ValvePlayerManager) parsePlayer(line string) (*Player, error) {
	line = strings.TrimSpace(line)
	if line == "#" || line == "" {
		return nil, ErrLineIsEmpty
	}

	line = strings.TrimPrefix(line, "# ")
	if line == "" {
		return nil, ErrLineIsEmpty
	}

	startQuote := strings.Index(line, `"`)
	endQuote := strings.LastIndex(line, `"`)
	if startQuote == -1 || endQuote == -1 || startQuote == endQuote {
		return nil, ErrNicknameNotFound
	}

	nickname := line[startQuote+1 : endQuote]

	remaining := line[:startQuote] + line[endQuote+1:]

	parts := strings.Fields(remaining)

	if len(parts) < 6 {
		return nil, ErrLineDoesNotMatch
	}

	player := &Player{
		ID:     parts[1],
		Name:   nickname,
		UniqID: parts[2],
	}

	if parts[2] == "HLTV" {
		player.Score = ""
		player.Ping, player.Addr = mgr.parseHTLVFields(parts)
	} else {
		player.Score = parts[3]

		if len(parts) >= 6 {
			player.Ping = parts[5]
		}

		if len(parts) >= 8 {
			player.Addr = mgr.stripPort(parts[7])
		}
	}

	return player, nil
}

func (mgr *ValvePlayerManager) parseHTLVFields(parts []string) (string, string) {
	ping := ""
	addr := ""

	for i := 3; i < len(parts); i++ {
		if strings.Contains(parts[i], ":") && !strings.Contains(parts[i], ".") {
			continue
		}

		_, err := strconv.Atoi(parts[i])
		if err == nil {
			ping = parts[i]
			if i+2 < len(parts) {
				addr = mgr.stripPort(parts[i+2])
			}

			break
		}
	}

	return ping, addr
}

func (mgr *ValvePlayerManager) stripPort(addr string) string {
	if addr == "" || addr == "loopback" {
		return addr
	}

	if colonIndex := strings.Index(addr, ":"); colonIndex != -1 {
		return addr[:colonIndex]
	}

	return addr
}

func (mgr *ValvePlayerManager) PlayersCommand() string {
	return "status"
}

func (mgr *ValvePlayerManager) KickCommand(player Player, reason string) string {
	sb := strings.Builder{}
	sb.Grow(64)

	sb.WriteString("kick #")
	sb.WriteString(player.UniqID)

	if reason != "" {
		sb.WriteString(" ")
		sb.WriteString(reason)
	}

	return sb.String()
}

func (mgr *ValvePlayerManager) BanCommand(player Player, reason string, time time.Duration) string {
	sb := strings.Builder{}
	sb.Grow(64)

	sb.WriteString("banid ")

	seconds := int(time.Seconds())
	sb.WriteString(strconv.Itoa(seconds))

	sb.WriteString(" #")
	sb.WriteString(player.UniqID)

	if reason != "" {
		sb.WriteString(" ")
		sb.WriteString(reason)
	}

	return sb.String()
}
