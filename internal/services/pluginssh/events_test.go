package pluginssh

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk"
	sshsdk "github.com/gameap/gameap/pkg/plugin/sdk/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testManagerID = "testplugin"

type fakeProvider struct {
	mu      sync.Mutex
	plugins map[string]*pkgplugin.LoadedPlugin
}

func (p *fakeProvider) GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	plugin, ok := p.plugins[pluginID]

	return plugin, ok
}

type fakeIDResolver struct {
	ids map[domain.Uint64ID]string
}

func (r *fakeIDResolver) GetPluginManagerID(dbID domain.Uint64ID) (string, bool) {
	id, ok := r.ids[dbID]

	return id, ok
}

// fakePluginInstance implements the plugin service plus the ssh completion
// capability, the way the real wrapper does.
type fakePluginInstance struct {
	sdk.EmptyPluginService

	mu         sync.Mutex
	completed  []*sshsdk.HandleExecCompletedRequest
	handleFunc func(ctx context.Context, req *sshsdk.HandleExecCompletedRequest) error
	hasHandler bool
}

func (f *fakePluginInstance) GetInfo(context.Context, *proto.GetInfoRequest) (*proto.PluginInfo, error) {
	return &proto.PluginInfo{Id: testManagerID}, nil
}

func (f *fakePluginInstance) HandleExecCompleted(
	ctx context.Context,
	req *sshsdk.HandleExecCompletedRequest,
) (*sshsdk.HandleExecCompletedResponse, error) {
	f.mu.Lock()
	f.completed = append(f.completed, req)
	handle := f.handleFunc
	f.mu.Unlock()

	if handle != nil {
		if err := handle(ctx, req); err != nil {
			return nil, err
		}
	}

	return &sshsdk.HandleExecCompletedResponse{}, nil
}

func (f *fakePluginInstance) HasSSHExecEventsHandler() bool { return f.hasHandler }

func (f *fakePluginInstance) recorded() []*sshsdk.HandleExecCompletedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]*sshsdk.HandleExecCompletedRequest(nil), f.completed...)
}

type eventsEnv struct {
	service  *Service
	sessions *Sessions
	instance *fakePluginInstance
	plugin   *pkgplugin.LoadedPlugin
}

func newEventsEnv(t *testing.T, cfg Config, hasHandler bool) *eventsEnv {
	t.Helper()

	instance := &fakePluginInstance{hasHandler: hasHandler}
	plugin := &pkgplugin.LoadedPlugin{
		Info:     &proto.PluginInfo{Id: testManagerID},
		Enabled:  true,
		Instance: instance,
	}
	provider := &fakeProvider{plugins: map[string]*pkgplugin.LoadedPlugin{testManagerID: plugin}}
	resolver := &fakeIDResolver{ids: map[domain.Uint64ID]string{domain.Uint64ID(testPluginID): testManagerID}}

	cfg.BlockPrivateIPs = false
	svc := newService(provider, resolver, cfg, nil, staticResolver{}, realDialer{})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.Start(ctx)

	sessions := svc.NewSessions(testPluginID)
	t.Cleanup(func() {
		sessions.Close()
		svc.Stop()
	})

	return &eventsEnv{service: svc, sessions: sessions, instance: instance, plugin: plugin}
}

func TestEvents_StartExecDeliversCompletion(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{}, true)
	handle := connectToTestServer(t, env.sessions, server)

	operationID, err := env.sessions.StartExec(context.Background(), ExecParams{
		Handle:           handle,
		Command:          "exit 5",
		NotifyCompletion: true,
	})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		return len(env.instance.recorded()) == 1
	}, 5*time.Second, 20*time.Millisecond)

	delivered := env.instance.recorded()[0]
	assert.Equal(t, operationID, delivered.OperationId)
	assert.Equal(t, handle, delivered.Handle)
	assert.Equal(t, sshsdk.ExecStatus_EXEC_STATUS_COMPLETED, delivered.Status)
	assert.False(t, delivered.Success, "a non-zero exit is not a success")
	assert.Equal(t, int32(5), delivered.ExitCode)
	assert.Positive(t, delivered.StartedAt)
	assert.Positive(t, delivered.FinishedAt)
}

// TestEvents_SubscribeAfterCompletionIsReplayed: a blocking Exec subscribes
// only after its wait budget expires, and the command may have finished in
// between — the event must not be lost.
func TestEvents_SubscribeAfterCompletionIsReplayed(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{}, true)
	handle := connectToTestServer(t, env.sessions, server)

	snapshot := runToCompletion(t, env.sessions, ExecParams{Handle: handle, Command: "echo done"})
	require.Empty(t, env.instance.recorded(), "an unsubscribed command must stay quiet")

	require.NoError(t, env.sessions.SubscribeCompletion(snapshot.OperationID))

	assert.Eventually(t, func() bool {
		return len(env.instance.recorded()) == 1
	}, 5*time.Second, 20*time.Millisecond)
}

// TestEvents_SubscribeBeforeCompletionDeliversOnce guards against the replay
// path and the normal path both firing.
func TestEvents_SubscribeBeforeCompletionDeliversOnce(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{}, true)
	handle := connectToTestServer(t, env.sessions, server)

	operationID, err := env.sessions.StartExec(context.Background(), ExecParams{
		Handle:  handle,
		Command: "sleep 200",
	})
	require.NoError(t, err)

	require.NoError(t, env.sessions.SubscribeCompletion(operationID))

	assert.Eventually(t, func() bool {
		return len(env.instance.recorded()) == 1
	}, 5*time.Second, 20*time.Millisecond)

	time.Sleep(200 * time.Millisecond)
	assert.Len(t, env.instance.recorded(), 1, "exactly one completion per operation")
}

// TestEvents_BusyPluginIsRetried: the plugin call gate serializes guest calls,
// so a completion landing while the plugin is inside another host call must be
// retried rather than dropped.
func TestEvents_BusyPluginIsRetried(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{BusyRetryDelay: 10 * time.Millisecond, BusyRetries: 3}, true)
	handle := connectToTestServer(t, env.sessions, server)

	var attempts int
	env.instance.mu.Lock()
	env.instance.handleFunc = func(context.Context, *sshsdk.HandleExecCompletedRequest) error {
		attempts++
		if attempts == 1 {
			return pkgplugin.ErrPluginBusy
		}

		return nil
	}
	env.instance.mu.Unlock()

	_, err := env.sessions.StartExec(context.Background(), ExecParams{
		Handle:           handle,
		Command:          "echo hi",
		NotifyCompletion: true,
	})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		return len(env.instance.recorded()) == 2
	}, 5*time.Second, 20*time.Millisecond)
}

// TestEvents_PluginWithoutHandlerIsNotCalled: a plugin compiled without the
// ssh module exports nothing to call.
func TestEvents_PluginWithoutHandlerIsNotCalled(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{}, false)
	handle := connectToTestServer(t, env.sessions, server)

	_, err := env.sessions.StartExec(context.Background(), ExecParams{
		Handle:           handle,
		Command:          "echo hi",
		NotifyCompletion: true,
	})
	require.NoError(t, err)

	time.Sleep(300 * time.Millisecond)
	assert.Empty(t, env.instance.recorded())
}

// TestEvents_ClosedSessionsDropPendingCallbacks: a reloaded plugin instance
// must not receive completions belonging to the previous one.
func TestEvents_ClosedSessionsDropPendingCallbacks(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{}, true)
	handle := connectToTestServer(t, env.sessions, server)

	_, err := env.sessions.StartExec(context.Background(), ExecParams{
		Handle:           handle,
		Command:          "sleep 30000",
		NotifyCompletion: true,
	})
	require.NoError(t, err)

	env.sessions.Close()

	time.Sleep(300 * time.Millisecond)
	assert.Empty(t, env.instance.recorded())
}

// TestService_StoppedServiceHandsOutClosedSessions: a plugin loading while the
// panel shuts down must get a set that refuses work rather than deadlocking
// the loader.
func TestService_StoppedServiceHandsOutClosedSessions(t *testing.T) {
	t.Parallel()
	svc := newService(nil, nil, Config{}, nil, staticResolver{}, realDialer{})
	svc.Start(context.Background())
	svc.Stop()

	done := make(chan *Sessions, 1)
	go func() { done <- svc.NewSessions(testPluginID) }()

	select {
	case sessions := <-done:
		_, err := sessions.StartExec(context.Background(), ExecParams{Handle: 1, Command: "echo hi"})
		assert.ErrorIs(t, err, ErrSessionsClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("NewSessions blocked after Stop")
	}
}
