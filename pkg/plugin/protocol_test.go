package plugin

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/protocol"
	"github.com/gameap/gameap/pkg/quercon/rcon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProtoPlugin struct {
	protocol.EmptyProtocolService

	openResp   *protocol.RconOpenResponse
	execResp   *protocol.RconExecuteResponse
	queryResp  *protocol.QueryServerResponse
	parseResp  *protocol.ParsePlayersResponse
	lastHandle uint64
	closed     bool
}

func (f *fakeProtoPlugin) RconOpen(
	_ context.Context,
	req *protocol.RconOpenRequest,
) (*protocol.RconOpenResponse, error) {
	f.lastHandle = req.ConnHandle

	return f.openResp, nil
}

func (f *fakeProtoPlugin) RconExecute(
	_ context.Context,
	_ *protocol.RconExecuteRequest,
) (*protocol.RconExecuteResponse, error) {
	return f.execResp, nil
}

func (f *fakeProtoPlugin) RconClose(
	_ context.Context,
	_ *protocol.RconCloseRequest,
) (*protocol.RconCloseResponse, error) {
	f.closed = true

	return &protocol.RconCloseResponse{}, nil
}

func (f *fakeProtoPlugin) QueryServer(
	_ context.Context,
	_ *protocol.QueryServerRequest,
) (*protocol.QueryServerResponse, error) {
	return f.queryResp, nil
}

func (f *fakeProtoPlugin) ParsePlayers(
	_ context.Context,
	_ *protocol.ParsePlayersRequest,
) (*protocol.ParsePlayersResponse, error) {
	return f.parseResp, nil
}

//nolint:unparam // id is fixed in tests but kept explicit for clarity
func managerWithPlugin(id string, svc protocol.ProtocolService) *Manager {
	m := NewManager(ManagerConfig{})
	m.plugins[NormalizePluginID(id)] = &LoadedPlugin{
		Info:     &proto.PluginInfo{Id: id},
		Protocol: svc,
		Enabled:  true,
	}

	return m
}

func TestRunner_CheckIP(t *testing.T) {
	metadata := netip.MustParseAddr("169.254.169.254")
	private := netip.MustParseAddr("10.0.0.5")
	public := netip.MustParseAddr("8.8.8.8")

	permissive := NewProtocolRunner(nil, nil, NetDialPolicy{})
	assert.NoError(t, permissive.checkIP(public, false))
	assert.NoError(t, permissive.checkIP(private, false), "private allowed when BlockPrivateIPs is off")
	assert.ErrorIs(t, permissive.checkIP(metadata, false), ErrDialBlocked, "metadata always blocked")

	strict := NewProtocolRunner(nil, nil, NetDialPolicy{BlockPrivateIPs: true})
	assert.NoError(t, strict.checkIP(public, false))
	assert.ErrorIs(t, strict.checkIP(private, false), ErrDialBlocked)
	assert.NoError(t, strict.checkIP(private, true), "allowlist bypasses private block")
	assert.ErrorIs(t, strict.checkIP(metadata, true), ErrDialBlocked, "allowlist never bypasses metadata")
}

func TestRunner_CheckIP_MappedAddresses(t *testing.T) {
	permissive := NewProtocolRunner(nil, nil, NetDialPolicy{})
	strict := NewProtocolRunner(nil, nil, NetDialPolicy{BlockPrivateIPs: true})

	tests := []struct {
		name    string
		addr    string
		runner  *ProtocolRunner
		blocked bool
	}{
		{"mapped_aws_metadata_permissive", "::ffff:169.254.169.254", permissive, true},
		{"mapped_alibaba_metadata_permissive", "::ffff:100.100.100.200", permissive, true},
		{"mapped_metadata_strict", "::ffff:169.254.169.254", strict, true},
		{"mapped_cgnat_strict", "::ffff:100.64.1.1", strict, true},
		{"mapped_private_strict", "::ffff:10.0.0.5", strict, true},
		{"mapped_cgnat_permissive", "::ffff:100.64.1.1", permissive, false},
		{"mapped_public", "::ffff:8.8.8.8", strict, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.runner.checkIP(netip.MustParseAddr(tt.addr), false)

			if tt.blocked {
				assert.ErrorIs(t, err, ErrDialBlocked)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRunner_CheckIP_MappedMetadataNotBypassedByAllowlist(t *testing.T) {
	strict := NewProtocolRunner(nil, nil, NetDialPolicy{BlockPrivateIPs: true})

	err := strict.checkIP(netip.MustParseAddr("::ffff:169.254.169.254"), true)

	assert.ErrorIs(t, err, ErrDialBlocked)
}

func TestPluginRconClient_OpenExecuteClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go func() { _, _ = conn.Read(make([]byte, 1)) }()
		}
	}()

	fake := &fakeProtoPlugin{
		openResp: &protocol.RconOpenResponse{Ok: true},
		execResp: &protocol.RconExecuteResponse{Output: "players: 3"},
	}
	registry := NewConnRegistry(8)
	runner := NewProtocolRunner(managerWithPlugin("plg", fake), registry, NetDialPolicy{MaxTimeout: 2 * time.Second})

	client, err := runner.RconClient("plg", "myproto", rcon.Config{Address: ln.Addr().String(), Password: "pw"})
	require.NoError(t, err)

	require.NoError(t, client.Open(context.Background()))
	assert.NotZero(t, fake.lastHandle)
	assert.Equal(t, 1, registry.Len())

	out, err := client.Execute(context.Background(), "status")
	require.NoError(t, err)
	assert.Equal(t, "players: 3", out)

	require.NoError(t, client.Close())
	assert.True(t, fake.closed)
	assert.Equal(t, 0, registry.Len(), "connection released on close")
}

func TestPluginRconClient_SecondOpenRejected(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go func() { _, _ = conn.Read(make([]byte, 1)) }()
		}
	}()

	fake := &fakeProtoPlugin{openResp: &protocol.RconOpenResponse{Ok: true}}
	registry := NewConnRegistry(8)
	runner := NewProtocolRunner(managerWithPlugin("plg", fake), registry, NetDialPolicy{MaxTimeout: 2 * time.Second})

	client, err := runner.RconClient("plg", "myproto", rcon.Config{Address: ln.Addr().String(), Password: "pw"})
	require.NoError(t, err)

	require.NoError(t, client.Open(context.Background()))
	firstHandle := fake.lastHandle
	require.NotZero(t, firstHandle)

	err = client.Open(context.Background())

	assert.ErrorIs(t, err, ErrRconAlreadyOpen)
	assert.Equal(t, 1, registry.Len(), "no extra connection registered")
	assert.Equal(t, firstHandle, fake.lastHandle, "first connection kept")

	require.NoError(t, client.Close())
	assert.Equal(t, 0, registry.Len(), "first connection released on close")
}

func TestPluginRconClient_AuthFailed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			_ = conn
		}
	}()

	fake := &fakeProtoPlugin{openResp: &protocol.RconOpenResponse{AuthFailed: true}}
	registry := NewConnRegistry(8)
	runner := NewProtocolRunner(managerWithPlugin("plg", fake), registry, NetDialPolicy{MaxTimeout: 2 * time.Second})

	client, err := runner.RconClient("plg", "myproto", rcon.Config{Address: ln.Addr().String(), Password: "bad"})
	require.NoError(t, err)

	err = client.Open(context.Background())
	assert.ErrorIs(t, err, rcon.ErrAuthenticationFailed)
	assert.Equal(t, 0, registry.Len(), "connection released on auth failure")
}

func TestRunner_Query(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()

	udpAddr := pc.LocalAddr().(*net.UDPAddr)

	fake := &fakeProtoPlugin{queryResp: &protocol.QueryServerResponse{
		Result: &protocol.QueryResult{
			Online:        true,
			Name:          "Test Server",
			Map:           "de_dust2",
			PlayersNum:    5,
			MaxPlayersNum: 16,
			Players:       []*protocol.QueryResultPlayer{{Name: "alice", Score: 10}},
		},
	}}
	registry := NewConnRegistry(8)
	runner := NewProtocolRunner(managerWithPlugin("plg", fake), registry, NetDialPolicy{MaxTimeout: 2 * time.Second})

	result, err := runner.Query(context.Background(), "plg", "q", "127.0.0.1", udpAddr.Port)
	require.NoError(t, err)
	assert.True(t, result.Online)
	assert.Equal(t, "Test Server", result.Name)
	assert.Equal(t, 5, result.PlayersNum)
	require.Len(t, result.Players, 1)
	assert.Equal(t, "alice", result.Players[0].Name)
	assert.Equal(t, 10, result.Players[0].Score)
	assert.Equal(t, 0, registry.Len(), "connection released after query")
}

func TestRunner_ParsePlayers(t *testing.T) {
	fake := &fakeProtoPlugin{parseResp: &protocol.ParsePlayersResponse{
		Players: []*protocol.RconPlayer{
			{Id: "1", Name: "bob", Uniqid: "STEAM_1", Score: "5", Ping: "30"},
		},
	}}
	runner := NewProtocolRunner(managerWithPlugin("plg", fake), NewConnRegistry(8), NetDialPolicy{MaxTimeout: time.Second})

	parsed, err := runner.ParsePlayers(context.Background(), "plg", "myproto", "raw output")
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	assert.Equal(t, "bob", parsed[0].Name)
	assert.Equal(t, "STEAM_1", parsed[0].UniqID)
	assert.Equal(t, "5", parsed[0].Score)
}
