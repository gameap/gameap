package application

import (
	"log/slog"
	"time"

	getqueryapi "github.com/gameap/gameap/internal/api/servers/getquery"
	rconbase "github.com/gameap/gameap/internal/api/servers/rcon/base"
	"github.com/gameap/gameap/internal/quercon"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/gameap/gameap/pkg/quercon/rcon"
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
		BuiltinRconProtocol:  rconbase.DetermineProtocol,
		BuiltinQueryProtocol: getqueryapi.QueryProtocolByEngine,
		BuiltinPlayerManager: rconbase.DeterminePlayerManager,
	}

	if c.config.Plugins.Disabled {
		return quercon.New(cfg)
	}

	manager := c.PluginManager()
	if manager == nil {
		return quercon.New(cfg)
	}

	if c.config.Plugin.Net.Enabled {
		runner := pkgplugin.NewProtocolRunner(manager, c.connRegistry(), pkgplugin.NetDialPolicy{
			BlockPrivateIPs: c.config.Plugin.Net.BlockPrivateIPs,
			AllowedHosts:    c.config.Plugin.Net.AllowedHosts,
			MaxTimeout:      time.Duration(c.config.Plugin.Net.MaxTimeoutSeconds) * time.Second,
		})
		cfg.RconExecutor = runner
		cfg.QueryExecutor = runner
	}

	// Without an executor a *_TRANSPORT_PLUGIN registration can never be run, so
	// it is dropped instead of shadowing the built-in tables with a registration
	// that would only ever fail. Declarative built-in mappings from the same
	// plugin keep working.
	cfg.RconProvider = &pluginRconProvider{manager: manager, pluginTransport: cfg.RconExecutor != nil}
	cfg.QueryProvider = &pluginQueryProvider{manager: manager, pluginTransport: cfg.QueryExecutor != nil}

	return quercon.New(cfg)
}

// pluginRconProvider adapts the plugin manager's RCON registrations to the
// resolver's provider interface.
type pluginRconProvider struct {
	manager *pkgplugin.Manager

	// pluginTransport admits RCON_TRANSPORT_PLUGIN registrations. It is off
	// when the plugin net library is disabled and such protocols cannot run.
	pluginTransport bool
}

func (p *pluginRconProvider) RconRegistrations() []quercon.RconRegistration {
	return mapRconRegistrations(p.manager.GetAllRconProtocols(), p.pluginTransport)
}

func mapRconRegistrations(
	regs []pkgplugin.RconProtocolRegistration,
	pluginTransport bool,
) []quercon.RconRegistration {
	out := make([]quercon.RconRegistration, 0, len(regs))

	for _, reg := range regs {
		pr := reg.Protocol
		if pr == nil {
			continue
		}

		transport, builtinProtocol := mapRconTransport(pr.Transport, pr.BuiltinProtocol)
		if transport == quercon.RconPlugin && !pluginTransport {
			continue
		}

		// A built-in name the panel does not implement can only ever fail, and the
		// registration would shadow the built-in tables. Dropping it keeps a game that also
		// has a built-in mapping working, and the warning makes the plugin's typo findable.
		if transport == quercon.RconBuiltin && !rcon.IsProtocolSupported(rcon.Protocol(builtinProtocol)) {
			slog.Warn("plugin rcon registration declares an unknown built-in protocol, dropping it",
				slog.String("plugin_id", reg.PluginID),
				slog.String("protocol_id", pr.Id),
				slog.String("builtin_protocol", builtinProtocol),
			)

			continue
		}

		out = append(out, quercon.RconRegistration{
			PluginID:        reg.PluginID,
			ProtocolID:      pr.Id,
			Name:            pr.Name,
			GameCodes:       pr.GameCodes,
			Engines:         pr.Engines,
			Transport:       transport,
			Players:         mapPlayerCapability(pr.Players),
			BuiltinProtocol: builtinProtocol,
		})
	}

	return out
}

// pluginQueryProvider adapts the plugin manager's Query registrations.
type pluginQueryProvider struct {
	manager *pkgplugin.Manager

	// pluginTransport admits QUERY_TRANSPORT_PLUGIN registrations, see
	// pluginRconProvider.
	pluginTransport bool
}

func (p *pluginQueryProvider) QueryRegistrations() []quercon.QueryRegistration {
	return mapQueryRegistrations(p.manager.GetAllQueryProtocols(), p.pluginTransport)
}

func mapQueryRegistrations(
	regs []pkgplugin.QueryProtocolRegistration,
	pluginTransport bool,
) []quercon.QueryRegistration {
	out := make([]quercon.QueryRegistration, 0, len(regs))

	for _, reg := range regs {
		pr := reg.Protocol
		if pr == nil {
			continue
		}

		transport := mapQueryTransport(pr.Transport)
		if transport == quercon.QueryPlugin && !pluginTransport {
			continue
		}

		out = append(out, quercon.QueryRegistration{
			PluginID:        reg.PluginID,
			ProtocolID:      pr.Id,
			Name:            pr.Name,
			GameCodes:       pr.GameCodes,
			Engines:         pr.Engines,
			Transport:       transport,
			BuiltinProtocol: pr.BuiltinProtocol,
		})
	}

	return out
}

// mapRconTransport translates a plugin's declared transport into the resolver's, resolving the
// built-in protocol name at the same time. The two legacy shorthand values are folded onto
// RconBuiltin here rather than in the resolver, so plugins built before builtin_protocol existed
// keep working while the resolver has a single built-in path.
func mapRconTransport(t protocol.RconTransport, builtinProtocol string) (quercon.RconTransport, string) {
	switch t {
	case protocol.RconTransport_RCON_TRANSPORT_BUILTIN:
		return quercon.RconBuiltin, builtinProtocol
	case protocol.RconTransport_RCON_TRANSPORT_BUILTIN_SOURCE:
		return quercon.RconBuiltin, string(rcon.ProtocolSource)
	case protocol.RconTransport_RCON_TRANSPORT_BUILTIN_GOLDSOURCE:
		return quercon.RconBuiltin, string(rcon.ProtocolGoldSrc)
	case protocol.RconTransport_RCON_TRANSPORT_PLUGIN:
		return quercon.RconPlugin, ""
	case protocol.RconTransport_RCON_TRANSPORT_UNSPECIFIED:
		return quercon.RconTransportUnspecified, ""
	default:
		return quercon.RconTransportUnspecified, ""
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
