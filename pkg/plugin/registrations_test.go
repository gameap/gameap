package plugin

import (
	"testing"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_GetAllRconProtocols_SortedAndFiltered(t *testing.T) {
	t.Parallel()
	m := NewManager(ManagerConfig{})
	m.plugins["bbb"] = &LoadedPlugin{
		Info: &proto.PluginInfo{Id: "bbb"}, Enabled: true,
		RconProtocols: []*protocol.RconProtocol{{Id: "p2", GameCodes: []string{"g2"}}},
	}
	m.plugins["aaa"] = &LoadedPlugin{
		Info: &proto.PluginInfo{Id: "aaa"}, Enabled: true,
		RconProtocols: []*protocol.RconProtocol{{Id: "p1", GameCodes: []string{"g1"}}},
	}
	disabled := &LoadedPlugin{
		Info: &proto.PluginInfo{Id: "ccc"}, Enabled: false,
		RconProtocols: []*protocol.RconProtocol{{Id: "p3"}},
	}
	disabled.Disable()
	m.plugins["ccc"] = disabled

	regs := m.GetAllRconProtocols()

	require.Len(t, regs, 2, "disabled plugin excluded")
	assert.Equal(t, "aaa", regs[0].PluginID)
	assert.Equal(t, "p1", regs[0].Protocol.Id)
	assert.Equal(t, "bbb", regs[1].PluginID)
}

func TestManager_GetAllQueryProtocols_SortedAndFiltered(t *testing.T) {
	t.Parallel()
	m := NewManager(ManagerConfig{})
	m.plugins["zzz"] = &LoadedPlugin{
		Info: &proto.PluginInfo{Id: "zzz"}, Enabled: true,
		QueryProtocols: []*protocol.QueryProtocol{{Id: "q2"}},
	}
	m.plugins["aaa"] = &LoadedPlugin{
		Info: &proto.PluginInfo{Id: "aaa"}, Enabled: true,
		QueryProtocols: []*protocol.QueryProtocol{{Id: "q1"}},
	}
	disabled := &LoadedPlugin{
		Info: &proto.PluginInfo{Id: "mmm"}, Enabled: false,
		QueryProtocols: []*protocol.QueryProtocol{{Id: "q3"}},
	}
	disabled.Disable()
	m.plugins["mmm"] = disabled

	regs := m.GetAllQueryProtocols()

	require.Len(t, regs, 2, "disabled plugin excluded")
	assert.Equal(t, "aaa", regs[0].PluginID)
	assert.Equal(t, "q1", regs[0].Protocol.Id)
	assert.Equal(t, "zzz", regs[1].PluginID)
}
