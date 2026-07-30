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
	regs := []pkgplugin.RconProtocolRegistration{
		{PluginID: "a", Protocol: &protocol.RconProtocol{
			Id:        "declarative",
			GameCodes: []string{"mygame"},
			Transport: protocol.RconTransport_RCON_TRANSPORT_BUILTIN_SOURCE,
		}},
		{PluginID: "b", Protocol: &protocol.RconProtocol{
			Id:        "wire",
			GameCodes: []string{"othergame"},
			Transport: protocol.RconTransport_RCON_TRANSPORT_PLUGIN,
		}},
		{PluginID: "c", Protocol: nil},
	}

	t.Run("net_enabled_keeps_plugin_transport", func(t *testing.T) {
		out := mapRconRegistrations(regs, true)

		require.Len(t, out, 2)
		assert.Equal(t, "declarative", out[0].ProtocolID)
		assert.Equal(t, quercon.RconBuiltinSource, out[0].Transport)
		assert.Equal(t, "wire", out[1].ProtocolID)
		assert.Equal(t, quercon.RconPlugin, out[1].Transport)
	})

	t.Run("net_disabled_drops_plugin_transport", func(t *testing.T) {
		out := mapRconRegistrations(regs, false)

		require.Len(t, out, 1, "unrunnable registration must not shadow the built-in tables")
		assert.Equal(t, "declarative", out[0].ProtocolID)
	})
}

func TestMapQueryRegistrations_PluginTransportGate(t *testing.T) {
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
		out := mapQueryRegistrations(regs, true)

		require.Len(t, out, 2)
		assert.Equal(t, "declarative", out[0].ProtocolID)
		assert.Equal(t, "source", out[0].BuiltinProtocol)
		assert.Equal(t, "wire", out[1].ProtocolID)
		assert.Equal(t, quercon.QueryPlugin, out[1].Transport)
	})

	t.Run("net_disabled_drops_plugin_transport", func(t *testing.T) {
		out := mapQueryRegistrations(regs, false)

		require.Len(t, out, 1, "unrunnable registration must not shadow the built-in tables")
		assert.Equal(t, "declarative", out[0].ProtocolID)
	})
}
