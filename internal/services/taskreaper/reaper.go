// Package taskreaper periodically transitions abandoned `working` daemon
// tasks to `error`. It complements the at-register reconciliation in the
// gateway service: that path catches restarts where the daemon comes back,
// while the reaper catches nodes that never reconnect (decommissioned,
// permanently offline, hardware lost) and would otherwise leave operations
// stuck in-progress in the UI forever.
package taskreaper

import (
	"context"
	"log/slog"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/pkg/errors"
)

// ReconcileReasonStaleSweep marks tasks reconciled by the periodic sweep —
// no daemon session exists for the node and the task has not been touched
// for the configured threshold.
const ReconcileReasonStaleSweep = "stale_sweep"

const (
	defaultInterval       = 1 * time.Minute
	defaultStaleThreshold = 10 * time.Minute
)

type Options struct {
	Interval       time.Duration
	StaleThreshold time.Duration
}

func (o *Options) applyDefaults() {
	if o.Interval <= 0 {
		o.Interval = defaultInterval
	}
	if o.StaleThreshold <= 0 {
		o.StaleThreshold = defaultStaleThreshold
	}
}

type Reaper struct {
	taskRepo   DaemonTaskRepository
	registry   SessionRegistry
	reconciler TaskReconciler
	opts       Options
	logger     *slog.Logger
}

func NewReaper(
	taskRepo DaemonTaskRepository,
	registry SessionRegistry,
	reconciler TaskReconciler,
	opts Options,
	logger *slog.Logger,
) *Reaper {
	if logger == nil {
		logger = slog.Default()
	}
	opts.applyDefaults()

	return &Reaper{
		taskRepo:   taskRepo,
		registry:   registry,
		reconciler: reconciler,
		opts:       opts,
		logger:     logger,
	}
}

func (r *Reaper) Start(ctx context.Context) error {
	go r.run(ctx)

	r.logger.Info("task reaper started",
		"interval", r.opts.Interval,
		"stale_threshold", r.opts.StaleThreshold,
	)

	return nil
}

func (r *Reaper) run(ctx context.Context) {
	ticker := time.NewTicker(r.opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.safeSweep(ctx)
		}
	}
}

// safeSweep runs one Sweep with panic recovery so a panic in reconciliation
// (e.g. a nil dependency) is logged and the reaper loop keeps running, rather
// than killing the goroutine forever and silently leaving tasks stuck in
// "working".
func (r *Reaper) safeSweep(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("task reaper sweep panicked", "panic", rec)
		}
	}()

	if err := r.Sweep(ctx); err != nil {
		r.logger.Warn("task reaper sweep failed", "error", err)
	}
}

// Sweep runs one reconciliation pass. Exposed so tests can drive it
// deterministically without spinning the ticker.
func (r *Reaper) Sweep(ctx context.Context) error {
	tasks, err := r.taskRepo.Find(ctx, &filters.FindDaemonTask{
		Statuses: []domain.DaemonTaskStatus{domain.DaemonTaskStatusWorking},
	}, nil, nil)
	if err != nil {
		return errors.WithMessage(err, "find working tasks")
	}

	if len(tasks) == 0 {
		return nil
	}

	cutoff := time.Now().Add(-r.opts.StaleThreshold)
	abandonedNodes := make(map[uint64]struct{})

	for i := range tasks {
		task := &tasks[i]

		nodeID := uint64(task.DedicatedServerID)
		if r.registry.IsConnectedAnywhere(nodeID) {
			continue
		}

		if task.UpdatedAt != nil && task.UpdatedAt.After(cutoff) {
			continue
		}

		abandonedNodes[nodeID] = struct{}{}
	}

	for nodeID := range abandonedNodes {
		if _, err := r.reconciler.ReconcileWorkingTasks(
			ctx, nodeID, nil, ReconcileReasonStaleSweep,
		); err != nil {
			r.logger.Warn("failed to reconcile working tasks during sweep",
				"node_id", nodeID,
				"error", err,
			)
		}
	}

	return nil
}
