package daemon

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/transfers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileService_DownloadStream_localTransferCapability(t *testing.T) {
	t.Parallel()

	t.Run("streams_part_through_registry_and_storage", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 71
		s.registry.setConnected(nodeID, true)
		s.registry.capabilities[nodeID] = map[string]bool{capabilityFileTransfer: true}

		const partPayload = "part-0-data"
		s.gateway.requestDownloadTask = func(taskCtx context.Context, _ uint64, transferID, _ string) error {
			state, ok := s.transferReg.Get(transferID)
			require.True(t, ok, "transfer must be registered before the download task runs")

			require.NoError(t, s.storage.Write(taskCtx, transfers.TransferPartPath(transferID, 0), []byte(partPayload)))
			state.AddPart()

			return nil
		}

		// ACT
		rc, err := s.service.DownloadStream(ctx, testNode(71, "/srv"), "/srv/large.bin")
		require.NoError(t, err)
		require.NotNil(t, rc)

		buf, readErr := io.ReadAll(rc)
		closeErr := rc.Close()

		// ASSERT
		require.NoError(t, readErr)
		require.NoError(t, closeErr)
		assert.Equal(t, partPayload, string(buf), "stream must surface the transferred part bytes")

		_, stillRegistered := s.transferReg.Get("")
		assert.False(t, stillRegistered, "registry must not retain phantom transfers")
	})

	t.Run("download_task_error_surfaces_to_caller", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 72
		s.registry.setConnected(nodeID, true)
		s.registry.capabilities[nodeID] = map[string]bool{capabilityFileTransfer: true}

		s.gateway.requestDownloadTask = func(_ context.Context, _ uint64, _, _ string) error {
			return errors.New("disk failure")
		}

		// ACT
		rc, err := s.service.DownloadStream(ctx, testNode(72, "/srv"), "/srv/large.bin")

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, rc, "reader must be nil when the download task fails before the first part")
		assert.Contains(t, err.Error(), "gateway download task", "error must be wrapped with the gateway context")
		assert.Contains(t, err.Error(), "disk failure", "original cause must be preserved")
	})

	t.Run("no_parts_completed_yields_empty_reader", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 73
		s.registry.setConnected(nodeID, true)
		s.registry.capabilities[nodeID] = map[string]bool{capabilityFileTransfer: true}

		// Return nil without adding any part: the spawned goroutine then calls Complete().
		s.gateway.requestDownloadTask = func(_ context.Context, _ uint64, _, _ string) error {
			return nil
		}

		// ACT
		rc, err := s.service.DownloadStream(ctx, testNode(73, "/srv"), "/srv/empty.bin")
		require.NoError(t, err)
		require.NotNil(t, rc)

		buf, readErr := io.ReadAll(rc)
		_ = rc.Close()

		// ASSERT
		require.NoError(t, readErr)
		assert.Empty(t, buf, "completing with zero parts must yield an empty reader")
	})
}

func TestFileService_DownloadStream_remoteS3(t *testing.T) {
	t.Parallel()

	t.Run("streams_part_via_storage_then_success_sentinel", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 81
		s.registry.connectedAnywhere[nodeID] = true

		const partPayload = "remote-part-0"
		s.dispatcher.dispatchDownloadTask = func(taskCtx context.Context, _ uint64, transferID, _ string) error {
			require.NoError(t, s.storage.Write(taskCtx, transfers.TransferPartPath(transferID, 0), []byte(partPayload)))
			// returning nil triggers writeSentinelIfMissing, which writes a success sentinel.
			return nil
		}

		// ACT
		rc, err := s.service.DownloadStream(ctx, testNode(81, "/srv"), "/srv/remote.bin")
		require.NoError(t, err)
		require.NotNil(t, rc)

		buf, readErr := io.ReadAll(rc)
		_ = rc.Close()

		// ASSERT
		require.NoError(t, readErr)
		assert.Equal(t, partPayload, string(buf), "remote stream must surface the transferred part bytes")
	})

	t.Run("dispatch_error_surfaces_to_caller", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 82
		s.registry.connectedAnywhere[nodeID] = true

		s.dispatcher.dispatchDownloadTask = func(_ context.Context, _ uint64, _, _ string) error {
			return errors.New("network partition")
		}

		// ACT
		rc, err := s.service.DownloadStream(ctx, testNode(82, "/srv"), "/srv/remote.bin")

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, rc)
		assert.Contains(t, err.Error(), "remote download task", "error must be wrapped with the remote context")
		assert.Contains(t, err.Error(), "network partition", "original cause must be preserved")
	})

	t.Run("failure_sentinel_surfaces_remote_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 83
		s.registry.connectedAnywhere[nodeID] = true

		s.dispatcher.dispatchDownloadTask = func(taskCtx context.Context, _ uint64, transferID, _ string) error {
			sentinel, marshalErr := json.Marshal(transfers.DoneInfo{Success: false, Error: "remote failure"})
			require.NoError(t, marshalErr)
			require.NoError(t, s.storage.Write(taskCtx, transfers.TransferDonePath(transferID), sentinel))

			return nil
		}

		// ACT
		rc, err := s.service.DownloadStream(ctx, testNode(83, "/srv"), "/srv/remote.bin")

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, rc)
		assert.Contains(t, err.Error(), "transfer failed on remote", "failure sentinel must surface as an error")
		assert.Contains(t, err.Error(), "remote failure", "remote error detail must be preserved")
	})
}

func TestFileService_waitForPartS3(t *testing.T) {
	t.Parallel()

	t.Run("returns_true_when_part_exists", func(t *testing.T) {
		t.Parallel()

		ctx := testContext(t)
		s := setupFileService(t)
		const transferID = "wfp-part"
		require.NoError(t, s.storage.Write(ctx, transfers.TransferPartPath(transferID, 0), []byte("x")))

		errCh := make(chan error, 1)
		available, err := s.service.waitForPartS3(ctx, transferID, 0, errCh)

		require.NoError(t, err)
		assert.True(t, available, "existing part must report available")
	})

	t.Run("returns_error_from_errCh", func(t *testing.T) {
		t.Parallel()

		ctx := testContext(t)
		s := setupFileService(t)

		errCh := make(chan error, 1)
		errCh <- errors.New("task boom")

		available, err := s.service.waitForPartS3(ctx, "wfp-err", 0, errCh)

		require.Error(t, err)
		assert.False(t, available)
		assert.Contains(t, err.Error(), "task boom", "errCh error must short-circuit the poll")
	})

	t.Run("failure_sentinel_returns_error", func(t *testing.T) {
		t.Parallel()

		ctx := testContext(t)
		s := setupFileService(t)
		const transferID = "wfp-failsent"
		sentinel, marshalErr := json.Marshal(transfers.DoneInfo{Success: false, Error: "boom"})
		require.NoError(t, marshalErr)
		require.NoError(t, s.storage.Write(ctx, transfers.TransferDonePath(transferID), sentinel))

		errCh := make(chan error, 1)
		available, err := s.service.waitForPartS3(ctx, transferID, 0, errCh)

		require.Error(t, err)
		assert.False(t, available)
		assert.Contains(t, err.Error(), "transfer failed on remote")
	})

	t.Run("success_sentinel_without_part_returns_false", func(t *testing.T) {
		t.Parallel()

		ctx := testContext(t)
		s := setupFileService(t)
		const transferID = "wfp-okdone"
		sentinel, marshalErr := json.Marshal(transfers.DoneInfo{Success: true, TotalParts: 0})
		require.NoError(t, marshalErr)
		require.NoError(t, s.storage.Write(ctx, transfers.TransferDonePath(transferID), sentinel))

		errCh := make(chan error, 1)
		available, err := s.service.waitForPartS3(ctx, transferID, 0, errCh)

		require.NoError(t, err)
		assert.False(t, available, "success sentinel with no further part must report not-available")
	})

	t.Run("malformed_sentinel_returns_error", func(t *testing.T) {
		t.Parallel()

		ctx := testContext(t)
		s := setupFileService(t)
		const transferID = "wfp-badsent"
		require.NoError(t, s.storage.Write(ctx, transfers.TransferDonePath(transferID), []byte("not-json")))

		errCh := make(chan error, 1)
		available, err := s.service.waitForPartS3(ctx, transferID, 0, errCh)

		require.Error(t, err)
		assert.False(t, available)
		assert.Contains(t, err.Error(), "reading transfer sentinel")
	})

	t.Run("context_cancelled_returns_ctx_error", func(t *testing.T) {
		t.Parallel()

		s := setupFileService(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		errCh := make(chan error, 1)
		available, err := s.service.waitForPartS3(ctx, "wfp-cancel", 0, errCh)

		require.Error(t, err)
		assert.False(t, available)
		assert.ErrorIs(t, err, context.Canceled, "cancelled context must surface during polling")
	})
}

func TestFileService_writeSentinelIfMissing(t *testing.T) {
	t.Parallel()

	t.Run("writes_success_sentinel_when_absent", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const transferID = "sentinel-write"

		// ACT
		s.service.writeSentinelIfMissing(ctx, transferID)

		// ASSERT
		got, err := s.service.readSentinel(ctx, transfers.TransferDonePath(transferID))
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.True(t, got.Success, "fallback sentinel must mark the transfer successful")
		assert.Equal(t, 0, got.TotalParts, "fallback sentinel reports zero parts")
	})

	t.Run("does_not_overwrite_existing_sentinel", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const transferID = "sentinel-keep"
		original, marshalErr := json.Marshal(transfers.DoneInfo{Success: false, Error: "original", TotalParts: 5})
		require.NoError(t, marshalErr)
		require.NoError(t, s.storage.Write(ctx, transfers.TransferDonePath(transferID), original))

		// ACT
		s.service.writeSentinelIfMissing(ctx, transferID)

		// ASSERT
		got, err := s.service.readSentinel(ctx, transfers.TransferDonePath(transferID))
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.False(t, got.Success, "existing sentinel must not be overwritten")
		assert.Equal(t, "original", got.Error, "existing sentinel error must be preserved")
		assert.Equal(t, 5, got.TotalParts, "existing sentinel parts count must be preserved")
	})
}

func TestFileService_readSentinel(t *testing.T) {
	t.Parallel()

	t.Run("parses_valid_sentinel", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const path = "transfers/read-ok/done"
		data, marshalErr := json.Marshal(transfers.DoneInfo{Success: true, TotalParts: 3, Checksum: "abc"})
		require.NoError(t, marshalErr)
		require.NoError(t, s.storage.Write(ctx, path, data))

		// ACT
		got, err := s.service.readSentinel(ctx, path)

		// ASSERT
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.True(t, got.Success)
		assert.Equal(t, 3, got.TotalParts)
		assert.Equal(t, "abc", got.Checksum)
	})

	t.Run("missing_file_returns_read_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)

		// ACT
		got, err := s.service.readSentinel(ctx, "transfers/missing/done")

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "read sentinel file")
	})

	t.Run("malformed_json_returns_parse_error", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const path = "transfers/read-bad/done"
		require.NoError(t, s.storage.Write(ctx, path, []byte("{not json")))

		// ACT
		got, err := s.service.readSentinel(ctx, path)

		// ASSERT
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "parse sentinel JSON")
	})
}

func TestChunkedCleanupReadCloser_Close(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const transferID = "cleanup-chunked"
	s.transferReg.Register(transferID)
	require.NoError(t, s.storage.Write(ctx, transfers.TransferPartPath(transferID, 0), []byte("p0")))
	require.NoError(t, s.storage.Write(ctx, transfers.TransferPartPath(transferID, 1), []byte("p1")))

	c := &chunkedCleanupReadCloser{
		ReadCloser:  io.NopCloser(strings.NewReader("")),
		transferID:  transferID,
		storage:     s.storage,
		transferReg: s.transferReg,
		logger:      slog.New(slog.DiscardHandler),
	}

	// ACT
	err := c.Close()

	// ASSERT
	require.NoError(t, err)
	_, stillRegistered := s.transferReg.Get(transferID)
	assert.False(t, stillRegistered, "Close must unregister the transfer")

	remaining, listErr := s.storage.List(ctx, transfers.TransferPrefix(transferID))
	require.NoError(t, listErr)
	assert.Empty(t, remaining, "Close must purge all transfer artifacts from storage")
}

func TestRemoteCleanupReadCloser_Close(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const transferID = "cleanup-remote"
	require.NoError(t, s.storage.Write(ctx, transfers.TransferPartPath(transferID, 0), []byte("p0")))
	require.NoError(t, s.storage.Write(ctx, transfers.TransferDonePath(transferID), []byte("{}")))

	c := &remoteCleanupReadCloser{
		ReadCloser: io.NopCloser(strings.NewReader("")),
		transferID: transferID,
		storage:    s.storage,
		logger:     slog.New(slog.DiscardHandler),
	}

	// ACT
	err := c.Close()

	// ASSERT
	require.NoError(t, err)
	remaining, listErr := s.storage.List(ctx, transfers.TransferPrefix(transferID))
	require.NoError(t, listErr)
	assert.Empty(t, remaining, "Close must purge all remote transfer artifacts from storage")
}

func TestFileService_UploadStream_dispatcherPath(t *testing.T) {
	t.Parallel()

	t.Run("streams_through_storage_and_dispatches_upload_task", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 91
		s.registry.connectedAnywhere[nodeID] = true

		const payload = "remote-upload-payload"
		var capturedTransferID string
		var capturedMode int32
		s.dispatcher.dispatchUploadTask = func(_ context.Context, _ uint64, transferID, _ string, mode int32, _ OwnerOptions) error {
			capturedTransferID = transferID
			capturedMode = mode

			return nil
		}

		// ACT
		err := s.service.UploadStream(
			ctx, testNode(91, "/srv"), "/srv/remote.bin",
			strings.NewReader(payload), uint64(len(payload)), 0o640, OwnerOptions{User: "gameap"},
		)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, int32(1), s.dispatcher.uploadTaskCalls.Load(), "dispatcher upload task must be called once")
		require.NotEmpty(t, capturedTransferID, "a transfer id must be generated")
		assert.Equal(t, int32(0o640), capturedMode, "mode must be masked and propagated to the dispatcher")
		assert.Equal(t, OwnerOptions{User: "gameap"}, s.dispatcher.lastUploadTaskOwner, "owner must propagate")

		data, readErr := s.storage.Read(ctx, transfers.TransferDataPath(capturedTransferID))
		require.NoError(t, readErr)
		assert.Equal(t, payload, string(data), "payload must be staged in storage for the dispatcher to consume")
	})

	t.Run("dispatch_error_cleans_up_storage", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		ctx := testContext(t)
		s := setupFileService(t)
		const nodeID uint64 = 92
		s.registry.connectedAnywhere[nodeID] = true

		var capturedTransferID string
		s.dispatcher.dispatchUploadTask = func(_ context.Context, _ uint64, transferID, _ string, _ int32, _ OwnerOptions) error {
			capturedTransferID = transferID

			return errors.New("dispatch boom")
		}

		// ACT
		err := s.service.UploadStream(
			ctx, testNode(92, "/srv"), "/srv/remote.bin",
			strings.NewReader("payload"), uint64(len("payload")), 0o644, OwnerOptions{},
		)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dispatched upload task", "error must be wrapped with the dispatch context")
		assert.Contains(t, err.Error(), "dispatch boom", "original cause must be preserved")
		assert.False(t, s.storage.Exists(ctx, transfers.TransferDataPath(capturedTransferID)),
			"staged storage object must be deleted when the dispatch fails")
	})
}
