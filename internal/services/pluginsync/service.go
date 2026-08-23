// Package pluginsync keeps every panel instance's loaded plugins in step with
// the plugins table.
//
// The design is state based, not event based. A pub-sub message carries no
// state: it only wakes the reconciler, which then re-reads the database and
// makes its own decisions. That is what makes the feature survive the
// transport it runs on — the pub-sub drivers deliver at most once with no
// persistence and no replay, so a message may be lost, duplicated or reordered
// without changing the outcome. The worst a lost message costs is one refresh
// interval of staleness.
//
// The database is the desired state and is never written from here. A load
// failure is local to one instance (bad disk, unreachable object store, a
// half-written file) while plugins.status is global, so an instance that wrote
// its own failure back would disable a working plugin everywhere. Failures stay
// in memory, in the log, and on the admin API.
//
// Nothing in a host library may call into this package: guest Initialize and
// Shutdown run under the manager's lock, and the reconciler takes the loader's
// lifecycle lock before the manager's — a host library reaching back here would
// invert that order.
package pluginsync

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/locker"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/pkg/errors"
)

const (
	defaultRefreshInterval    = 60 * time.Second
	defaultMinBackoff         = 15 * time.Second
	defaultMaxBackoff         = 15 * time.Minute
	defaultContentionBackoff  = 10 * time.Second
	defaultUnloadTimeout      = 15 * time.Second
	defaultRefreshSubsTimeout = 30 * time.Second
	defaultDownloadTimeout    = 5 * time.Minute
	defaultDownloadLockTTL    = 5 * time.Minute

	// tickJitterFraction spreads periodic passes so a rolling deploy does not
	// leave every replica reconciling — and re-downloading — in lockstep.
	tickJitterFraction = 10
)

// Pass results reported to the PassObserver.
const (
	PassResultOK     = "ok"
	PassResultFailed = "failed"
)

type Options struct {
	// RefreshInterval bounds how long a missed hint can leave this instance
	// stale.
	RefreshInterval time.Duration
	// MinBackoff and MaxBackoff bound the per-plugin retry delay after a failed
	// apply of an active row.
	MinBackoff time.Duration
	MaxBackoff time.Duration
	// ContentionBackoff is the short, flat retry used when another instance
	// holds the download lock or a local handler holds the plugin.
	// Contention is not failure.
	ContentionBackoff time.Duration
	// UnloadTimeout bounds the guest shutdown of a plugin being removed.
	UnloadTimeout time.Duration
	// RefreshSubsTimeout bounds the subscription rebuild, which calls every
	// enabled guest while holding the dispatcher's write lock.
	RefreshSubsTimeout time.Duration
	// DownloadTimeout and DownloadLockTTL bound recovery of a missing file.
	DownloadTimeout time.Duration
	DownloadLockTTL time.Duration

	Clock Clock
}

func (o *Options) applyDefaults() {
	if o.RefreshInterval <= 0 {
		o.RefreshInterval = defaultRefreshInterval
	}
	if o.MinBackoff <= 0 {
		o.MinBackoff = defaultMinBackoff
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = defaultMaxBackoff
	}
	if o.MaxBackoff < o.MinBackoff {
		o.MaxBackoff = o.MinBackoff
	}
	if o.ContentionBackoff <= 0 {
		o.ContentionBackoff = defaultContentionBackoff
	}
	if o.UnloadTimeout <= 0 {
		o.UnloadTimeout = defaultUnloadTimeout
	}
	if o.RefreshSubsTimeout <= 0 {
		o.RefreshSubsTimeout = defaultRefreshSubsTimeout
	}
	if o.DownloadTimeout <= 0 {
		o.DownloadTimeout = defaultDownloadTimeout
	}
	if o.DownloadLockTTL <= 0 {
		o.DownloadLockTTL = defaultDownloadLockTTL
	}
	if o.Clock == nil {
		o.Clock = systemClock{}
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

type Service struct {
	repo    Repository
	loader  Loader
	plugins PluginProvider
	subs    SubscriptionRefresher
	archive ArchiveEvents
	files   FileStore
	store   StoreDownloader
	locks   locker.Locker
	bus     pubsub.PubSub
	audit   audit.Logger
	metrics PassObserver

	pluginsDir string
	opts       Options
	logger     *slog.Logger
	clock      Clock
	jitter     func(maxJitter time.Duration) time.Duration

	// runMu keeps two passes of this instance from overlapping. The race
	// against the admin HTTP handlers is handled a layer down, by the loader's
	// per-plugin lifecycle locks, which both go through.
	runMu sync.Mutex

	mu    sync.Mutex
	state map[domain.Uint64ID]*pluginState
	// seen holds the plugin IDs present in the last successful database read.
	// A loaded module whose ID was never seen is left alone: it may belong to
	// something else, and a failed read must never be mistaken for a deletion.
	seen map[domain.Uint64ID]struct{}
	wake chan struct{}

	cancel context.CancelFunc
	loopWG sync.WaitGroup
}

type Deps struct {
	Repo    Repository
	Loader  Loader
	Plugins PluginProvider
	Subs    SubscriptionRefresher
	Archive ArchiveEvents
	Files   FileStore
	// Store may be nil; a missing plugin file then simply cannot be recovered.
	Store StoreDownloader
	Locks locker.Locker
	Bus   pubsub.PubSub
	// Audit may be nil; reloads are then only logged.
	Audit audit.Logger
	// Metrics may be nil.
	Metrics PassObserver

	PluginsDir string
}

func New(deps Deps, opts Options, logger *slog.Logger) *Service {
	opts.applyDefaults()

	if logger == nil {
		logger = slog.Default()
	}

	if deps.Audit == nil {
		deps.Audit = audit.NopLogger{}
	}

	return &Service{
		repo:       deps.Repo,
		loader:     deps.Loader,
		plugins:    deps.Plugins,
		subs:       deps.Subs,
		archive:    deps.Archive,
		files:      deps.Files,
		store:      deps.Store,
		locks:      deps.Locks,
		bus:        deps.Bus,
		audit:      deps.Audit,
		metrics:    deps.Metrics,
		pluginsDir: deps.PluginsDir,
		opts:       opts,
		logger:     logger,
		clock:      opts.Clock,
		jitter:     defaultJitter,
		state:      make(map[domain.Uint64ID]*pluginState),
		seen:       make(map[domain.Uint64ID]struct{}),
		wake:       make(chan struct{}, 1),
	}
}

func defaultJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}

	return time.Duration(rand.Int64N(int64(maxJitter) + 1)) //nolint:gosec // G404: jitter is not security-sensitive
}

// Subscribe registers the hint handler. Call it before the plugins are loaded
// so a peer's change during the load window is not missed, and before the
// pub-sub transport starts — every driver accepts subscriptions made ahead of
// Start and replays them when it connects.
func (s *Service) Subscribe(ctx context.Context) error {
	if s == nil || s.bus == nil {
		return nil
	}

	// The message's Source is not checked. Handling this instance's own hint is
	// harmless because a pass is idempotent, and filtering on it would break
	// outright when PUBSUB_INSTANCE_ID is left unset and every replica shares
	// the same identifier.
	err := s.bus.Subscribe(ctx, channels.PluginSync, func(_ context.Context, msg *pubsub.Message) error {
		payload, err := messages.ParsePayload[messages.PluginSyncPayload](msg)
		if err != nil {
			// A malformed payload must not tear the subscription down; the
			// periodic pass covers whatever it was about.
			s.logger.Warn("failed to parse plugin sync hint", slog.String("error", err.Error()))

			return nil
		}

		s.logger.Debug("plugin sync hint received",
			slog.Uint64("plugin_id", payload.PluginID),
			slog.String("action", payload.Action))

		s.Kick()

		return nil
	})

	return errors.WithMessage(err, "failed to subscribe to plugin sync hints")
}

// Start runs a first pass and then keeps reconciling in the background.
//
// The first pass is synchronous on purpose: it records the state of every
// plugin the loader has just brought up, repairs the files the startup load
// could not find, and leaves a clean baseline for the hints that follow.
func (s *Service) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	if err := s.ReconcileNow(runCtx); err != nil {
		// Not fatal: the loop retries, and refusing to start over a database
		// hiccup would be worse than starting stale.
		s.logger.Error("initial plugin reconcile did not complete",
			slog.String("error", err.Error()))
	}

	s.loopWG.Go(func() {
		s.run(runCtx)
	})

	s.logger.Info("plugin sync started",
		slog.Duration("refresh_interval", s.opts.RefreshInterval),
		slog.Duration("min_backoff", s.opts.MinBackoff),
		slog.Duration("max_backoff", s.opts.MaxBackoff))

	return nil
}

// Stop halts the loop and waits for an in-flight pass to finish. Every guest
// call the pass can make carries a deadline, so the wait is bounded.
func (s *Service) Stop() {
	if s == nil {
		return
	}

	if s.cancel != nil {
		s.cancel()
	}

	s.loopWG.Wait()
}

// Kick asks for a pass as soon as one can run. Repeated calls collapse into a
// single pass, which is why duplicate hints cost nothing.
//
// Like Notify and Snapshot, it tolerates a nil receiver: the container hands
// out a nil *Service when plugin sync is switched off, and that value still
// reaches the handlers through an interface, where a plain nil check would
// not catch it.
func (s *Service) Kick() {
	if s == nil {
		return
	}

	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Notify publishes a hint that a plugin changed (plugininstall.SyncNotifier).
// It is best effort: the hint only shortens the delay before peers notice, so
// a publish failure degrades latency, never correctness.
func (s *Service) Notify(ctx context.Context, pluginID domain.Uint64ID, action string) {
	if s == nil || s.bus == nil {
		return
	}

	msg, err := messages.NewMessage(channels.PluginSync, messages.TypePluginSync, messages.PluginSyncPayload{
		PluginID: uint64(pluginID),
		Action:   action,
	})
	if err != nil {
		s.logger.Warn("failed to build plugin sync hint", slog.String("error", err.Error()))

		return
	}

	if err := s.bus.Publish(ctx, channels.PluginSync, msg); err != nil {
		s.logger.Warn("failed to publish plugin sync hint",
			slog.Uint64("plugin_id", uint64(pluginID)),
			slog.String("action", action),
			slog.String("error", err.Error()))
	}
}

// Snapshot reports what this instance applied, keyed by plugin database ID.
func (s *Service) Snapshot() map[domain.Uint64ID]Status {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[domain.Uint64ID]Status, len(s.state))
	for id, st := range s.state {
		result[id] = st.status(id)
	}

	return result
}

// Pending counts the plugins this instance could not bring in line with the
// database (retrying or failed).
func (s *Service) Pending() int {
	if s == nil {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	pending := 0
	for _, st := range s.state {
		if st.lastErr != "" {
			pending++
		}
	}

	return pending
}

func (s *Service) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
			s.safeReconcile(ctx)
		case <-s.clock.After(s.nextDelay()):
			s.safeReconcile(ctx)
		}
	}
}

// nextDelay is the refresh interval, shortened when a plugin is due for a retry
// sooner, and jittered so replicas do not line up.
func (s *Service) nextDelay() time.Duration {
	delay := s.opts.RefreshInterval

	s.mu.Lock()
	now := s.clock.Now()
	for _, st := range s.state {
		if !st.scheduled || st.nextAttempt.IsZero() {
			continue
		}
		if d := st.nextAttempt.Sub(now); d < delay {
			delay = max(d, 0)
		}
	}
	s.mu.Unlock()

	return delay + s.jitter(s.opts.RefreshInterval/tickJitterFraction)
}

func (s *Service) safeReconcile(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			s.logger.Error("plugin reconcile panicked", slog.Any("panic", rec))
		}
	}()

	if err := s.ReconcileNow(ctx); err != nil {
		s.logger.Warn("plugin reconcile did not complete",
			slog.String("error", err.Error()))
	}
}
