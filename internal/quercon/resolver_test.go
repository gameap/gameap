package quercon

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/quercon/query"
	"github.com/gameap/gameap/pkg/quercon/rcon"
	"github.com/gameap/gameap/pkg/quercon/rcon/players"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRconProvider struct {
	regs []RconRegistration
}

func (f *fakeRconProvider) RconRegistrations() []RconRegistration { return f.regs }

type fakeQueryProvider struct {
	regs []QueryRegistration
}

func (f *fakeQueryProvider) QueryRegistrations() []QueryRegistration { return f.regs }

type fakeRconExecutor struct {
	client       rcon.Client
	clientErr    error
	gotPluginID  string
	gotProtoID   string
	parsePlayers []players.Player
}

func (f *fakeRconExecutor) RconClient(pluginID, protocolID string, _ rcon.Config) (rcon.Client, error) {
	f.gotPluginID = pluginID
	f.gotProtoID = protocolID

	return f.client, f.clientErr
}

func (f *fakeRconExecutor) ParsePlayers(_ context.Context, _, _, _ string) ([]players.Player, error) {
	return f.parsePlayers, nil
}

type fakeQueryExecutor struct {
	result      *query.Result
	gotPluginID string
	gotProtoID  string
}

func (f *fakeQueryExecutor) Query(_ context.Context, pluginID, protocolID, _ string, _ int) (*query.Result, error) {
	f.gotPluginID = pluginID
	f.gotProtoID = protocolID

	return f.result, nil
}

func builtinRcon(game domain.Game) (rcon.Protocol, error) {
	switch game.Code {
	case "cs":
		return rcon.ProtocolGoldSrc, nil
	case "cs2":
		return rcon.ProtocolSource, nil
	default:
		return "", assert.AnError
	}
}

func builtinQuery(engine string) (query.Protocol, bool) {
	if engine == "source" {
		return query.ProtocolSource, true
	}

	return "", false
}

func baseConfig() Config {
	return Config{
		BuiltinRconProtocol:   builtinRcon,
		BuiltinQueryProtocol:  builtinQuery,
		BuiltinPlayerManager:  players.NewPlayerManagerByGameCode,
		PlayerManagementCheck: players.IsPlayerManagementSupported,
	}
}

func TestResolveRcon_PluginOverridesBuiltin(t *testing.T) {
	cfg := baseConfig()
	cfg.RconProvider = &fakeRconProvider{regs: []RconRegistration{
		{PluginID: "p1", ProtocolID: "custom", GameCodes: []string{"cs"}, Transport: RconPlugin},
	}}
	r := New(cfg)

	reg, ok := r.resolveRcon(domain.Game{Code: "cs", Engine: "goldsource"})

	require.True(t, ok)
	assert.Equal(t, "custom", reg.ProtocolID)
	assert.Equal(t, RconPlugin, reg.Transport)
}

func TestResolveRcon_GameCodeTakesPrecedenceOverEngine(t *testing.T) {
	cfg := baseConfig()
	cfg.RconProvider = &fakeRconProvider{regs: []RconRegistration{
		{PluginID: "p1", ProtocolID: "by-engine", Engines: []string{"source"}, Transport: RconBuiltinSource},
		{PluginID: "p2", ProtocolID: "by-code", GameCodes: []string{"cs2"}, Transport: RconBuiltinGoldSource},
	}}
	r := New(cfg)

	reg, ok := r.resolveRcon(domain.Game{Code: "cs2", Engine: "source"})

	require.True(t, ok)
	assert.Equal(t, "by-code", reg.ProtocolID)
}

func TestResolveRcon_NoProviderNoMatch(t *testing.T) {
	r := New(baseConfig())

	_, ok := r.resolveRcon(domain.Game{Code: "cs", Engine: "goldsource"})

	assert.False(t, ok)
}

func TestRconClient_BuiltinFallback(t *testing.T) {
	r := New(baseConfig())

	client, err := r.RconClient(context.Background(), domain.Game{Code: "cs2"}, rcon.Config{Address: "127.0.0.1:27015"})

	require.NoError(t, err)
	assert.IsType(t, &rcon.Source{}, client)
}

func TestRconClient_UnsupportedGameWrapsSentinel(t *testing.T) {
	r := New(baseConfig())

	_, err := r.RconClient(context.Background(), domain.Game{Code: "unknown"}, rcon.Config{})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRconProtocolUnsupported)
}

func TestRconClient_PluginTransportRoutesToExecutor(t *testing.T) {
	exec := &fakeRconExecutor{client: &rcon.Source{}}
	cfg := baseConfig()
	cfg.RconProvider = &fakeRconProvider{regs: []RconRegistration{
		{PluginID: "p9", ProtocolID: "myproto", GameCodes: []string{"newgame"}, Transport: RconPlugin},
	}}
	cfg.RconExecutor = exec
	r := New(cfg)

	_, err := r.RconClient(context.Background(), domain.Game{Code: "newgame"}, rcon.Config{})

	require.NoError(t, err)
	assert.Equal(t, "p9", exec.gotPluginID)
	assert.Equal(t, "myproto", exec.gotProtoID)
}

func TestRconClient_PluginTransportWithoutExecutorErrors(t *testing.T) {
	cfg := baseConfig()
	cfg.RconProvider = &fakeRconProvider{regs: []RconRegistration{
		{PluginID: "p9", ProtocolID: "myproto", GameCodes: []string{"newgame"}, Transport: RconPlugin},
	}}
	r := New(cfg)

	_, err := r.RconClient(context.Background(), domain.Game{Code: "newgame"}, rcon.Config{})

	assert.ErrorIs(t, err, ErrPluginProtocolUnavailable)
}

func TestQuery_PluginBuiltinAlias(t *testing.T) {
	cfg := baseConfig()
	cfg.QueryProvider = &fakeQueryProvider{regs: []QueryRegistration{
		{PluginID: "p1", ProtocolID: "q", GameCodes: []string{"newgame"}, Transport: QueryBuiltin, BuiltinProtocol: "bogus"},
	}}
	r := New(cfg)

	// The engine name doesn't match any built-in, so if the plugin registration
	// were NOT consulted the resolver would return ErrQueryProtocolUnsupported.
	// Instead the QueryBuiltin branch routes to query.Query, which rejects the
	// unknown "bogus" protocol with its own error (offline, no network I/O).
	_, err := r.Query(context.Background(), domain.Game{Code: "newgame", Engine: "customengine"}, "host", 1)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrQueryProtocolUnsupported)
}

func TestQuery_PluginTransportRoutesToExecutor(t *testing.T) {
	exec := &fakeQueryExecutor{result: &query.Result{Online: true, Name: "srv"}}
	cfg := baseConfig()
	cfg.QueryProvider = &fakeQueryProvider{regs: []QueryRegistration{
		{PluginID: "pq", ProtocolID: "qp", Engines: []string{"custom"}, Transport: QueryPlugin},
	}}
	cfg.QueryExecutor = exec
	r := New(cfg)

	result, err := r.Query(context.Background(), domain.Game{Engine: "custom"}, "host", 1234)

	require.NoError(t, err)
	assert.True(t, result.Online)
	assert.Equal(t, "pq", exec.gotPluginID)
	assert.Equal(t, "qp", exec.gotProtoID)
}

func TestQuery_UnsupportedEngine(t *testing.T) {
	r := New(baseConfig())

	_, err := r.Query(context.Background(), domain.Game{Engine: "nope"}, "host", 1)

	assert.ErrorIs(t, err, ErrQueryProtocolUnsupported)
}

func TestRconFeatures(t *testing.T) {
	cfg := baseConfig()
	cfg.RconProvider = &fakeRconProvider{regs: []RconRegistration{
		{PluginID: "p1", ProtocolID: "c", GameCodes: []string{"newgame"}, Transport: RconPlugin,
			Players: PlayerCapability{Supported: true}},
	}}
	cfg.RconExecutor = &fakeRconExecutor{}
	r := New(cfg)

	// Plugin-registered game.
	rconOK, playersOK := r.RconFeatures(domain.Game{Code: "newgame"})
	assert.True(t, rconOK)
	assert.True(t, playersOK)

	// Built-in game (cs supports goldsource rcon + valve players).
	rconOK, playersOK = r.RconFeatures(domain.Game{Code: "cs"})
	assert.True(t, rconOK)
	assert.True(t, playersOK)

	// Unknown game.
	rconOK, playersOK = r.RconFeatures(domain.Game{Code: "unknown"})
	assert.False(t, rconOK)
	assert.False(t, playersOK)
}

func TestRconFeatures_UnrunnableRegistrations(t *testing.T) {
	tests := []struct {
		name        string
		reg         RconRegistration
		withExec    bool
		wantRcon    bool
		wantPlayers bool
	}{
		{
			name: "plugin_transport_without_executor",
			reg: RconRegistration{PluginID: "p1", ProtocolID: "c", GameCodes: []string{"newgame"},
				Transport: RconPlugin, Players: PlayerCapability{Supported: true}},
			wantRcon:    false,
			wantPlayers: true,
		},
		{
			name: "plugin_transport_with_executor",
			reg: RconRegistration{PluginID: "p1", ProtocolID: "c", GameCodes: []string{"newgame"},
				Transport: RconPlugin, Players: PlayerCapability{Supported: true}},
			withExec:    true,
			wantRcon:    true,
			wantPlayers: true,
		},
		{
			name: "unspecified_transport",
			reg: RconRegistration{PluginID: "p1", ProtocolID: "c", GameCodes: []string{"newgame"},
				Transport: RconTransportUnspecified},
			withExec:    true,
			wantRcon:    false,
			wantPlayers: false,
		},
		{
			name: "builtin_transport_needs_no_executor",
			reg: RconRegistration{PluginID: "p1", ProtocolID: "c", GameCodes: []string{"newgame"},
				Transport: RconBuiltinSource},
			wantRcon:    true,
			wantPlayers: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			cfg.RconProvider = &fakeRconProvider{regs: []RconRegistration{tt.reg}}
			if tt.withExec {
				cfg.RconExecutor = &fakeRconExecutor{}
			}

			rconOK, playersOK := New(cfg).RconFeatures(domain.Game{Code: "newgame"})

			assert.Equal(t, tt.wantRcon, rconOK)
			assert.Equal(t, tt.wantPlayers, playersOK)
		})
	}
}

func TestPluginPlayerManager_TemplatesAndParse(t *testing.T) {
	exec := &fakeRconExecutor{parsePlayers: []players.Player{{Name: "alice"}}}
	reg := RconRegistration{
		PluginID:   "p1",
		ProtocolID: "custom",
		Players: PlayerCapability{
			Supported:      true,
			PlayersCommand: "listplayers",
			KickCommand:    "kick {uniqid} {reason}",
			BanCommand:     "ban {uniqid} {duration}",
			ParseViaPlugin: true,
		},
	}
	cfg := baseConfig()
	cfg.RconExecutor = exec
	r := New(cfg)

	pm, err := r.newPluginPlayerManager(domain.Game{Code: "newgame"}, reg)
	require.NoError(t, err)

	assert.Equal(t, "listplayers", pm.PlayersCommand())

	kick, err := pm.KickCommand(players.Player{UniqID: "STEAM_1"}, "cheating")
	require.NoError(t, err)
	assert.Equal(t, "kick STEAM_1 cheating", kick)

	ban, err := pm.BanCommand(players.Player{UniqID: "STEAM_1"}, "", 2*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, "ban STEAM_1 120", ban)

	parsed, err := pm.ParsePlayers("raw")
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	assert.Equal(t, "alice", parsed[0].Name)
}

func TestPluginPlayerManager_MissingIdentityField(t *testing.T) {
	reg := RconRegistration{
		Players: PlayerCapability{Supported: true, KickCommand: "kick {uniqid}", ParseViaPlugin: true},
	}
	r := New(baseConfig())

	pm, err := r.newPluginPlayerManager(domain.Game{Code: "newgame"}, reg)
	require.NoError(t, err)

	_, err = pm.KickCommand(players.Player{}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "{uniqid}")
}
