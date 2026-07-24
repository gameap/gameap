package application

import (
	"time"

	getqueryapi "github.com/gameap/gameap/internal/api/servers/getquery"
	rconbase "github.com/gameap/gameap/internal/api/servers/rcon/base"
	"github.com/gameap/gameap/internal/quercon"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
)

// QuerconResolver returns the RCON/Query protocol resolver. Plugin protocol
// registrations (which may override built-ins) are consulted in front of the
// panel's built-in protocol tables. When plugins are disabled the resolver
// behaves exactly like the built-in tables.
func (c *Container) QuerconResolver() *quercon.Resolver {
	if c.querconResolver == nil {
		c.querconResolver = c.createQuerconResolver()
	}

	return c.querconResolver
}

func (c *Container) createQuerconResolver() *quercon.Resolver {
	cfg := quercon.Config{
		BuiltinRconProtocol:   rconbase.DetermineProtocol,
		BuiltinQueryProtocol:  getqueryapi.QueryProtocolByEngine,
		BuiltinPlayerManager:  players.NewPlayerManagerByGameCode,
		PlayerManagementCheck: players.IsPlayerManagementSupported,
	}

	if !c.config.Plugins.Disabled {
		if manager := c.PluginManager(); manager != nil {
			cfg.RconProvider = &pluginRconProvider{manager: manager}
			cfg.QueryProvider = &pluginQueryProvider{manager: manager}

			if c.config.Plugin.Net.Enabled {
				runner := pkgplugin.NewProtocolRunner(manager, c.connRegistry(), pkgplugin.NetDialPolicy{
					BlockPrivateIPs: c.config.Plugin.Net.BlockPrivateIPs,
					AllowedHosts:    c.config.Plugin.Net.AllowedHosts,
					MaxTimeout:      time.Duration(c.config.Plugin.Net.MaxTimeoutSeconds) * time.Second,
				})
				cfg.RconExecutor = runner
				cfg.QueryExecutor = runner
			}
		}
	}

	return quercon.New(cfg)
}

// pluginRconProvider adapts the plugin manager's RCON registrations to the
// resolver's provider interface.
type pluginRconProvider struct {
	manager *pkgplugin.Manager
}

func (p *pluginRconProvider) RconRegistrations() []quercon.RconRegistration {
	regs := p.manager.GetAllRconProtocols()
	out := make([]quercon.RconRegistration, 0, len(regs))

	for _, reg := range regs {
		pr := reg.Protocol
		if pr == nil {
			continue
		}

		out = append(out, quercon.RconRegistration{
			PluginID:   reg.PluginID,
			ProtocolID: pr.Id,
			Name:       pr.Name,
			GameCodes:  pr.GameCodes,
			Engines:    pr.Engines,
			Transport:  mapRconTransport(pr.Transport),
			Players:    mapPlayerCapability(pr.Players),
		})
	}

	return out
}

// pluginQueryProvider adapts the plugin manager's Query registrations.
type pluginQueryProvider struct {
	manager *pkgplugin.Manager
}

func (p *pluginQueryProvider) QueryRegistrations() []quercon.QueryRegistration {
	regs := p.manager.GetAllQueryProtocols()
	out := make([]quercon.QueryRegistration, 0, len(regs))

	for _, reg := range regs {
		pr := reg.Protocol
		if pr == nil {
			continue
		}

		out = append(out, quercon.QueryRegistration{
			PluginID:        reg.PluginID,
			ProtocolID:      pr.Id,
			Name:            pr.Name,
			GameCodes:       pr.GameCodes,
			Engines:         pr.Engines,
			Transport:       mapQueryTransport(pr.Transport),
			BuiltinProtocol: pr.BuiltinProtocol,
		})
	}

	return out
}

func mapRconTransport(t protocol.RconTransport) quercon.RconTransport {
	switch t {
	case protocol.RconTransport_RCON_TRANSPORT_BUILTIN_SOURCE:
		return quercon.RconBuiltinSource
	case protocol.RconTransport_RCON_TRANSPORT_BUILTIN_GOLDSOURCE:
		return quercon.RconBuiltinGoldSource
	case protocol.RconTransport_RCON_TRANSPORT_PLUGIN:
		return quercon.RconPlugin
	case protocol.RconTransport_RCON_TRANSPORT_UNSPECIFIED:
		return quercon.RconTransportUnspecified
	default:
		return quercon.RconTransportUnspecified
	}
}

func mapQueryTransport(t protocol.QueryTransport) quercon.QueryTransport {
	switch t {
	case protocol.QueryTransport_QUERY_TRANSPORT_BUILTIN:
		return quercon.QueryBuiltin
	case protocol.QueryTransport_QUERY_TRANSPORT_PLUGIN:
		return quercon.QueryPlugin
	case protocol.QueryTransport_QUERY_TRANSPORT_UNSPECIFIED:
		return quercon.QueryTransportUnspecified
	default:
		return quercon.QueryTransportUnspecified
	}
}

func mapPlayerCapability(pc *protocol.PlayerCapability) quercon.PlayerCapability {
	if pc == nil {
		return quercon.PlayerCapability{}
	}

	return quercon.PlayerCapability{
		Supported:      pc.Supported,
		PlayersCommand: pc.PlayersCommand,
		KickCommand:    pc.KickCommand,
		BanCommand:     pc.BanCommand,
		ParseViaPlugin: pc.ParseViaPlugin,
	}
}
