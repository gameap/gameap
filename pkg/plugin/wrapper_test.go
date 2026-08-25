package plugin

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"io"
	"sync"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/gamemods"
	"github.com/gameap/gameap/pkg/plugin/sdk/games"
	"github.com/gameap/gameap/pkg/plugin/sdk/host"
	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
	domainproto "github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

//go:embed testdata/server-logger.wasm.gz
var serverLoggerWASMGz []byte

func decompressServerLoggerWASM() ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(serverLoggerWASMGz))
	if err != nil {
		return nil, errors.Wrap(err, "open gzip reader for embedded server-logger wasm")
	}
	defer func() { _ = gr.Close() }()

	wasm, err := io.ReadAll(gr)
	if err != nil {
		return nil, errors.Wrap(err, "decompress embedded server-logger wasm")
	}

	return wasm, nil
}

// stubLogService satisfies log.LogService; the WASM plugin calls Log to emit log lines.
type stubLogService struct {
	mu    sync.Mutex
	calls []*log.LogRequest
}

func (s *stubLogService) Log(_ context.Context, req *log.LogRequest) (*log.LogResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)

	return &log.LogResponse{}, nil
}

// stubGamesService satisfies games.GamesService.
type stubGamesService struct{}

func (s *stubGamesService) FindGames(_ context.Context, _ *games.FindGamesRequest) (*games.FindGamesResponse, error) {
	return &games.FindGamesResponse{}, nil
}

func (s *stubGamesService) GetGame(_ context.Context, _ *games.GetGameRequest) (*games.GetGameResponse, error) {
	return &games.GetGameResponse{Found: false}, nil
}

// stubGameModsService satisfies gamemods.GameModsService.
type stubGameModsService struct{}

func (s *stubGameModsService) FindGameMods(
	_ context.Context,
	_ *gamemods.FindGameModsRequest,
) (*gamemods.FindGameModsResponse, error) {
	return &gamemods.FindGameModsResponse{}, nil
}

func (s *stubGameModsService) GetGameMod(
	_ context.Context,
	_ *gamemods.GetGameModRequest,
) (*gamemods.GetGameModResponse, error) {
	return &gamemods.GetGameModResponse{Found: false}, nil
}

// stubServersService satisfies servers.ServersService.
type stubServersService struct{}

func (s *stubServersService) FindServers(
	_ context.Context,
	_ *servers.FindServersRequest,
) (*servers.FindServersResponse, error) {
	return &servers.FindServersResponse{}, nil
}

func (s *stubServersService) GetServer(
	_ context.Context,
	_ *servers.GetServerRequest,
) (*servers.GetServerResponse, error) {
	return &servers.GetServerResponse{Found: false}, nil
}

func (s *stubServersService) SaveServer(
	_ context.Context,
	_ *servers.SaveServerRequest,
) (*servers.SaveServerResponse, error) {
	return &servers.SaveServerResponse{Success: true}, nil
}

func (s *stubServersService) DeleteServer(
	_ context.Context,
	_ *servers.DeleteServerRequest,
) (*servers.DeleteServerResponse, error) {
	return &servers.DeleteServerResponse{Success: true}, nil
}

// hostLibFunc adapts a function to the HostLibrary interface.
type hostLibFunc func(ctx context.Context, r wazero.Runtime) error

func (f hostLibFunc) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return f(ctx, r)
}

// stubSchedulerService satisfies scheduler.SchedulerService; the example
// plugin registers its demo task during Initialize.
type stubSchedulerService struct {
	mu       sync.Mutex
	addCalls []*scheduler.AddTaskRequest
}

func (s *stubSchedulerService) AddTask(
	_ context.Context,
	req *scheduler.AddTaskRequest,
) (*scheduler.AddTaskResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addCalls = append(s.addCalls, req)

	return &scheduler.AddTaskResponse{Success: true}, nil
}

func (s *stubSchedulerService) RemoveTask(
	_ context.Context,
	_ *scheduler.RemoveTaskRequest,
) (*scheduler.RemoveTaskResponse, error) {
	return &scheduler.RemoveTaskResponse{Success: true}, nil
}

func (s *stubSchedulerService) ListTasks(
	_ context.Context,
	_ *scheduler.ListTasksRequest,
) (*scheduler.ListTasksResponse, error) {
	return &scheduler.ListTasksResponse{}, nil
}

// stubHostService satisfies host.HostService; the example plugin reads its
// grants and host info during Initialize.
type stubHostService struct{}

func (s *stubHostService) GetGrants(context.Context, *host.GetGrantsRequest) (*host.GetGrantsResponse, error) {
	return &host.GetGrantsResponse{Permissions: []string{"listen_events"}}, nil
}

func (s *stubHostService) GetHostInfo(
	context.Context,
	*host.GetHostInfoRequest,
) (*host.GetHostInfoResponse, error) {
	return &host.GetHostInfoResponse{PanelVersion: "test", PluginApiVersion: 1, InstanceId: "test"}, nil
}

// Shared plugin instance — Manager.Load is expensive because of WASM compilation,
// and the wrapper API is read-only/idempotent for these tests. Each test queries
// the same loaded plugin without observable cross-test interference. The WASM
// itself is embedded (gzipped) under testdata/, so tests do not depend on the
// example artifact being present on disk at runtime.
var (
	sharedPluginOnce sync.Once
	sharedManager    *Manager
	sharedPlugin     *LoadedPlugin
	sharedHostStub   = &stubHostService{}
	errSharedLoad    error
)

func loadSharedServerLoggerWASM(t *testing.T) *LoadedPlugin {
	t.Helper()

	sharedPluginOnce.Do(func() {
		wasmBytes, err := decompressServerLoggerWASM()
		if err != nil {
			errSharedLoad = err

			return
		}

		cfg := ManagerConfig{Libraries: sharedTestHostLibraries()}
		sharedManager = NewManager(cfg)
		sharedPlugin, errSharedLoad = sharedManager.Load(
			context.Background(), wasmBytes, map[string]string{}, 0,
		)
	})

	require.NoError(t, errSharedLoad, "Manager.Load must succeed for the embedded WASM artifact")
	require.NotNil(t, sharedPlugin)

	return sharedPlugin
}

func TestPluginServiceWrapper_GetInfo(t *testing.T) {
	t.Parallel()
	// ARRANGE
	plugin := loadSharedServerLoggerWASM(t)

	// ACT
	info, err := plugin.Instance.GetInfo(context.Background(), &proto.GetInfoRequest{})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "fwgfo26jzwnm4", info.Id, "plugin must return its declared ID")
	assert.Equal(t, "Server Logger", info.Name)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, "GameAP", info.Author)
	assert.Equal(t, "1", info.ApiVersion)
	assert.Equal(t, "Logs server lifecycle events", info.Description)
}

func TestPluginServiceWrapper_Initialize(t *testing.T) {
	t.Parallel()
	// ARRANGE
	plugin := loadSharedServerLoggerWASM(t)

	// ACT
	resp, err := plugin.Instance.Initialize(context.Background(), &proto.InitializeRequest{
		Context: &proto.PluginContext{PluginId: "fwgfo26jzwnm4"},
		Config:  map[string]string{"key": "value"},
	})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Result)
	assert.True(t, resp.Result.Success, "the example plugin reports successful initialization")
}

func TestPluginServiceWrapper_HandleEvent(t *testing.T) {
	t.Parallel()
	t.Run("returns_unhandled_when_payload_is_missing", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		plugin := loadSharedServerLoggerWASM(t)
		event := &proto.Event{
			Type:      proto.EventType_EVENT_TYPE_SERVER_POST_START,
			Timestamp: 1700000000,
			Context:   &proto.PluginContext{PluginId: "fwgfo26jzwnm4", RequestId: "req-1"},
		}

		// ACT
		result, err := plugin.Instance.HandleEvent(context.Background(), event)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.Handled, "missing server payload must yield Handled=false")
	})

	t.Run("returns_handled_when_server_payload_present", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		plugin := loadSharedServerLoggerWASM(t)
		event := &proto.Event{
			Type:      proto.EventType_EVENT_TYPE_SERVER_POST_START,
			Timestamp: 1700000000,
			Context:   &proto.PluginContext{PluginId: "fwgfo26jzwnm4", RequestId: "req-1"},
			Payload: &proto.Event_ServerEvent{
				ServerEvent: &proto.ServerEventPayload{
					Server: &domainproto.Server{
						Id:         42,
						Name:       "test-server",
						GameId:     "cs",
						GameModId:  1,
						ServerIp:   "127.0.0.1",
						ServerPort: 27015,
					},
				},
			},
		}

		// ACT
		result, err := plugin.Instance.HandleEvent(context.Background(), event)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Handled, "complete server payload must yield Handled=true")
	})
}

func TestPluginServiceWrapper_GetSubscribedEvents(t *testing.T) {
	t.Parallel()
	// ARRANGE
	plugin := loadSharedServerLoggerWASM(t)

	// ACT
	resp, err := plugin.Instance.GetSubscribedEvents(
		context.Background(),
		&proto.GetSubscribedEventsRequest{},
	)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEmpty(t, resp.Events, "the example plugin subscribes to at least one event")
	// The example subscribes to all 14 SERVER_PRE_*/POST_* events.
	assert.Contains(t, resp.Events, proto.EventType_EVENT_TYPE_SERVER_PRE_START)
	assert.Contains(t, resp.Events, proto.EventType_EVENT_TYPE_SERVER_POST_START)
	assert.Contains(t, resp.Events, proto.EventType_EVENT_TYPE_SERVER_PRE_DELETE)
	assert.Contains(t, resp.Events, proto.EventType_EVENT_TYPE_SERVER_POST_DELETE)
}

func TestPluginServiceWrapper_Shutdown(t *testing.T) {
	t.Parallel()
	// ARRANGE
	plugin := loadSharedServerLoggerWASM(t)

	// ACT
	resp, err := plugin.Instance.Shutdown(context.Background(), &proto.ShutdownRequest{
		Context: &proto.PluginContext{PluginId: "fwgfo26jzwnm4"},
	})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Result)
	assert.True(t, resp.Result.Success)
}

func TestPluginServiceWrapper_GetHTTPRoutes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	plugin := loadSharedServerLoggerWASM(t)

	// ACT
	resp, err := plugin.Instance.GetHTTPRoutes(context.Background(), &proto.GetHTTPRoutesRequest{})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Routes, 3, "the example plugin declares 3 HTTP routes")

	paths := make([]string, 0, len(resp.Routes))
	for _, r := range resp.Routes {
		paths = append(paths, r.Path)
	}
	assert.Contains(t, paths, "/status")
	assert.Contains(t, paths, "/stats")
	assert.Contains(t, paths, "/servers/{id}")

	for _, r := range resp.Routes {
		require.NotEmpty(t, r.Methods, "every route must declare at least one method")
	}
}

func TestPluginServiceWrapper_HandleHTTPRequest(t *testing.T) {
	t.Parallel()
	t.Run("status_endpoint_returns_ok_payload", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		plugin := loadSharedServerLoggerWASM(t)
		req := &proto.HTTPRequest{
			Method:  "GET",
			Path:    "/status",
			Context: &proto.PluginContext{PluginId: "fwgfo26jzwnm4"},
		}

		// ACT
		resp, err := plugin.Instance.HandleHTTPRequest(context.Background(), req)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(200), resp.StatusCode)
		assert.Equal(t, "application/json", resp.Headers["Content-Type"])
		assert.Contains(t, string(resp.Body), `"status":"ok"`)
		assert.Contains(t, string(resp.Body), `"plugin":"server-logger"`)
	})

	t.Run("unknown_path_returns_404", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		plugin := loadSharedServerLoggerWASM(t)
		req := &proto.HTTPRequest{
			Method:  "GET",
			Path:    "/does-not-exist",
			Context: &proto.PluginContext{PluginId: "fwgfo26jzwnm4"},
		}

		// ACT
		resp, err := plugin.Instance.HandleHTTPRequest(context.Background(), req)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int32(404), resp.StatusCode)
	})
}

func TestPluginServiceWrapper_GetFrontendBundle(t *testing.T) {
	t.Parallel()
	// ARRANGE
	plugin := loadSharedServerLoggerWASM(t)

	// ACT
	resp, err := plugin.Instance.GetFrontendBundle(
		context.Background(),
		&proto.GetFrontendBundleRequest{},
	)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	// The example plugin declares HasBundle=true and embeds the frontend dist.
	assert.True(t, resp.HasBundle, "example plugin advertises a frontend bundle")
	assert.NotEmpty(t, resp.Bundle, "bundle bytes must be non-empty when HasBundle=true")
}

func TestPluginServiceWrapper_GetServerAbilities(t *testing.T) {
	t.Parallel()
	// ARRANGE
	plugin := loadSharedServerLoggerWASM(t)

	// ACT
	resp, err := plugin.Instance.GetServerAbilities(
		context.Background(),
		&proto.GetServerAbilitiesRequest{},
	)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Abilities, 2, "example plugin declares 2 server abilities")

	names := []string{resp.Abilities[0].Name, resp.Abilities[1].Name}
	assert.Contains(t, names, "view-logs")
	assert.Contains(t, names, "export-logs")
}

func TestPluginServiceWrapper_GetFrontendBundle_NilFunctionReturnsEmpty(t *testing.T) {
	t.Parallel()
	// ARRANGE
	w := &pluginServiceWrapper{getfrontendbundle: nil}

	// ACT
	resp, err := w.GetFrontendBundle(context.Background(), &proto.GetFrontendBundleRequest{})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.HasBundle, "wrapper must report no bundle when export is missing")
	assert.Empty(t, resp.Bundle)
}

func TestPluginServiceWrapper_GetServerAbilities_NilFunctionReturnsEmpty(t *testing.T) {
	t.Parallel()
	// ARRANGE
	w := &pluginServiceWrapper{getserverabilities: nil}

	// ACT
	resp, err := w.GetServerAbilities(context.Background(), &proto.GetServerAbilitiesRequest{})

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Abilities, "wrapper must report nil abilities when export is missing")
}

// sharedTestHostLibraries are the host modules the embedded example plugin
// imports; a runtime missing any of them fails to instantiate the module.
func sharedTestHostLibraries() []HostLibrary {
	return []HostLibrary{
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return log.Instantiate(ctx, r, &stubLogService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return games.Instantiate(ctx, r, &stubGamesService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return gamemods.Instantiate(ctx, r, &stubGameModsService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return servers.Instantiate(ctx, r, &stubServersService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return scheduler.Instantiate(ctx, r, &stubSchedulerService{})
		}),
		hostLibFunc(func(ctx context.Context, r wazero.Runtime) error {
			return host.Instantiate(ctx, r, sharedHostStub)
		}),
	}
}
