//go:build wasip1

// Example plugin: extend RCON and Query protocols.
//
// It demonstrates both extension modes:
//   - A declarative RCON registration that maps the game code "mygame" onto the
//     panel's built-in Source engine and adds player management (the panel runs
//     the RCON; the plugin only parses the players list). Any protocol the panel
//     implements can be named here, including the UDP ones.
//   - A plugin-implemented Query protocol: the host opens and guards the UDP
//     connection, and the plugin does the wire I/O over the gameap-net library.
//
// Protocol extension lives in the optional ProtocolService (pkg/plugin/sdk/
// protocol), separate from the core PluginService. The plugin registers both.
//
// Build:
//
//	tinygo build -o protocol-extension.wasm -target=wasip1 -buildmode=c-shared .
package main

import (
	"context"
	"strings"

	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk"
	netsdk "github.com/gameap/gameap/pkg/plugin/sdk/net"
	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
)

func main() {}

func init() {
	plugin := &ProtocolExtensionPlugin{}
	pluginproto.RegisterPluginService(plugin)
	protocol.RegisterProtocolService(plugin)
}

// ProtocolExtensionPlugin implements the core PluginService (via
// EmptyPluginService) and the optional ProtocolService (via
// EmptyProtocolService), overriding only the methods it needs.
type ProtocolExtensionPlugin struct {
	sdk.EmptyPluginService
	protocol.EmptyProtocolService
}

func (p *ProtocolExtensionPlugin) GetInfo(
	_ context.Context,
	_ *pluginproto.GetInfoRequest,
) (*pluginproto.PluginInfo, error) {
	return &pluginproto.PluginInfo{
		Id:          "protoextqrcn",
		Name:        "Protocol Extension Example",
		Version:     "1.0.0",
		Description: "Adds a new game to Source RCON and a custom Query protocol",
		Author:      "GameAP",
		ApiVersion:  "1",
	}, nil
}

// GetRconProtocols maps a new game onto the built-in Source RCON engine and
// declares player management. Kick/ban are templates the host renders;
// ParseViaPlugin routes the players-list output to ParsePlayers below.
func (p *ProtocolExtensionPlugin) GetRconProtocols(
	_ context.Context,
	_ *protocol.GetRconProtocolsRequest,
) (*protocol.GetRconProtocolsResponse, error) {
	return &protocol.GetRconProtocolsResponse{
		Protocols: []*protocol.RconProtocol{
			{
				Id:        "mygame-rcon",
				Name:      "My Game (Source RCON)",
				GameCodes: []string{"mygame"},
				Transport: protocol.RconTransport_RCON_TRANSPORT_BUILTIN_SOURCE,
				Players: &protocol.PlayerCapability{
					Supported:      true,
					PlayersCommand: "status",
					KickCommand:    "kickid {id} {reason}",
					BanCommand:     "banid {duration} {id}",
					ParseViaPlugin: true,
				},
			},
		},
	}, nil
}

// GetQueryProtocols registers a Query protocol the plugin implements itself.
func (p *ProtocolExtensionPlugin) GetQueryProtocols(
	_ context.Context,
	_ *protocol.GetQueryProtocolsRequest,
) (*protocol.GetQueryProtocolsResponse, error) {
	return &protocol.GetQueryProtocolsResponse{
		Protocols: []*protocol.QueryProtocol{
			{
				Id:        "mygame-query",
				Name:      "My Game (custom query)",
				Engines:   []string{"mygame"},
				Transport: protocol.QueryTransport_QUERY_TRANSPORT_PLUGIN,
			},
		},
	}, nil
}

// ParsePlayers turns raw "status" output into structured players. Each non-empty
// line is expected to be: <id> <name> [score] [ping].
func (p *ProtocolExtensionPlugin) ParsePlayers(
	_ context.Context,
	req *protocol.ParsePlayersRequest,
) (*protocol.ParsePlayersResponse, error) {
	var parsed []*protocol.RconPlayer

	for _, line := range strings.Split(req.Raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		player := &protocol.RconPlayer{Id: fields[0], Name: fields[1]}
		if len(fields) >= 3 {
			player.Score = fields[2]
		}
		if len(fields) >= 4 {
			player.Ping = fields[3]
		}

		parsed = append(parsed, player)
	}

	return &protocol.ParsePlayersResponse{Players: parsed}, nil
}

// QueryServer performs the query over the host-managed UDP connection. The host
// has already dialed and guarded the target; the plugin only reads and writes
// through the gameap-net library using the provided handle.
func (p *ProtocolExtensionPlugin) QueryServer(
	_ context.Context,
	req *protocol.QueryServerRequest,
) (*protocol.QueryServerResponse, error) {
	net := netsdk.NewNetService()
	ctx := context.Background()

	sendResp, err := net.Send(ctx, &netsdk.NetSendRequest{
		Handle: req.ConnHandle,
		Data:   []byte("\xFF\xFF\xFF\xFFping"),
	})
	if err != nil {
		return queryError(err.Error()), nil
	}
	if sendResp.Error != nil {
		return queryError(*sendResp.Error), nil
	}

	recvResp, err := net.Recv(ctx, &netsdk.NetRecvRequest{
		Handle:    req.ConnHandle,
		MaxBytes:  1400,
		TimeoutMs: 1000,
	})
	if err != nil {
		return queryError(err.Error()), nil
	}
	if recvResp.Error != nil {
		return queryError(*recvResp.Error), nil
	}

	// A real protocol would parse recvResp.Data. This example reports the server
	// as online when it answered the probe.
	online := !recvResp.Timeout && len(recvResp.Data) > 0

	return &protocol.QueryServerResponse{
		Result: &protocol.QueryResult{
			Online: online,
			Name:   "My Game Server",
		},
	}, nil
}

func queryError(msg string) *protocol.QueryServerResponse {
	return &protocol.QueryServerResponse{Error: &msg}
}
