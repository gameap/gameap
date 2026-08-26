package pluginssh

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk"
	sshsdk "github.com/gameap/gameap/pkg/plugin/sdk/ssh"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serviceTestBareInstance announces the ssh capability the way the wrapper of a
// plugin built with the sdk/ssh module does, but exports no callback: the guest
// imported the module and never registered a handler. Resolving must answer
// "nothing to call" instead of asserting its way into a panic.
type serviceTestBareInstance struct {
	sdk.EmptyPluginService
}

func (serviceTestBareInstance) HasSSHExecEventsHandler() bool { return true }

// serviceTestSetHandler installs the guest side of the completion callback.
func serviceTestSetHandler(
	env *eventsEnv,
	handle func(ctx context.Context, req *sshsdk.HandleExecCompletedRequest) error,
) {
	env.instance.mu.Lock()
	defer env.instance.mu.Unlock()

	env.instance.handleFunc = handle
}

// serviceTestAttempts counts how many times the panel called into the guest:
// the fake records the request before running the injected behaviour, so a
// retried delivery shows up as another entry.
func serviceTestAttempts(env *eventsEnv) int {
	return len(env.instance.recorded())
}

func TestService_NewAppliesDefaults(t *testing.T) {
	t.Parallel()
	svc := New(nil, nil, Config{}, nil)
	t.Cleanup(svc.Stop)

	require.NotNil(t, svc.logger, "a caller that passes no logger must not blow up on the first log line")
	assert.Equal(t, Config{}.withDefaults(), svc.cfg, "an operator who configured nothing gets the packaged limits")
	assert.NotNil(t, svc.resolver)
	assert.NotNil(t, svc.dialer)

	sessions := svc.NewSessions(testPluginID)
	t.Cleanup(sessions.Close)

	_, err := sessions.StartExec(context.Background(), ExecParams{Handle: 1, Command: "echo hi"})
	assert.ErrorIs(t, err, ErrConnectionNotFound, "the set is live: it refuses the handle, not the whole set")
}

// TestService_StopIsIdempotent: shutdown is reachable twice (the panel stopping
// while a plugin unload is already tearing the same service down).
func TestService_StopIsIdempotent(t *testing.T) {
	t.Parallel()
	svc := newService(nil, nil, Config{}, nil, staticResolver{}, realDialer{})
	svc.Start(context.Background())
	sessions := svc.NewSessions(testPluginID)

	svc.Stop()
	assert.NotPanics(t, svc.Stop, "the second Stop must not wait on the delivery group again")

	_, err := sessions.StartExec(context.Background(), ExecParams{Handle: 1, Command: "echo hi"})
	assert.ErrorIs(t, err, ErrSessionsClosed, "Stop closes every session set it handed out")
}

// TestService_CompletionAfterStopIsDropped: a callback scheduled while the
// panel is shutting down must not reach a plugin whose runtime is being closed.
func TestService_CompletionAfterStopIsDropped(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{}, true)
	handle := connectToTestServer(t, env.sessions, server)

	snapshot := runToCompletion(t, env.sessions, ExecParams{Handle: handle, Command: "echo hi"})
	op, err := env.sessions.operation(snapshot.OperationID)
	require.NoError(t, err)
	require.Empty(t, env.instance.recorded(), "an unsubscribed command must stay quiet")

	env.service.Stop()
	env.service.notifyCompleted(env.sessions, op)

	time.Sleep(300 * time.Millisecond)
	assert.Empty(t, env.instance.recorded(), "a stopped service schedules nothing")
}

func TestService_ResolveHandler(t *testing.T) {
	t.Parallel()

	compactID := pkgplugin.CompactPluginID(domain.Uint64ID(testPluginID))

	loaded := func(managerID string, enabled, hasHandler bool) *pkgplugin.LoadedPlugin {
		return &pkgplugin.LoadedPlugin{
			Info:     &proto.PluginInfo{Id: managerID},
			Enabled:  enabled,
			Instance: &fakePluginInstance{hasHandler: hasHandler},
		}
	}

	registry := func(managerID string, plugin *pkgplugin.LoadedPlugin) PluginProvider {
		return &fakeProvider{plugins: map[string]*pkgplugin.LoadedPlugin{managerID: plugin}}
	}

	knownID := &fakeIDResolver{ids: map[domain.Uint64ID]string{domain.Uint64ID(testPluginID): testManagerID}}

	tests := []struct {
		name   string
		build  func() (PluginProvider, ManagerIDResolver)
		wantOK bool
	}{
		{
			name: "service_without_a_plugin_provider_resolves_nothing",
			build: func() (PluginProvider, ManagerIDResolver) {
				return nil, knownID
			},
		},
		{
			name: "manager_that_never_loaded_the_plugin_resolves_nothing",
			build: func() (PluginProvider, ManagerIDResolver) {
				return &fakeProvider{plugins: map[string]*pkgplugin.LoadedPlugin{}}, knownID
			},
		},
		{
			name: "plugin_disabled_by_the_operator_is_skipped",
			build: func() (PluginProvider, ManagerIDResolver) {
				return registry(testManagerID, loaded(testManagerID, false, true)), knownID
			},
		},
		{
			name: "plugin_disabled_at_runtime_is_skipped",
			build: func() (PluginProvider, ManagerIDResolver) {
				plugin := loaded(testManagerID, true, true)
				plugin.Disable()

				return registry(testManagerID, plugin), knownID
			},
		},
		{
			name: "plugin_built_without_the_ssh_module_is_skipped",
			build: func() (PluginProvider, ManagerIDResolver) {
				return registry(testManagerID, loaded(testManagerID, true, false)), knownID
			},
		},
		{
			name: "instance_announcing_a_handler_it_does_not_export_is_skipped",
			build: func() (PluginProvider, ManagerIDResolver) {
				plugin := &pkgplugin.LoadedPlugin{
					Info:     &proto.PluginInfo{Id: testManagerID},
					Enabled:  true,
					Instance: &serviceTestBareInstance{},
				}

				return registry(testManagerID, plugin), knownID
			},
		},
		{
			name: "resolved_manager_id_is_preferred",
			build: func() (PluginProvider, ManagerIDResolver) {
				return registry(testManagerID, loaded(testManagerID, true, true)), knownID
			},
			wantOK: true,
		},
		{
			name: "service_without_an_id_resolver_falls_back_to_the_compact_id",
			build: func() (PluginProvider, ManagerIDResolver) {
				return registry(compactID, loaded(compactID, true, true)), nil
			},
			wantOK: true,
		},
		{
			name: "db_id_unknown_to_the_resolver_falls_back_to_the_compact_id",
			build: func() (PluginProvider, ManagerIDResolver) {
				return registry(compactID, loaded(compactID, true, true)), &fakeIDResolver{}
			},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			provider, idResolver := tt.build()
			svc := newService(provider, idResolver, Config{}, nil, staticResolver{}, realDialer{})
			t.Cleanup(svc.Stop)

			handler, plugin, ok := svc.resolveHandler(testPluginID)

			assert.Equal(t, tt.wantOK, ok)

			if !tt.wantOK {
				assert.Nil(t, handler, "an unresolved plugin must not hand out a handler to call")
				assert.Nil(t, plugin)

				return
			}

			assert.NotNil(t, handler)
			require.NotNil(t, plugin, "the plugin is what a failed guest call is disabled through")
			assert.True(t, plugin.IsEnabled())
		})
	}
}

// TestService_BusyPluginRetriesAreBounded: a plugin that never frees its call
// gate must not keep a delivery goroutine retrying for the lifetime of the
// panel.
func TestService_BusyPluginRetriesAreBounded(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{BusyRetries: 1, BusyRetryDelay: time.Millisecond}, true)
	handle := connectToTestServer(t, env.sessions, server)

	serviceTestSetHandler(env, func(context.Context, *sshsdk.HandleExecCompletedRequest) error {
		return pkgplugin.ErrPluginBusy
	})

	_, err := env.sessions.StartExec(context.Background(), ExecParams{
		Handle:           handle,
		Command:          "echo hi",
		NotifyCompletion: true,
	})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		return serviceTestAttempts(env) == 2
	}, 5*time.Second, 20*time.Millisecond)

	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, 2, serviceTestAttempts(env),
		"the first call plus BusyRetries retries, then the event is dropped")
	assert.True(t, env.plugin.IsEnabled(), "a busy plugin is not a broken one")
}

// TestService_BusyRetriesStopWhenTheInstanceIsUnloaded: an unloaded module
// instance has nothing left to deliver to, so the backoff must give up rather
// than wait out the full delay.
func TestService_BusyRetriesStopWhenTheInstanceIsUnloaded(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{BusyRetries: 5, BusyRetryDelay: 500 * time.Millisecond}, true)
	handle := connectToTestServer(t, env.sessions, server)

	serviceTestSetHandler(env, func(context.Context, *sshsdk.HandleExecCompletedRequest) error {
		return pkgplugin.ErrPluginBusy
	})

	_, err := env.sessions.StartExec(context.Background(), ExecParams{
		Handle:           handle,
		Command:          "echo hi",
		NotifyCompletion: true,
	})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		return serviceTestAttempts(env) == 1
	}, 5*time.Second, 10*time.Millisecond)

	env.sessions.Close()

	time.Sleep(800 * time.Millisecond)
	assert.Equal(t, 1, serviceTestAttempts(env), "no retry may land after the instance is gone")
}

// TestService_BusyRetriesStopWhenThePanelShutsDown: the same, for the other way
// a retry loop can outlive its purpose.
func TestService_BusyRetriesStopWhenThePanelShutsDown(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{BusyRetries: 5, BusyRetryDelay: 500 * time.Millisecond}, true)
	handle := connectToTestServer(t, env.sessions, server)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	env.service.Start(ctx)

	serviceTestSetHandler(env, func(context.Context, *sshsdk.HandleExecCompletedRequest) error {
		return pkgplugin.ErrPluginBusy
	})

	_, err := env.sessions.StartExec(context.Background(), ExecParams{
		Handle:           handle,
		Command:          "echo hi",
		NotifyCompletion: true,
	})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		return serviceTestAttempts(env) == 1
	}, 5*time.Second, 10*time.Millisecond)

	cancel()

	time.Sleep(800 * time.Millisecond)
	assert.Equal(t, 1, serviceTestAttempts(env), "a shutting-down panel stops retrying")
}

// TestService_HangingPluginIsDisabled: a guest that cannot answer within the
// call budget has wedged its runtime, and every later event would wedge another
// delivery goroutine behind it. The plugin stops receiving work until it is
// reloaded.
func TestService_HangingPluginIsDisabled(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	env := newEventsEnv(t, Config{
		CompletionCallTimeout: 20 * time.Millisecond,
		BusyRetries:           3,
		BusyRetryDelay:        time.Millisecond,
	}, true)
	handle := connectToTestServer(t, env.sessions, server)

	serviceTestSetHandler(env, func(ctx context.Context, _ *sshsdk.HandleExecCompletedRequest) error {
		<-ctx.Done()

		return ctx.Err()
	})

	require.True(t, env.plugin.IsEnabled())

	_, err := env.sessions.StartExec(context.Background(), ExecParams{
		Handle:           handle,
		Command:          "echo hi",
		NotifyCompletion: true,
	})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		return !env.plugin.IsEnabled()
	}, 5*time.Second, 20*time.Millisecond)

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, 1, serviceTestAttempts(env), "a call that outran its budget is not retried")
}

// TestService_GuestErrorKeepsPluginEnabled: a handler that returns an error did
// answer, so the panel loses the event and nothing else — a plugin is not
// disabled for a bug in one callback.
func TestService_GuestErrorKeepsPluginEnabled(t *testing.T) {
	t.Parallel()
	server := newTestSSHServer(t)
	// The budget is deliberately long: only a call that outran it disables the
	// plugin, and this test is about the branch that does not.
	env := newEventsEnv(t, Config{
		CompletionCallTimeout: 10 * time.Second,
		BusyRetries:           3,
		BusyRetryDelay:        time.Millisecond,
	}, true)
	handle := connectToTestServer(t, env.sessions, server)

	serviceTestSetHandler(env, func(context.Context, *sshsdk.HandleExecCompletedRequest) error {
		return errors.New("guest trapped while handling the completion")
	})

	_, err := env.sessions.StartExec(context.Background(), ExecParams{
		Handle:           handle,
		Command:          "echo hi",
		NotifyCompletion: true,
	})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		return serviceTestAttempts(env) == 1
	}, 5*time.Second, 20*time.Millisecond)

	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, 1, serviceTestAttempts(env), "an ordinary guest error is not retried")
	assert.True(t, env.plugin.IsEnabled(), "one failed callback must not take the plugin down")
}
