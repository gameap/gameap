package application

import (
	"testing"

	"github.com/gameap/gameap/internal/quercon"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapRconRegistrations_PluginTransportGate(t *testing.T) {
	t.Parallel()

	regs := []pkgplugin.RconProtocolRegistration{
		{PluginID: "a", Protocol: &protocol.RconProtocol{
			Id:        "declarative",
			GameCodes: []string{"mygame"},
			Transport: protocol.RconTransport_RCON_TRANSPORT_BUILTIN_SOURCE,
		}},
		{PluginID: "a2", Protocol: &protocol.RconProtocol{
			Id:              "declarative-new",
			GameCodes:       []string{"newgame"},
			Transport:       protocol.RconTransport_RCON_TRANSPORT_BUILTIN,
			BuiltinProtocol: "quake3",
		}},
		{PluginID: "b", Protocol: &protocol.RconProtocol{
			Id:        "wire",
			GameCodes: []string{"othergame"},
			Transport: protocol.RconTransport_RCON_TRANSPORT_PLUGIN,
		}},
		{PluginID: "c", Protocol: nil},
	}

	t.Run("net_enabled_keeps_plugin_transport", func(t *testing.T) {
		t.Parallel()

		out := mapRconRegistrations(regs, true)

		require.Len(t, out, 3)
		assert.Equal(t, "declarative", out[0].ProtocolID)
		assert.Equal(t, quercon.RconBuiltin, out[0].Transport)
		assert.Equal(t, "source", out[0].BuiltinProtocol,
			"the legacy shorthand transport must still resolve to the source engine")
		assert.Equal(t, "declarative-new", out[1].ProtocolID)
		assert.Equal(t, quercon.RconBuiltin, out[1].Transport)
		assert.Equal(t, "quake3", out[1].BuiltinProtocol)
		assert.Equal(t, "wire", out[2].ProtocolID)
		assert.Equal(t, quercon.RconPlugin, out[2].Transport)
	})

	t.Run("net_disabled_drops_plugin_transport", func(t *testing.T) {
		t.Parallel()

		out := mapRconRegistrations(regs, false)

		require.Len(t, out, 2, "unrunnable registration must not shadow the built-in tables")
		assert.Equal(t, "declarative", out[0].ProtocolID)
		assert.Equal(t, "declarative-new", out[1].ProtocolID)
	})
}

// TestMapRconTransport is the guard for the single riskiest part of collapsing the built-in
// transports: if the legacy enum values stopped resolving to a protocol name, every plugin that
// registers a built-in RCON protocol would silently lose it.
func TestMapRconTransport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		transport       protocol.RconTransport
		builtinProtocol string
		wantTransport   quercon.RconTransport
		wantProtocol    string
	}{
		{
			name:          "legacy_source_shorthand",
			transport:     protocol.RconTransport_RCON_TRANSPORT_BUILTIN_SOURCE,
			wantTransport: quercon.RconBuiltin,
			wantProtocol:  "source",
		},
		{
			name:          "legacy_goldsource_shorthand",
			transport:     protocol.RconTransport_RCON_TRANSPORT_BUILTIN_GOLDSOURCE,
			wantTransport: quercon.RconBuiltin,
			wantProtocol:  "goldsource",
		},
		{
			name:            "builtin_carries_the_declared_name",
			transport:       protocol.RconTransport_RCON_TRANSPORT_BUILTIN,
			builtinProtocol: "battleye",
			wantTransport:   quercon.RconBuiltin,
			wantProtocol:    "battleye",
		},
		{
			name:            "plugin_transport_ignores_the_name",
			transport:       protocol.RconTransport_RCON_TRANSPORT_PLUGIN,
			builtinProtocol: "quake3",
			wantTransport:   quercon.RconPlugin,
		},
		{
			name:          "unspecified",
			transport:     protocol.RconTransport_RCON_TRANSPORT_UNSPECIFIED,
			wantTransport: quercon.RconTransportUnspecified,
		},
		{
			name:          "out_of_range_value",
			transport:     protocol.RconTransport(99),
			wantTransport: quercon.RconTransportUnspecified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			transport, builtinProtocol := mapRconTransport(tt.transport, tt.builtinProtocol)

			assert.Equal(t, tt.wantTransport, transport)
			assert.Equal(t, tt.wantProtocol, builtinProtocol)
		})
	}
}

func TestMapRconRegistrations_DropsUnknownBuiltinProtocol(t *testing.T) {
	t.Parallel()

	regs := []pkgplugin.RconProtocolRegistration{
		{PluginID: "a", Protocol: &protocol.RconProtocol{
			Id:              "typo",
			GameCodes:       []string{"mygame"},
			Transport:       protocol.RconTransport_RCON_TRANSPORT_BUILTIN,
			BuiltinProtocol: "quaake3",
		}},
		{PluginID: "b", Protocol: &protocol.RconProtocol{
			Id:        "empty",
			GameCodes: []string{"othergame"},
			Transport: protocol.RconTransport_RCON_TRANSPORT_BUILTIN,
		}},
		{PluginID: "c", Protocol: &protocol.RconProtocol{
			Id:              "good",
			GameCodes:       []string{"thirdgame"},
			Transport:       protocol.RconTransport_RCON_TRANSPORT_BUILTIN,
			BuiltinProtocol: "samp",
		}},
	}

	out := mapRconRegistrations(regs, true)

	require.Len(t, out, 1, "a registration naming a protocol the panel lacks must not shadow the built-in tables")
	assert.Equal(t, "good", out[0].ProtocolID)
}

func TestMapQueryRegistrations_PluginTransportGate(t *testing.T) {
	t.Parallel()

	regs := []pkgplugin.QueryProtocolRegistration{
		{PluginID: "a", Protocol: &protocol.QueryProtocol{
			Id:              "declarative",
			Engines:         []string{"myengine"},
			Transport:       protocol.QueryTransport_QUERY_TRANSPORT_BUILTIN,
			BuiltinProtocol: "source",
		}},
		{PluginID: "b", Protocol: &protocol.QueryProtocol{
			Id:        "wire",
			Engines:   []string{"otherengine"},
			Transport: protocol.QueryTransport_QUERY_TRANSPORT_PLUGIN,
		}},
		{PluginID: "c", Protocol: nil},
	}

	t.Run("net_enabled_keeps_plugin_transport", func(t *testing.T) {
		t.Parallel()

		out := mapQueryRegistrations(regs, true)

		require.Len(t, out, 2)
		assert.Equal(t, "declarative", out[0].ProtocolID)
		assert.Equal(t, "source", out[0].BuiltinProtocol)
		assert.Equal(t, "wire", out[1].ProtocolID)
		assert.Equal(t, quercon.QueryPlugin, out[1].Transport)
	})

	t.Run("net_disabled_drops_plugin_transport", func(t *testing.T) {
		t.Parallel()

		out := mapQueryRegistrations(regs, false)

		require.Len(t, out, 1, "unrunnable registration must not shadow the built-in tables")
		assert.Equal(t, "declarative", out[0].ProtocolID)
	})
}

func TestMapQueryRegistrations_DropsUnknownBuiltinProtocol(t *testing.T) {
	t.Parallel()

	regs := []pkgplugin.QueryProtocolRegistration{
		{PluginID: "a", Protocol: &protocol.QueryProtocol{
			Id:              "typo",
			Engines:         []string{"myengine"},
			Transport:       protocol.QueryTransport_QUERY_TRANSPORT_BUILTIN,
			BuiltinProtocol: "sourse",
		}},
		{PluginID: "b", Protocol: &protocol.QueryProtocol{
			Id:        "empty",
			Engines:   []string{"otherengine"},
			Transport: protocol.QueryTransport_QUERY_TRANSPORT_BUILTIN,
		}},
		{PluginID: "c", Protocol: &protocol.QueryProtocol{
			Id:              "good",
			Engines:         []string{"thirdengine"},
			Transport:       protocol.QueryTransport_QUERY_TRANSPORT_BUILTIN,
			BuiltinProtocol: "gamespy3",
		}},
	}

	out := mapQueryRegistrations(regs, true)

	require.Len(t, out, 1, "a registration naming a protocol the panel lacks must not shadow the built-in tables")
	assert.Equal(t, "good", out[0].ProtocolID)
}
