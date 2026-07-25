// Package pluginscheduler runs periodic tasks registered by plugins via the
// gameap-scheduler host module. Definitions are persisted in the database;
// every panel instance keeps an in-memory copy, fires on Unix-epoch-aligned
// slots and takes distributed locks so each slot runs on exactly one instance.
package pluginscheduler

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/locker"
	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
	"github.com/pkg/errors"

	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

const (
	defaultMinInterval       = time.Second
	defaultMaxTasksPerPlugin = 32
	defaultCallTimeout       = 60 * time.Second
	defaultMaxCallTimeout    = 5 * time.Minute
	defaultMaxRetries        = 10
	defaultMaxRetryDelay     = 10 * time.Minute
	defaultMaxJitter         = 30 * time.Second
	defaultRefreshInterval   = 30 * time.Second

	// Slot locks are never released: they expire on their own and deduplicate
	// a slot between instances, absorbing clock skew up to the TTL.
	slotLockMinTTL = 5 * time.Second
	slotLockMaxTTL = time.Hour

	// runLockSlack pads the run lock TTL beyond the handler call timeout so
	// the lock survives marshalling overhead around the guest call.
	runLockSlack = 30 * time.Second
)

type Options struct {
	// MinInterval is the floor for task intervals.
	MinInterval time.Duration
	// MaxTasksPerPlugin caps registrations per plugin.
	MaxTasksPerPlugin int
	// DefaultCallTimeout bounds one handler call when the task does not set
	// its own timeout.
	DefaultCallTimeout time.Duration
	// MaxCallTimeout is the ceiling for per-task timeout overrides.
	MaxCallTimeout time.Duration
	// MaxRetries, MaxRetryDelay and MaxJitter cap the per-task retry policy.
	MaxRetries    int
	MaxRetryDelay time.Duration
	MaxJitter     time.Duration
	// RefreshInterval is how often definitions are re-read from the database
	// to pick up registrations made by other panel instances.
	RefreshInterval time.Duration
	Clock           Clock
}

func (o *Options) applyDefaults() {
	if o.MinInterval <= 0 {
		o.MinInterval = defaultMinInterval
	}
	if o.MaxTasksPerPlugin <= 0 {
		o.MaxTasksPerPlugin = defaultMaxTasksPerPlugin
	}
	if o.DefaultCallTimeout <= 0 {
		o.DefaultCallTimeout = defaultCallTimeout
	}
	if o.MaxCallTimeout <= 0 {
		o.MaxCallTimeout = defaultMaxCallTimeout
	}
	if o.MaxRetries <= 0 {
		o.MaxRetries = defaultMaxRetries
	}
	if o.MaxRetryDelay <= 0 {
		o.MaxRetryDelay = defaultMaxRetryDelay
	}
	if o.MaxJitter <= 0 {
		o.MaxJitter = defaultMaxJitter
	}
	if o.RefreshInterval <= 0 {
		o.RefreshInterval = defaultRefreshInterval
	}
	if o.Clock == nil {
		o.Clock = systemClock{}
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

type taskKey struct {
	pluginID domain.Uint64ID
	name     string
}

type taskState struct {
	task domain.PluginScheduledTask
	// nextAt is the next epoch-aligned slot; zero disarms the task.
	nextAt time.Time
	// running is the cheap local overlap guard; the run lock is the
	// cross-instance one.
	running bool
}

type Service struct {
	repo     Repository
	plugins  PluginProvider
	resolver ManagerIDResolver
	locks    locker.Locker
	opts     Options
	logger   *slog.Logger
	clock    Clock
	jitter   func(maxJitter time.Duration) time.Duration

	mu    sync.Mutex
	tasks map[taskKey]*taskState
	wake  chan struct{}

	cancel context.CancelFunc
	loopWG sync.WaitGroup
	runWG  sync.WaitGroup
}

func New(
	repo Repository,
	plugins PluginProvider,
	resolver ManagerIDResolver,
	locks locker.Locker,
	opts Options,
	logger *slog.Logger,
) *Service {
	opts.applyDefaults()

	if logger == nil {
		logger = slog.Default()
	}

	return &Service{
		repo:     repo,
		plugins:  plugins,
		resolver: resolver,
		locks:    locks,
		opts:     opts,
		logger:   logger,
		clock:    opts.Clock,
		jitter:   defaultJitter,
		tasks:    make(map[taskKey]*taskState),
		wake:     make(chan struct{}, 1),
	}
}

func defaultJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}

	return time.Duration(rand.Int64N(int64(maxJitter) + 1)) //nolint:gosec // G404: jitter is not security-sensitive
}

// nextSlot returns the first Unix-epoch multiple of interval strictly after
// now; every instance computes the same slot grid.
func nextSlot(now time.Time, interval time.Duration) time.Time {
	iv := interval.Milliseconds()
	if iv <= 0 {
		return time.Time{}
	}

	return time.UnixMilli((now.UnixMilli()/iv + 1) * iv)
}

func (s *Service) Start(ctx context.Context) error {
	if err := s.reload(ctx); err != nil {
		return errors.WithMessage(err, "failed to load scheduled tasks")
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.loopWG.Add(1)
	go s.run(runCtx)

	s.mu.Lock()
	tasks := len(s.tasks)
	s.mu.Unlock()

	s.logger.Info("plugin scheduler started",
		"tasks", tasks,
		"min_interval", s.opts.MinInterval,
		"refresh_interval", s.opts.RefreshInterval,
	)

	return nil
}

// Stop halts the loop and joins in-flight runs. Runs are bounded: every guest
// call carries an explicit deadline, so the join cannot hang indefinitely.
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}

	s.loopWG.Wait()
	s.runWG.Wait()
}

func (s *Service) run(ctx context.Context) {
	defer s.loopWG.Done()

	refreshC := s.clock.After(s.opts.RefreshInterval)

	for {
		var fireC <-chan time.Time
		if d, ok := s.durationToNext(); ok {
			fireC = s.clock.After(d)
		}

		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-refreshC:
			s.safeRefresh(ctx)
			refreshC = s.clock.After(s.opts.RefreshInterval)
		case <-fireC:
			s.safeFire(ctx)
		}
	}
}

func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) durationToNext() (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var next time.Time
	for _, st := range s.tasks {
		if st.nextAt.IsZero() {
			continue
		}
		if next.IsZero() || st.nextAt.Before(next) {
			next = st.nextAt
		}
	}

	if next.IsZero() {
		return 0, false
	}

	return max(next.Sub(s.clock.Now()), 0), true
}

func (s *Service) safeFire(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("plugin scheduler fire panicked", "panic", rec)
		}
	}()

	s.fireDue(ctx, s.clock.Now())
}

func (s *Service) safeRefresh(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("plugin scheduler refresh panicked", "panic", rec)
		}
	}()

	if err := s.reload(ctx); err != nil {
		s.logger.Warn("failed to refresh scheduled tasks, keeping current registry",
			slog.String("error", err.Error()))
	}
}

// reload merges the database state into the in-memory registry: it adds tasks
// registered by other instances, applies definition changes and drops removed
// ones. The DB read runs outside the mutex so guest registrations never stall
// behind a query.
func (s *Service) reload(ctx context.Context) error {
	fresh, err := s.repo.FindAll(ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to fetch scheduled tasks")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	seen := make(map[taskKey]struct{}, len(fresh))

	for _, task := range fresh {
		if task.Interval <= 0 {
			s.logger.Error("stored scheduled task has invalid interval, task disarmed",
				slog.Uint64("plugin_id", uint64(task.PluginID)),
				slog.String("task", task.Name),
			)

			continue
		}

		key := taskKey{pluginID: task.PluginID, name: task.Name}
		seen[key] = struct{}{}

		st, ok := s.tasks[key]
		if !ok {
			s.tasks[key] = &taskState{task: task, nextAt: nextSlot(now, task.Interval)}

			continue
		}

		intervalChanged := st.task.Interval != task.Interval
		st.task = task
		if intervalChanged {
			st.nextAt = nextSlot(now, task.Interval)
		}
	}

	for key := range s.tasks {
		if _, ok := seen[key]; !ok {
			delete(s.tasks, key)
		}
	}

	return nil
}

// fireDue starts a run goroutine for every task whose slot has come. It only
// touches the local registry: plugin resolution and locking happen inside the
// run goroutine, because host functions may call AddTask while the plugin
// manager lock is held (guest Initialize/Shutdown) — touching the manager
// from under s.mu would invert lock order.
func (s *Service) fireDue(ctx context.Context, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, st := range s.tasks {
		if st.nextAt.IsZero() || st.nextAt.After(now) {
			continue
		}

		slot := st.nextAt
		st.nextAt = nextSlot(now, st.task.Interval)

		if st.running {
			s.logger.Warn("scheduled task is still running locally, skipping slot",
				slog.Uint64("plugin_id", uint64(key.pluginID)),
				slog.String("task", key.name),
				slog.Time("slot", slot),
			)

			continue
		}

		st.running = true
		s.runWG.Add(1)
		go s.runTask(ctx, key, st.task, slot)
	}
}

func (s *Service) clearRunning(key taskKey) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if st, ok := s.tasks[key]; ok {
		st.running = false
	}
}

func (s *Service) runTask(ctx context.Context, key taskKey, task domain.PluginScheduledTask, slot time.Time) {
	defer s.runWG.Done()
	defer s.clearRunning(key)
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("scheduled task run panicked",
				slog.Uint64("plugin_id", uint64(key.pluginID)),
				slog.String("task", key.name),
				slog.Any("panic", rec),
			)
		}
	}()

	lp, handler, skipReason := s.resolveRunnable(task.PluginID)
	if handler == nil {
		// The slot lock is deliberately not taken: an instance that does
		// have the plugin loaded should win this slot.
		s.logger.Debug("scheduled task not runnable on this instance, leaving slot to others",
			slog.Uint64("plugin_id", uint64(key.pluginID)),
			slog.String("task", key.name),
			slog.String("reason", skipReason),
		)

		return
	}

	if !s.acquireSlot(ctx, key, task.Interval, slot) {
		return
	}

	runLock, ok := s.acquireRunLock(ctx, key, task.Timeout)
	if !ok {
		return
	}
	defer func() {
		if err := runLock.Release(context.WithoutCancel(ctx)); err != nil {
			s.logger.Warn("failed to release scheduled task run lock",
				slog.Uint64("plugin_id", uint64(key.pluginID)),
				slog.String("task", key.name),
				slog.String("error", err.Error()),
			)
		}
	}()

	s.runAttempts(ctx, key, task, slot, lp, handler, runLock)
}

// resolveRunnable performs the local checks; it must be called without s.mu
// held (GetPlugin takes the manager lock).
func (s *Service) resolveRunnable(
	pluginID domain.Uint64ID,
) (*pkgplugin.LoadedPlugin, scheduler.ScheduledTaskHandler, string) {
	managerID, ok := s.resolver.GetPluginManagerID(pluginID)
	if !ok {
		managerID = pkgplugin.CompactPluginID(pluginID)
	}

	lp, ok := s.plugins.GetPlugin(managerID)
	if !ok {
		return nil, nil, "plugin is not loaded"
	}

	if !lp.IsEnabled() {
		return nil, nil, "plugin is disabled"
	}

	handler, ok := lp.Instance.(scheduler.ScheduledTaskHandler)
	if !ok || !lp.HasScheduledTaskHandler() {
		return nil, nil, "plugin does not export a scheduled task handler"
	}

	return lp, handler, ""
}

func (s *Service) acquireSlot(ctx context.Context, key taskKey, interval time.Duration, slot time.Time) bool {
	slotKey := "pluginscheduler:slot:" + strconv.FormatUint(uint64(key.pluginID), 10) +
		":" + key.name + ":" + strconv.FormatInt(slot.UnixMilli(), 10)

	ttl := min(max(interval, slotLockMinTTL), slotLockMaxTTL)

	_, err := s.locks.Acquire(ctx, slotKey, ttl)
	if err == nil {
		return true
	}

	if errors.Is(err, locker.ErrLocked) {
		s.logger.Debug("scheduled task slot taken by another instance",
			slog.Uint64("plugin_id", uint64(key.pluginID)),
			slog.String("task", key.name),
			slog.Time("slot", slot),
		)

		return false
	}

	s.logger.Error("failed to acquire scheduled task slot lock, skipping slot",
		slog.Uint64("plugin_id", uint64(key.pluginID)),
		slog.String("task", key.name),
		slog.String("error", err.Error()),
	)

	return false
}

func (s *Service) acquireRunLock(ctx context.Context, key taskKey, taskTimeout time.Duration) (locker.Lock, bool) {
	runKey := "pluginscheduler:run:" + strconv.FormatUint(uint64(key.pluginID), 10) + ":" + key.name

	lock, err := s.locks.Acquire(ctx, runKey, s.callTimeout(taskTimeout)+runLockSlack)
	if err == nil {
		return lock, true
	}

	if errors.Is(err, locker.ErrLocked) {
		s.logger.Warn("previous scheduled task run is still in progress, skipping slot",
			slog.Uint64("plugin_id", uint64(key.pluginID)),
			slog.String("task", key.name),
		)

		return nil, false
	}

	s.logger.Error("failed to acquire scheduled task run lock, skipping slot",
		slog.Uint64("plugin_id", uint64(key.pluginID)),
		slog.String("task", key.name),
		slog.String("error", err.Error()),
	)

	return nil, false
}

func (s *Service) callTimeout(taskTimeout time.Duration) time.Duration {
	if taskTimeout > 0 {
		return taskTimeout
	}

	return s.opts.DefaultCallTimeout
}

func (s *Service) runAttempts(
	ctx context.Context,
	key taskKey,
	task domain.PluginScheduledTask,
	slot time.Time,
	lp *pkgplugin.LoadedPlugin,
	handler scheduler.ScheduledTaskHandler,
	runLock locker.Lock,
) {
	timeout := s.callTimeout(task.Timeout)

	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return
		}

		runErr := s.invoke(ctx, lp, handler, &task, slot, attempt, timeout)
		if runErr == nil {
			s.logger.Debug("scheduled task completed",
				slog.Uint64("plugin_id", uint64(key.pluginID)),
				slog.String("task", key.name),
				slog.Int("attempt", attempt),
			)

			return
		}

		s.logger.Error("scheduled task run failed",
			slog.Uint64("plugin_id", uint64(key.pluginID)),
			slog.String("task", key.name),
			slog.Time("slot", slot),
			slog.Int("attempt", attempt),
			slog.String("error", runErr.Error()),
		)

		//nolint:gosec // G115: MaxRetries is capped by validation far below MaxInt
		if task.ErrorPolicy != domain.PluginScheduledTaskErrorPolicyRetry || attempt > int(task.MaxRetries) {
			return
		}

		delay := task.RetryDelay + s.jitter(task.MaxJitter)

		if err := runLock.Refresh(context.WithoutCancel(ctx), delay+timeout+runLockSlack); err != nil {
			s.logger.Warn("scheduled task run lock lost, aborting retries",
				slog.Uint64("plugin_id", uint64(key.pluginID)),
				slog.String("task", key.name),
				slog.String("error", err.Error()),
			)

			return
		}

		select {
		case <-ctx.Done():
			return
		case <-s.clock.After(delay):
		}
	}
}

func (s *Service) invoke(
	ctx context.Context,
	lp *pkgplugin.LoadedPlugin,
	handler scheduler.ScheduledTaskHandler,
	task *domain.PluginScheduledTask,
	slot time.Time,
	attempt int,
	timeout time.Duration,
) error {
	if !lp.IsEnabled() {
		return errors.New("plugin was disabled")
	}

	// Detached from the run context with an explicit deadline: shutdown must
	// not close the module mid-call, mirroring the wrapper's own contract.
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()

	_, err := handler.HandleScheduledTask(callCtx, &scheduler.HandleScheduledTaskRequest{
		TaskName:    task.Name,
		ScheduledAt: slot.UnixMilli(),
		Attempt:     uint32(attempt), //nolint:gosec // attempt is small and positive
	})
	if err == nil {
		return nil
	}

	// A deadline hit while the run context is alive means the guest ate the
	// whole budget inside the call: the module may be mid-execution, so it is
	// disabled until reload. ErrPluginBusy is different — the guest was never
	// entered.
	if callCtx.Err() != nil && ctx.Err() == nil && !errors.Is(err, pkgplugin.ErrPluginBusy) {
		lp.Disable()
		s.logger.Error("scheduled task handler timed out, plugin disabled until reload",
			slog.Uint64("plugin_id", uint64(task.PluginID)),
			slog.String("task", task.Name),
		)
	}

	return err
}
