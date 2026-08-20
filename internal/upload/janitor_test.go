package upload_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJanitor_Sweep(t *testing.T) {
	t.Parallel()
	payload := []byte("0123456789")
	checksum := sha256Hex(t, payload)

	t.Run("removes_expired_session", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		storage := files.NewInMemoryFileManager()
		clock := &fakeClock{now: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)}
		svc := upload.NewService(storage, &fakeDaemon{}, clock, nil, defaultConfig())

		sess, err := svc.Create(context.Background(), upload.CreateParams{
			ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
			FullPath: testFullPath, TotalSize: 10, ExpectedChecksum: checksum,
		})
		require.NoError(t, err)
		require.NoError(t, svc.WriteChunk(
			context.Background(), sess.UploadID, testUserID, 0, bytes.NewReader(payload[0:4]),
		))

		clock.now = clock.now.Add(2 * time.Hour) // past 1h SessionTTL

		// ACT
		janitor := upload.NewJanitor(storage, clock, time.Minute, nil)
		janitor.Sweep(context.Background())

		// ASSERT
		assert.False(t, storage.Exists(context.Background(), "transfers/"+sess.UploadID+"/upload.json"))
		assert.False(t, storage.Exists(context.Background(), "transfers/"+sess.UploadID+"/chunks/000000"))
	})

	t.Run("keeps_active_session", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		storage := files.NewInMemoryFileManager()
		clock := &fakeClock{now: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)}
		svc := upload.NewService(storage, &fakeDaemon{}, clock, nil, defaultConfig())

		sess, err := svc.Create(context.Background(), upload.CreateParams{
			ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
			FullPath: testFullPath, TotalSize: 10, ExpectedChecksum: checksum,
		})
		require.NoError(t, err)

		clock.now = clock.now.Add(30 * time.Minute) // half of SessionTTL

		// ACT
		janitor := upload.NewJanitor(storage, clock, time.Minute, nil)
		janitor.Sweep(context.Background())

		// ASSERT
		assert.True(t, storage.Exists(context.Background(), "transfers/"+sess.UploadID+"/upload.json"))
	})

	t.Run("noop_on_empty_storage", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		storage := files.NewInMemoryFileManager()
		clock := &fakeClock{now: time.Now()}

		// ACT — must not panic on an empty storage.
		janitor := upload.NewJanitor(storage, clock, time.Minute, nil)
		janitor.Sweep(context.Background())

		// ASSERT
		entries, err := storage.List(context.Background(), "")
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("ignores_orphaned_chunk_without_metadata", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — chunk exists without a sibling upload.json.
		storage := files.NewInMemoryFileManager()
		require.NoError(t, storage.Write(
			context.Background(), "transfers/orphan/chunks/000000", []byte("data"),
		))

		// ACT
		janitor := upload.NewJanitor(storage, &fakeClock{now: time.Now()}, time.Minute, nil)
		janitor.Sweep(context.Background())

		// ASSERT — orphan must remain because the janitor cannot decide if the
		// session is still being created on a peer instance.
		assert.True(t, storage.Exists(context.Background(), "transfers/orphan/chunks/000000"))
	})

	t.Run("removes_only_expired_in_mixed_set", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — one expired and one active session in the same storage.
		storage := files.NewInMemoryFileManager()
		clock := &fakeClock{now: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)}
		svc := upload.NewService(storage, &fakeDaemon{}, clock, nil, defaultConfig())

		expired, err := svc.Create(context.Background(), upload.CreateParams{
			ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
			FullPath: testFullPath, TotalSize: 10, ExpectedChecksum: checksum,
		})
		require.NoError(t, err)

		clock.now = clock.now.Add(2 * time.Hour) // expired now lives in the past

		active, err := svc.Create(context.Background(), upload.CreateParams{
			ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
			FullPath: testFullPath, TotalSize: 10, ExpectedChecksum: checksum,
		})
		require.NoError(t, err)

		// ACT
		janitor := upload.NewJanitor(storage, clock, time.Minute, nil)
		janitor.Sweep(context.Background())

		// ASSERT
		assert.False(t, storage.Exists(context.Background(), "transfers/"+expired.UploadID+"/upload.json"),
			"expired session must be deleted")
		assert.True(t, storage.Exists(context.Background(), "transfers/"+active.UploadID+"/upload.json"),
			"active session must survive")
	})

	t.Run("skips_session_with_corrupted_metadata", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — non-JSON bytes in upload.json must not crash the sweep.
		storage := files.NewInMemoryFileManager()
		require.NoError(t, storage.Write(
			context.Background(), "transfers/broken/upload.json", []byte("not-json"),
		))
		require.NoError(t, storage.Write(
			context.Background(), "transfers/broken/chunks/000000", []byte("ignored"),
		))

		// ACT
		janitor := upload.NewJanitor(storage, &fakeClock{now: time.Now()}, time.Minute, nil)
		janitor.Sweep(context.Background())

		// ASSERT — corrupted session is left in place (logged-and-skipped).
		assert.True(t, storage.Exists(context.Background(), "transfers/broken/upload.json"))
		assert.True(t, storage.Exists(context.Background(), "transfers/broken/chunks/000000"))
	})
}

func TestJanitor_Run(t *testing.T) {
	t.Parallel()
	t.Run("returns_immediately_when_interval_is_zero", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		storage := files.NewInMemoryFileManager()
		janitor := upload.NewJanitor(storage, &fakeClock{now: time.Now()}, 0, nil)

		// ACT — Run with non-positive interval is a documented no-op; must
		// return without consuming the context cancel.
		done := make(chan struct{})
		go func() {
			janitor.Run(context.Background())
			close(done)
		}()

		// ASSERT
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Run did not return for interval <= 0")
		}
	})

	t.Run("returns_when_context_cancelled", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		storage := files.NewInMemoryFileManager()
		janitor := upload.NewJanitor(storage, &fakeClock{now: time.Now()}, 10*time.Millisecond, nil)
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			janitor.Run(ctx)
			close(done)
		}()

		// ACT
		cancel()

		// ASSERT
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Run did not return after context cancel")
		}
	})
}
