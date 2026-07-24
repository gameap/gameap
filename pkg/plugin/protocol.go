package plugin

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/gameap/gameap/pkg/netutil"
	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/gameap/gameap/pkg/quercon/query"
	"github.com/gameap/gameap/pkg/quercon/rcon"
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
	"github.com/pkg/errors"
)

const defaultProtocolTimeout = 10 * time.Second

var (
	// ErrDialBlocked is returned when a game-server target fails the dial policy.
	ErrDialBlocked = errors.New("plugin net: target address is blocked")
	// ErrHostNotResolved is returned when a game-server hostname does not resolve.
	ErrHostNotResolved = errors.New("plugin net: hostname did not resolve")
)

// netResolver lets tests stub DNS.
type netResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// NetDialPolicy governs which game-server addresses the host will dial on behalf
// of a plugin-implemented protocol.
type NetDialPolicy struct {
	BlockPrivateIPs bool
	AllowedHosts    []string
	MaxTimeout      time.Duration
}

// ProtocolRunner executes plugin-implemented RCON/Query protocols. It owns the
// only dial: it opens and guards the connection to the game server, registers it
// in the shared ConnRegistry, invokes the plugin's protocol RPCs (which perform
// I/O over the gameap-net library using the handle), and tears the connection
// down. A plugin can therefore only ever reach the server the host dialed for it.
type ProtocolRunner struct {
	manager      *Manager
	registry     *ConnRegistry
	policy       NetDialPolicy
	resolver     netResolver
	dialer       *net.Dialer
	allowedHosts map[string]struct{}
}

// NewProtocolRunner builds a runner. registry must be the same instance given to
// the gameap-net host-library factory.
func NewProtocolRunner(manager *Manager, registry *ConnRegistry, policy NetDialPolicy) *ProtocolRunner {
	if policy.MaxTimeout <= 0 {
		policy.MaxTimeout = defaultProtocolTimeout
	}

	allowed := make(map[string]struct{}, len(policy.AllowedHosts))
	for _, h := range policy.AllowedHosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			allowed[h] = struct{}{}
		}
	}

	return &ProtocolRunner{
		manager:      manager,
		registry:     registry,
		policy:       policy,
		resolver:     net.DefaultResolver,
		dialer:       &net.Dialer{Timeout: policy.MaxTimeout},
		allowedHosts: allowed,
	}
}

// RconClient returns an rcon.Client backed by the given plugin protocol.
func (r *ProtocolRunner) RconClient(pluginID, protocolID string, cfg rcon.Config) (rcon.Client, error) {
	return &pluginRconClient{
		runner:     r,
		pluginID:   pluginID,
		protocolID: protocolID,
		cfg:        cfg,
	}, nil
}

// Query performs a server query via the given plugin protocol.
func (r *ProtocolRunner) Query(
	ctx context.Context,
	pluginID, protocolID, host string,
	port int,
) (*query.Result, error) {
	address := net.JoinHostPort(host, strconv.Itoa(port))

	handle, release, err := r.openHandle(ctx, "udp", address, pluginID)
	if err != nil {
		return nil, err
	}
	defer release()

	plugin, ok := r.manager.GetPlugin(pluginID)
	if !ok || plugin.Protocol == nil {
		return nil, errors.Wrapf(ErrPluginNotFound, "plugin: %s", pluginID)
	}

	callCtx, cancel := r.callContext(ctx)
	defer cancel()

	resp, err := plugin.Protocol.QueryServer(callCtx, &protocol.QueryServerRequest{
		ProtocolId: protocolID,
		ConnHandle: handle,
		Address:    address,
	})
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, errors.New(*resp.Error)
	}

	return queryResultFromProto(resp.Result), nil
}

// ParsePlayers parses a raw players list via the plugin's ParsePlayers RPC.
func (r *ProtocolRunner) ParsePlayers(
	ctx context.Context,
	pluginID, protocolID, raw string,
) ([]players.Player, error) {
	plugin, ok := r.manager.GetPlugin(pluginID)
	if !ok || plugin.Protocol == nil {
		return nil, errors.Wrapf(ErrPluginNotFound, "plugin: %s", pluginID)
	}

	callCtx, cancel := r.callContext(ctx)
	defer cancel()

	resp, err := plugin.Protocol.ParsePlayers(callCtx, &protocol.ParsePlayersRequest{
		ProtocolId: protocolID,
		Raw:        raw,
	})
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, errors.New(*resp.Error)
	}

	out := make([]players.Player, 0, len(resp.Players))
	for _, p := range resp.Players {
		out = append(out, players.Player{
			ID:     p.Id,
			Name:   p.Name,
			Ping:   p.Ping,
			Score:  p.Score,
			Addr:   p.Addr,
			UniqID: p.Uniqid,
		})
	}

	return out, nil
}

// openHandle dials the guarded address, registers the connection for the plugin,
// and returns the handle plus a release func that closes and unregisters it.
func (r *ProtocolRunner) openHandle(
	ctx context.Context,
	network, address, pluginID string,
) (uint64, func(), error) {
	conn, err := r.dial(ctx, network, address)
	if err != nil {
		return 0, nil, err
	}

	handle, err := r.registry.Register(conn, pluginID, time.Now().Add(r.policy.MaxTimeout))
	if err != nil {
		_ = conn.Close()

		return 0, nil, err
	}

	return handle, func() { r.registry.Discard(handle) }, nil
}

func (r *ProtocolRunner) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.policy.MaxTimeout)
}

func (r *ProtocolRunner) dial(ctx context.Context, network, address string) (net.Conn, error) {
	ip, port, err := r.resolveAndCheck(ctx, address)
	if err != nil {
		return nil, err
	}

	return r.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
}

func (r *ProtocolRunner) resolveAndCheck(ctx context.Context, address string) (netip.Addr, string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return netip.Addr{}, "", errors.Wrap(err, "invalid address")
	}

	allowBypass := r.hostAllowed(host)

	if ip, ipErr := netip.ParseAddr(host); ipErr == nil {
		if err := r.checkIP(ip, allowBypass); err != nil {
			return netip.Addr{}, "", err
		}

		return ip, port, nil
	}

	ips, err := r.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, "", errors.Wrap(ErrHostNotResolved, err.Error())
	}

	if len(ips) == 0 {
		return netip.Addr{}, "", errors.Wrap(ErrHostNotResolved, host)
	}

	for _, ip := range ips {
		if err := r.checkIP(ip, allowBypass); err != nil {
			return netip.Addr{}, "", err
		}
	}

	return ips[0], port, nil
}

func (r *ProtocolRunner) checkIP(ip netip.Addr, allowBypass bool) error {
	if netutil.IsCloudMetadataIP(ip) {
		return errors.Wrapf(ErrDialBlocked, "ip=%s reason=%s", ip, netutil.BlockReasonCloudMetadata)
	}

	if !r.policy.BlockPrivateIPs || allowBypass {
		return nil
	}

	if reason := netutil.BlockReason(ip); reason != "" {
		return errors.Wrapf(ErrDialBlocked, "ip=%s reason=%s", ip, reason)
	}

	return nil
}

func (r *ProtocolRunner) hostAllowed(host string) bool {
	if len(r.allowedHosts) == 0 {
		return false
	}

	_, ok := r.allowedHosts[strings.ToLower(host)]

	return ok
}

func queryResultFromProto(qr *protocol.QueryResult) *query.Result {
	if qr == nil {
		return &query.Result{QueryTime: time.Now()}
	}

	resultPlayers := make([]query.ResultPlayer, 0, len(qr.Players))
	for _, p := range qr.Players {
		resultPlayers = append(resultPlayers, query.ResultPlayer{Name: p.Name, Score: int(p.Score)})
	}

	return &query.Result{
		QueryTime:     time.Now(),
		Online:        qr.Online,
		Name:          qr.Name,
		Map:           qr.Map,
		PlayersNum:    int(qr.PlayersNum),
		MaxPlayersNum: int(qr.MaxPlayersNum),
		Players:       resultPlayers,
	}
}
