package pluginscheduler

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/locker"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

type fakeClock struct {
	mu             sync.Mutex
	now            time.Time
	afterDurations []time.Duration
	// afterCh is returned from After when set; otherwise After returns a
	// closed channel so waits complete immediately.
	afterCh chan time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.afterDurations = append(c.afterDurations, d)

	if c.afterCh != nil {
		return c.afterCh
	}

	ch := make(chan time.Time)
	close(ch)

	return ch
}

func (c *fakeClock) recordedAfter() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]time.Duration, len(c.afterDurations))
	copy(out, c.afterDurations)

	return out
}

type acquireCall struct {
	key string
	ttl time.Duration
}

type fakeLock struct {
	mu           sync.Mutex
	refreshErr   error
	refreshCalls []time.Duration
	released     bool
}

func (l *fakeLock) Release(_ context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.released = true

	return nil
}

func (l *fakeLock) Refresh(_ context.Context, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refreshCalls = append(l.refreshCalls, ttl)

	return l.refreshErr
}

type fakeLocker struct {
	mu       sync.Mutex
	acquired []acquireCall
	// deny returns a non-nil error for keys that must not be granted.
	deny func(key string) error
	// refreshErr is copied onto every created lock.
	refreshErr error
	locks      map[string]*fakeLock
}

func newFakeLocker() *fakeLocker {
	return &fakeLocker{locks: make(map[string]*fakeLock)}
}

func (f *fakeLocker) Acquire(_ context.Context, key string, ttl time.Duration) (locker.Lock, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.acquired = append(f.acquired, acquireCall{key: key, ttl: ttl})

	if f.deny != nil {
		if err := f.deny(key); err != nil {
			return nil, err
		}
	}

	lock := &fakeLock{refreshErr: f.refreshErr}
	f.locks[key] = lock

	return lock, nil
}

func (f *fakeLocker) acquiredKeys() []acquireCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]acquireCall, len(f.acquired))
	copy(out, f.acquired)

	return out
}

type fakeProvider struct {
	mu      sync.Mutex
	plugins map[string]*pkgplugin.LoadedPlugin
}

func (p *fakeProvider) GetPlugin(pluginID string) (*pkgplugin.LoadedPlugin, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	lp, ok := p.plugins[pluginID]

	return lp, ok
}

type fakeResolver struct {
	ids map[domain.Uint64ID]string
}

func (r *fakeResolver) GetPluginManagerID(dbID domain.Uint64ID) (string, bool) {
	id, ok := r.ids[dbID]

	return id, ok
}

// fakePluginInstance implements proto.PluginService plus the scheduled task
// handler capability, the way the real wrapper does.
type fakePluginInstance struct {
	mu         sync.Mutex
	requests   []*scheduler.HandleScheduledTaskRequest
	handleFunc func(ctx context.Context, req *scheduler.HandleScheduledTaskRequest) error
	hasHandler bool
}

func (f *fakePluginInstance) HandleScheduledTask(
	ctx context.Context,
	req *scheduler.HandleScheduledTaskRequest,
) (*scheduler.HandleScheduledTaskResponse, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	handle := f.handleFunc
	f.mu.Unlock()

	if handle != nil {
		if err := handle(ctx, req); err != nil {
			return nil, err
		}
	}

	return &scheduler.HandleScheduledTaskResponse{}, nil
}

func (f *fakePluginInstance) HasScheduledTaskHandler() bool { return f.hasHandler }

func (f *fakePluginInstance) recordedRequests() []*scheduler.HandleScheduledTaskRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]*scheduler.HandleScheduledTaskRequest, len(f.requests))
	copy(out, f.requests)

	return out
}

func (f *fakePluginInstance) GetInfo(context.Context, *proto.GetInfoRequest) (*proto.PluginInfo, error) {
	return &proto.PluginInfo{Id: "fake"}, nil
}

func (f *fakePluginInstance) Initialize(
	context.Context, *proto.InitializeRequest,
) (*proto.InitializeResponse, error) {
	return &proto.InitializeResponse{}, nil
}

func (f *fakePluginInstance) Shutdown(context.Context, *proto.ShutdownRequest) (*proto.ShutdownResponse, error) {
	return &proto.ShutdownResponse{}, nil
}

func (f *fakePluginInstance) HandleEvent(context.Context, *proto.Event) (*proto.EventResult, error) {
	return &proto.EventResult{}, nil
}

func (f *fakePluginInstance) GetSubscribedEvents(
	context.Context, *proto.GetSubscribedEventsRequest,
) (*proto.GetSubscribedEventsResponse, error) {
	return &proto.GetSubscribedEventsResponse{}, nil
}

func (f *fakePluginInstance) GetHTTPRoutes(
	context.Context, *proto.GetHTTPRoutesRequest,
) (*proto.GetHTTPRoutesResponse, error) {
	return &proto.GetHTTPRoutesResponse{}, nil
}

func (f *fakePluginInstance) HandleHTTPRequest(
	context.Context, *proto.HTTPRequest,
) (*proto.HTTPResponse, error) {
	return &proto.HTTPResponse{}, nil
}

func (f *fakePluginInstance) GetFrontendBundle(
	context.Context, *proto.GetFrontendBundleRequest,
) (*proto.GetFrontendBundleResponse, error) {
	return &proto.GetFrontendBundleResponse{}, nil
}

func (f *fakePluginInstance) GetServerAbilities(
	context.Context, *proto.GetServerAbilitiesRequest,
) (*proto.GetServerAbilitiesResponse, error) {
	return &proto.GetServerAbilitiesResponse{}, nil
}

func (f *fakePluginInstance) GetAssets(
	context.Context, *proto.GetAssetsRequest,
) (*proto.GetAssetsResponse, error) {
	return &proto.GetAssetsResponse{}, nil
}

type testEnv struct {
	service  *Service
	repo     *inmemory.PluginScheduledTaskRepository
	clock    *fakeClock
	locks    *fakeLocker
	provider *fakeProvider
	resolver *fakeResolver
	instance *fakePluginInstance
	plugin   *pkgplugin.LoadedPlugin
}

const testManagerID = "mgr-1"

func newTestEnv(t *testing.T, mutate func(opts *Options)) *testEnv {
	t.Helper()

	clock := &fakeClock{now: time.UnixMilli(10_000_500)}
	repo := inmemory.NewPluginScheduledTaskRepository()
	locks := newFakeLocker()
	instance := &fakePluginInstance{hasHandler: true}
	plugin := &pkgplugin.LoadedPlugin{
		Info:     &proto.PluginInfo{Id: testManagerID},
		Enabled:  true,
		Instance: instance,
	}
	provider := &fakeProvider{plugins: map[string]*pkgplugin.LoadedPlugin{testManagerID: plugin}}
	resolver := &fakeResolver{ids: map[domain.Uint64ID]string{1: testManagerID}}

	opts := Options{Clock: clock}
	if mutate != nil {
		mutate(&opts)
	}

	service := New(repo, provider, resolver, locks, opts, slog.New(slog.DiscardHandler))

	return &testEnv{
		service:  service,
		repo:     repo,
		clock:    clock,
		locks:    locks,
		provider: provider,
		resolver: resolver,
		instance: instance,
		plugin:   plugin,
	}
}

func testTask(interval time.Duration) domain.PluginScheduledTask {
	return domain.PluginScheduledTask{
		PluginID:    1,
		Name:        "stats-report",
		Interval:    interval,
		ErrorPolicy: domain.PluginScheduledTaskErrorPolicyIgnore,
	}
}

func (e *testEnv) runTaskSync(ctx context.Context, task domain.PluginScheduledTask, slot time.Time) {
	key := taskKey{pluginID: task.PluginID, name: task.Name}

	e.service.mu.Lock()
	e.service.tasks[key] = &taskState{task: task, running: true}
	e.service.mu.Unlock()

	e.service.runWG.Add(1)
	e.service.runTask(ctx, key, task, slot)
}

func TestNextSlot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		nowMS    int64
		interval time.Duration
		wantMS   int64
	}{
		{name: "aligns_to_epoch_grid", nowMS: 10_500, interval: time.Second, wantMS: 11_000},
		{name: "boundary_moves_to_next_slot", nowMS: 10_000, interval: time.Second, wantMS: 11_000},
		{name: "sub_second_interval", nowMS: 10_120, interval: 250 * time.Millisecond, wantMS: 10_250},
		{name: "large_interval", nowMS: 3_700_000, interval: time.Hour, wantMS: 7_200_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := nextSlot(time.UnixMilli(tt.nowMS), tt.interval)

			assert.Equal(t, time.UnixMilli(tt.wantMS), got)
		})
	}
}

func TestNextSlot_InvalidInterval(t *testing.T) {
	t.Parallel()

	assert.True(t, nextSlot(time.UnixMilli(10_000), 0).IsZero())
	assert.True(t, nextSlot(time.UnixMilli(10_000), -time.Second).IsZero())
}

func TestFireDue_FiresAndAdvancesSlot(t *testing.T) {
	t.Parallel()

	// ARRANGE
	env := newTestEnv(t, nil)
	task := testTask(time.Second)
	key := taskKey{pluginID: task.PluginID, name: task.Name}
	slot := time.UnixMilli(10_000_000)

	env.service.mu.Lock()
	env.service.tasks[key] = &taskState{task: task, nextAt: slot}
	env.service.mu.Unlock()

	// ACT
	env.service.fireDue(context.Background(), env.clock.Now())
	env.service.runWG.Wait()

	// ASSERT
	requests := env.instance.recordedRequests()
	require.Len(t, requests, 1)
	assert.Equal(t, "stats-report", requests[0].TaskName)
	assert.Equal(t, slot.UnixMilli(), requests[0].ScheduledAt)
	assert.Equal(t, uint32(1), requests[0].Attempt)

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	assert.Equal(t, time.UnixMilli(10_001_000), env.service.tasks[key].nextAt)
	assert.False(t, env.service.tasks[key].running)
}

func TestFireDue_SkipsWhenRunningLocally(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	task := testTask(time.Second)
	key := taskKey{pluginID: task.PluginID, name: task.Name}

	env.service.mu.Lock()
	env.service.tasks[key] = &taskState{task: task, nextAt: time.UnixMilli(10_000_000), running: true}
	env.service.mu.Unlock()

	env.service.fireDue(context.Background(), env.clock.Now())
	env.service.runWG.Wait()

	require.Len(t, env.instance.recordedRequests(), 0)

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	assert.Equal(t, time.UnixMilli(10_001_000), env.service.tasks[key].nextAt,
		"a skipped slot must still advance the schedule")
}

func TestFireDue_NotDueTaskUntouched(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	task := testTask(time.Second)
	key := taskKey{pluginID: task.PluginID, name: task.Name}
	future := env.clock.Now().Add(time.Minute)

	env.service.mu.Lock()
	env.service.tasks[key] = &taskState{task: task, nextAt: future}
	env.service.mu.Unlock()

	env.service.fireDue(context.Background(), env.clock.Now())
	env.service.runWG.Wait()

	require.Len(t, env.instance.recordedRequests(), 0)

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	assert.Equal(t, future, env.service.tasks[key].nextAt)
}

func TestRunTask_SkipsBeforeLocksWhenPluginNotLoaded(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	env.provider.plugins = map[string]*pkgplugin.LoadedPlugin{}

	env.runTaskSync(context.Background(), testTask(time.Second), time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 0)
	assert.Len(t, env.locks.acquiredKeys(), 0,
		"an instance without the plugin must leave the slot lock to other instances")
}

func TestRunTask_SkipsWhenPluginDisabled(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	env.plugin.Disable()

	env.runTaskSync(context.Background(), testTask(time.Second), time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 0)
	assert.Len(t, env.locks.acquiredKeys(), 0)
}

func TestRunTask_SkipsWhenHandlerNotExported(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	env.instance.hasHandler = false

	env.runTaskSync(context.Background(), testTask(time.Second), time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 0)
	assert.Len(t, env.locks.acquiredKeys(), 0)
}

func TestRunTask_SlotLockDeniedSkipsInvocation(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	env.locks.deny = func(key string) error {
		if strings.HasPrefix(key, "pluginscheduler:slot:") {
			return locker.ErrLocked
		}

		return nil
	}

	env.runTaskSync(context.Background(), testTask(time.Second), time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 0)

	acquired := env.locks.acquiredKeys()
	require.Len(t, acquired, 1)
	assert.Equal(t, "pluginscheduler:slot:1:stats-report:10000000", acquired[0].key)
}

func TestRunTask_RunLockDeniedSkipsInvocation(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	env.locks.deny = func(key string) error {
		if strings.HasPrefix(key, "pluginscheduler:run:") {
			return locker.ErrLocked
		}

		return nil
	}

	env.runTaskSync(context.Background(), testTask(time.Second), time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 0)

	acquired := env.locks.acquiredKeys()
	require.Len(t, acquired, 2)
	assert.Equal(t, "pluginscheduler:run:1:stats-report", acquired[1].key)
}

func TestRunTask_SuccessTakesBothLocksAndReleasesRunLock(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)

	env.runTaskSync(context.Background(), testTask(time.Second), time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 1)

	acquired := env.locks.acquiredKeys()
	require.Len(t, acquired, 2)
	assert.Equal(t, "pluginscheduler:slot:1:stats-report:10000000", acquired[0].key)
	assert.Equal(t, time.Second*5, acquired[0].ttl, "slot lock TTL is clamped to the minimum")
	assert.Equal(t, "pluginscheduler:run:1:stats-report", acquired[1].key)
	assert.Equal(t, defaultCallTimeout+runLockSlack, acquired[1].ttl)

	runLock := env.locks.locks["pluginscheduler:run:1:stats-report"]
	require.NotNil(t, runLock)
	assert.True(t, runLock.released)

	slotLock := env.locks.locks["pluginscheduler:slot:1:stats-report:10000000"]
	require.NotNil(t, slotLock)
	assert.False(t, slotLock.released, "slot locks expire by TTL and are never released")
}

func TestRunTask_RetryPolicyRetriesWithJitter(t *testing.T) {
	t.Parallel()

	// ARRANGE
	env := newTestEnv(t, nil)
	env.service.jitter = func(time.Duration) time.Duration { return 7 * time.Millisecond }
	env.instance.handleFunc = func(context.Context, *scheduler.HandleScheduledTaskRequest) error {
		return errors.New("handler exploded")
	}

	task := testTask(time.Second)
	task.ErrorPolicy = domain.PluginScheduledTaskErrorPolicyRetry
	task.MaxRetries = 2
	task.RetryDelay = 100 * time.Millisecond
	task.MaxJitter = 50 * time.Millisecond
	task.Timeout = 10 * time.Second

	// ACT
	env.runTaskSync(context.Background(), task, time.UnixMilli(10_000_000))

	// ASSERT: 1 initial attempt + 2 retries, 1-based attempt numbers
	requests := env.instance.recordedRequests()
	require.Len(t, requests, 3)
	assert.Equal(t, uint32(1), requests[0].Attempt)
	assert.Equal(t, uint32(2), requests[1].Attempt)
	assert.Equal(t, uint32(3), requests[2].Attempt)

	waits := env.clock.recordedAfter()
	require.Len(t, waits, 2)
	assert.Equal(t, 107*time.Millisecond, waits[0], "retry delay plus fixed jitter")
	assert.Equal(t, 107*time.Millisecond, waits[1])

	runLock := env.locks.locks["pluginscheduler:run:1:stats-report"]
	require.NotNil(t, runLock)
	require.Len(t, runLock.refreshCalls, 2)
	assert.Equal(t, 107*time.Millisecond+10*time.Second+runLockSlack, runLock.refreshCalls[0])
	assert.True(t, runLock.released)
}

func TestRunTask_IgnorePolicySingleAttempt(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	env.instance.handleFunc = func(context.Context, *scheduler.HandleScheduledTaskRequest) error {
		return errors.New("handler exploded")
	}

	env.runTaskSync(context.Background(), testTask(time.Second), time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 1)
	assert.Len(t, env.clock.recordedAfter(), 0)
}

func TestRunTask_RetryAbortsWhenRunLockLost(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	env.locks.refreshErr = locker.ErrLockLost
	env.instance.handleFunc = func(context.Context, *scheduler.HandleScheduledTaskRequest) error {
		return errors.New("handler exploded")
	}

	task := testTask(time.Second)
	task.ErrorPolicy = domain.PluginScheduledTaskErrorPolicyRetry
	task.MaxRetries = 5

	env.runTaskSync(context.Background(), task, time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 1,
		"a lost run lock must abort retries to avoid concurrent duplicates")

	runLock := env.locks.locks["pluginscheduler:run:1:stats-report"]
	require.NotNil(t, runLock)
	require.Len(t, runLock.refreshCalls, 1)
}

func TestRunTask_CancelledContextStopsBeforeInvocation(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	env.runTaskSync(ctx, testTask(time.Second), time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 0)
}

func TestRunTask_TimeoutDisablesPlugin(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	env.instance.handleFunc = func(ctx context.Context, _ *scheduler.HandleScheduledTaskRequest) error {
		<-ctx.Done()

		return ctx.Err()
	}

	task := testTask(time.Second)
	task.Timeout = 20 * time.Millisecond

	env.runTaskSync(context.Background(), task, time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 1)
	assert.False(t, env.plugin.IsEnabled(),
		"a handler eating the whole call budget must disable the plugin, mirroring the event dispatcher")
}

func TestRunTask_BusyErrorDoesNotDisablePlugin(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	env.instance.handleFunc = func(context.Context, *scheduler.HandleScheduledTaskRequest) error {
		return errors.Wrap(pkgplugin.ErrPluginBusy, "gate is taken")
	}

	env.runTaskSync(context.Background(), testTask(time.Second), time.UnixMilli(10_000_000))

	require.Len(t, env.instance.recordedRequests(), 1)
	assert.True(t, env.plugin.IsEnabled(), "busy means the guest was never entered; the module is healthy")
}

func TestRunTask_PanicRecoveredAndRunningCleared(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	env.instance.handleFunc = func(context.Context, *scheduler.HandleScheduledTaskRequest) error {
		panic("handler exploded")
	}

	task := testTask(time.Second)
	key := taskKey{pluginID: task.PluginID, name: task.Name}

	env.runTaskSync(context.Background(), task, time.UnixMilli(10_000_000))

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	assert.False(t, env.service.tasks[key].running)
}

func TestReload_HydratesFromRepository(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	task := testTask(time.Second)
	require.NoError(t, env.repo.Upsert(context.Background(), &task))

	require.NoError(t, env.service.reload(context.Background()))

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	st, ok := env.service.tasks[taskKey{pluginID: 1, name: "stats-report"}]
	require.True(t, ok)
	assert.Equal(t, time.UnixMilli(10_001_000), st.nextAt)
}

func TestReload_AddsChangesAndRemoves(t *testing.T) {
	t.Parallel()

	// ARRANGE: registry has a stale task and one whose interval changed
	env := newTestEnv(t, nil)
	now := env.clock.Now()

	kept := testTask(time.Second)
	require.NoError(t, env.repo.Upsert(context.Background(), &kept))

	changed := testTask(time.Second)
	changed.Name = "changed"
	changed.Interval = 2 * time.Second
	require.NoError(t, env.repo.Upsert(context.Background(), &changed))

	env.service.mu.Lock()
	keptAt := nextSlot(now, time.Second)
	env.service.tasks[taskKey{pluginID: 1, name: "stats-report"}] = &taskState{task: kept, nextAt: keptAt}
	env.service.tasks[taskKey{pluginID: 1, name: "changed"}] = &taskState{
		task:   testTask(time.Second),
		nextAt: keptAt,
	}
	env.service.tasks[taskKey{pluginID: 9, name: "removed"}] = &taskState{task: testTask(time.Second)}
	env.service.mu.Unlock()

	// ACT
	require.NoError(t, env.service.reload(context.Background()))

	// ASSERT
	env.service.mu.Lock()
	defer env.service.mu.Unlock()

	require.Len(t, env.service.tasks, 2)
	assert.Equal(t, keptAt, env.service.tasks[taskKey{pluginID: 1, name: "stats-report"}].nextAt,
		"unchanged interval keeps the armed slot")
	assert.Equal(t, nextSlot(now, 2*time.Second), env.service.tasks[taskKey{pluginID: 1, name: "changed"}].nextAt,
		"changed interval recomputes the slot")
	_, removed := env.service.tasks[taskKey{pluginID: 9, name: "removed"}]
	assert.False(t, removed)
}

func TestReload_SkipsInvalidInterval(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	task := testTask(0)
	require.NoError(t, env.repo.Upsert(context.Background(), &task))

	require.NoError(t, env.service.reload(context.Background()))

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	assert.Len(t, env.service.tasks, 0)
}

type erroringRepo struct {
	Repository

	findAllErr error
}

func (r *erroringRepo) FindAll(ctx context.Context) ([]domain.PluginScheduledTask, error) {
	if r.findAllErr != nil {
		return nil, r.findAllErr
	}

	return r.Repository.FindAll(ctx)
}

func TestSafeRefresh_KeepsRegistryOnRepositoryError(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)
	task := testTask(time.Second)
	key := taskKey{pluginID: task.PluginID, name: task.Name}

	env.service.mu.Lock()
	env.service.tasks[key] = &taskState{task: task, nextAt: time.UnixMilli(10_001_000)}
	env.service.mu.Unlock()

	env.service.repo = &erroringRepo{Repository: env.repo, findAllErr: errors.New("db is down")}

	env.service.safeRefresh(context.Background())

	env.service.mu.Lock()
	defer env.service.mu.Unlock()
	require.Len(t, env.service.tasks, 1, "a failed refresh must not wipe the registry")
}

func TestDurationToNext(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, nil)

	_, ok := env.service.durationToNext()
	assert.False(t, ok, "no armed tasks means nothing to wait for")

	env.service.mu.Lock()
	env.service.tasks[taskKey{pluginID: 1, name: "a"}] = &taskState{nextAt: env.clock.Now().Add(3 * time.Second)}
	env.service.tasks[taskKey{pluginID: 1, name: "b"}] = &taskState{nextAt: env.clock.Now().Add(time.Second)}
	env.service.tasks[taskKey{pluginID: 1, name: "c"}] = &taskState{}
	env.service.mu.Unlock()

	d, ok := env.service.durationToNext()
	require.True(t, ok)
	assert.Equal(t, time.Second, d)
}

func TestServiceLoop_FiresRegisteredTask(t *testing.T) {
	t.Parallel()

	// Integration of the real loop with the system clock and tiny intervals.
	repo := inmemory.NewPluginScheduledTaskRepository()
	instance := &fakePluginInstance{hasHandler: true}
	plugin := &pkgplugin.LoadedPlugin{Info: &proto.PluginInfo{Id: testManagerID}, Enabled: true, Instance: instance}
	provider := &fakeProvider{plugins: map[string]*pkgplugin.LoadedPlugin{testManagerID: plugin}}
	resolver := &fakeResolver{ids: map[domain.Uint64ID]string{1: testManagerID}}

	service := New(repo, provider, resolver, locker.NewInMemoryLocker(), Options{
		MinInterval:     10 * time.Millisecond,
		RefreshInterval: time.Hour,
	}, slog.New(slog.DiscardHandler))

	require.NoError(t, service.Start(context.Background()))
	t.Cleanup(service.Stop)

	task := testTask(50 * time.Millisecond)
	require.NoError(t, service.AddTask(context.Background(), task))

	require.Eventually(t, func() bool {
		return len(instance.recordedRequests()) >= 1
	}, 5*time.Second, 10*time.Millisecond)

	service.Stop()

	requests := instance.recordedRequests()
	require.NotEmpty(t, requests)
	assert.Equal(t, "stats-report", requests[0].TaskName)
	assert.Zero(t, requests[0].ScheduledAt%(50*time.Millisecond).Milliseconds(),
		"slots must be aligned to epoch multiples of the interval")
}
