package pluginsync_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/services/pluginsync"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wasmContent = "\x00asm\x01\x00\x00\x00"

func activeRow(id domain.Uint64ID) domain.Plugin {
	return domain.Plugin{
		ID:       id,
		Name:     "plugin-" + string(rune('a'+id)),
		Version:  "1.0.0",
		Filename: new(pluginFile(id)),
		Source:   new("https://plugins.example/api/plugins/store-" + string(rune('a'+id))),
		Checksum: new(internalplugin.FileChecksum([]byte(wasmContent))),
		Status:   domain.PluginStatusActive,
	}
}

func pluginFile(id domain.Uint64ID) string {
	return "p" + string(rune('a'+id)) + ".wasm"
}

type env struct {
	ctx     context.Context
	repo    *fakeRepo
	loader  *fakeLoader
	subs    *fakeSubs
	archive *fakeArchive
	files   *fakeFiles
	store   *fakeStore
	locks   *fakeLocker
	clock   *fakeClock
	audit   *auditCapture
	passes  *passRecorder
	service *pluginsync.Service
}

func newEnv(t *testing.T, rows ...domain.Plugin) *env {
	t.Helper()

	e := &env{
		ctx:     context.Background(),
		repo:    newFakeRepo(rows...),
		loader:  newFakeLoader(),
		subs:    &fakeSubs{},
		archive: &fakeArchive{},
		files:   newFakeFiles(),
		store:   &fakeStore{data: []byte(wasmContent)},
		locks:   &fakeLocker{locked: make(map[string]bool)},
		clock:   &fakeClock{now: time.Unix(1_700_000_000, 0)},
		audit:   &auditCapture{},
		passes:  &passRecorder{},
	}

	for _, row := range rows {
		require.NoError(t, e.files.Write(e.ctx, "plugins/"+pluginFile(row.ID), []byte(wasmContent)))
	}

	e.service = pluginsync.New(pluginsync.Deps{
		Repo: e.repo, Loader: e.loader, Plugins: e.loader, Subs: e.subs, Archive: e.archive,
		Files: e.files, Store: e.store, Locks: e.locks, Audit: e.audit, Metrics: e.passes, PluginsDir: "plugins",
	}, pluginsync.Options{
		RefreshInterval: time.Minute, MinBackoff: 15 * time.Second, MaxBackoff: 2 * time.Minute,
		ContentionBackoff: 10 * time.Second, Clock: e.clock,
	}, slog.New(slog.DiscardHandler))

	return e
}

func (e *env) pass(t *testing.T) {
	t.Helper()
	require.NoError(t, e.service.ReconcileNow(e.ctx))
}

func TestReconcile_loads_active_rows_and_then_holds_still(t *testing.T) {
	t.Parallel()
	e := newEnv(t, activeRow(1), activeRow(2))

	e.pass(t)

	assert.True(t, e.loader.isRunning(1))
	assert.True(t, e.loader.isRunning(2))
	assert.Equal(t, 1, e.subs.count(), "subscriptions rebuilt once per pass that moved something")

	status := e.service.Snapshot()
	assert.Equal(t, pluginsync.StateInSync, status[1].State)
	assert.Equal(t, pluginsync.StateInSync, status[2].State)
	assert.Equal(t, 0, e.service.Pending())

	e.pass(t)
	e.pass(t)

	assert.Equal(t, 2, e.loader.applyCount(), "a steady state costs no loader calls beyond the state lookup")
	assert.Equal(t, 1, e.subs.count(), "nothing moved, nothing refreshed")
	assert.Equal(t, []string{"ok", "ok", "ok"}, e.passes.results)
}

func TestReconcile_adopts_modules_the_startup_load_built(t *testing.T) {
	t.Parallel()
	row := activeRow(1)
	e := newEnv(t, row)
	e.loader.preload(row)

	e.pass(t)

	assert.Equal(t, 0, e.loader.applyCount())
	assert.Equal(t, pluginsync.StateInSync, e.service.Snapshot()[1].State)
	assert.Equal(t, 0, e.subs.count())
}

func TestReconcile_unloads_rows_that_went_away_or_were_disabled(t *testing.T) {
	t.Parallel()
	e := newEnv(t, activeRow(1), activeRow(2), activeRow(3))

	e.pass(t)
	require.True(t, e.loader.isRunning(1))

	e.repo.remove(1)
	e.repo.update(2, func(p *domain.Plugin) { p.Status = domain.PluginStatusDisabled })

	e.pass(t)

	assert.False(t, e.loader.isRunning(1), "row deleted")
	assert.False(t, e.loader.isRunning(2), "status disabled")
	assert.True(t, e.loader.isRunning(3))
	assert.Equal(t, []string{internalplugin.TriggerSync, internalplugin.TriggerSync}, e.loader.triggers)
	assert.ElementsMatch(t, []uint64{1, 2}, e.archive.removed)
	assert.NotContains(t, e.service.Snapshot(), domain.Uint64ID(1))
	assert.Equal(t, 2, e.subs.count())

	e.repo.put(activeRow(2))
	e.repo.update(2, func(p *domain.Plugin) { p.Status = domain.PluginStatusDisabled })
	e.pass(t)
	assert.False(t, e.loader.isRunning(2), "a disabled row is never loaded")
}

func TestReconcile_leaves_modules_it_never_saw_in_the_database_alone(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.loader.preload(activeRow(9))

	e.pass(t)

	assert.True(t, e.loader.isRunning(9), "an autoload or externally registered module is not this reconciler's to remove")
	assert.Empty(t, e.loader.unloads)
}

func TestReconcile_reloads_when_the_row_changed(t *testing.T) {
	t.Parallel()

	mutations := map[string]func(*domain.Plugin){
		"version":    func(p *domain.Plugin) { p.Version = "2.0.0" },
		"checksum":   func(p *domain.Plugin) { p.Checksum = new("deadbeef") },
		"config":     func(p *domain.Plugin) { p.Config = map[string]any{"api_key": "k"} },
		"generation": func(p *domain.Plugin) { p.Generation++ },
	}

	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			row := activeRow(1)
			e := newEnv(t, row)
			e.loader.preload(row)

			e.repo.update(1, mutate)
			if name == "checksum" {
				e.loader.applyErr[1] = nil
				require.NoError(t, e.files.Write(e.ctx, "plugins/"+pluginFile(1), []byte(wasmContent)))
				// The file on disk no longer matches the recorded checksum and the
				// store serves the same bytes; that is a repair failure, not a
				// reload — covered by the file tests. Use a matching checksum here.
				e.repo.update(1, func(p *domain.Plugin) {
					p.Checksum = new(internalplugin.FileChecksum([]byte(wasmContent)))
					p.Version = "1.0.1"
				})
			}

			e.pass(t)

			assert.Equal(t, 1, e.loader.applyCount())
			assert.True(t, e.loader.isRunning(1))
			assert.Equal(t, 1, e.subs.count())

			reloads := e.audit.all()
			require.Len(t, reloads, 1)
			assert.Equal(t, audit.EventPluginReloaded, reloads[0].Type)
			assert.Equal(t, audit.OutcomeSuccess, reloads[0].Outcome)
		})
	}
}

func TestReconcile_permission_change_refreshes_subscriptions_without_a_reload(t *testing.T) {
	t.Parallel()
	row := activeRow(1)
	e := newEnv(t, row)
	e.loader.preload(row)

	e.pass(t)
	require.Equal(t, 0, e.subs.count())

	e.repo.update(1, func(p *domain.Plugin) {
		p.AllowedPermissions = []domain.PluginPermission{domain.PluginPermissionManageRBAC}
	})
	e.pass(t)
	assert.Equal(t, 0, e.loader.applyCount())
	assert.Equal(t, 0, e.subs.count(), "grants are read per host call; no subscription change either")

	e.repo.update(1, func(p *domain.Plugin) {
		p.AllowedPermissions = []domain.PluginPermission{domain.PluginPermissionListenEvents}
	})
	e.pass(t)
	assert.Equal(t, 0, e.loader.applyCount())
	assert.Equal(t, 1, e.subs.count(), "listen_events decides the subscriptions")

	e.pass(t)
	assert.Equal(t, 1, e.subs.count())
}

func TestReconcile_read_failure_touches_nothing(t *testing.T) {
	t.Parallel()
	e := newEnv(t, activeRow(1))
	e.pass(t)

	e.repo.readErr = errors.New("database is down")

	err := e.service.ReconcileNow(e.ctx)
	require.Error(t, err)
	assert.True(t, e.loader.isRunning(1))
	assert.Empty(t, e.loader.unloads)
	assert.Equal(t, []string{"ok", "failed"}, e.passes.results)
}

func TestReconcile_active_row_failures_back_off_and_a_changed_row_resets(t *testing.T) {
	t.Parallel()
	e := newEnv(t, activeRow(1))
	e.loader.applyErr[1] = errors.New("initialize failed")

	e.pass(t)
	require.Equal(t, 1, e.loader.applyCount())

	status := e.service.Snapshot()[1]
	assert.Equal(t, pluginsync.StateRetrying, status.State)
	assert.Equal(t, 1, status.Failures)
	assert.Contains(t, status.LastError, "initialize failed")
	assert.Equal(t, e.clock.Now().Add(15*time.Second), status.NextAttempt)
	assert.Equal(t, 1, e.service.Pending())

	e.pass(t)
	assert.Equal(t, 1, e.loader.applyCount(), "not due yet")

	e.clock.advance(15 * time.Second)
	e.pass(t)
	assert.Equal(t, 2, e.loader.applyCount())
	assert.Equal(t, e.clock.Now().Add(30*time.Second), e.service.Snapshot()[1].NextAttempt, "delay doubles")

	e.clock.advance(30 * time.Second)
	e.pass(t)
	e.clock.advance(time.Minute)
	e.pass(t)
	e.clock.advance(2 * time.Minute)
	e.pass(t)
	assert.Equal(t, e.clock.Now().Add(2*time.Minute), e.service.Snapshot()[1].NextAttempt, "capped at max backoff")

	e.repo.update(1, func(p *domain.Plugin) { p.Version = "1.0.1" })
	e.loader.applyErr[1] = nil
	e.pass(t)
	assert.True(t, e.loader.isRunning(1), "a changed row is retried at once")
	assert.Equal(t, pluginsync.StateInSync, e.service.Snapshot()[1].State)
	assert.Equal(t, 0, e.service.Pending())
}

func TestReconcile_error_rows(t *testing.T) {
	t.Parallel()

	t.Run("running_here_is_left_alone", func(t *testing.T) {
		t.Parallel()
		row := activeRow(1)
		e := newEnv(t, row)
		e.loader.preload(row)
		e.repo.update(1, func(p *domain.Plugin) { p.MarkError("peer failed", time.Now()) })

		e.pass(t)
		assert.Equal(t, 0, e.loader.applyCount())
		assert.True(t, e.loader.isRunning(1))
		assert.Equal(t, pluginsync.StateInSync, e.service.Snapshot()[1].State)
	})

	t.Run("absent_is_attempted_once_and_not_retried_on_a_timer", func(t *testing.T) {
		t.Parallel()
		row := activeRow(1)
		row.MarkError("startup failed", time.Now())
		e := newEnv(t, row)
		e.loader.applyErr[1] = errors.New("still broken")

		e.pass(t)
		assert.Equal(t, 1, e.loader.applyCount())

		status := e.service.Snapshot()[1]
		assert.Equal(t, pluginsync.StateFailed, status.State)
		assert.True(t, status.NextAttempt.IsZero())

		e.clock.advance(time.Hour)
		e.pass(t)
		assert.Equal(t, 1, e.loader.applyCount(), "the recovery supervisor owns timed retries of error rows")
	})

	t.Run("absent_is_retried_when_the_row_changes", func(t *testing.T) {
		t.Parallel()
		row := activeRow(1)
		row.MarkError("startup failed", time.Now())
		e := newEnv(t, row)
		e.loader.applyErr[1] = errors.New("still broken")

		e.pass(t)
		require.Equal(t, 1, e.loader.applyCount())

		e.loader.applyErr[1] = nil
		e.repo.update(1, func(p *domain.Plugin) { p.Generation++ })
		e.pass(t)
		assert.Equal(t, 2, e.loader.applyCount(), "an operator reload elsewhere bumps the generation")
		assert.True(t, e.loader.isRunning(1))
	})

	t.Run("absent_already_attempted_by_the_startup_load_is_not_attempted_again", func(t *testing.T) {
		t.Parallel()
		row := activeRow(1)
		row.MarkError("startup failed", time.Now())
		e := newEnv(t, row)
		e.loader.attempted[1] = internalplugin.Fingerprint(&row)

		e.pass(t)
		assert.Equal(t, 0, e.loader.applyCount())
	})

	t.Run("absent_is_attempted_again_after_the_file_was_repaired", func(t *testing.T) {
		t.Parallel()
		row := activeRow(1)
		row.MarkError("plugin file not found", time.Now())
		e := newEnv(t, row)
		e.loader.attempted[1] = internalplugin.Fingerprint(&row)
		e.files.files = map[string][]byte{}

		e.pass(t)
		assert.Equal(t, 1, e.loader.applyCount(), "the store restored the file, so the load is worth another try")
		assert.True(t, e.loader.isRunning(1))
		assert.Equal(t, []string{"store-b@1.0.0"}, e.store.downloads)
	})
}

func TestReconcile_reloads_a_module_disabled_at_runtime(t *testing.T) {
	t.Parallel()
	row := activeRow(1)
	e := newEnv(t, row)
	loaded := e.loader.preload(row)
	loaded.Disable()

	e.pass(t)
	assert.Equal(t, 1, e.loader.applyCount())
	assert.True(t, e.loader.RuntimeState(1).Enabled)
}

func TestReconcile_held_plugin_is_contention_not_failure(t *testing.T) {
	t.Parallel()
	e := newEnv(t, activeRow(1))
	e.loader.held[1] = true

	e.pass(t)

	status := e.service.Snapshot()[1]
	assert.Equal(t, pluginsync.StateRetrying, status.State)
	assert.Equal(t, 0, status.Failures)
	assert.Equal(t, e.clock.Now().Add(10*time.Second), status.NextAttempt)

	e.loader.held[1] = false
	e.clock.advance(10 * time.Second)
	e.pass(t)
	assert.True(t, e.loader.isRunning(1))
}

func TestReconcile_skips_updating_rows(t *testing.T) {
	t.Parallel()
	row := activeRow(1)
	row.Status = domain.PluginStatusUpdating
	e := newEnv(t, row)

	e.pass(t)
	assert.Equal(t, 0, e.loader.applyCount())
	assert.Empty(t, e.loader.unloads)
}

func TestReconcile_refresh_failure_does_not_fail_the_pass(t *testing.T) {
	t.Parallel()
	e := newEnv(t, activeRow(1))
	e.subs.err = errors.New("guest hung")

	e.pass(t)
	assert.True(t, e.loader.isRunning(1))
}

func TestService_nil_receiver_is_safe(t *testing.T) {
	t.Parallel()

	var svc *pluginsync.Service
	svc.Kick()
	svc.Notify(context.Background(), 1, "install")
	svc.Stop()
	require.NoError(t, svc.Subscribe(context.Background()))
	require.NoError(t, svc.Start(context.Background()))
	assert.Nil(t, svc.Snapshot())
	assert.Equal(t, 0, svc.Pending())
}
