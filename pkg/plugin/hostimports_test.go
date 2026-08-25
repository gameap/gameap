package plugin

import (
	"context"
	_ "embed"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodecmd"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

//go:embed testdata/importing.wasm
var importingWASM []byte

// Host stubs for the three modules importing.wasm links against: the test
// only needs the modules to exist; nodecmd additionally counts its calls.
type stubNodeCmd struct {
	calls int
}

func (s *stubNodeCmd) ExecuteCommand(context.Context, *nodecmd.ExecuteCommandRequest) (*nodecmd.ExecuteCommandResponse, error) {
	s.calls++

	return &nodecmd.ExecuteCommandResponse{Output: "ok"}, nil
}

type stubNodeFS struct {
	nodefs.NodeFSService
}

type stubServers struct {
	servers.ServersService
}

type hostStubLibrary struct {
	instantiate func(ctx context.Context, r wazero.Runtime) error
}

func (l hostStubLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return l.instantiate(ctx, r)
}

func importingLibraries(cmd *stubNodeCmd) []HostLibrary {
	return []HostLibrary{
		hostStubLibrary{func(ctx context.Context, r wazero.Runtime) error {
			return nodecmd.Instantiate(ctx, r, cmd)
		}},
		hostStubLibrary{func(ctx context.Context, r wazero.Runtime) error {
			return nodefs.Instantiate(ctx, r, stubNodeFS{})
		}},
		hostStubLibrary{func(ctx context.Context, r wazero.Runtime) error {
			return servers.Instantiate(ctx, r, stubServers{})
		}},
	}
}

// observerRecorder collects Observer signals for assertions.
type observerRecorder struct {
	mu     sync.Mutex
	guest  []string
	host   []string
	events []string
}

func (o *observerRecorder) GuestCall(pluginID uint64, export string, _ time.Duration, result string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.guest = append(o.guest, label(pluginID)+":"+export+":"+result)
}

func (o *observerRecorder) HostCall(pluginID uint64, module, function string, _ time.Duration, panicked bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	result := "ok"
	if panicked {
		result = "panic"
	}

	o.host = append(o.host, label(pluginID)+":"+module+"."+function+":"+result)
}

func (o *observerRecorder) EventDispatched(eventType proto.EventType, result string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, proto.EventType_name[int32(eventType)]+":"+result)
}

func (o *observerRecorder) snapshot() (guest, host, events []string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	return append([]string(nil), o.guest...), append([]string(nil), o.host...), append([]string(nil), o.events...)
}

func label(pluginID uint64) string {
	switch pluginID {
	case 0:
		return "transient"
	default:
		return "plugin"
	}
}

func TestLoad_records_host_imports(t *testing.T) {
	t.Parallel()

	manager := NewManager(ManagerConfig{Libraries: importingLibraries(&stubNodeCmd{})})

	loaded, err := manager.Load(context.Background(), importingWASM, nil, 42)
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	assert.Equal(t, []HostImport{
		{Module: "gameap-nodecmd", Function: "execute_command"},
		{Module: "gameap-nodefs", Function: "read_dir"},
		{Module: "gameap-servers", Function: "find_servers"},
	}, loaded.HostImports, "sorted, only gameap-* modules, whether or not the function is ever called")
}

func TestLoad_observes_guest_and_host_calls(t *testing.T) {
	t.Parallel()

	cmd := &stubNodeCmd{}
	observer := &observerRecorder{}
	manager := NewManager(ManagerConfig{
		Libraries: importingLibraries(cmd),
		Observer:  observer,
	})

	loaded, err := manager.Load(context.Background(), importingWASM, nil, 42)
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	_, err = loaded.Instance.HandleEvent(context.Background(), &proto.Event{
		Type: proto.EventType_EVENT_TYPE_SERVER_POST_START,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, cmd.calls)

	guest, host, _ := observer.snapshot()

	assert.Contains(t, guest, "plugin:plugin_service_get_info:ok", "load-time calls are observed too")
	assert.Contains(t, guest, "plugin:plugin_service_handle_event:ok")
	assert.Equal(t, []string{"plugin:gameap-nodecmd.execute_command:ok"}, host,
		"exactly the host function the guest called, labelled with the plugin's database id")
}

func TestLoadTransient_is_not_observed_for_host_calls(t *testing.T) {
	t.Parallel()

	observer := &observerRecorder{}
	manager := NewManager(ManagerConfig{
		Libraries: importingLibraries(&stubNodeCmd{}),
		Observer:  observer,
	})

	loaded, err := manager.LoadTransient(context.Background(), importingWASM, nil, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = loaded.Close(context.Background()) })

	_, err = loaded.Instance.HandleEvent(context.Background(), &proto.Event{
		Type: proto.EventType_EVENT_TYPE_SERVER_POST_START,
	})
	require.NoError(t, err)

	_, host, _ := observer.snapshot()
	assert.Empty(t, host, "dry-run loads register their host libraries on the plain runtime")
}

func TestObservedRuntime_counts_panicking_host_function(t *testing.T) {
	t.Parallel()

	observer := &observerRecorder{}
	panicking := hostStubLibrary{func(ctx context.Context, r wazero.Runtime) error {
		return nodecmd.Instantiate(ctx, r, panickingNodeCmd{})
	}}

	manager := NewManager(ManagerConfig{
		Libraries: append([]HostLibrary{panicking}, importingLibraries(&stubNodeCmd{})[1:]...),
		Observer:  observer,
	})

	loaded, err := manager.Load(context.Background(), importingWASM, nil, 7)
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	_, err = loaded.Instance.HandleEvent(context.Background(), &proto.Event{
		Type: proto.EventType_EVENT_TYPE_SERVER_POST_START,
	})
	require.Error(t, err, "a panicking host function traps the guest call")

	_, host, _ := observer.snapshot()
	assert.Equal(t, []string{"plugin:gameap-nodecmd.execute_command:panic"}, host)
}

// panickingNodeCmd fails with a Go error, which the generated glue turns
// into a panic (the SDK contract for host errors).
type panickingNodeCmd struct{}

func (panickingNodeCmd) ExecuteCommand(context.Context, *nodecmd.ExecuteCommandRequest) (*nodecmd.ExecuteCommandResponse, error) {
	return nil, assert.AnError
}
