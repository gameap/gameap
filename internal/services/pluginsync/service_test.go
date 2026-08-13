package pluginsync_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/internal/services/pluginsync"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	wasmBody       = "test data"
	nextVersion    = "2.0.0"
	wasmChecksum   = "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9"
	pluginsDir     = "plugins"
	pluginFilename = "1.wasm"
	pluginDBID     = domain.Uint64ID(1)
)

type harness struct {
	svc      *pluginsync.Service
	repo     *fakeRepo
	loader   *fakeLoader
	provider *fakeProvider
	subs     *fakeSubs
	archive  *fakeArchive
	files    *files.InMemoryFileManager
	store    *fakeDownloader
	locks    *fakeLocker
	clock    *fakeClock
}

func newHarness(t *testing.T, opts ...func(*pluginsync.Deps, *pluginsync.Options)) *harness {
	t.Helper()

	provider := newFakeProvider()
	h := &harness{
		repo:     &fakeRepo{},
		loader:   newFakeLoader(provider),
		provider: provider,
		subs:     &fakeSubs{},
		archive:  &fakeArchive{},
		files:    files.NewInMemoryFileManager(),
		store:    &fakeDownloader{data: []byte(wasmBody)},
		locks:    &fakeLocker{},
		clock:    newFakeClock(),
	}

	deps := pluginsync.Deps{
		Repo:       h.repo,
		Loader:     h.loader,
		Plugins:    h.provider,
		Subs:       h.subs,
		Archive:    h.archive,
		Files:      h.files,
		Store:      h.store,
		Locks:      h.locks,
		PluginsDir: pluginsDir,
	}
	options := pluginsync.Options{Clock: h.clock}

	for _, opt := range opts {
		opt(&deps, &options)
	}

	h.svc = pluginsync.New(deps, options, slog.New(slog.DiscardHandler))

	return h
}

// writePluginFile puts a plugin file where the loader expects it.
func (h *harness) writePluginFile(t *testing.T) {
	t.Helper()

	require.NoError(t, h.files.Write(context.Background(), pluginsDir+"/"+pluginFilename, []byte(wasmBody)))
}

func activeRow() domain.Plugin {
	return domain.Plugin{
		ID:       pluginDBID,
		Name:     "test-plugin",
		Version:  "1.0.0",
		Filename: new(pluginFilename),
		Checksum: new(wasmChecksum),
		Source:   new("https://plugins.gameap.dev/api/plugins/testplugin"),
		Status:   domain.PluginStatusActive,
	}
}

func TestService_ReconcileNow(t *testing.T) {
	ctx := context.Background()

	t.Run("active_row_that_is_not_loaded_is_loaded", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		h.repo.set(activeRow())

		require.NoError(t, h.svc.ReconcileNow(ctx))

		calls := h.loader.loadCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "1.wasm", calls[0].filename)
		assert.Equal(t, uint64(1), calls[0].pluginID)
		assert.False(t, calls[0].reload)
		assert.Equal(t, 1, h.subs.count())
	})

	t.Run("already_loaded_row_is_left_alone", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		h.repo.set(activeRow())
		require.NoError(t, h.svc.ReconcileNow(ctx))

		require.NoError(t, h.svc.ReconcileNow(ctx))

		require.Len(t, h.loader.loadCalls(), 1, "a settled plugin must not be touched again")
		assert.Equal(t, 1, h.subs.count(), "subscriptions must not be rebuilt when nothing moved")
	})

	t.Run("disabled_row_is_unloaded", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		h.repo.set(activeRow())
		require.NoError(t, h.svc.ReconcileNow(ctx))

		disabled := activeRow()
		disabled.Status = domain.PluginStatusDisabled
		h.repo.set(disabled)

		require.NoError(t, h.svc.ReconcileNow(ctx))

		require.Len(t, h.loader.unloadedIDs(), 1)
		assert.Empty(t, h.provider.GetPlugins())
		assert.Equal(t, []uint64{1}, h.archive.removedIDs())
		assert.Equal(t, 2, h.subs.count())
	})

	t.Run("deleted_row_is_unloaded", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		h.repo.set(activeRow())
		require.NoError(t, h.svc.ReconcileNow(ctx))

		h.repo.set()

		require.NoError(t, h.svc.ReconcileNow(ctx))

		require.Len(t, h.loader.unloadedIDs(), 1)
		assert.Empty(t, h.provider.GetPlugins())
	})

	t.Run("version_change_reloads_the_plugin", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		h.repo.set(activeRow())
		require.NoError(t, h.svc.ReconcileNow(ctx))

		updated := activeRow()
		updated.Version = nextVersion
		h.repo.set(updated)
		h.provider.setVersion(1, nextVersion)

		require.NoError(t, h.svc.ReconcileNow(ctx))

		calls := h.loader.loadCalls()
		require.Len(t, calls, 2)
		assert.True(t, calls[1].reload, "a version change must replace the module in place")
	})

	t.Run("checksum_change_at_the_same_version_reloads_the_plugin", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		h.repo.set(activeRow())
		require.NoError(t, h.svc.ReconcileNow(ctx))

		rebuilt := activeRow()
		rebuilt.Checksum = new(plugininstall.Checksum([]byte("other build")))
		h.repo.set(rebuilt)
		require.NoError(t, h.files.Write(ctx, pluginsDir+"/1.wasm", []byte("other build")))

		require.NoError(t, h.svc.ReconcileNow(ctx))

		calls := h.loader.loadCalls()
		require.Len(t, calls, 2)
		assert.True(t, calls[1].reload)
	})

	t.Run("permission_change_does_not_reload_the_plugin", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		h.repo.set(activeRow())
		require.NoError(t, h.svc.ReconcileNow(ctx))

		regranted := activeRow()
		regranted.AllowedPermissions = []domain.PluginPermission{domain.PluginPermissionManageRBAC}
		regranted.Priority = 42
		h.repo.set(regranted)

		require.NoError(t, h.svc.ReconcileNow(ctx))

		require.Len(t, h.loader.loadCalls(), 1,
			"grants are read from the database per call, so they cost no downtime")
	})

	t.Run("loaded_plugin_without_a_row_is_kept_on_the_first_pass", func(t *testing.T) {
		h := newHarness(t)
		h.provider.add("foreign-plugin", &pkgplugin.LoadedPlugin{
			Info: &proto.PluginInfo{Id: "foreign-plugin", Version: "1.0.0"},
		})

		require.NoError(t, h.svc.ReconcileNow(ctx))

		assert.Empty(t, h.loader.unloadedIDs(), "a module never seen in the database is not ours to remove")
		require.Len(t, h.provider.GetPlugins(), 1)
	})

	t.Run("database_read_failure_unloads_nothing", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		h.repo.set(activeRow())
		require.NoError(t, h.svc.ReconcileNow(ctx))

		h.repo.setErr(errRepoUnavailable)

		err := h.svc.ReconcileNow(ctx)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "database unavailable")
		assert.Empty(t, h.loader.unloadedIDs())
		require.Len(t, h.provider.GetPlugins(), 1)
	})

	t.Run("subscription_refresh_error_does_not_fail_the_pass", func(t *testing.T) {
		h := newHarness(t)
		h.subs.err = errRepoUnavailable
		h.writePluginFile(t)
		h.repo.set(activeRow())

		require.NoError(t, h.svc.ReconcileNow(ctx))

		require.Len(t, h.loader.loadCalls(), 1)
	})

	t.Run("state_of_a_removed_plugin_is_pruned", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		h.repo.set(activeRow())
		require.NoError(t, h.svc.ReconcileNow(ctx))
		require.Len(t, h.svc.Snapshot(), 1)

		h.repo.set()
		require.NoError(t, h.svc.ReconcileNow(ctx))

		assert.Empty(t, h.svc.Snapshot())
	})
}

// uploadedRow models a hand-uploaded plugin: no other instance can fetch its
// file, so a missing file is a hard failure rather than something to recover.
func uploadedRow() domain.Plugin {
	row := activeRow()
	row.Source = new("file://" + pluginFilename)

	return row
}

func TestService_backoff(t *testing.T) {
	ctx := context.Background()

	t.Run("failure_schedules_a_retry_and_repeats_double", func(t *testing.T) {
		h := newHarness(t)
		h.repo.set(uploadedRow()) // no file, and nothing can fetch it

		require.NoError(t, h.svc.ReconcileNow(ctx))

		status := h.svc.Snapshot()[1]
		require.Equal(t, pluginsync.SyncStateRetrying, status.State)
		assert.Equal(t, 1, status.Failures)
		assert.Equal(t, h.clock.Now().Add(15*time.Second), status.NextAttempt)

		h.clock.advance(20 * time.Second)
		require.NoError(t, h.svc.ReconcileNow(ctx))

		status = h.svc.Snapshot()[1]
		assert.Equal(t, 2, status.Failures)
		assert.Equal(t, h.clock.Now().Add(30*time.Second), status.NextAttempt)
	})

	t.Run("plugin_inside_its_backoff_window_is_skipped", func(t *testing.T) {
		h := newHarness(t)
		h.repo.set(uploadedRow())
		require.NoError(t, h.svc.ReconcileNow(ctx))

		h.writePluginFile(t)
		require.NoError(t, h.svc.ReconcileNow(ctx))

		assert.Empty(t, h.loader.loadCalls(), "the retry is not due yet")
	})

	t.Run("fingerprint_change_retries_immediately", func(t *testing.T) {
		h := newHarness(t)
		h.repo.set(uploadedRow())
		require.NoError(t, h.svc.ReconcileNow(ctx))

		// The operator re-uploads a fixed build; the file is now present and
		// the row describes different content.
		fixed := uploadedRow()
		fixed.Version = "1.0.1"
		h.repo.set(fixed)
		h.writePluginFile(t)

		require.NoError(t, h.svc.ReconcileNow(ctx))

		require.Len(t, h.loader.loadCalls(), 1, "a fixed plugin must not wait out the backoff")
		assert.Equal(t, pluginsync.SyncStateInSync, h.svc.Snapshot()[1].State)
	})

	t.Run("backoff_is_capped", func(t *testing.T) {
		h := newHarness(t, func(_ *pluginsync.Deps, o *pluginsync.Options) {
			o.MinBackoff = time.Second
			o.MaxBackoff = 4 * time.Second
		})
		h.repo.set(uploadedRow())

		for range 6 {
			require.NoError(t, h.svc.ReconcileNow(ctx))
			h.clock.advance(time.Minute)
		}

		status := h.svc.Snapshot()[1]
		assert.Equal(t, 6, status.Failures)
		assert.Equal(t, h.clock.Now().Add(-time.Minute+4*time.Second), status.NextAttempt)
	})
}

func TestService_Forget(t *testing.T) {
	ctx := context.Background()

	h := newHarness(t)
	h.writePluginFile(t)
	h.repo.set(activeRow())
	require.NoError(t, h.svc.ReconcileNow(ctx))

	// An admin handler updates the plugin itself and drops what the reconciler
	// recorded, so the next pass must adopt its work rather than redo it.
	updated := activeRow()
	updated.Version = nextVersion
	h.repo.set(updated)
	h.provider.setVersion(1, nextVersion)
	h.provider.add(h.loader.managerID(1), &pkgplugin.LoadedPlugin{
		Info:    &proto.PluginInfo{Id: h.loader.managerID(1), Version: nextVersion},
		Enabled: true,
	})
	h.svc.Forget(1)

	require.NoError(t, h.svc.ReconcileNow(ctx))

	require.Len(t, h.loader.loadCalls(), 1, "the handler's work must be adopted, not repeated")
	assert.Equal(t, pluginsync.SyncStateInSync, h.svc.Snapshot()[1].State)
}

func TestService_ReconcileNow_is_serialized(t *testing.T) {
	ctx := context.Background()

	h := newHarness(t)
	h.writePluginFile(t)
	h.repo.set(activeRow())

	h.loader.entering = make(chan struct{})
	h.loader.release = make(chan struct{})

	first := make(chan error, 1)
	go func() { first <- h.svc.ReconcileNow(ctx) }()

	<-h.loader.entering

	second := make(chan error, 1)
	go func() { second <- h.svc.ReconcileNow(ctx) }()

	select {
	case <-second:
		t.Fatal("a second pass ran while the first was still applying")
	case <-time.After(100 * time.Millisecond):
	}

	h.loader.entering = nil
	close(h.loader.release)

	require.NoError(t, <-first)
	require.NoError(t, <-second)
}
