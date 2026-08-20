package filetransfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/pubsub/messages"
	"github.com/gameap/gameap/internal/transfers"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	errTestRecvFailure = errors.New("simulated recv failure")
	errTestSendFailure = errors.New("simulated send failure")
)

const testTransferID = "transfer-test-1"

// uploadStream implements proto.FileTransferService_UploadFileServer for tests.
// The production UploadFile handler only calls Context, Recv, and SendAndClose;
// SetHeader/SendHeader/SetTrailer/SendMsg/RecvMsg are never invoked, so
// embedding grpc.ServerStream as a nil interface is safe here.
type uploadStream struct {
	grpc.ServerStream

	ctx     context.Context //nolint:containedctx // test stub
	chunks  chan *proto.UploadChunk
	recvErr error

	mu       sync.Mutex
	closeRes *proto.UploadResult
	sendErr  error
}

func newUploadStream(ctx context.Context) *uploadStream {
	return &uploadStream{
		ctx:    ctx,
		chunks: make(chan *proto.UploadChunk, 8),
	}
}

func (s *uploadStream) Context() context.Context { return s.ctx }

func (s *uploadStream) Recv() (*proto.UploadChunk, error) {
	if s.recvErr != nil {
		return nil, s.recvErr
	}
	chunk, ok := <-s.chunks
	if !ok {
		return nil, io.EOF
	}

	return chunk, nil
}

func (s *uploadStream) SendAndClose(res *proto.UploadResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sendErr != nil {
		return s.sendErr
	}
	s.closeRes = res

	return nil
}

func (s *uploadStream) sendChunk(c *proto.UploadChunk) { s.chunks <- c }
func (s *uploadStream) closeStream()                   { close(s.chunks) }

func (s *uploadStream) lastResponse() *proto.UploadResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.closeRes
}

// downloadStream implements proto.FileTransferService_DownloadFileServer.
type downloadStream struct {
	grpc.ServerStream

	ctx context.Context //nolint:containedctx // test stub

	mu      sync.Mutex
	sent    []*proto.DownloadChunk
	sendErr error
}

func newDownloadStream(ctx context.Context) *downloadStream {
	return &downloadStream{ctx: ctx}
}

func (s *downloadStream) Context() context.Context { return s.ctx }

func (s *downloadStream) Send(c *proto.DownloadChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sendErr != nil {
		return s.sendErr
	}
	// Production gRPC marshals the message immediately, so reuse of the
	// underlying byte buffer by the caller is safe. The in-process stub must
	// copy to mimic that contract — otherwise later Reads into the same buffer
	// would mutate previously sent chunks.
	cp := &proto.DownloadChunk{
		Data:           append([]byte(nil), c.GetData()...),
		Offset:         c.GetOffset(),
		TotalSize:      c.GetTotalSize(),
		ChecksumSha256: c.GetChecksumSha256(),
		IsFinal:        c.GetIsFinal(),
	}
	s.sent = append(s.sent, cp)

	return nil
}

func (s *downloadStream) chunks() []*proto.DownloadChunk {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*proto.DownloadChunk, len(s.sent))
	copy(out, s.sent)

	return out
}

// failingPublisher always errors on Publish; used to verify the service does
// not surface publish failures.
type failingPublisher struct{}

func (failingPublisher) Publish(_ context.Context, _ string, _ *pubsub.Message) error {
	return errTestSendFailure
}

// eofTogetherReader returns the entire payload in a single Read call together
// with io.EOF, mimicking storage backends (e.g. S3 GetObject) that signal end
// of stream alongside the last bytes. The default bytes.Reader does NOT do
// this; tests that need to exercise the production "is_final" / checksum tail
// path must use this reader.
type eofTogetherReader struct {
	data []byte
	read bool
}

func (r *eofTogetherReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.read = true

	return n, io.EOF
}

func (*eofTogetherReader) Close() error { return nil }

// eofTogetherStorage wraps an InMemoryFileManager and returns eofTogetherReader
// from ReadStream. Other methods delegate to the wrapped manager.
type eofTogetherStorage struct {
	*files.InMemoryFileManager
}

func (s eofTogetherStorage) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	data, err := s.Read(ctx, path)
	if err != nil {
		return nil, err
	}

	return &eofTogetherReader{data: data}, nil
}

// testService bundles a Service together with the in-memory dependencies it
// drives, so each test can inspect them after the call returns.
type testService struct {
	svc     *Service
	storage *files.InMemoryFileManager
	bus     *memory.Memory
	reg     *transfers.Registry
}

func newTestService(t *testing.T) *testService {
	t.Helper()

	storage := files.NewInMemoryFileManager()
	bus := memory.New()
	t.Cleanup(func() { _ = bus.Close() })

	reg := transfers.NewRegistry()
	logger := slog.New(slog.DiscardHandler)

	return &testService{
		svc:     NewService(storage, bus, reg, logger),
		storage: storage,
		bus:     bus,
		reg:     reg,
	}
}

// computeSHA256 is a tiny helper so tests can ask "what should the checksum
// of this payload be?" instead of duplicating the hashing dance.
func computeSHA256(data []byte) string {
	h := sha256.Sum256(data)

	return hex.EncodeToString(h[:])
}

func runUpload(t *testing.T, svc *Service, stream *uploadStream) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- svc.UploadFile(stream) }()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not return within 5s")

		return nil
	}
}

func TestService_UploadFile_small_payload_is_persisted_to_storage(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	payload := []byte("hello, world")
	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId:     testTransferID,
			Path:           "/dest/file.txt",
			TotalSize:      int64(len(payload)),
			ChecksumSha256: computeSHA256(payload),
		},
		Data: payload,
	})
	stream.closeStream()

	// ACT
	err := runUpload(t, ts.svc, stream)

	// ASSERT
	require.NoError(t, err)

	stored, readErr := ts.storage.Read(context.Background(), transfers.TransferPartPath(testTransferID, 0))
	require.NoError(t, readErr)
	assert.Equal(t, payload, stored, "first part bytes must equal sent payload")

	res := stream.lastResponse()
	require.NotNil(t, res, "SendAndClose must have been called")
	assert.True(t, res.GetSuccess())
	assert.Equal(t, int64(len(payload)), res.GetBytesWritten())
	assert.Equal(t, computeSHA256(payload), res.GetChecksumSha256())
}

func TestService_UploadFile_multiple_chunks_aggregate_into_single_part(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	first := []byte("first-piece")
	second := []byte("second-piece")
	third := []byte("third-piece")
	full := append(append(append([]byte{}, first...), second...), third...)

	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId:     testTransferID,
			Path:           "/dest/file.bin",
			TotalSize:      int64(len(full)),
			ChecksumSha256: computeSHA256(full),
		},
		Data: first,
	})
	stream.sendChunk(&proto.UploadChunk{Data: second})
	stream.sendChunk(&proto.UploadChunk{Data: third})
	stream.closeStream()

	// ACT
	err := runUpload(t, ts.svc, stream)

	// ASSERT
	require.NoError(t, err)

	stored, readErr := ts.storage.Read(context.Background(), transfers.TransferPartPath(testTransferID, 0))
	require.NoError(t, readErr)
	assert.Equal(t, full, stored, "single part must contain concatenation of all chunks")

	res := stream.lastResponse()
	require.NotNil(t, res)
	assert.Equal(t, int64(len(full)), res.GetBytesWritten())
	assert.Equal(t, computeSHA256(full), res.GetChecksumSha256())
}

func TestService_UploadFile_empty_chunks_are_skipped(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	payload := []byte("payload-data")

	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId:     testTransferID,
			Path:           "/dest/file.bin",
			TotalSize:      int64(len(payload)),
			ChecksumSha256: computeSHA256(payload),
		},
		Data: payload,
	})
	// Empty data chunks must not produce additional parts.
	stream.sendChunk(&proto.UploadChunk{Data: nil})
	stream.sendChunk(&proto.UploadChunk{Data: []byte{}})
	stream.closeStream()

	// ACT
	err := runUpload(t, ts.svc, stream)

	// ASSERT
	require.NoError(t, err)
	stored, readErr := ts.storage.Read(context.Background(), transfers.TransferPartPath(testTransferID, 0))
	require.NoError(t, readErr)
	assert.Equal(t, payload, stored)
	// Part 1 must not exist because empty chunks did not trigger flushPart.
	assert.False(t,
		ts.storage.Exists(context.Background(), transfers.TransferPartPath(testTransferID, 1)),
		"empty chunks must not create additional parts",
	)
}

func TestService_UploadFile_checksum_mismatch_returns_data_loss(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	payload := []byte("payload")

	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId:     testTransferID,
			Path:           "/dest/file",
			TotalSize:      int64(len(payload)),
			ChecksumSha256: "0000000000000000000000000000000000000000000000000000000000000000",
		},
		Data: payload,
	})
	stream.closeStream()

	// ACT
	err := runUpload(t, ts.svc, stream)

	// ASSERT
	require.Error(t, err)
	assert.Equal(t, codes.DataLoss, status.Code(err), "mismatch must surface as DataLoss")
	assert.Contains(t, err.Error(), "checksum mismatch")

	donePath := transfers.TransferDonePath(testTransferID)
	require.True(t, ts.storage.Exists(context.Background(), donePath), "sentinel file must be written even on failure")

	doneBytes, readErr := ts.storage.Read(context.Background(), donePath)
	require.NoError(t, readErr)

	var info transfers.DoneInfo
	require.NoError(t, json.Unmarshal(doneBytes, &info))
	assert.False(t, info.Success)
	assert.Contains(t, info.Error, "checksum mismatch")
}

func TestService_UploadFile_missing_metadata_in_first_chunk_returns_invalid_argument(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{Metadata: nil, Data: []byte("data")})
	stream.closeStream()

	// ACT
	err := runUpload(t, ts.svc, stream)

	// ASSERT
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, err.Error(), "metadata")
}

func TestService_UploadFile_first_recv_error_returns_invalid_argument(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	stream := newUploadStream(context.Background())
	stream.recvErr = errTestRecvFailure

	// ACT
	err := runUpload(t, ts.svc, stream)

	// ASSERT
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, err.Error(), "first chunk")
}

func TestService_UploadFile_stream_canceled_mid_upload_unregisters_transfer(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	stream := newUploadStream(ctx)
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId: testTransferID,
			Path:       "/dest/file",
			TotalSize:  1024,
		},
		// No Data so receiveAndStore enters the Recv loop.
	})

	done := make(chan error, 1)
	go func() { done <- ts.svc.UploadFile(stream) }()

	// Give UploadFile a chance to register the transfer before cancelling.
	require.Eventually(t, func() bool {
		ts.svc.mu.RLock()
		defer ts.svc.mu.RUnlock()

		return len(ts.svc.activeTransfers) == 1
	}, time.Second, 5*time.Millisecond, "transfer should be registered")

	// ACT
	cancel()

	// ASSERT
	select {
	case err := <-done:
		require.Error(t, err)
		assert.Equal(t, codes.Canceled, status.Code(err))
	case <-time.After(2 * time.Second):
		t.Fatal("UploadFile did not return after context cancel")
	}

	ts.svc.mu.RLock()
	defer ts.svc.mu.RUnlock()
	assert.Empty(t, ts.svc.activeTransfers, "active transfers map must be cleared on exit")
}

func TestService_UploadFile_publishes_transfer_complete_on_success(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	payload := []byte("ok-payload")
	checksum := computeSHA256(payload)

	received := make(chan *pubsub.Message, 1)
	subErr := ts.bus.Subscribe(context.Background(),
		channels.BuildDaemonFileTransferCompleteChannel(testTransferID),
		func(_ context.Context, msg *pubsub.Message) error {
			received <- msg

			return nil
		})
	require.NoError(t, subErr)

	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId:     testTransferID,
			Path:           "/dest/file",
			TotalSize:      int64(len(payload)),
			ChecksumSha256: checksum,
		},
		Data: payload,
	})
	stream.closeStream()

	// ACT
	err := runUpload(t, ts.svc, stream)

	// ASSERT
	require.NoError(t, err)

	select {
	case msg := <-received:
		assert.Equal(t, messages.TypeDaemonFileTransferComplete, msg.Type)
		payload, parseErr := messages.ParsePayload[messages.FileTransferCompletePayload](msg)
		require.NoError(t, parseErr)
		assert.Equal(t, testTransferID, payload.TransferID)
		assert.True(t, payload.Success, "publish payload must report success")
		assert.Equal(t, checksum, payload.Checksum)
		assert.Empty(t, payload.Error)
	case <-time.After(time.Second):
		t.Fatal("expected transfer complete event was not published")
	}
}

func TestService_UploadFile_publishes_transfer_complete_on_checksum_mismatch(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	payload := []byte("bad-payload")

	received := make(chan *pubsub.Message, 1)
	subErr := ts.bus.Subscribe(context.Background(),
		channels.BuildDaemonFileTransferCompleteChannel(testTransferID),
		func(_ context.Context, msg *pubsub.Message) error {
			received <- msg

			return nil
		})
	require.NoError(t, subErr)

	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId:     testTransferID,
			Path:           "/dest/file",
			TotalSize:      int64(len(payload)),
			ChecksumSha256: "deadbeef",
		},
		Data: payload,
	})
	stream.closeStream()

	// ACT
	err := runUpload(t, ts.svc, stream)

	// ASSERT
	require.Error(t, err)

	select {
	case msg := <-received:
		assert.Equal(t, messages.TypeDaemonFileTransferComplete, msg.Type)
		decoded, parseErr := messages.ParsePayload[messages.FileTransferCompletePayload](msg)
		require.NoError(t, parseErr)
		assert.False(t, decoded.Success)
		assert.Contains(t, decoded.Error, "checksum mismatch")
	case <-time.After(time.Second):
		t.Fatal("expected failure event was not published")
	}
}

func TestService_UploadFile_sets_registry_state_on_success(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	payload := []byte("registered-data")

	state := ts.reg.Register(testTransferID)

	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId:     testTransferID,
			Path:           "/dest/file",
			TotalSize:      int64(len(payload)),
			ChecksumSha256: computeSHA256(payload),
		},
		Data: payload,
	})
	stream.closeStream()

	// ACT
	err := runUpload(t, ts.svc, stream)

	// ASSERT
	require.NoError(t, err)

	available, waitErr := state.WaitForPart(context.Background(), 0)
	require.NoError(t, waitErr)
	assert.True(t, available, "part 0 must be reported as available after upload completes")

	// A second WaitForPart with the part already consumed should report
	// completion (no error, no available data).
	available, waitErr = state.WaitForPart(context.Background(), 1)
	require.NoError(t, waitErr)
	assert.False(t, available, "no further parts beyond what was written")
}

func TestService_UploadFile_sets_registry_error_on_checksum_mismatch(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	state := ts.reg.Register(testTransferID)

	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId:     testTransferID,
			Path:           "/dest/file",
			TotalSize:      4,
			ChecksumSha256: "deadbeef",
		},
		Data: []byte("data"),
	})
	stream.closeStream()

	// ACT
	err := runUpload(t, ts.svc, stream)

	// ASSERT
	require.Error(t, err)

	_, waitErr := state.WaitForPart(context.Background(), 99)
	require.Error(t, waitErr)
	assert.Contains(t, waitErr.Error(), "checksum mismatch", "registry state must carry the checksum mismatch error")
}

func TestService_UploadFile_publisher_error_does_not_mask_success(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storage := files.NewInMemoryFileManager()
	logger := slog.New(slog.DiscardHandler)
	svc := NewService(storage, failingPublisher{}, transfers.NewRegistry(), logger)

	payload := []byte("publisher-fails")
	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId:     testTransferID,
			Path:           "/dest/file",
			TotalSize:      int64(len(payload)),
			ChecksumSha256: computeSHA256(payload),
		},
		Data: payload,
	})
	stream.closeStream()

	// ACT
	err := runUpload(t, svc, stream)

	// ASSERT
	require.NoError(t, err, "publisher errors must be logged but not surfaced to the gRPC client")
	assert.True(t, storage.Exists(context.Background(), transfers.TransferPartPath(testTransferID, 0)))
}

func TestService_UploadFile_nil_logger_falls_back_to_default(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storage := files.NewInMemoryFileManager()
	bus := memory.New()
	t.Cleanup(func() { _ = bus.Close() })

	svc := NewService(storage, bus, transfers.NewRegistry(), nil)
	require.NotNil(t, svc.logger, "nil logger argument must be replaced with a default")

	payload := []byte("hi")
	stream := newUploadStream(context.Background())
	stream.sendChunk(&proto.UploadChunk{
		Metadata: &proto.UploadMetadata{
			TransferId:     testTransferID,
			Path:           "/dest/file",
			TotalSize:      int64(len(payload)),
			ChecksumSha256: computeSHA256(payload),
		},
		Data: payload,
	})
	stream.closeStream()

	// ACT
	err := runUpload(t, svc, stream)

	// ASSERT
	require.NoError(t, err)
}

func TestService_DownloadFile_streams_all_chunks_with_correct_offsets(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	const transferID = "download-1"
	// Pick a size that spans multiple defaultChunkSize (64KiB) reads.
	payload := make([]byte, 200*1024)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	require.NoError(t,
		ts.storage.Write(context.Background(), transfers.TransferDataPath(transferID), payload),
	)

	stream := newDownloadStream(context.Background())

	// ACT
	err := ts.svc.DownloadFile(&proto.DownloadRequest{Path: transferID}, stream)

	// ASSERT
	require.NoError(t, err)

	chunks := stream.chunks()
	require.NotEmpty(t, chunks, "at least one chunk should be sent")

	var assembled []byte
	for _, c := range chunks {
		assembled = append(assembled, c.GetData()...)
	}
	assert.Equal(t, payload, assembled, "concatenated chunks must equal stored payload")

	// Each non-final chunk must carry an offset that matches the cumulative
	// bytes sent so far (the receiving client uses Offset to seek into the
	// destination file).
	var offsetCheck int64
	for i, c := range chunks {
		assert.Equal(t, offsetCheck, c.GetOffset(),
			"chunk %d must report cumulative offset", i)
		offsetCheck += int64(len(c.GetData()))
	}
}

func TestService_DownloadFile_final_chunk_carries_is_final_and_checksum(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storage := eofTogetherStorage{InMemoryFileManager: files.NewInMemoryFileManager()}
	bus := memory.New()
	t.Cleanup(func() { _ = bus.Close() })

	svc := NewService(storage, bus, transfers.NewRegistry(), slog.New(slog.DiscardHandler))

	const transferID = "download-final"
	payload := []byte("final-chunk-payload")
	require.NoError(t,
		storage.Write(context.Background(), transfers.TransferDataPath(transferID), payload),
	)

	stream := newDownloadStream(context.Background())

	// ACT
	err := svc.DownloadFile(&proto.DownloadRequest{Path: transferID}, stream)

	// ASSERT
	require.NoError(t, err)
	chunks := stream.chunks()
	require.Len(t, chunks, 1, "single read returning EOF together must produce exactly one chunk")
	assert.Equal(t, payload, chunks[0].GetData())
	assert.True(t, chunks[0].GetIsFinal(), "is_final must be set when the read returns EOF together with data")
	assert.Equal(t, computeSHA256(payload), chunks[0].GetChecksumSha256(),
		"checksum must be present on the final chunk")
}

func TestService_DownloadFile_offset_skips_leading_bytes(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	const transferID = "download-offset"
	payload := []byte("0123456789ABCDEF")
	require.NoError(t,
		ts.storage.Write(context.Background(), transfers.TransferDataPath(transferID), payload),
	)

	stream := newDownloadStream(context.Background())

	// ACT
	err := ts.svc.DownloadFile(&proto.DownloadRequest{Path: transferID, Offset: 4}, stream)

	// ASSERT
	require.NoError(t, err)

	chunks := stream.chunks()
	require.NotEmpty(t, chunks)
	assert.Equal(t, int64(4), chunks[0].GetOffset(), "first chunk's offset must equal the requested offset")

	var assembled []byte
	for _, c := range chunks {
		assembled = append(assembled, c.GetData()...)
	}
	assert.Equal(t, payload[4:], assembled)
	assert.Equal(t, computeSHA256(payload), chunks[len(chunks)-1].GetChecksumSha256(),
		"final checksum must describe the full transfer payload even for resumed download")
}

func TestService_DownloadFile_unknown_path_returns_not_found(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	stream := newDownloadStream(context.Background())

	// ACT
	err := ts.svc.DownloadFile(&proto.DownloadRequest{Path: "missing"}, stream)

	// ASSERT
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Empty(t, stream.chunks(), "no chunks should be sent when file is missing")
}

func TestService_DownloadFile_canceled_context_returns_canceled(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	const transferID = "download-cancel"
	require.NoError(t,
		ts.storage.Write(context.Background(),
			transfers.TransferDataPath(transferID),
			[]byte("payload-bytes")),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stream := newDownloadStream(ctx)

	// ACT
	err := ts.svc.DownloadFile(&proto.DownloadRequest{Path: transferID}, stream)

	// ASSERT
	require.Error(t, err)
	assert.Equal(t, codes.Canceled, status.Code(err))
}

func TestService_DownloadFile_send_error_returns_internal(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	const transferID = "download-senderr"
	require.NoError(t,
		ts.storage.Write(context.Background(),
			transfers.TransferDataPath(transferID),
			[]byte("data")),
	)

	stream := newDownloadStream(context.Background())
	stream.sendErr = errTestSendFailure

	// ACT
	err := ts.svc.DownloadFile(&proto.DownloadRequest{Path: transferID}, stream)

	// ASSERT
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestService_CancelTransfer_unknown_id_returns_false(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)

	// ACT
	ok := ts.svc.CancelTransfer("unknown")

	// ASSERT
	assert.False(t, ok)
}

func TestService_CancelTransfer_known_id_invokes_cancel_func(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ts.svc.registerTransfer(testTransferID, "/dest/file", 1024, cancel)

	// ACT
	ok := ts.svc.CancelTransfer(testTransferID)

	// ASSERT
	assert.True(t, ok)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("registered cancel func was not invoked")
	}
}

func TestService_CancelAll_cancels_every_active_transfer(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	cancels := make([]context.CancelFunc, 0, 3)
	contexts := make([]context.Context, 0, 3)
	for i := range 3 {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		cancels = append(cancels, cancel)
		contexts = append(contexts, ctx)
		ts.svc.registerTransfer("t-"+string(rune('a'+i)), "/p", 0, cancel)
	}
	require.Len(t, cancels, 3)

	// ACT
	ts.svc.CancelAll()

	// ASSERT
	for i, ctx := range contexts {
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatalf("transfer %d was not canceled by CancelAll", i)
		}
	}
}

func TestService_publishTransferComplete_emits_on_expected_channel(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)

	received := make(chan *pubsub.Message, 1)
	subErr := ts.bus.Subscribe(context.Background(),
		channels.BuildDaemonFileTransferCompleteChannel("direct-call"),
		func(_ context.Context, msg *pubsub.Message) error {
			received <- msg

			return nil
		})
	require.NoError(t, subErr)

	// ACT
	ts.svc.publishTransferComplete("direct-call", true, "abcdef", "")

	// ASSERT
	select {
	case msg := <-received:
		assert.Equal(t, channels.BuildDaemonFileTransferCompleteChannel("direct-call"), msg.Channel)
		assert.Equal(t, messages.TypeDaemonFileTransferComplete, msg.Type)
		payload, parseErr := messages.ParsePayload[messages.FileTransferCompletePayload](msg)
		require.NoError(t, parseErr)
		assert.Equal(t, "direct-call", payload.TransferID)
		assert.True(t, payload.Success)
		assert.Equal(t, "abcdef", payload.Checksum)
	case <-time.After(time.Second):
		t.Fatal("publishTransferComplete did not emit on the expected channel")
	}
}

func TestService_writeSentinel_persists_done_info(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	const transferID = "sentinel-1"

	// ACT
	ts.svc.writeSentinel(transferID, 5, "abc123", "")

	// ASSERT
	donePath := transfers.TransferDonePath(transferID)
	require.True(t, ts.storage.Exists(context.Background(), donePath))

	bytesWritten, readErr := ts.storage.Read(context.Background(), donePath)
	require.NoError(t, readErr)

	var info transfers.DoneInfo
	require.NoError(t, json.Unmarshal(bytesWritten, &info))
	assert.True(t, info.Success, "empty error message must produce success=true")
	assert.Equal(t, "abc123", info.Checksum)
	assert.Equal(t, 5, info.TotalParts)
	assert.Empty(t, info.Error)
}

func TestService_WaitForCompletion_returns_immediately_when_idle(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)

	done := make(chan struct{})

	// ACT
	go func() {
		ts.svc.WaitForCompletion()
		close(done)
	}()

	// ASSERT
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitForCompletion must return immediately when there are no active transfers")
	}
}

func TestTransferIDFromListEntry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		entry string
		want  string
	}{
		{
			name:  "local_backend_basename_only",
			entry: "abc123",
			want:  "abc123",
		},
		{
			name:  "s3_backend_relative_with_trailing_slash",
			entry: "abc123/",
			want:  "abc123",
		},
		{
			name:  "s3_backend_relative_nested",
			entry: "abc123/parts/000000",
			want:  "abc123",
		},
		{
			name:  "inmemory_backend_full_recursive_path",
			entry: "transfers/abc123/parts/000000",
			want:  "abc123",
		},
		{
			name:  "inmemory_backend_done_sentinel",
			entry: "transfers/abc123/done",
			want:  "abc123",
		},
		{
			name:  "empty_entry",
			entry: "",
			want:  "",
		},
		{
			name:  "prefix_only",
			entry: "transfers/",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, transferIDFromListEntry(tt.entry))
		})
	}
}

func TestService_cleanupStaleTransfers_deletes_completed_transfer_files(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	ctx := context.Background()
	const completedID = "completed-xyz"

	require.NoError(t, ts.storage.Write(ctx, transfers.TransferPartPath(completedID, 0), []byte("data")))
	require.NoError(t, ts.storage.Write(ctx, transfers.TransferDonePath(completedID), []byte(`{"success":true}`)))

	// ACT
	require.NoError(t, ts.svc.cleanupStaleTransfers(ctx))

	// ASSERT
	assert.False(t, ts.storage.Exists(ctx, transfers.TransferPartPath(completedID, 0)),
		"part file must be removed for completed transfer")
	assert.False(t, ts.storage.Exists(ctx, transfers.TransferDonePath(completedID)),
		"done sentinel must be removed for completed transfer")
}

func TestService_cleanupStaleTransfers_preserves_in_flight_transfer(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	ctx := context.Background()
	const inFlightID = "active-pqr"
	const completedID = "completed-stu"

	require.NoError(t, ts.storage.Write(ctx, transfers.TransferPartPath(inFlightID, 0), []byte("active-data")))
	require.NoError(t, ts.storage.Write(ctx, transfers.TransferPartPath(completedID, 0), []byte("done-data")))

	ts.svc.registerTransfer(inFlightID, "/dest/active.bin", 100, func() {})
	t.Cleanup(func() { ts.svc.unregisterTransfer(inFlightID) })

	// ACT
	require.NoError(t, ts.svc.cleanupStaleTransfers(ctx))

	// ASSERT
	assert.True(t, ts.storage.Exists(ctx, transfers.TransferPartPath(inFlightID, 0)),
		"in-flight transfer files must NOT be removed")
	assert.False(t, ts.storage.Exists(ctx, transfers.TransferPartPath(completedID, 0)),
		"non-active transfer files must be removed")
}

func TestService_cleanupStaleTransfers_dedupes_multiple_entries_per_transfer(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	ctx := context.Background()
	const transferID = "multi-part"

	require.NoError(t, ts.storage.Write(ctx, transfers.TransferPartPath(transferID, 0), []byte("p0")))
	require.NoError(t, ts.storage.Write(ctx, transfers.TransferPartPath(transferID, 1), []byte("p1")))
	require.NoError(t, ts.storage.Write(ctx, transfers.TransferPartPath(transferID, 2), []byte("p2")))
	require.NoError(t, ts.storage.Write(ctx, transfers.TransferDonePath(transferID), []byte(`{}`)))

	// ACT
	require.NoError(t, ts.svc.cleanupStaleTransfers(ctx))

	// ASSERT
	for partNum := range 3 {
		assert.False(t, ts.storage.Exists(ctx, transfers.TransferPartPath(transferID, partNum)),
			"part %d must be removed", partNum)
	}
	assert.False(t, ts.storage.Exists(ctx, transfers.TransferDonePath(transferID)))
}

func TestService_cleanupStaleTransfers_empty_storage_is_noop(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)

	// ACT + ASSERT
	require.NoError(t, ts.svc.cleanupStaleTransfers(context.Background()))
}

func TestService_cleanupStaleTransfers_canceled_context_returns_ctx_err(t *testing.T) {
	t.Parallel()
	// ARRANGE
	ts := newTestService(t)
	ctx := context.Background()

	for i := range 5 {
		require.NoError(t, ts.storage.Write(ctx, transfers.TransferPartPath("tid-"+string(rune('a'+i)), 0), []byte("x")))
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	// ACT
	err := ts.svc.cleanupStaleTransfers(canceled)

	// ASSERT
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
