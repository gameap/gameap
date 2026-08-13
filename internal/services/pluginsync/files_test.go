package pluginsync_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/locker"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/internal/services/pluginsync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_missing_plugin_file(t *testing.T) {
	ctx := context.Background()
	const pluginPath = pluginsDir + "/1.wasm"

	t.Run("present_and_matching_file_is_not_downloaded", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		h.repo.set(activeRow())

		require.NoError(t, h.svc.ReconcileNow(ctx))

		assert.Empty(t, h.store.downloadCalls())
		assert.Empty(t, h.locks.acquiredKeys())
	})

	t.Run("missing_file_is_downloaded_from_the_store", func(t *testing.T) {
		h := newHarness(t)
		h.repo.set(activeRow())

		require.NoError(t, h.svc.ReconcileNow(ctx))

		calls := h.store.downloadCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "testplugin", calls[0].pluginID, "the store ID comes from the recorded source URL")
		assert.Equal(t, "1.0.0", calls[0].version)
		assert.True(t, h.files.Exists(ctx, pluginPath))
		require.Len(t, h.loader.loadCalls(), 1)
	})

	t.Run("file_with_a_stale_checksum_is_refetched", func(t *testing.T) {
		h := newHarness(t)
		require.NoError(t, h.files.Write(ctx, pluginPath, []byte("stale build")))
		h.repo.set(activeRow())

		require.NoError(t, h.svc.ReconcileNow(ctx))

		require.Len(t, h.store.downloadCalls(), 1)
		data, err := h.files.Read(ctx, pluginPath)
		require.NoError(t, err)
		assert.Equal(t, wasmBody, string(data))
	})

	t.Run("uploaded_plugin_is_not_downloaded", func(t *testing.T) {
		h := newHarness(t)
		h.repo.set(uploadedRow())

		require.NoError(t, h.svc.ReconcileNow(ctx))

		assert.Empty(t, h.store.downloadCalls())
		status := h.svc.Snapshot()[1]
		assert.Equal(t, pluginsync.SyncStateRetrying, status.State)
		assert.Contains(t, status.LastError, "not installed from the store")
	})

	t.Run("row_without_a_checksum_is_not_downloaded", func(t *testing.T) {
		h := newHarness(t)
		row := activeRow()
		row.Checksum = nil
		h.repo.set(row)

		require.NoError(t, h.svc.ReconcileNow(ctx))

		assert.Empty(t, h.store.downloadCalls(),
			"unverifiable bytes must never be written into storage other instances may share")
		assert.Contains(t, h.svc.Snapshot()[1].LastError, "no recorded checksum")
	})

	t.Run("legacy_row_without_a_checksum_trusts_the_file_it_has", func(t *testing.T) {
		h := newHarness(t)
		h.writePluginFile(t)
		row := activeRow()
		row.Checksum = nil
		h.repo.set(row)

		require.NoError(t, h.svc.ReconcileNow(ctx))

		assert.Empty(t, h.store.downloadCalls())
		require.Len(t, h.loader.loadCalls(), 1)
	})

	t.Run("download_that_does_not_match_the_checksum_is_not_written", func(t *testing.T) {
		h := newHarness(t)
		h.store.data = []byte("something else entirely")
		h.repo.set(activeRow())

		require.NoError(t, h.svc.ReconcileNow(ctx))

		require.False(t, h.files.Exists(ctx, pluginPath))
		assert.Empty(t, h.loader.loadCalls())
		assert.Contains(t, h.svc.Snapshot()[1].LastError, "does not match the recorded checksum")
	})

	t.Run("download_lock_held_by_a_peer_skips_the_plugin", func(t *testing.T) {
		h := newHarness(t)
		h.locks.deny = func(_ string) error { return locker.ErrLocked }
		h.repo.set(activeRow())

		require.NoError(t, h.svc.ReconcileNow(ctx))

		assert.Empty(t, h.store.downloadCalls(), "the peer holding the lock is doing the work")
		status := h.svc.Snapshot()[1]
		assert.Equal(t, 0, status.Failures, "contention is not a failure")
		assert.Equal(t, pluginsync.SyncStateRetrying, status.State,
			"a plugin waiting on a peer's download is not running here")
		assert.Equal(t, h.clock.Now().Add(10*time.Second), status.NextAttempt)
	})

	t.Run("peer_that_wrote_the_file_first_saves_the_download", func(t *testing.T) {
		h := newHarness(t)
		h.repo.set(activeRow())
		h.locks.onAcquire = func() {
			_ = h.files.Write(ctx, pluginPath, []byte(wasmBody))
		}

		require.NoError(t, h.svc.ReconcileNow(ctx))

		assert.Empty(t, h.store.downloadCalls())
		require.Len(t, h.loader.loadCalls(), 1)
		require.Len(t, h.locks.acquiredKeys(), 1)
		assert.Equal(t, "pluginsync:download:testplugin:1.0.0", h.locks.acquiredKeys()[0])
	})

	t.Run("checksum_helper_agrees_with_the_recorded_value", func(t *testing.T) {
		assert.Equal(t, wasmChecksum, plugininstall.Checksum([]byte(wasmBody)))
	})
}
