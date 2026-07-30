// Package quercon resolves the RCON/Query protocol to use for a game server,
// consulting plugin-registered protocols first (which may override built-ins)
// and falling back to the panel's built-in protocol tables.
//
// The resolver is intentionally free of any internal/api dependency: the
// built-in resolution is injected as function values (wired by the application
// container from the existing built-in tables), and the plugin side is provided
// through small interfaces. This keeps the resolver a pure, unit-testable unit.
package quercon

import (
	"context"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/quercon/query"
	"github.com/gameap/gameap/pkg/quercon/rcon"
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
	"github.com/pkg/errors"
)

var (
	// ErrPluginProtocolUnavailable is returned when a game resolves to a
	// plugin-implemented protocol but the plugin executor is not wired
	// (e.g. plugins disabled) so it cannot be run.
	ErrPluginProtocolUnavailable = errors.New("plugin protocol executor is unavailable")

	// ErrQueryProtocolUnsupported is returned when no plugin or built-in query
	// protocol matches the game.
	ErrQueryProtocolUnsupported = errors.New("unsupported game engine for query")

	// ErrRconProtocolUnsupported is returned when no plugin or built-in RCON
	// protocol matches the game.
	ErrRconProtocolUnsupported = errors.New("unsupported game for rcon")
)

// RconTransport selects how a resolved RCON registration is executed.
type RconTransport int

const (
	RconTransportUnspecified RconTransport = iota
	RconBuiltinSource
	RconBuiltinGoldSource
	RconPlugin
)

// QueryTransport selects how a resolved Query registration is executed.
type QueryTransport int

const (
	QueryTransportUnspecified QueryTransport = iota
	QueryBuiltin
	QueryPlugin
)

// PlayerCapability describes plugin player-management support for a protocol.
type PlayerCapability struct {
	Supported      bool
	PlayersCommand string
	KickCommand    string
	BanCommand     string
	ParseViaPlugin bool
}

// RconRegistration is a plugin RCON protocol registration in resolver terms.
type RconRegistration struct {
	PluginID   string
	ProtocolID string
	Name       string
	GameCodes  []string
	Engines    []string
	Transport  RconTransport
	Players    PlayerCapability
}

// QueryRegistration is a plugin Query protocol registration in resolver terms.
type QueryRegistration struct {
	PluginID        string
	ProtocolID      string
	Name            string
	GameCodes       []string
	Engines         []string
	Transport       QueryTransport
	BuiltinProtocol string
}

// RconProvider lists plugin RCON registrations. Implementations must return
// them ordered deterministically (the container's adapter sorts by plugin ID).
type RconProvider interface {
	RconRegistrations() []RconRegistration
}

// QueryProvider lists plugin Query registrations.
type QueryProvider interface {
	QueryRegistrations() []QueryRegistration
}

// PluginRconExecutor runs plugin-implemented RCON protocols. Implemented in
// pkg/plugin (host runner over the gameap-net library); nil until wired.
type PluginRconExecutor interface {
	// RconClient returns an rcon.Client backed by the given plugin protocol.
	RconClient(pluginID, protocolID string, cfg rcon.Config) (rcon.Client, error)
	// ParsePlayers parses a raw players list via the plugin's ParsePlayers RPC.
	ParsePlayers(ctx context.Context, pluginID, protocolID, raw string) ([]players.Player, error)
}

// PluginQueryExecutor runs plugin-implemented Query protocols.
type PluginQueryExecutor interface {
	Query(ctx context.Context, pluginID, protocolID, host string, port int) (*query.Result, error)
}

// Config wires the resolver. All plugin-side fields are optional (nil disables
// the corresponding plugin path). The built-in fields are required and are
// wired from the existing built-in tables.
type Config struct {
	RconProvider  RconProvider
	QueryProvider QueryProvider
	RconExecutor  PluginRconExecutor
	QueryExecutor PluginQueryExecutor

	BuiltinRconProtocol   func(game domain.Game) (rcon.Protocol, error)
	BuiltinQueryProtocol  func(engine string) (query.Protocol, bool)
	BuiltinPlayerManager  func(gameCode string) (players.PlayerManager, error)
	PlayerManagementCheck func(gameCode string) bool
}

// Resolver resolves and constructs protocol clients for game servers.
type Resolver struct {
	cfg Config
}

// New creates a Resolver from the given configuration.
func New(cfg Config) *Resolver {
	return &Resolver{cfg: cfg}
}

// RconClient returns a ready (not yet opened) rcon.Client for the game. The
// caller supplies address/password/timeout via cfg; the protocol is chosen by
// resolution. Plugin registrations override the built-in tables.
func (r *Resolver) RconClient(_ context.Context, game domain.Game, cfg rcon.Config) (rcon.Client, error) {
	if reg, ok := r.resolveRcon(game); ok {
		switch reg.Transport {
		case RconBuiltinSource:
			cfg.Protocol = rcon.ProtocolSource

			return rcon.NewClient(cfg)
		case RconBuiltinGoldSource:
			cfg.Protocol = rcon.ProtocolGoldSrc

			return rcon.NewClient(cfg)
		case RconPlugin:
			if r.cfg.RconExecutor == nil {
				return nil, errors.WithMessagef(ErrPluginProtocolUnavailable, "rcon protocol %q", reg.ProtocolID)
			}

			return r.cfg.RconExecutor.RconClient(reg.PluginID, reg.ProtocolID, cfg)
		default:
			return nil, errors.Errorf("unknown rcon transport for protocol %q", reg.ProtocolID)
		}
	}

	protocol, err := r.cfg.BuiltinRconProtocol(game)
	if err != nil {
		return nil, errors.WithMessage(ErrRconProtocolUnsupported, err.Error())
	}

	cfg.Protocol = protocol

	return rcon.NewClient(cfg)
}

// PlayerManager returns the player manager for the game, or a
// players.ErrPlayersManagementNotSupported-wrapped error when unavailable.
func (r *Resolver) PlayerManager(_ context.Context, game domain.Game) (players.PlayerManager, error) {
	if reg, ok := r.resolveRcon(game); ok {
		if !reg.Players.Supported {
			return nil, players.ErrPlayersManagementNotSupported
		}

		return r.newPluginPlayerManager(game, reg)
	}

	return r.cfg.BuiltinPlayerManager(game.Code)
}

// Query performs a server query for the game, choosing the plugin or built-in
// query protocol.
func (r *Resolver) Query(ctx context.Context, game domain.Game, host string, port int) (*query.Result, error) {
	if reg, ok := r.resolveQuery(game); ok {
		switch reg.Transport {
		case QueryBuiltin:
			return query.Query(ctx, host, port, query.Protocol(reg.BuiltinProtocol))
		case QueryPlugin:
			if r.cfg.QueryExecutor == nil {
				return nil, errors.WithMessagef(ErrPluginProtocolUnavailable, "query protocol %q", reg.ProtocolID)
			}

			return r.cfg.QueryExecutor.Query(ctx, reg.PluginID, reg.ProtocolID, host, port)
		default:
			return nil, errors.Errorf("unknown query transport for protocol %q", reg.ProtocolID)
		}
	}

	protocol, ok := r.cfg.BuiltinQueryProtocol(strings.ToLower(game.Engine))
	if !ok {
		return nil, ErrQueryProtocolUnsupported
	}

	return query.Query(ctx, host, port, protocol)
}

// RconFeatures reports whether RCON and player management are available for the
// game, powering the features endpoint.
func (r *Resolver) RconFeatures(game domain.Game) (rconSupported, playersSupported bool) {
	if reg, ok := r.resolveRcon(game); ok {
		return r.rconRegistrationExecutable(reg), reg.Players.Supported
	}

	protocol, err := r.cfg.BuiltinRconProtocol(game)
	if err != nil {
		return false, false
	}

	return rcon.IsProtocolSupported(protocol), r.cfg.PlayerManagementCheck(game.Code)
}

// rconRegistrationExecutable reports whether a matched plugin registration can
// actually yield a client, applying the same transport rules as RconClient. An
// unknown or plugin transport with no executor wired resolves to "no RCON"
// rather than an advertised feature that fails on use.
func (r *Resolver) rconRegistrationExecutable(reg RconRegistration) bool {
	switch reg.Transport {
	case RconBuiltinSource, RconBuiltinGoldSource:
		return true
	case RconPlugin:
		return r.cfg.RconExecutor != nil
	case RconTransportUnspecified:
		return false
	default:
		return false
	}
}

// resolveRcon finds the plugin RCON registration that applies to the game.
// Game-code matches take precedence over engine matches; within a pass the
// first registration wins (registrations are pre-sorted by plugin ID).
func (r *Resolver) resolveRcon(game domain.Game) (RconRegistration, bool) {
	if r.cfg.RconProvider == nil {
		return RconRegistration{}, false
	}

	regs := r.cfg.RconProvider.RconRegistrations()

	for _, reg := range regs {
		if containsFold(reg.GameCodes, game.Code) {
			return reg, true
		}
	}

	for _, reg := range regs {
		if containsFold(reg.Engines, game.Engine) {
			return reg, true
		}
	}

	return RconRegistration{}, false
}

func (r *Resolver) resolveQuery(game domain.Game) (QueryRegistration, bool) {
	if r.cfg.QueryProvider == nil {
		return QueryRegistration{}, false
	}

	regs := r.cfg.QueryProvider.QueryRegistrations()

	for _, reg := range regs {
		if containsFold(reg.GameCodes, game.Code) {
			return reg, true
		}
	}

	for _, reg := range regs {
		if containsFold(reg.Engines, game.Engine) {
			return reg, true
		}
	}

	return QueryRegistration{}, false
}

func containsFold(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(v, target) {
			return true
		}
	}

	return false
}
