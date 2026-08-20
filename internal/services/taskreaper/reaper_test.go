package taskreaper

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTaskRepo struct {
	tasks []domain.DaemonTask
}

func (f *fakeTaskRepo) Find(
	_ context.Context,
	filter *filters.FindDaemonTask,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.DaemonTask, error) {
	if filter == nil {
		out := make([]domain.DaemonTask, len(f.tasks))
		copy(out, f.tasks)

		return out, nil
	}

	statusSet := make(map[domain.DaemonTaskStatus]struct{}, len(filter.Statuses))
	for _, s := range filter.Statuses {
		statusSet[s] = struct{}{}
	}

	out := make([]domain.DaemonTask, 0, len(f.tasks))
	for _, t := range f.tasks {
		if len(statusSet) > 0 {
			if _, ok := statusSet[t.Status]; !ok {
				continue
			}
		}
		out = append(out, t)
	}

	return out, nil
}

type fakeRegistry struct {
	connected map[uint64]struct{}
}

func (f *fakeRegistry) IsConnectedAnywhere(nodeID uint64) bool {
	_, ok := f.connected[nodeID]

	return ok
}

type recordedReconcile struct {
	nodeID      uint64
	inFlightIDs []uint64
	reason      string
}

type fakeReconciler struct {
	mu    sync.Mutex
	calls []recordedReconcile
}

func (f *fakeReconciler) ReconcileWorkingTasks(
	_ context.Context, nodeID uint64, inFlightIDs []uint64, reason string,
) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, recordedReconcile{
		nodeID:      nodeID,
		inFlightIDs: inFlightIDs,
		reason:      reason,
	})

	return 1, nil
}

func (f *fakeReconciler) recordedNodeIDs() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	ids := make([]uint64, 0, len(f.calls))
	for _, c := range f.calls {
		ids = append(ids, c.nodeID)
	}

	return ids
}

func TestReaperSweep(t *testing.T) {
	t.Parallel()

	now := time.Now()
	stale := now.Add(-30 * time.Minute)
	fresh := now.Add(-5 * time.Second)

	type taskSpec struct {
		id                uint
		dedicatedServerID uint
		status            domain.DaemonTaskStatus
		updatedAt         time.Time
	}

	tests := []struct {
		name           string
		seed           []taskSpec
		connected      []uint64
		staleThreshold time.Duration
		wantNodeIDs    []uint64
		wantReason     string
	}{
		{
			name: "stale_disconnected_node_is_reaped",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 7, status: domain.DaemonTaskStatusWorking, updatedAt: stale},
			},
			connected:      nil,
			staleThreshold: 10 * time.Minute,
			wantNodeIDs:    []uint64{7},
			wantReason:     ReconcileReasonStaleSweep,
		},
		{
			name: "connected_node_skipped",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 7, status: domain.DaemonTaskStatusWorking, updatedAt: stale},
			},
			connected:      []uint64{7},
			staleThreshold: 10 * time.Minute,
			wantNodeIDs:    nil,
		},
		{
			name: "recent_update_within_threshold_skipped",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 7, status: domain.DaemonTaskStatusWorking, updatedAt: fresh},
			},
			connected:      nil,
			staleThreshold: 10 * time.Minute,
			wantNodeIDs:    nil,
		},
		{
			name: "task_with_nil_updated_at_treated_as_stale",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 7, status: domain.DaemonTaskStatusWorking},
			},
			connected:      nil,
			staleThreshold: 10 * time.Minute,
			wantNodeIDs:    []uint64{7},
			wantReason:     ReconcileReasonStaleSweep,
		},
		{
			name: "mixed_nodes_only_disconnected_and_stale_reaped",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 7, status: domain.DaemonTaskStatusWorking, updatedAt: stale},
				{id: 2, dedicatedServerID: 8, status: domain.DaemonTaskStatusWorking, updatedAt: stale},
				{id: 3, dedicatedServerID: 9, status: domain.DaemonTaskStatusWorking, updatedAt: fresh},
			},
			connected:      []uint64{8},
			staleThreshold: 10 * time.Minute,
			wantNodeIDs:    []uint64{7},
			wantReason:     ReconcileReasonStaleSweep,
		},
		{
			name: "single_call_per_node_when_multiple_stale_tasks",
			seed: []taskSpec{
				{id: 1, dedicatedServerID: 7, status: domain.DaemonTaskStatusWorking, updatedAt: stale},
				{id: 2, dedicatedServerID: 7, status: domain.DaemonTaskStatusWorking, updatedAt: stale},
			},
			connected:      nil,
			staleThreshold: 10 * time.Minute,
			wantNodeIDs:    []uint64{7},
			wantReason:     ReconcileReasonStaleSweep,
		},
		{
			name:           "no_tasks_is_noop",
			seed:           nil,
			connected:      nil,
			staleThreshold: 10 * time.Minute,
			wantNodeIDs:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeTaskRepo{}
			for _, s := range tt.seed {
				task := domain.DaemonTask{
					ID:                s.id,
					DedicatedServerID: s.dedicatedServerID,
					Task:              domain.DaemonTaskTypeCmdExec,
					Status:            s.status,
				}
				if !s.updatedAt.IsZero() {
					ts := s.updatedAt
					task.UpdatedAt = &ts
				}
				repo.tasks = append(repo.tasks, task)
			}

			registry := &fakeRegistry{connected: make(map[uint64]struct{})}
			for _, id := range tt.connected {
				registry.connected[id] = struct{}{}
			}

			reconciler := &fakeReconciler{}

			reaper := NewReaper(repo, registry, reconciler, Options{
				StaleThreshold: tt.staleThreshold,
			}, slog.Default())

			require.NoError(t, reaper.Sweep(context.Background()))

			gotIDs := reconciler.recordedNodeIDs()
			assert.ElementsMatch(t, tt.wantNodeIDs, gotIDs)

			if tt.wantReason != "" {
				reconciler.mu.Lock()
				for _, c := range reconciler.calls {
					assert.Equal(t, tt.wantReason, c.reason)
					assert.Nil(t, c.inFlightIDs, "stale sweep must pass nil InFlightIDs")
				}
				reconciler.mu.Unlock()
			}
		})
	}
}

type panicTaskRepo struct{}

func (panicTaskRepo) Find(
	context.Context, *filters.FindDaemonTask, []filters.Sorting, *filters.Pagination,
) ([]domain.DaemonTask, error) {
	panic("reaper dependency exploded")
}

func TestReaper_safeSweep_recoversPanic(t *testing.T) {
	t.Parallel()

	reaper := NewReaper(
		panicTaskRepo{},
		&fakeRegistry{connected: make(map[uint64]struct{})},
		&fakeReconciler{},
		Options{},
		slog.New(slog.DiscardHandler),
	)

	assert.NotPanics(t, func() {
		reaper.safeSweep(context.Background())
	}, "a panic in Sweep must be recovered so the reaper loop survives")
}

func TestReaperOptionsApplyDefaults(t *testing.T) {
	t.Parallel()

	o := Options{}
	o.applyDefaults()
	assert.Equal(t, defaultInterval, o.Interval)
	assert.Equal(t, defaultStaleThreshold, o.StaleThreshold)

	o = Options{Interval: 5 * time.Second, StaleThreshold: 30 * time.Second}
	o.applyDefaults()
	assert.Equal(t, 5*time.Second, o.Interval)
	assert.Equal(t, 30*time.Second, o.StaleThreshold)
}

var (
	errTestRepoFind  = errors.New("simulated repo find failure")
	errTestReconcile = errors.New("simulated reconcile failure")
)

type errorTaskRepo struct{ err error }

func (r *errorTaskRepo) Find(
	context.Context, *filters.FindDaemonTask, []filters.Sorting, *filters.Pagination,
) ([]domain.DaemonTask, error) {
	return nil, r.err
}

type errorReconciler struct {
	fakeReconciler

	err error
}

func (r *errorReconciler) ReconcileWorkingTasks(
	ctx context.Context, nodeID uint64, inFlightIDs []uint64, reason string,
) (int, error) {
	_, _ = r.fakeReconciler.ReconcileWorkingTasks(ctx, nodeID, inFlightIDs, reason)

	return 0, r.err
}

func TestReaperSweep_FindError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	reaper := NewReaper(
		&errorTaskRepo{err: errTestRepoFind},
		&fakeRegistry{connected: make(map[uint64]struct{})},
		&fakeReconciler{},
		Options{},
		slog.New(slog.DiscardHandler),
	)

	// ACT
	err := reaper.Sweep(context.Background())

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "find working tasks", "error message mismatch")
	assert.ErrorIs(t, err, errTestRepoFind, "the repository error must be wrapped, not replaced")
}

func TestReaperSweep_ReconcileErrorIsIgnored(t *testing.T) {
	t.Parallel()

	// ARRANGE
	stale := time.Now().Add(-30 * time.Minute)
	repo := &fakeTaskRepo{tasks: []domain.DaemonTask{
		{
			ID:                1,
			DedicatedServerID: 7,
			Task:              domain.DaemonTaskTypeCmdExec,
			Status:            domain.DaemonTaskStatusWorking,
			UpdatedAt:         &stale,
		},
	}}
	reconciler := &errorReconciler{err: errTestReconcile}
	reaper := NewReaper(
		repo,
		&fakeRegistry{connected: make(map[uint64]struct{})},
		reconciler,
		Options{StaleThreshold: 10 * time.Minute},
		slog.New(slog.DiscardHandler),
	)

	// ACT
	err := reaper.Sweep(context.Background())

	// ASSERT
	require.NoError(t, err, "reconcile errors are logged and must not bubble up out of Sweep")
	assert.Equal(t, []uint64{7}, reconciler.recordedNodeIDs(), "the abandoned node must still be reconciled")
}

func TestReaper_safeSweep_logsSweepError(t *testing.T) {
	t.Parallel()

	// ARRANGE
	reaper := NewReaper(
		&errorTaskRepo{err: errTestRepoFind},
		&fakeRegistry{connected: make(map[uint64]struct{})},
		&fakeReconciler{},
		Options{},
		slog.New(slog.DiscardHandler),
	)

	// ACT & ASSERT
	assert.NotPanics(t, func() {
		reaper.safeSweep(context.Background())
	}, "a Sweep error must be logged so the reaper loop survives")
}

func TestReaperStart(t *testing.T) {
	t.Parallel()

	t.Run("sweeps_on_ticker", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		stale := time.Now().Add(-30 * time.Minute)
		repo := &fakeTaskRepo{tasks: []domain.DaemonTask{
			{
				ID:                1,
				DedicatedServerID: 7,
				Task:              domain.DaemonTaskTypeCmdExec,
				Status:            domain.DaemonTaskStatusWorking,
				UpdatedAt:         &stale,
			},
		}}
		reconciler := &fakeReconciler{}
		reaper := NewReaper(
			repo,
			&fakeRegistry{connected: make(map[uint64]struct{})},
			reconciler,
			Options{Interval: 10 * time.Millisecond, StaleThreshold: 10 * time.Minute},
			slog.New(slog.DiscardHandler),
		)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// ACT
		err := reaper.Start(ctx)

		// ASSERT
		require.NoError(t, err)
		assert.Eventually(t, func() bool {
			return len(reconciler.recordedNodeIDs()) > 0
		}, 2*time.Second, 10*time.Millisecond, "the ticker must drive periodic sweeps")

		// The short-interval ticker keeps firing, so the exact number of
		// sweeps by the time we read is timing-dependent. Assert the invariant
		// that actually matters: every sweep reconciles only the abandoned node.
		recorded := reconciler.recordedNodeIDs()
		require.NotEmpty(t, recorded)
		for _, nodeID := range recorded {
			assert.Equal(t, uint64(7), nodeID, "every ticker-driven sweep must reconcile only the abandoned node")
		}
	})

	t.Run("stops_on_context_cancel", func(t *testing.T) {
		t.Parallel()

		// ARRANGE — a huge interval guarantees the ticker never fires, so the
		// goroutine can only leave the select through the ctx.Done branch. The
		// repo is seeded with the same stale, abandoned task as sweeps_on_ticker
		// so that any eager sweep slipping in before the first tick would record
		// reconciler activity — without it, an empty repo makes the assertion
		// below pass even if a sweep did run.
		stale := time.Now().Add(-30 * time.Minute)
		repo := &fakeTaskRepo{tasks: []domain.DaemonTask{
			{
				ID:                1,
				DedicatedServerID: 7,
				Task:              domain.DaemonTaskTypeCmdExec,
				Status:            domain.DaemonTaskStatusWorking,
				UpdatedAt:         &stale,
			},
		}}
		reconciler := &fakeReconciler{}
		reaper := NewReaper(
			repo,
			&fakeRegistry{connected: make(map[uint64]struct{})},
			reconciler,
			Options{Interval: time.Hour, StaleThreshold: 10 * time.Minute},
			slog.New(slog.DiscardHandler),
		)
		ctx, cancel := context.WithCancel(context.Background())

		// ACT
		err := reaper.Start(ctx)
		cancel()
		time.Sleep(50 * time.Millisecond)

		// ASSERT
		require.NoError(t, err)
		assert.Empty(t, reconciler.recordedNodeIDs(),
			"no sweep must run before the first tick, even with a reconcilable task present")
	})
}

func TestNewReaper_NilLogger(t *testing.T) {
	t.Parallel()

	// ARRANGE & ACT
	reaper := NewReaper(
		&fakeTaskRepo{},
		&fakeRegistry{connected: make(map[uint64]struct{})},
		&fakeReconciler{},
		Options{},
		nil,
	)

	// ASSERT
	require.NotNil(t, reaper, "a nil logger must fall back to the default logger")
	require.NoError(t, reaper.Sweep(context.Background()))
}
