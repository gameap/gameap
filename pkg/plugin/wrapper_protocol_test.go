package plugin

import (
	"context"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/api"
)

// unusedExport stands in for a guest export that must never be invoked: every
// case using it is rejected by the call gate before the function is reached.
// The embedded nil interface panics if that assumption ever breaks.
type unusedExport struct {
	api.Function
}

// protocolCall names one ProtocolService method and the two ways to reach it:
// against a wrapper that declares no exports, and against one that declares
// the export but cannot get through the call gate.
type protocolCall struct {
	name string
	// withoutExport invokes the method on a wrapper with a nil export.
	withoutExport func(context.Context, *pluginServiceWrapper) (any, error)
	// withExport invokes the method on a wrapper whose export is set.
	withExport func(context.Context, *pluginServiceWrapper) (any, error)
	// setExport marks the guest export as present.
	setExport func(*pluginServiceWrapper)
	// optional methods degrade to an empty response instead of erroring.
	optional bool
	// wantExportName is the suffix ErrExportNotFound is wrapped with.
	wantExportName string
}

func protocolCalls() []protocolCall {
	return []protocolCall{
		{
			name:     "GetRconProtocols",
			optional: true,
			setExport: func(w *pluginServiceWrapper) {
				w.getrconprotocols = unusedExport{}
			},
			withoutExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.GetRconProtocols(ctx, &protocol.GetRconProtocolsRequest{})
			},
			withExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.GetRconProtocols(ctx, &protocol.GetRconProtocolsRequest{})
			},
		},
		{
			name:     "GetQueryProtocols",
			optional: true,
			setExport: func(w *pluginServiceWrapper) {
				w.getqueryprotocols = unusedExport{}
			},
			withoutExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.GetQueryProtocols(ctx, &protocol.GetQueryProtocolsRequest{})
			},
			withExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.GetQueryProtocols(ctx, &protocol.GetQueryProtocolsRequest{})
			},
		},
		{
			name:           "RconOpen",
			wantExportName: "protocol_service_rcon_open",
			setExport: func(w *pluginServiceWrapper) {
				w.rconopen = unusedExport{}
			},
			withoutExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.RconOpen(ctx, &protocol.RconOpenRequest{ProtocolId: "p"})
			},
			withExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.RconOpen(ctx, &protocol.RconOpenRequest{ProtocolId: "p"})
			},
		},
		{
			name:           "RconExecute",
			wantExportName: "protocol_service_rcon_execute",
			setExport: func(w *pluginServiceWrapper) {
				w.rconexecute = unusedExport{}
			},
			withoutExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.RconExecute(ctx, &protocol.RconExecuteRequest{ProtocolId: "p"})
			},
			withExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.RconExecute(ctx, &protocol.RconExecuteRequest{ProtocolId: "p"})
			},
		},
		{
			name:           "RconClose",
			wantExportName: "protocol_service_rcon_close",
			setExport: func(w *pluginServiceWrapper) {
				w.rconclose = unusedExport{}
			},
			withoutExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.RconClose(ctx, &protocol.RconCloseRequest{ProtocolId: "p"})
			},
			withExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.RconClose(ctx, &protocol.RconCloseRequest{ProtocolId: "p"})
			},
		},
		{
			name:           "QueryServer",
			wantExportName: "protocol_service_query_server",
			setExport: func(w *pluginServiceWrapper) {
				w.queryserver = unusedExport{}
			},
			withoutExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.QueryServer(ctx, &protocol.QueryServerRequest{ProtocolId: "p"})
			},
			withExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.QueryServer(ctx, &protocol.QueryServerRequest{ProtocolId: "p"})
			},
		},
		{
			name:           "ParsePlayers",
			wantExportName: "protocol_service_parse_players",
			setExport: func(w *pluginServiceWrapper) {
				w.parseplayers = unusedExport{}
			},
			withoutExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.ParsePlayers(ctx, &protocol.ParsePlayersRequest{ProtocolId: "p"})
			},
			withExport: func(ctx context.Context, w *pluginServiceWrapper) (any, error) {
				return w.ParsePlayers(ctx, &protocol.ParsePlayersRequest{ProtocolId: "p"})
			},
		},
	}
}

// A plugin that does not implement the optional ProtocolService must not break
// the panel: discovery degrades to "no protocols", and the protocols the plugin
// never declared report a missing export rather than panicking.
func TestPluginServiceWrapper_Protocol_MissingExports(t *testing.T) {
	t.Parallel()
	for _, tt := range protocolCalls() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			wrapper := &pluginServiceWrapper{gate: make(chan struct{}, 1)}

			// ACT
			resp, err := tt.withoutExport(context.Background(), wrapper)

			// ASSERT
			if tt.optional {
				require.NoError(t, err, "discovery must degrade to an empty response")
				require.NotNil(t, resp, "an empty response must still be returned")

				switch r := resp.(type) {
				case *protocol.GetRconProtocolsResponse:
					assert.Empty(t, r.Protocols, "a plugin without the export declares no RCON protocols")
				case *protocol.GetQueryProtocolsResponse:
					assert.Empty(t, r.Protocols, "a plugin without the export declares no query protocols")
				default:
					t.Fatalf("unexpected discovery response type %T", resp)
				}

				return
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrExportNotFound)
			assert.Contains(t, err.Error(), tt.wantExportName, "error must name the missing export")
			assert.Nil(t, resp)
		})
	}
}

// Protocol methods share the core PluginService call gate, so a caller that
// cannot acquire it must surface ErrPluginBusy instead of reaching the guest.
func TestPluginServiceWrapper_Protocol_CallGateRejection(t *testing.T) {
	t.Parallel()
	for _, tt := range protocolCalls() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// ARRANGE
			wrapper := &pluginServiceWrapper{gate: make(chan struct{}, 1)}
			tt.setExport(wrapper)
			wrapper.gate <- struct{}{}

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			// ACT
			resp, err := tt.withExport(ctx, wrapper)

			// ASSERT
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPluginBusy)
			assert.Nil(t, resp, "no response may be returned when the guest was never called")
		})
	}
}

// The wrapper must satisfy the optional ProtocolService for every loaded
// plugin, including ones that implement none of its exports.
func TestPluginServiceWrapper_Protocol_LoadedPluginWithoutProtocolSupport(t *testing.T) {
	t.Parallel()
	// ARRANGE
	plugin := loadSharedServerLoggerWASM(t)

	svc, ok := plugin.Instance.(protocol.ProtocolService)
	require.True(t, ok, "every loaded plugin must expose the ProtocolService surface")

	// ACT
	rcon, rconErr := svc.GetRconProtocols(context.Background(), &protocol.GetRconProtocolsRequest{})
	query, queryErr := svc.GetQueryProtocols(context.Background(), &protocol.GetQueryProtocolsRequest{})
	_, openErr := svc.RconOpen(context.Background(), &protocol.RconOpenRequest{ProtocolId: "p"})

	// ASSERT
	require.NoError(t, rconErr)
	require.NoError(t, queryErr)
	assert.Empty(t, rcon.Protocols, "a plugin without protocol support declares no RCON protocols")
	assert.Empty(t, query.Protocols, "a plugin without protocol support declares no query protocols")
	assert.ErrorIs(t, openErr, ErrExportNotFound)
}
