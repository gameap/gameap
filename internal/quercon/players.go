package quercon

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
	"github.com/pkg/errors"
)

// pluginPlayerManager implements players.PlayerManager for a plugin-registered
// RCON protocol. Commands come from the registration's templates; parsing is
// delegated either to the plugin (ParseViaPlugin) or to a built-in parser.
type pluginPlayerManager struct {
	reg      RconRegistration
	exec     PluginRconExecutor
	fallback players.PlayerManager
}

func (r *Resolver) newPluginPlayerManager(game domain.Game, reg RconRegistration) (players.PlayerManager, error) {
	var fallback players.PlayerManager

	if !reg.Players.ParseViaPlugin {
		pm, err := r.cfg.BuiltinPlayerManager(game.Code)
		if err != nil {
			return nil, errors.WithMessagef(err,
				"plugin rcon protocol %q declares player management without a plugin parser", reg.ProtocolID)
		}

		fallback = pm
	}

	return &pluginPlayerManager{reg: reg, exec: r.cfg.RconExecutor, fallback: fallback}, nil
}

func (m *pluginPlayerManager) PlayersCommand() string {
	return m.reg.Players.PlayersCommand
}

func (m *pluginPlayerManager) ParsePlayers(data string) ([]players.Player, error) {
	if m.reg.Players.ParseViaPlugin {
		if m.exec == nil {
			return nil, ErrPluginProtocolUnavailable
		}

		return m.exec.ParsePlayers(context.Background(), m.reg.PluginID, m.reg.ProtocolID, data)
	}

	if m.fallback != nil {
		return m.fallback.ParsePlayers(data)
	}

	return nil, players.ErrPlayersManagementNotSupported
}

func (m *pluginPlayerManager) KickCommand(player players.Player, reason string) (string, error) {
	return renderPlayerTemplate(m.reg.Players.KickCommand, player, reason, 0)
}

func (m *pluginPlayerManager) BanCommand(player players.Player, reason string, banTime time.Duration) (string, error) {
	return renderPlayerTemplate(m.reg.Players.BanCommand, player, reason, banTime)
}

// renderPlayerTemplate substitutes {id} {name} {uniqid} {ping} {score} {addr}
// {reason} {duration} placeholders. Identity placeholders ({id}/{name}/{uniqid})
// that appear in the template must resolve to a non-empty value, mirroring the
// early validation the built-in managers perform. {duration} is whole seconds,
// consistent with the built-in valve BanCommand.
func renderPlayerTemplate(
	tmpl string,
	player players.Player,
	reason string,
	banTime time.Duration,
) (string, error) {
	if tmpl == "" {
		return "", errors.New("command template is empty")
	}

	identity := []struct {
		key string
		val string
	}{
		{"{id}", player.ID},
		{"{name}", player.Name},
		{"{uniqid}", player.UniqID},
	}

	for _, ph := range identity {
		if strings.Contains(tmpl, ph.key) && ph.val == "" {
			return "", errors.Errorf("player field %s required by command template is empty", ph.key)
		}
	}

	replacer := strings.NewReplacer(
		"{id}", player.ID,
		"{name}", player.Name,
		"{uniqid}", player.UniqID,
		"{ping}", player.Ping,
		"{score}", player.Score,
		"{addr}", player.Addr,
		"{reason}", reason,
		"{duration}", strconv.Itoa(int(banTime.Seconds())),
	)

	return replacer.Replace(tmpl), nil
}
