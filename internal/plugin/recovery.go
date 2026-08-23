package plugin

import (
	"context"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

const (
	defaultRecoveryInitialDelay = 30 * time.Second
	defaultRecoveryMaxDelay     = 10 * time.Minute
	defaultRecoveryMaxAttempts  = 5
	defaultRecoveryFactor       = 2.0

	// recoveryLookupTimeout bounds the database work done when a plugin is
	// reported disabled.
	recoveryLookupTimeout = 15 * time.Second

	auditReasonGuestTimeout      = "guest_call_timeout"
	auditReasonGuestExited       = "guest_exited"
	auditReasonRecoveryExhausted = "recovery_exhausted"
	auditReasonLoadFailed        = "load_failed"
)

// RecoveryOptions tunes the automatic reload of plugins disabled at runtime.
type RecoveryOptions struct {
	// InitialDelay is the wait before the first reload; every further
	// attempt waits Factor times longer, capped at MaxDelay.
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Factor       float64
	// MaxAttempts bounds consecutive reloads; after that the plugin stays in
	// status "error" until an operator reloads it or the panel restarts.
	// The count starts over once a reloaded plugin stayed healthy for
	// longer than MaxDelay.
	MaxAttempts int
	// DisableReload keeps the bookkeeping (status, reason, audit) but never
	// reloads: a disabled plugin stays off until an operator acts.
	DisableReload bool

	newTimer func(delay time.Duration, fn func()) recoveryTimer
	now      func() time.Time
}

func (o RecoveryOptions) withDefaults() RecoveryOptions {
	if o.InitialDelay <= 0 {
		o.InitialDelay = defaultRecoveryInitialDelay
	}

	if o.MaxDelay <= 0 {
		o.MaxDelay = defaultRecoveryMaxDelay
	}

	if o.MaxDelay < o.InitialDelay {
		o.MaxDelay = o.InitialDelay
	}

	if o.Factor < 1 {
		o.Factor = defaultRecoveryFactor
	}

	if o.MaxAttempts <= 0 {
		o.MaxAttempts = defaultRecoveryMaxAttempts
	}

	if o.newTimer == nil {
		o.newTimer = func(delay time.Duration, fn func()) recoveryTimer {
			return time.AfterFunc(delay, fn)
		}
	}

	if o.now == nil {
		o.now = time.Now
	}

	return o
}

type recoveryTimer interface {
	Stop() bool
}

// Supervisor brings plugins back after the runtime disabled them: it records
// the reason on the plugin row, writes the audit trail and reloads the plugin
// with exponential backoff. It is the target of pkgplugin.DisableHook.
type Supervisor struct {
	loader *Loader
	repo   repositories.PluginRepository
	audit  audit.Logger
	logger *slog.Logger
	opts   RecoveryOptions

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	entries map[domain.Uint64ID]*recoveryEntry
	stopped bool
}

type recoveryEntry struct {
	pluginID string
	attempts int
	// timer is non-nil while a reload is pending.
	timer recoveryTimer
	// healthySince is the last successful automatic reload; a plugin that
	// stays up longer than MaxDelay after it starts a fresh attempt series.
	healthySince time.Time
	exhausted    bool
}

type scheduleOutcome int

const (
	scheduleSkipped scheduleOutcome = iota
	scheduleArmed
	scheduleExhausted
)

func NewSupervisor(
	loader *Loader,
	repo repositories.PluginRepository,
	auditLogger audit.Logger,
	opts RecoveryOptions,
	logger *slog.Logger,
) *Supervisor {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())

	supervisor := &Supervisor{
		loader:  loader,
		repo:    repo,
		audit:   auditLogger,
		logger:  logger,
		opts:    opts.withDefaults(),
		ctx:     ctx,
		cancel:  cancel,
		entries: make(map[domain.Uint64ID]*recoveryEntry),
	}

	loader.setRecovery(supervisor)

	return supervisor
}

// OnPluginDisabled implements pkgplugin.DisableHook. It runs on the hook's
// goroutine with no runtime lock held.
func (s *Supervisor) OnPluginDisabled(pluginID string, dbID uint64, reason string) {
	if dbID == 0 {
		s.logger.Warn("disabled plugin has no database record, not reloading",
			slog.String("plugin", pluginID),
			slog.String("reason", reason))

		return
	}

	if !s.begin() {
		return
	}
	defer s.wg.Done()

	id := domain.Uint64ID(dbID)

	ctx, cancel := context.WithTimeout(s.ctx, recoveryLookupTimeout)
	defer cancel()

	plugin, err := s.findPlugin(ctx, id)
	if err != nil {
		s.logger.Warn("disabled plugin lookup failed, not reloading",
			slog.Uint64("plugin_id", dbID),
			slog.String("plugin", pluginID),
			slog.String("error", err.Error()))

		return
	}

	if plugin.Status == domain.PluginStatusDisabled || plugin.Status == domain.PluginStatusUpdating {
		s.logger.Info("plugin disabled at runtime while in an operator-managed state, not reloading",
			slog.Uint64("plugin_id", dbID),
			slog.String("plugin", pluginID),
			slog.String("status", string(plugin.Status)))

		return
	}

	plugin.MarkError(reason, s.opts.now())
	s.save(ctx, plugin)

	audit.SystemOp(ctx, s.audit, audit.EventPluginDisabled, audit.CategoryPluginOp, audit.OutcomeFailure,
		"plugin", strconv.FormatUint(dbID, 10), "disable", disableAuditReason(reason),
		slog.String("plugin", pluginID), slog.String("detail", reason))

	if s.opts.DisableReload {
		s.logger.Warn("plugin disabled at runtime, automatic reload is off",
			slog.Uint64("plugin_id", dbID),
			slog.String("plugin", pluginID),
			slog.String("reason", reason))

		return
	}

	s.applySchedule(ctx, id, s.schedule(id, pluginID), reason)
}

// Forget drops the recovery state of a plugin, cancelling a pending reload.
func (s *Supervisor) Forget(dbID domain.Uint64ID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[dbID]
	if !ok {
		return
	}

	if entry.timer != nil {
		entry.timer.Stop()
	}

	delete(s.entries, dbID)
}

// Stop cancels pending reloads and waits for the ones in flight.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	s.stopped = true
	for _, entry := range s.entries {
		if entry.timer != nil {
			entry.timer.Stop()
			entry.timer = nil
		}
	}
	s.mu.Unlock()

	s.cancel()
	s.wg.Wait()
}

// begin registers a unit of work unless the supervisor is stopped.
func (s *Supervisor) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return false
	}

	s.wg.Add(1)

	return true
}

// schedule arms the next reload timer for the plugin and reports what it did.
func (s *Supervisor) schedule(dbID domain.Uint64ID, pluginID string) scheduleOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return scheduleSkipped
	}

	entry, ok := s.entries[dbID]
	if !ok {
		entry = &recoveryEntry{pluginID: pluginID}
		s.entries[dbID] = entry
	}

	if entry.timer != nil || entry.exhausted {
		return scheduleSkipped
	}

	now := s.opts.now()
	if !entry.healthySince.IsZero() && now.Sub(entry.healthySince) > s.opts.MaxDelay {
		entry.attempts = 0
	}

	entry.attempts++

	if entry.attempts > s.opts.MaxAttempts {
		entry.exhausted = true

		return scheduleExhausted
	}

	delay := s.delayFor(entry.attempts)
	entry.timer = s.opts.newTimer(delay, func() { s.attempt(dbID) })

	s.logger.Warn("plugin disabled, automatic reload scheduled",
		slog.Uint64("plugin_id", uint64(dbID)),
		slog.String("plugin", pluginID),
		slog.Int("attempt", entry.attempts),
		slog.Int("max_attempts", s.opts.MaxAttempts),
		slog.Duration("delay", delay))

	return scheduleArmed
}

// applySchedule performs the side effects of an exhausted series outside the
// supervisor lock: the row keeps the reason plus a note that the panel gave up.
func (s *Supervisor) applySchedule(ctx context.Context, dbID domain.Uint64ID, outcome scheduleOutcome, reason string) {
	if outcome != scheduleExhausted {
		return
	}

	s.logger.Error("automatic plugin reload attempts exhausted, plugin stays disabled until reloaded",
		slog.Uint64("plugin_id", uint64(dbID)),
		slog.Int("max_attempts", s.opts.MaxAttempts))

	plugin, err := s.findPlugin(ctx, dbID)
	if err != nil {
		return
	}

	plugin.MarkError(reason+"; automatic reload attempts exhausted ("+strconv.Itoa(s.opts.MaxAttempts)+")", s.opts.now())
	s.save(ctx, plugin)

	audit.SystemOp(ctx, s.audit, audit.EventPluginDisabled, audit.CategoryPluginOp, audit.OutcomeFailure,
		"plugin", strconv.FormatUint(uint64(dbID), 10), "disable", auditReasonRecoveryExhausted,
		slog.Int("attempts", s.opts.MaxAttempts))
}

func (s *Supervisor) delayFor(attempt int) time.Duration {
	delay := float64(s.opts.InitialDelay) * math.Pow(s.opts.Factor, float64(attempt-1))
	if delay > float64(s.opts.MaxDelay) || math.IsInf(delay, 1) {
		return s.opts.MaxDelay
	}

	return time.Duration(delay)
}

// attempt runs on the timer goroutine: it re-checks the row (the plugin may
// have been uninstalled or reloaded by an operator meanwhile), reloads and
// either records success or schedules the next attempt.
func (s *Supervisor) attempt(dbID domain.Uint64ID) {
	if !s.begin() {
		return
	}
	defer s.wg.Done()

	s.mu.Lock()
	entry, ok := s.entries[dbID]
	if !ok {
		s.mu.Unlock()

		return
	}
	entry.timer = nil
	attempt := entry.attempts
	pluginID := entry.pluginID
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(s.ctx, reloadTimeout)
	defer cancel()

	resourceID := strconv.FormatUint(uint64(dbID), 10)
	attrs := []slog.Attr{slog.String("trigger", "auto"), slog.Int("attempt", attempt)}

	plugin, err := s.findPlugin(ctx, dbID)
	if err != nil {
		if errors.Is(err, ErrPluginNotInstalled) {
			s.Forget(dbID)

			return
		}

		s.logger.Error("plugin lookup before automatic reload failed",
			slog.Uint64("plugin_id", uint64(dbID)), slog.String("error", err.Error()))
		s.applySchedule(ctx, dbID, s.schedule(dbID, pluginID), "plugin lookup failed")

		return
	}

	if plugin.Status != domain.PluginStatusError {
		s.logger.Info("plugin is no longer in error state, skipping automatic reload",
			slog.Uint64("plugin_id", uint64(dbID)),
			slog.String("status", string(plugin.Status)))
		s.Forget(dbID)

		return
	}

	if _, _, err := s.loader.reload(ctx, dbID); err != nil {
		s.logger.Error("automatic plugin reload failed",
			slog.Uint64("plugin_id", uint64(dbID)),
			slog.String("plugin", pluginID),
			slog.Int("attempt", attempt),
			slog.String("error", err.Error()))

		audit.SystemOp(ctx, s.audit, audit.EventPluginReloaded, audit.CategoryPluginOp, audit.OutcomeFailure,
			"plugin", resourceID, "reload", auditReasonLoadFailed, attrs...)

		s.applySchedule(ctx, dbID, s.schedule(dbID, pluginID), LoadErrorText(err))

		return
	}

	s.mu.Lock()
	if entry, ok := s.entries[dbID]; ok {
		entry.healthySince = s.opts.now()
	}
	s.mu.Unlock()

	s.logger.Info("plugin reloaded automatically",
		slog.Uint64("plugin_id", uint64(dbID)),
		slog.String("plugin", pluginID),
		slog.Int("attempt", attempt))

	audit.SystemOp(ctx, s.audit, audit.EventPluginReloaded, audit.CategoryPluginOp, audit.OutcomeSuccess,
		"plugin", resourceID, "reload", "", attrs...)
}

func (s *Supervisor) findPlugin(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, error) {
	plugins, err := s.repo.Find(ctx, filters.FindPluginByIDs(dbID), nil, &filters.Pagination{Limit: 1})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find plugin")
	}

	if len(plugins) == 0 {
		return nil, ErrPluginNotInstalled
	}

	return &plugins[0], nil
}

func (s *Supervisor) save(ctx context.Context, plugin *domain.Plugin) {
	if err := s.repo.Save(ctx, plugin); err != nil {
		s.logger.Warn("failed to record plugin error state",
			slog.Uint64("plugin_id", uint64(plugin.ID)),
			slog.String("error", err.Error()))
	}
}

func disableAuditReason(reason string) string {
	if strings.HasPrefix(reason, pkgplugin.DisableReasonGuestExited) {
		return auditReasonGuestExited
	}

	return auditReasonGuestTimeout
}
