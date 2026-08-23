package plugin

import (
	"bytes"
	"context"
	_ "embed"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/misbehaving.wasm
var misbehavingWASM []byte

type disableCall struct {
	pluginID string
	dbID     uint64
	reason   string
}

// disableRecorder captures DisableHook invocations; the hook runs on its own
// goroutine, so readers wait on the channel.
type disableRecorder struct {
	calls chan disableCall
}

func newDisableRecorder() *disableRecorder {
	return &disableRecorder{calls: make(chan disableCall, 8)}
}

func (r *disableRecorder) hook(pluginID string, dbID uint64, reason string) {
	r.calls <- disableCall{pluginID: pluginID, dbID: dbID, reason: reason}
}

func (r *disableRecorder) wait(t *testing.T) disableCall {
	t.Helper()

	select {
	case call := <-r.calls:
		return call
	case <-time.After(5 * time.Second):
		t.Fatal("disable hook was not invoked")

		return disableCall{}
	}
}

func (r *disableRecorder) assertSilent(t *testing.T) {
	t.Helper()

	select {
	case call := <-r.calls:
		t.Fatalf("unexpected disable hook call: %+v", call)
	case <-time.After(50 * time.Millisecond):
	}
}

func loadMisbehavingPlugin(t *testing.T, cfg ManagerConfig) (*Manager, *LoadedPlugin) {
	t.Helper()

	manager := NewManager(cfg)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	loaded, err := manager.Load(context.Background(), misbehavingWASM, nil, 42)
	require.NoError(t, err)
	require.Equal(t, "misbehaving", loaded.Info.Id)

	return manager, loaded
}

func TestDisableWithReason_first_reason_wins_and_hook_fires_once(t *testing.T) {
	t.Parallel()
	recorder := newDisableRecorder()
	plugin := &LoadedPlugin{
		Info:       &proto.PluginInfo{Id: "hooked"},
		Enabled:    true,
		DBID:       7,
		onDisabled: recorder.hook,
	}

	var wg sync.WaitGroup
	for _, reason := range []string{"first", "second", "third"} {
		wg.Go(func() {
			plugin.DisableWithReason(reason)
		})
	}
	wg.Wait()

	assert.False(t, plugin.IsEnabled())

	reason, ok := plugin.DisabledReason()
	require.True(t, ok)
	assert.Contains(t, []string{"first", "second", "third"}, reason)

	call := recorder.wait(t)
	assert.Equal(t, "hooked", call.pluginID)
	assert.Equal(t, uint64(7), call.dbID)
	assert.Equal(t, reason, call.reason)
	recorder.assertSilent(t)
}

func TestDisable_silent_keeps_no_reason_and_skips_hook(t *testing.T) {
	t.Parallel()
	recorder := newDisableRecorder()
	plugin := &LoadedPlugin{
		Info:       &proto.PluginInfo{Id: "silent"},
		Enabled:    true,
		onDisabled: recorder.hook,
	}

	plugin.Disable()
	plugin.DisableWithReason("too late")

	assert.False(t, plugin.IsEnabled())

	_, ok := plugin.DisabledReason()
	assert.False(t, ok)
	recorder.assertSilent(t)
}

func TestDisableWithReason_without_hook(t *testing.T) {
	t.Parallel()
	plugin := &LoadedPlugin{Info: &proto.PluginInfo{Id: "nohook"}, Enabled: true}

	plugin.DisableWithReason(DisableReasonHTTPTimeout)

	reason, ok := plugin.DisabledReason()
	require.True(t, ok)
	assert.Equal(t, DisableReasonHTTPTimeout, reason)
}

func TestEventTimeoutReason(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "event handler timed out (SERVER_PRE_START)",
		EventTimeoutReason(proto.EventType_EVENT_TYPE_SERVER_PRE_START))
	assert.Equal(t, "event handler timed out (12345)", EventTimeoutReason(proto.EventType(12345)))
}

func TestShortErrorText(t *testing.T) {
	t.Parallel()
	assert.Empty(t, shortErrorText(nil))
	assert.Equal(t, "module closed with exit_code(3)",
		shortErrorText(errors.New("module closed with exit_code(3)\nwasm stack trace:\n\tfn()")))

	long := bytes.Repeat([]byte("x"), shortErrorMaxLen+50)
	assert.Len(t, shortErrorText(errors.New(string(long))), shortErrorMaxLen)
}

func TestMemorySize_unavailable_for_mock_instance(t *testing.T) {
	t.Parallel()
	plugin := &LoadedPlugin{Info: &proto.PluginInfo{Id: "mock"}, Enabled: true, Instance: &mockPluginService{}}

	_, ok := plugin.MemorySize()
	assert.False(t, ok)
}

func TestMisbehavingPlugin_guest_exit_disables_with_reason(t *testing.T) {
	t.Parallel()
	recorder := newDisableRecorder()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	_, loaded := loadMisbehavingPlugin(t, ManagerConfig{
		GuestLogger:      logger,
		OnPluginDisabled: recorder.hook,
	})
	assert.Equal(t, uint64(42), loaded.DBID)

	size, ok := loaded.MemorySize()
	require.True(t, ok)
	assert.Equal(t, uint64(wasmPageSize), size)

	_, err := loaded.Instance.HandleEvent(context.Background(), &proto.Event{
		Type: proto.EventType_EVENT_TYPE_SERVER_POST_START,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit_code(3)")

	assert.False(t, loaded.IsEnabled())
	reason, hasReason := loaded.DisabledReason()
	require.True(t, hasReason)
	assert.Contains(t, reason, DisableReasonGuestExited)
	assert.Contains(t, reason, "exit_code(3)")

	call := recorder.wait(t)
	assert.Equal(t, "misbehaving", call.pluginID)
	assert.Equal(t, uint64(42), call.dbID)
	assert.Equal(t, reason, call.reason)

	_, ok = loaded.MemorySize()
	assert.False(t, ok, "a disabled plugin reports no memory size")

	assert.Contains(t, logs.String(), "hello from guest")
	assert.Contains(t, logs.String(), "stream=stdout")
	assert.Contains(t, logs.String(), "plugin_id=misbehaving")
	assert.Contains(t, logs.String(), "level=DEBUG")
}

func TestMisbehavingPlugin_call_deadline_is_left_to_the_caller(t *testing.T) {
	t.Parallel()
	recorder := newDisableRecorder()

	_, loaded := loadMisbehavingPlugin(t, ManagerConfig{OnPluginDisabled: recorder.hook})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := loaded.Instance.HandleHTTPRequest(ctx, &proto.HTTPRequest{Method: "GET", Path: "/"})
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// The wrapper must not pick a reason for a deadline: the caller knows
	// what the guest was doing and reports that.
	assert.True(t, loaded.IsEnabled())
	recorder.assertSilent(t)

	loaded.DisableWithReason(DisableReasonHTTPTimeout + " (GET /)")

	call := recorder.wait(t)
	assert.Equal(t, DisableReasonHTTPTimeout+" (GET /)", call.reason)
	assert.Equal(t, uint64(42), call.dbID)

	_, ok := loaded.MemorySize()
	assert.False(t, ok)
}

func TestMisbehavingPlugin_unload_after_guest_exit(t *testing.T) {
	t.Parallel()
	manager, loaded := loadMisbehavingPlugin(t, ManagerConfig{})

	_, err := loaded.Instance.HandleEvent(context.Background(), &proto.Event{
		Type: proto.EventType_EVENT_TYPE_SERVER_POST_START,
	})
	require.Error(t, err)

	require.NoError(t, manager.Unload(context.Background(), loaded.Info.Id))

	_, exists := manager.GetPlugin(loaded.Info.Id)
	assert.False(t, exists)
}

func TestLoadTransient_does_not_wire_disable_hook(t *testing.T) {
	t.Parallel()
	recorder := newDisableRecorder()
	manager := NewManager(ManagerConfig{OnPluginDisabled: recorder.hook})
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })

	loaded, err := manager.LoadTransient(context.Background(), misbehavingWASM, nil, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = loaded.Close(context.Background()) })

	assert.Equal(t, uint64(0), loaded.DBID)

	_, err = loaded.Instance.HandleEvent(context.Background(), &proto.Event{})
	require.Error(t, err)

	assert.False(t, loaded.IsEnabled())
	recorder.assertSilent(t)
}
