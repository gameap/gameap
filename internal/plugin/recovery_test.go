package plugin

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

func (a *auditCapture) ofType(t audit.EventType) []audit.Event {
	var result []audit.Event
	for _, e := range a.snapshot() {
		if e.Type == t {
			result = append(result, e)
		}
	}

	return result
}

// fakeTimers records scheduled reloads and fires them on demand.
type fakeTimers struct {
	mu      sync.Mutex
	pending []*fakeTimer
}

type fakeTimer struct {
	delay   time.Duration
	fn      func()
	stopped bool
}

func (f *fakeTimer) Stop() bool {
	f.stopped = true

	return true
}

func (f *fakeTimers) newTimer(delay time.Duration, fn func()) recoveryTimer {
	f.mu.Lock()
	defer f.mu.Unlock()

	timer := &fakeTimer{delay: delay, fn: fn}
	f.pending = append(f.pending, timer)

	return timer
}

func (f *fakeTimers) delays() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()

	result := make([]time.Duration, 0, len(f.pending))
	for _, timer := range f.pending {
		result = append(result, timer.delay)
	}

	return result
}

func (f *fakeTimers) last() *fakeTimer {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.pending) == 0 {
		return nil
	}

	return f.pending[len(f.pending)-1]
}

// fire runs the latest timer synchronously, as time.AfterFunc would on its
// own goroutine.
func (f *fakeTimers) fire(t *testing.T) {
	t.Helper()

	timer := f.last()
	require.NotNil(t, timer, "a reload must be scheduled")
	require.False(t, timer.stopped, "the scheduled reload must not have been cancelled")

	timer.fn()
}

type recoveryEnv struct {
	ctx      context.Context
	repo     *inmemory.PluginRepository
	files    files.FileManager
	manager  *mockPluginManager
	loader   *Loader
	timers   *fakeTimers
	audit    *auditCapture
	refresh  *refreshRecorder
	now      time.Time
	nowMu    sync.Mutex
	recovery *Supervisor
}

func (e *recoveryEnv) advance(d time.Duration) {
	e.nowMu.Lock()
	defer e.nowMu.Unlock()
	e.now = e.now.Add(d)
}

func (e *recoveryEnv) currentTime() time.Time {
	e.nowMu.Lock()
	defer e.nowMu.Unlock()

	return e.now
}

func newRecoveryEnv(t *testing.T, manager *mockPluginManager, opts RecoveryOptions) *recoveryEnv {
	t.Helper()

	env := &recoveryEnv{
		ctx:     context.Background(),
		repo:    inmemory.NewPluginRepository(),
		files:   files.NewInMemoryFileManager(),
		manager: manager,
		timers:  &fakeTimers{},
		audit:   &auditCapture{},
		refresh: &refreshRecorder{},
		now:     time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC),
	}

	env.loader = NewLoader(manager, env.files, env.repo, nil, "plugins", WithSubscriptionRefresher(env.refresh))

	opts.newTimer = env.timers.newTimer
	opts.now = env.currentTime
	env.recovery = NewSupervisor(env.loader, env.repo, env.audit, opts, slog.New(slog.DiscardHandler))
	t.Cleanup(env.recovery.Stop)

	return env
}

func (e *recoveryEnv) installPlugin(t *testing.T, id domain.Uint64ID, content string) *domain.Plugin {
	t.Helper()

	plugin := seedPlugin(e.ctx, t, e.repo, id, domain.PluginStatusActive)
	require.NoError(t, e.files.Write(e.ctx, "plugins/"+*plugin.Filename, []byte(content)))
	e.loader.RegisterPluginID(id, "instance-"+pluginIDString(uint64(id)))

	return plugin
}

func (e *recoveryEnv) row(t *testing.T, id domain.Uint64ID) domain.Plugin {
	t.Helper()

	return findPlugin(e.ctx, t, e.repo, id)
}

func TestSupervisor_OnPluginDisabled_marks_row_audits_and_schedules(t *testing.T) {
	t.Parallel()
	env := newRecoveryEnv(t, failingManager(), RecoveryOptions{})
	plugin := env.installPlugin(t, 1001, "fine")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out (GET /x)")

	row := env.row(t, plugin.ID)
	assert.Equal(t, domain.PluginStatusError, row.Status)
	require.NotNil(t, row.LastError)
	assert.Equal(t, "http handler timed out (GET /x)", *row.LastError)
	require.NotNil(t, row.LastErrorAt)
	assert.Equal(t, env.currentTime(), *row.LastErrorAt)

	disabled := env.audit.ofType(audit.EventPluginDisabled)
	require.Len(t, disabled, 1)
	assert.Equal(t, audit.OutcomeFailure, disabled[0].Outcome)
	assert.Equal(t, audit.AuthMethodSystem, disabled[0].AuthMethod)
	assert.Equal(t, "guest_call_timeout", disabled[0].Reason)
	assert.Equal(t, "plugin", disabled[0].ResourceType)
	assert.Equal(t, "1001", disabled[0].ResourceID)
	assert.Equal(t, "disable", disabled[0].Action)

	assert.Equal(t, []time.Duration{30 * time.Second}, env.timers.delays())
}

func TestSupervisor_guest_exit_reason_token(t *testing.T) {
	t.Parallel()
	env := newRecoveryEnv(t, failingManager(), RecoveryOptions{})
	plugin := env.installPlugin(t, 1002, "fine")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), pkgplugin.DisableReasonGuestExited+" (exit_code(2))")

	disabled := env.audit.ofType(audit.EventPluginDisabled)
	require.Len(t, disabled, 1)
	assert.Equal(t, "guest_exited", disabled[0].Reason)
}

func TestSupervisor_attempt_reloads_and_audits_success(t *testing.T) {
	t.Parallel()
	manager := failingManager()
	var unloaded []string
	manager.unloadFunc = func(_ context.Context, pluginID string) error {
		unloaded = append(unloaded, pluginID)

		return nil
	}
	env := newRecoveryEnv(t, manager, RecoveryOptions{})
	plugin := env.installPlugin(t, 1003, "fine")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "event handler timed out (SERVER_PRE_START)")
	env.advance(30 * time.Second)
	env.timers.fire(t)

	assert.Equal(t, []string{"instance-" + pluginIDString(1003)}, unloaded)
	assert.Equal(t, 1, manager.loadedCount)
	assert.Equal(t, 1, env.refresh.count())

	row := env.row(t, plugin.ID)
	assert.Equal(t, domain.PluginStatusActive, row.Status)
	assert.Nil(t, row.LastError)

	reloaded := env.audit.ofType(audit.EventPluginReloaded)
	require.Len(t, reloaded, 1)
	assert.Equal(t, audit.OutcomeSuccess, reloaded[0].Outcome)
	assert.Equal(t, audit.AuthMethodSystem, reloaded[0].AuthMethod)
	assert.Equal(t, "reload", reloaded[0].Action)
	assert.Equal(t, "1003", reloaded[0].ResourceID)

	env.recovery.mu.Lock()
	entry := env.recovery.entries[plugin.ID]
	env.recovery.mu.Unlock()
	require.NotNil(t, entry)
	assert.Nil(t, entry.timer)
	assert.Equal(t, env.currentTime(), entry.healthySince)
}

func TestSupervisor_backoff_sequence_then_exhausted(t *testing.T) {
	t.Parallel()
	env := newRecoveryEnv(t, failingManager("broken"), RecoveryOptions{MaxAttempts: 5})
	plugin := env.installPlugin(t, 1004, "broken")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")

	for range 4 {
		env.timers.fire(t)
	}

	assert.Equal(t, []time.Duration{
		30 * time.Second, 60 * time.Second, 120 * time.Second, 240 * time.Second, 480 * time.Second,
	}, env.timers.delays())

	env.timers.fire(t)

	assert.Len(t, env.timers.delays(), 5, "no timer after the attempts are exhausted")

	row := env.row(t, plugin.ID)
	assert.Equal(t, domain.PluginStatusError, row.Status)
	require.NotNil(t, row.LastError)
	assert.Contains(t, *row.LastError, "simulated load failure for broken")
	assert.Contains(t, *row.LastError, "automatic reload attempts exhausted (5)")

	failed := env.audit.ofType(audit.EventPluginReloaded)
	require.Len(t, failed, 5)
	for _, event := range failed {
		assert.Equal(t, audit.OutcomeFailure, event.Outcome)
		assert.Equal(t, "load_failed", event.Reason)
	}

	disabled := env.audit.ofType(audit.EventPluginDisabled)
	require.Len(t, disabled, 2)
	assert.Equal(t, "recovery_exhausted", disabled[1].Reason)

	// A later runtime disable of the same plugin no longer schedules anything.
	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	assert.Len(t, env.timers.delays(), 5)
}

func TestSupervisor_delay_is_capped_at_max_delay(t *testing.T) {
	t.Parallel()
	env := newRecoveryEnv(t, failingManager("broken"), RecoveryOptions{
		InitialDelay: time.Minute,
		MaxDelay:     3 * time.Minute,
		MaxAttempts:  4,
	})
	plugin := env.installPlugin(t, 1005, "broken")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	env.timers.fire(t)
	env.timers.fire(t)
	env.timers.fire(t)

	assert.Equal(t, []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute, 3 * time.Minute}, env.timers.delays())
}

func TestSupervisor_attempts_reset_after_a_healthy_period(t *testing.T) {
	t.Parallel()
	env := newRecoveryEnv(t, failingManager(), RecoveryOptions{MaxAttempts: 2, MaxDelay: 10 * time.Minute})
	plugin := env.installPlugin(t, 1006, "fine")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	env.timers.fire(t)
	assert.Equal(t, []time.Duration{30 * time.Second}, env.timers.delays())

	// Disabled again right away: the series continues.
	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	env.timers.fire(t)
	assert.Equal(t, []time.Duration{30 * time.Second, 60 * time.Second}, env.timers.delays())

	// Healthy for longer than MaxDelay: the count starts over.
	env.advance(11 * time.Minute)
	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	assert.Equal(t, []time.Duration{30 * time.Second, 60 * time.Second, 30 * time.Second}, env.timers.delays())
}

func TestSupervisor_skips_rows_it_must_not_touch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status domain.PluginStatus
	}{
		{name: "disabled_by_operator", status: domain.PluginStatusDisabled},
		{name: "updating", status: domain.PluginStatusUpdating},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newRecoveryEnv(t, failingManager(), RecoveryOptions{})
			plugin := seedPlugin(env.ctx, t, env.repo, 1007, tt.status)

			env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")

			assert.Equal(t, tt.status, env.row(t, plugin.ID).Status)
			assert.Nil(t, env.row(t, plugin.ID).LastError)
			assert.Empty(t, env.timers.delays())
			assert.Empty(t, env.audit.snapshot())
		})
	}
}

func TestSupervisor_ignores_unknown_plugins(t *testing.T) {
	t.Parallel()
	env := newRecoveryEnv(t, failingManager(), RecoveryOptions{})

	env.recovery.OnPluginDisabled("transient", 0, "http handler timed out")
	env.recovery.OnPluginDisabled("missing", 4242, "http handler timed out")

	assert.Empty(t, env.timers.delays())
	assert.Empty(t, env.audit.snapshot())
}

func TestSupervisor_attempt_skips_when_row_changed(t *testing.T) {
	t.Parallel()

	t.Run("uninstalled_meanwhile", func(t *testing.T) {
		t.Parallel()
		env := newRecoveryEnv(t, failingManager(), RecoveryOptions{})
		plugin := env.installPlugin(t, 1008, "fine")

		env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
		require.NoError(t, env.repo.Delete(env.ctx, plugin.ID))

		env.timers.fire(t)

		assert.Equal(t, 0, env.manager.loadedCount)
		env.recovery.mu.Lock()
		_, exists := env.recovery.entries[plugin.ID]
		env.recovery.mu.Unlock()
		assert.False(t, exists)
	})

	t.Run("reloaded_by_operator_meanwhile", func(t *testing.T) {
		t.Parallel()
		env := newRecoveryEnv(t, failingManager(), RecoveryOptions{})
		plugin := env.installPlugin(t, 1009, "fine")

		env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")

		row := env.row(t, plugin.ID)
		row.MarkActive(env.currentTime())
		require.NoError(t, env.repo.Save(env.ctx, &row))

		env.timers.fire(t)

		assert.Equal(t, 0, env.manager.loadedCount)
		assert.Empty(t, env.audit.ofType(audit.EventPluginReloaded))
	})
}

func TestSupervisor_Forget_cancels_pending_reload(t *testing.T) {
	t.Parallel()
	env := newRecoveryEnv(t, failingManager(), RecoveryOptions{})
	plugin := env.installPlugin(t, 1010, "fine")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	timer := env.timers.last()
	require.NotNil(t, timer)

	env.loader.Forget(plugin.ID)

	assert.True(t, timer.stopped)

	// A new disable starts a fresh series.
	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	assert.Equal(t, []time.Duration{30 * time.Second, 30 * time.Second}, env.timers.delays())
}

func TestSupervisor_Unload_through_loader_cancels_recovery(t *testing.T) {
	t.Parallel()
	env := newRecoveryEnv(t, failingManager(), RecoveryOptions{})
	plugin := env.installPlugin(t, 1011, "fine")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	timer := env.timers.last()
	require.NotNil(t, timer)

	require.NoError(t, env.loader.Unload(env.ctx, "instance-"+pluginIDString(1011)))

	assert.True(t, timer.stopped)
}

func TestSupervisor_Stop_prevents_new_work(t *testing.T) {
	t.Parallel()
	env := newRecoveryEnv(t, failingManager(), RecoveryOptions{})
	plugin := env.installPlugin(t, 1012, "fine")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	timer := env.timers.last()
	require.NotNil(t, timer)

	env.recovery.Stop()

	assert.True(t, timer.stopped)

	timer.fn()
	assert.Equal(t, 0, env.manager.loadedCount, "a timer firing after Stop does nothing")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	assert.Len(t, env.timers.delays(), 1)
}

func TestRecoveryOptions_defaults(t *testing.T) {
	t.Parallel()
	opts := RecoveryOptions{}.withDefaults()

	assert.Equal(t, 30*time.Second, opts.InitialDelay)
	assert.Equal(t, 10*time.Minute, opts.MaxDelay)
	assert.Equal(t, 5, opts.MaxAttempts)
	assert.InDelta(t, 2.0, opts.Factor, 0)
	assert.NotNil(t, opts.newTimer)
	assert.NotNil(t, opts.now)

	capped := RecoveryOptions{InitialDelay: time.Hour, MaxDelay: time.Minute}.withDefaults()
	assert.Equal(t, time.Hour, capped.MaxDelay, "max delay never undercuts the initial delay")
}

func TestSupervisor_reload_disabled_keeps_bookkeeping(t *testing.T) {
	t.Parallel()
	env := newRecoveryEnv(t, failingManager(), RecoveryOptions{DisableReload: true})
	plugin := env.installPlugin(t, 1013, "fine")

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")

	row := env.row(t, plugin.ID)
	assert.Equal(t, domain.PluginStatusError, row.Status)
	require.NotNil(t, row.LastError)
	assert.Equal(t, "http handler timed out", *row.LastError)
	assert.Len(t, env.audit.ofType(audit.EventPluginDisabled), 1)
	assert.Empty(t, env.timers.delays(), "no reload is scheduled")
}

func TestSupervisor_Stop_during_attempt_records_nothing(t *testing.T) {
	t.Parallel()
	manager := failingManager()
	env := newRecoveryEnv(t, manager, RecoveryOptions{})
	plugin := env.installPlugin(t, 1014, "fine")

	// The reload observes the supervisor's context: simulate Stop racing
	// the attempt by cancelling it from inside the load.
	manager.loadFunc = func(ctx context.Context, _ []byte, _ map[string]string, _ uint64) (*pkgplugin.LoadedPlugin, error) {
		env.recovery.cancel()

		return nil, ctx.Err()
	}

	env.recovery.OnPluginDisabled("instance", uint64(plugin.ID), "http handler timed out")
	env.timers.fire(t)

	assert.Empty(t, env.audit.ofType(audit.EventPluginReloaded), "a reload cut short by shutdown is not a failure")
	assert.Len(t, env.timers.delays(), 1, "no further attempt is scheduled")

	row := env.row(t, plugin.ID)
	require.NotNil(t, row.LastError)
	assert.Equal(t, "http handler timed out", *row.LastError)
}
