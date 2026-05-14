package upload_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errFakeDaemonDown = errors.New("fake daemon down")

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time { return f.now }

type fakeDaemon struct {
	mu        sync.Mutex
	calls     []daemonCall
	returnErr error
	block     bool
}

type daemonCall struct {
	nodeID     uint
	fullPath   string
	transferID string
	checksum   string
	totalSize  uint64
}

func (f *fakeDaemon) UploadStreamPrepared(
	ctx context.Context,
	node *domain.Node,
	fullPath, transferID, checksum string,
	totalSize uint64,
	_ daemon.OwnerOptions,
) error {
	if f.block {
		<-ctx.Done()

		return ctx.Err()
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, daemonCall{
		nodeID:     node.ID,
		fullPath:   fullPath,
		transferID: transferID,
		checksum:   checksum,
		totalSize:  totalSize,
	})

	return f.returnErr
}

const (
	testServerID = uint(7)
	testNodeID   = uint(3)
	testUserID   = uint(11)
	testFullPath = "/srv/gameap/servers/cs/configs/big.bin"
)

func defaultConfig() upload.Config {
	return upload.Config{
		ChunkSize:             4,
		SessionTTL:            time.Hour,
		MaxChunks:             1000,
		DaemonDispatchTimeout: time.Second,
	}
}

// newTestSetup wires the service against an in-memory storage and a fake
// daemon. Tests asserting on time-based behaviour mutate clock.now between
// Arrange and Act.
func newTestSetup(t *testing.T) (*upload.Service, *files.InMemoryFileManager, *fakeDaemon, *fakeClock) {
	t.Helper()

	return newTestSetupWithConfig(t, defaultConfig())
}

func newTestSetupWithConfig(
	t *testing.T,
	cfg upload.Config,
) (*upload.Service, *files.InMemoryFileManager, *fakeDaemon, *fakeClock) {
	t.Helper()
	storage := files.NewInMemoryFileManager()
	daemon := &fakeDaemon{}
	clock := &fakeClock{now: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)}
	svc := upload.NewService(storage, daemon, clock, nil, cfg)

	return svc, storage, daemon, clock
}

// newServiceOnly is a thin convenience for tests that only care about the
// Service handle and avoid the dogsled lint trigger of `_, _, _, _`.
func newServiceOnly(t *testing.T) *upload.Service {
	t.Helper()
	svc, _, _, _ := newTestSetup(t) //nolint:dogsled // 4 returns from setup, only svc needed

	return svc
}

func sha256Hex(t *testing.T, data []byte) string {
	t.Helper()
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

func makeNode() *domain.Node {
	return &domain.Node{ID: testNodeID, WorkPath: "/srv/gameap"}
}

// TestService_Complete_LocalStorage exercises the upload service against a real
// LocalFileManager. Regression coverage for a bug where Storage.List returns
// bare filenames on Local/S3 (e.g. "000000") but indexFromChunkPath only
// accepted full paths ("transfers/<id>/chunks/000000"), so receivedChunks was
// always empty and Complete unconditionally returned ErrIncompleteUpload. The
// in-memory tests miss this because InMemoryFileManager.List returns full
// recursive keys.
func TestService_Complete_LocalStorage(t *testing.T) {
	payload := []byte("0123456789")
	checksum := sha256Hex(t, payload)

	storage := files.NewLocalFileManager(t.TempDir())
	daemon := &fakeDaemon{}
	clock := &fakeClock{now: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)}
	svc := upload.NewService(storage, daemon, clock, nil, defaultConfig())

	sess, err := svc.Create(context.Background(), upload.CreateParams{
		ServerID:         testServerID,
		NodeID:           testNodeID,
		UserID:           testUserID,
		FullPath:         testFullPath,
		TotalSize:        uint64(len(payload)),
		ExpectedChecksum: checksum,
	})
	require.NoError(t, err)

	require.NoError(t, svc.WriteChunk(context.Background(), sess.UploadID, testUserID, 0, bytes.NewReader(payload[0:4])))
	require.NoError(t, svc.WriteChunk(context.Background(), sess.UploadID, testUserID, 1, bytes.NewReader(payload[4:8])))
	require.NoError(t, svc.WriteChunk(context.Background(), sess.UploadID, testUserID, 2, bytes.NewReader(payload[8:10])))

	require.NoError(t, svc.Complete(context.Background(), sess.UploadID, testUserID, makeNode()))

	require.Len(t, daemon.calls, 1)
	assert.Equal(t, sess.UploadID, daemon.calls[0].transferID)
	assert.Equal(t, checksum, daemon.calls[0].checksum)
}

// TestService_Status_LocalStorage covers GET /sessions/{id} on Local storage.
// Same root cause as Complete_LocalStorage: receivedChunks must accept bare
// filenames produced by LocalFileManager.List. Without the fix, the response
// reports zero received_chunks and a full missing list, defeating client-side
// resume.
func TestService_Status_LocalStorage(t *testing.T) {
	payload := []byte("0123456789")
	checksum := sha256Hex(t, payload)

	storage := files.NewLocalFileManager(t.TempDir())
	daemon := &fakeDaemon{}
	clock := &fakeClock{now: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)}
	svc := upload.NewService(storage, daemon, clock, nil, defaultConfig())

	sess, err := svc.Create(context.Background(), upload.CreateParams{
		ServerID:         testServerID,
		NodeID:           testNodeID,
		UserID:           testUserID,
		FullPath:         testFullPath,
		TotalSize:        uint64(len(payload)),
		ExpectedChecksum: checksum,
	})
	require.NoError(t, err)

	require.NoError(t, svc.WriteChunk(context.Background(), sess.UploadID, testUserID, 0, bytes.NewReader(payload[0:4])))
	require.NoError(t, svc.WriteChunk(context.Background(), sess.UploadID, testUserID, 2, bytes.NewReader(payload[8:10])))

	status, err := svc.Status(context.Background(), sess.UploadID, testUserID)
	require.NoError(t, err)
	assert.Equal(t, []uint{0, 2}, status.ReceivedChunks)
	assert.Equal(t, []uint{1}, status.MissingChunks)
	assert.False(t, status.Completed)
}

func TestService_Create(t *testing.T) {
	tests := []struct {
		name      string
		cfg       upload.Config
		params    func(checksum string) upload.CreateParams
		wantError string
		check     func(t *testing.T, sess *upload.Session, storage *files.InMemoryFileManager)
	}{
		{
			name: "writes_metadata_and_computes_total_chunks",
			cfg:  defaultConfig(),
			params: func(c string) upload.CreateParams {
				return upload.CreateParams{
					ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
					FullPath: testFullPath, TotalSize: 10, ExpectedChecksum: c,
				}
			},
			check: func(t *testing.T, sess *upload.Session, storage *files.InMemoryFileManager) {
				t.Helper()
				assert.Equal(t, uint(3), sess.TotalChunks, "10 bytes / 4-byte chunks = 3 chunks")
				assert.Equal(t, uint64(4), sess.ChunkSize)
				assert.Equal(t, uint64(10), sess.TotalSize)
				assert.Equal(t, testFullPath, sess.FullPath)
				assert.Equal(t, testUserID, sess.UserID)
				assert.Equal(t, testNodeID, sess.NodeID)
				assert.Equal(t, testServerID, sess.ServerID)
				assert.NotEmpty(t, sess.UploadID)

				raw, err := storage.Read(context.Background(), "transfers/"+sess.UploadID+"/upload.json")
				require.NoError(t, err, "metadata file must be persisted")

				var stored upload.Session
				require.NoError(t, json.Unmarshal(raw, &stored), "metadata must be valid JSON")
				assert.Equal(t, sess.UploadID, stored.UploadID, "persisted metadata reflects returned session")
				assert.Equal(t, sess.ExpectedChecksum, stored.ExpectedChecksum)
				assert.WithinDuration(t, sess.CreatedAt, stored.CreatedAt, time.Second)
			},
		},
		{
			name: "lowercases_uppercase_checksum",
			cfg:  defaultConfig(),
			params: func(string) upload.CreateParams {
				return upload.CreateParams{
					ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
					FullPath:         testFullPath,
					TotalSize:        10,
					ExpectedChecksum: strings.Repeat("A", 64),
				}
			},
			check: func(t *testing.T, sess *upload.Session, _ *files.InMemoryFileManager) {
				t.Helper()
				assert.Equal(t, strings.Repeat("a", 64), sess.ExpectedChecksum,
					"checksum must be normalised to lowercase for stable comparison")
			},
		},
		{
			name: "rejects_zero_total_size",
			cfg:  defaultConfig(),
			params: func(c string) upload.CreateParams {
				return upload.CreateParams{
					ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
					FullPath: testFullPath, TotalSize: 0, ExpectedChecksum: c,
				}
			},
			wantError: "total_size must be positive",
		},
		{
			name: "rejects_short_checksum",
			cfg:  defaultConfig(),
			params: func(string) upload.CreateParams {
				return upload.CreateParams{
					ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
					FullPath: testFullPath, TotalSize: 10, ExpectedChecksum: "deadbeef",
				}
			},
			wantError: "expected_checksum must be 64 lowercase hex characters",
		},
		{
			name: "rejects_non_hex_checksum",
			cfg:  defaultConfig(),
			params: func(string) upload.CreateParams {
				return upload.CreateParams{
					ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
					FullPath: testFullPath, TotalSize: 10,
					ExpectedChecksum: strings.Repeat("z", 64),
				}
			},
			wantError: "expected_checksum must be 64 lowercase hex characters",
		},
		{
			name: "rejects_too_many_chunks",
			cfg:  upload.Config{ChunkSize: 4, SessionTTL: time.Hour, MaxChunks: 5},
			params: func(c string) upload.CreateParams {
				return upload.CreateParams{
					ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
					FullPath: testFullPath, TotalSize: 10_000_000, ExpectedChecksum: c,
				}
			},
			wantError: "too many chunks",
		},
		{
			name: "rejects_when_chunk_size_zero",
			cfg:  upload.Config{ChunkSize: 0, SessionTTL: time.Hour, MaxChunks: 1000},
			params: func(c string) upload.CreateParams {
				return upload.CreateParams{
					ServerID: testServerID, NodeID: testNodeID, UserID: testUserID,
					FullPath: testFullPath, TotalSize: 10, ExpectedChecksum: c,
				}
			},
			wantError: "chunk size is not configured",
		},
	}

	checksum := sha256Hex(t, []byte("payload10b"))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			svc, storage, _, _ := newTestSetupWithConfig(t, tt.cfg)

			// ACT
			sess, err := svc.Create(context.Background(), tt.params(checksum))

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Nil(t, sess, "no session returned on error")

				return
			}
			require.NoError(t, err)
			require.NotNil(t, sess)
			if tt.check != nil {
				tt.check(t, sess, storage)
			}
		})
	}
}

func createSession(
	t *testing.T,
	svc *upload.Service,
	totalSize uint64,
	checksum string,
) *upload.Session {
	t.Helper()
	sess, err := svc.Create(context.Background(), upload.CreateParams{
		ServerID:         testServerID,
		NodeID:           testNodeID,
		UserID:           testUserID,
		FullPath:         testFullPath,
		TotalSize:        totalSize,
		ExpectedChecksum: checksum,
	})
	require.NoError(t, err)

	return sess
}

func TestService_WriteChunk(t *testing.T) {
	payload := []byte("0123456789") // 10 bytes, with chunkSize=4 => 3 chunks: 4,4,2
	checksum := sha256Hex(t, payload)

	tests := []struct {
		name      string
		setup     func(t *testing.T, svc *upload.Service) (uploadID string)
		userID    uint
		index     uint
		body      []byte
		wantError string
		check     func(t *testing.T, storage *files.InMemoryFileManager, uploadID string)
	}{
		{
			name: "stores_chunk_at_expected_path",
			setup: func(t *testing.T, svc *upload.Service) string {
				t.Helper()

				return createSession(t, svc, 10, checksum).UploadID
			},
			userID: testUserID,
			index:  0,
			body:   payload[0:4],
			check: func(t *testing.T, storage *files.InMemoryFileManager, uploadID string) {
				t.Helper()
				stored, err := storage.Read(context.Background(), "transfers/"+uploadID+"/chunks/000000")
				require.NoError(t, err)
				assert.Equal(t, payload[0:4], stored)
			},
		},
		{
			name: "stores_smaller_last_chunk",
			setup: func(t *testing.T, svc *upload.Service) string {
				t.Helper()

				return createSession(t, svc, 10, checksum).UploadID
			},
			userID: testUserID,
			index:  2,
			body:   payload[8:10],
			check: func(t *testing.T, storage *files.InMemoryFileManager, uploadID string) {
				t.Helper()
				stored, err := storage.Read(context.Background(), "transfers/"+uploadID+"/chunks/000002")
				require.NoError(t, err)
				assert.Equal(t, payload[8:10], stored)
			},
		},
		{
			name: "rejects_oversized_chunk",
			setup: func(t *testing.T, svc *upload.Service) string {
				t.Helper()

				return createSession(t, svc, 10, checksum).UploadID
			},
			userID:    testUserID,
			index:     0,
			body:      []byte("12345"),
			wantError: "chunk size does not match expected size",
			check: func(t *testing.T, storage *files.InMemoryFileManager, uploadID string) {
				t.Helper()
				assert.False(t, storage.Exists(context.Background(), "transfers/"+uploadID+"/chunks/000000"),
					"oversized chunk must not be persisted")
			},
		},
		{
			name: "rejects_undersized_non_terminal_chunk",
			setup: func(t *testing.T, svc *upload.Service) string {
				t.Helper()

				return createSession(t, svc, 10, checksum).UploadID
			},
			userID:    testUserID,
			index:     0,
			body:      []byte("12"),
			wantError: "chunk size does not match expected size",
		},
		{
			name: "rejects_index_out_of_range_far",
			setup: func(t *testing.T, svc *upload.Service) string {
				t.Helper()

				return createSession(t, svc, 10, checksum).UploadID
			},
			userID:    testUserID,
			index:     99,
			body:      payload[0:4],
			wantError: "chunk index out of range",
		},
		{
			name: "rejects_index_at_total_chunks_boundary",
			setup: func(t *testing.T, svc *upload.Service) string {
				t.Helper()

				// 10 bytes / 4-byte chunks = 3 chunks (indices 0..2). Index=3 must reject.
				return createSession(t, svc, 10, checksum).UploadID
			},
			userID:    testUserID,
			index:     3,
			body:      payload[0:4],
			wantError: "chunk index out of range",
		},
		{
			name: "rejects_other_user",
			setup: func(t *testing.T, svc *upload.Service) string {
				t.Helper()

				return createSession(t, svc, 10, checksum).UploadID
			},
			userID:    999,
			index:     0,
			body:      payload[0:4],
			wantError: "belongs to another user",
		},
		{
			name: "rejects_unknown_session",
			setup: func(*testing.T, *upload.Service) string {
				return "nonexistent"
			},
			userID:    testUserID,
			index:     0,
			body:      payload[0:4],
			wantError: "upload session not found",
		},
		{
			name: "idempotent_on_same_index",
			setup: func(t *testing.T, svc *upload.Service) string {
				t.Helper()
				uploadID := createSession(t, svc, 10, checksum).UploadID
				require.NoError(t, svc.WriteChunk(
					context.Background(), uploadID, testUserID, 0, bytes.NewReader(payload[0:4]),
				))

				return uploadID
			},
			userID: testUserID,
			index:  0,
			body:   payload[0:4],
			check: func(t *testing.T, storage *files.InMemoryFileManager, uploadID string) {
				t.Helper()
				stored, err := storage.Read(context.Background(), "transfers/"+uploadID+"/chunks/000000")
				require.NoError(t, err)
				assert.Equal(t, payload[0:4], stored)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			svc, storage, _, _ := newTestSetup(t)
			uploadID := tt.setup(t, svc)

			// ACT
			err := svc.WriteChunk(
				context.Background(), uploadID, tt.userID, tt.index, bytes.NewReader(tt.body),
			)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			} else {
				require.NoError(t, err)
			}
			if tt.check != nil {
				tt.check(t, storage, uploadID)
			}
		})
	}
}

func TestService_WriteChunk_RejectsExpiredSession(t *testing.T) {
	// ARRANGE
	payload := []byte("0123456789")
	checksum := sha256Hex(t, payload)
	svc, _, _, clock := newTestSetup(t)
	uploadID := createSession(t, svc, 10, checksum).UploadID
	clock.now = clock.now.Add(2 * time.Hour) // past ExpiresAt = createdAt + 1h

	// ACT
	err := svc.WriteChunk(context.Background(), uploadID, testUserID, 0, bytes.NewReader(payload[0:4]))

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestService_WriteChunk_RejectsAfterCompletion(t *testing.T) {
	// ARRANGE — happy-path Complete writes the done sentinel.
	payload := []byte("0123456789")
	checksum := sha256Hex(t, payload)
	svc := newServiceOnly(t)
	uploadID := uploadAllChunks(t, svc, payload, checksum)
	require.NoError(t, svc.Complete(context.Background(), uploadID, testUserID, makeNode()))

	// ACT — try to write any chunk after completion.
	err := svc.WriteChunk(context.Background(), uploadID, testUserID, 0, bytes.NewReader(payload[0:4]))

	// ASSERT
	require.Error(t, err)
	assert.True(t, errors.Is(err, upload.ErrSessionAlreadyDone),
		"expected ErrSessionAlreadyDone, got %v", err)
}

func TestService_Status(t *testing.T) {
	payload := []byte("0123456789")
	checksum := sha256Hex(t, payload)

	t.Run("returns_received_and_missing_chunks", func(t *testing.T) {
		// ARRANGE
		svc, _, _, _ := newTestSetup(t)
		uploadID := createSession(t, svc, 10, checksum).UploadID
		require.NoError(t, svc.WriteChunk(
			context.Background(), uploadID, testUserID, 0, bytes.NewReader(payload[0:4]),
		))
		require.NoError(t, svc.WriteChunk(
			context.Background(), uploadID, testUserID, 2, bytes.NewReader(payload[8:10]),
		))

		// ACT
		status, err := svc.Status(context.Background(), uploadID, testUserID)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, []uint{0, 2}, status.ReceivedChunks)
		assert.Equal(t, []uint{1}, status.MissingChunks)
		assert.False(t, status.Completed)
		assert.Equal(t, uint64(6), status.UploadedBytes, "4 (chunk 0) + 2 (chunk 2) = 6 bytes")
	})

	t.Run("returns_empty_lists_when_no_chunks_uploaded", func(t *testing.T) {
		// ARRANGE
		svc, _, _, _ := newTestSetup(t)
		uploadID := createSession(t, svc, 10, checksum).UploadID

		// ACT
		status, err := svc.Status(context.Background(), uploadID, testUserID)

		// ASSERT
		require.NoError(t, err)
		assert.NotNil(t, status.ReceivedChunks, "ReceivedChunks must be a non-nil empty slice")
		assert.Empty(t, status.ReceivedChunks)
		assert.Equal(t, []uint{0, 1, 2}, status.MissingChunks, "all 3 chunks reported missing")
		assert.Equal(t, uint64(0), status.UploadedBytes)
		assert.False(t, status.Completed)
	})

	t.Run("marks_completed_when_done_present", func(t *testing.T) {
		// ARRANGE
		svc, storage, daemon, _ := newTestSetup(t)
		uploadID := uploadAllChunks(t, svc, payload, checksum)
		require.NoError(t, svc.Complete(context.Background(), uploadID, testUserID, makeNode()))
		require.Len(t, daemon.calls, 1)

		// ACT
		status, err := svc.Status(context.Background(), uploadID, testUserID)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, status.Completed)
		assert.True(t, storage.Exists(context.Background(), "transfers/"+uploadID+"/done"))
	})

	t.Run("rejects_other_user", func(t *testing.T) {
		// ARRANGE
		svc, _, _, _ := newTestSetup(t)
		uploadID := createSession(t, svc, 10, checksum).UploadID

		// ACT
		_, err := svc.Status(context.Background(), uploadID, 999)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "belongs to another user")
	})

	t.Run("rejects_unknown_session", func(t *testing.T) {
		// ARRANGE
		svc, _, _, _ := newTestSetup(t)

		// ACT
		_, err := svc.Status(context.Background(), "nope", testUserID)

		// ASSERT
		require.Error(t, err)
		assert.True(t, errors.Is(err, upload.ErrSessionNotFound))
	})
}

func uploadAllChunks(t *testing.T, svc *upload.Service, payload []byte, checksum string) string {
	t.Helper()
	uploadID := createSession(t, svc, uint64(len(payload)), checksum).UploadID
	require.NoError(t, svc.WriteChunk(context.Background(), uploadID, testUserID, 0, bytes.NewReader(payload[0:4])))
	require.NoError(t, svc.WriteChunk(context.Background(), uploadID, testUserID, 1, bytes.NewReader(payload[4:8])))
	require.NoError(t, svc.WriteChunk(context.Background(), uploadID, testUserID, 2, bytes.NewReader(payload[8:10])))

	return uploadID
}

func TestService_Complete(t *testing.T) {
	payload := []byte("0123456789")
	checksum := sha256Hex(t, payload)

	t.Run("assembles_and_dispatches_when_checksum_matches", func(t *testing.T) {
		// ARRANGE
		svc, storage, daemon, _ := newTestSetup(t)
		uploadID := uploadAllChunks(t, svc, payload, checksum)

		// ACT
		require.NoError(t, svc.Complete(context.Background(), uploadID, testUserID, makeNode()))

		// ASSERT
		require.Len(t, daemon.calls, 1)
		assert.Equal(t, uploadID, daemon.calls[0].transferID)
		assert.Equal(t, testFullPath, daemon.calls[0].fullPath)
		assert.Equal(t, checksum, daemon.calls[0].checksum)
		assert.Equal(t, uint64(10), daemon.calls[0].totalSize)
		assert.Equal(t, testNodeID, daemon.calls[0].nodeID)

		assertDoneSentinel(t, storage, uploadID, true, checksum)
		waitForCleanup(t, storage, uploadID)
	})

	t.Run("idempotent_when_done_present", func(t *testing.T) {
		// ARRANGE
		svc, storage, daemon, _ := newTestSetup(t)
		uploadID := uploadAllChunks(t, svc, payload, checksum)
		require.NoError(t, svc.Complete(context.Background(), uploadID, testUserID, makeNode()))
		waitForCleanup(t, storage, uploadID)

		// ACT — second Complete must be a no-op.
		require.NoError(t, svc.Complete(context.Background(), uploadID, testUserID, makeNode()))

		// ASSERT
		assert.Len(t, daemon.calls, 1, "daemon called only on first Complete")
	})

	t.Run("returns_unprocessable_on_checksum_mismatch_and_keeps_chunks", func(t *testing.T) {
		// ARRANGE — wrongChecksum at Create makes the assembled hash mismatch.
		svc, storage, daemon, _ := newTestSetup(t)
		wrongChecksum := strings.Repeat("a", 64)
		sess := createSession(t, svc, 10, wrongChecksum)
		require.NoError(t, svc.WriteChunk(
			context.Background(), sess.UploadID, testUserID, 0, bytes.NewReader(payload[0:4]),
		))
		require.NoError(t, svc.WriteChunk(
			context.Background(), sess.UploadID, testUserID, 1, bytes.NewReader(payload[4:8]),
		))
		require.NoError(t, svc.WriteChunk(
			context.Background(), sess.UploadID, testUserID, 2, bytes.NewReader(payload[8:10]),
		))

		// ACT
		err := svc.Complete(context.Background(), sess.UploadID, testUserID, makeNode())

		// ASSERT
		require.Error(t, err)
		assert.True(t, errors.Is(err, upload.ErrChecksumMismatch))
		assert.Empty(t, daemon.calls)
		assert.False(t, storage.Exists(context.Background(), "transfers/"+sess.UploadID+"/done"))
		assert.False(t, storage.Exists(context.Background(), "transfers/"+sess.UploadID+"/data"))
		assert.True(t, storage.Exists(context.Background(), "transfers/"+sess.UploadID+"/chunks/000000"),
			"chunks must remain so client can retry from corrupted ones")
	})

	t.Run("keeps_chunks_when_daemon_dispatch_returns_error", func(t *testing.T) {
		// ARRANGE
		svc, storage, daemon, _ := newTestSetup(t)
		daemon.returnErr = errFakeDaemonDown
		uploadID := uploadAllChunks(t, svc, payload, checksum)

		// ACT
		err := svc.Complete(context.Background(), uploadID, testUserID, makeNode())

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fake daemon down")
		assert.False(t, storage.Exists(context.Background(), "transfers/"+uploadID+"/done"))
		assert.False(t, storage.Exists(context.Background(), "transfers/"+uploadID+"/data"))
		assert.True(t, storage.Exists(context.Background(), "transfers/"+uploadID+"/chunks/000000"))
	})

	t.Run("returns_when_daemon_dispatch_times_out", func(t *testing.T) {
		// ARRANGE
		cfg := defaultConfig()
		cfg.DaemonDispatchTimeout = 20 * time.Millisecond
		svc, storage, daemon, _ := newTestSetupWithConfig(t, cfg)
		daemon.block = true
		uploadID := uploadAllChunks(t, svc, payload, checksum)

		// ACT
		err := svc.Complete(context.Background(), uploadID, testUserID, makeNode())

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dispatch upload to daemon")
		assert.Contains(t, err.Error(), context.DeadlineExceeded.Error())
		assert.False(t, storage.Exists(context.Background(), "transfers/"+uploadID+"/done"))
		assert.False(t, storage.Exists(context.Background(), "transfers/"+uploadID+"/data"))
		assert.True(t, storage.Exists(context.Background(), "transfers/"+uploadID+"/chunks/000000"))
	})

	t.Run("returns_incomplete_when_chunk_missing", func(t *testing.T) {
		// ARRANGE
		svc, _, daemon, _ := newTestSetup(t)
		uploadID := createSession(t, svc, 10, checksum).UploadID
		require.NoError(t, svc.WriteChunk(
			context.Background(), uploadID, testUserID, 0, bytes.NewReader(payload[0:4]),
		))

		// ACT
		err := svc.Complete(context.Background(), uploadID, testUserID, makeNode())

		// ASSERT
		require.Error(t, err)
		assert.True(t, errors.Is(err, upload.ErrIncompleteUpload))
		assert.Empty(t, daemon.calls)
	})

	t.Run("rejects_node_mismatch", func(t *testing.T) {
		// ARRANGE
		svc, _, _, _ := newTestSetup(t)
		uploadID := uploadAllChunks(t, svc, payload, checksum)

		// ACT — Complete with a node that doesn't own this session.
		err := svc.Complete(context.Background(), uploadID, testUserID, &domain.Node{ID: 999})

		// ASSERT
		require.Error(t, err)
		assert.True(t, errors.Is(err, upload.ErrNodeMismatch))
	})

	t.Run("rejects_nil_node", func(t *testing.T) {
		// ARRANGE
		svc, _, _, _ := newTestSetup(t)
		uploadID := uploadAllChunks(t, svc, payload, checksum)

		// ACT
		err := svc.Complete(context.Background(), uploadID, testUserID, nil)

		// ASSERT
		require.Error(t, err)
		assert.True(t, errors.Is(err, upload.ErrNodeMismatch))
	})

	t.Run("rejects_other_user", func(t *testing.T) {
		// ARRANGE
		svc, _, _, _ := newTestSetup(t)
		uploadID := uploadAllChunks(t, svc, payload, checksum)

		// ACT
		err := svc.Complete(context.Background(), uploadID, 999, makeNode())

		// ASSERT
		require.Error(t, err)
		assert.True(t, errors.Is(err, upload.ErrSessionForbidden))
	})
}

func TestService_Abort(t *testing.T) {
	payload := []byte("0123456789")
	checksum := sha256Hex(t, payload)

	t.Run("removes_full_transfer_prefix", func(t *testing.T) {
		// ARRANGE
		svc, storage, _, _ := newTestSetup(t)
		uploadID := createSession(t, svc, 10, checksum).UploadID
		require.NoError(t, svc.WriteChunk(
			context.Background(), uploadID, testUserID, 0, bytes.NewReader(payload[0:4]),
		))

		// ACT
		require.NoError(t, svc.Abort(context.Background(), uploadID, testUserID))

		// ASSERT
		assert.False(t, storage.Exists(context.Background(), "transfers/"+uploadID+"/upload.json"))
		assert.False(t, storage.Exists(context.Background(), "transfers/"+uploadID+"/chunks/000000"))
	})

	t.Run("returns_nil_when_session_not_found", func(t *testing.T) {
		// ARRANGE
		svc, _, _, _ := newTestSetup(t)

		// ACT
		err := svc.Abort(context.Background(), "missing", testUserID)

		// ASSERT
		require.NoError(t, err, "Abort is idempotent — missing session is not an error")
	})

	t.Run("idempotent_when_called_after_abort", func(t *testing.T) {
		// ARRANGE
		svc, _, _, _ := newTestSetup(t)
		uploadID := createSession(t, svc, 10, checksum).UploadID
		require.NoError(t, svc.Abort(context.Background(), uploadID, testUserID))

		// ACT — second Abort hits the missing-session branch.
		err := svc.Abort(context.Background(), uploadID, testUserID)

		// ASSERT
		require.NoError(t, err)
	})

	t.Run("rejects_other_user", func(t *testing.T) {
		// ARRANGE
		svc, _, _, _ := newTestSetup(t)
		uploadID := createSession(t, svc, 10, checksum).UploadID

		// ACT
		err := svc.Abort(context.Background(), uploadID, 999)

		// ASSERT
		require.Error(t, err)
		assert.True(t, errors.Is(err, upload.ErrSessionForbidden))
	})
}

func TestService_WriteChunk_ParallelDistinctIndices(t *testing.T) {
	// ARRANGE
	payload := bytes.Repeat([]byte("X"), 40)
	checksum := sha256Hex(t, payload)
	svc, storage, _, _ := newTestSetup(t)
	uploadID := createSession(t, svc, 40, checksum).UploadID

	// ACT — 10 chunks written concurrently from distinct goroutines.
	const totalChunks = 10
	var wg sync.WaitGroup
	errs := make(chan error, totalChunks)
	for i := range totalChunks {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			start := idx * 4
			errs <- svc.WriteChunk(
				context.Background(), uploadID, testUserID, uint(idx),
				bytes.NewReader(payload[start:start+4]),
			)
		}(i)
	}
	wg.Wait()
	close(errs)

	// ASSERT
	for err := range errs {
		require.NoError(t, err)
	}
	for i := range totalChunks {
		path := fmt.Sprintf("transfers/%s/chunks/%06d", uploadID, i)
		assert.True(t, storage.Exists(context.Background(), path), "chunk %d missing", i)
	}
}

// assertDoneSentinel waits up to 1s for the done file to appear (Complete
// writes it synchronously but the cleanup goroutine may race the read in
// principle), then asserts its JSON contents.
func assertDoneSentinel(
	t *testing.T,
	storage *files.InMemoryFileManager,
	uploadID string,
	wantSuccess bool,
	wantChecksum string,
) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if storage.Exists(context.Background(), "transfers/"+uploadID+"/done") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	raw, err := storage.Read(context.Background(), "transfers/"+uploadID+"/done")
	require.NoError(t, err)
	var info upload.DoneInfo
	require.NoError(t, json.Unmarshal(raw, &info))
	assert.Equal(t, wantSuccess, info.Success)
	assert.Equal(t, wantChecksum, info.Checksum)
}

// waitForCleanup waits for Service.cleanupAfterComplete (a fire-and-forget
// goroutine) to remove the chunk files. Polling is the only option because the
// cleanup is detached from the Complete return value by design.
func waitForCleanup(t *testing.T, storage *files.InMemoryFileManager, uploadID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	chunkPath := "transfers/" + uploadID + "/chunks/000000"
	for time.Now().Before(deadline) {
		if !storage.Exists(context.Background(), chunkPath) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("chunks not cleaned up after Complete: %s still exists", chunkPath)
}
