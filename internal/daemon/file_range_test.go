package daemon

import (
	"bytes"
	"context"
	"io"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rangeCall records one ranged read request as a route received it.
type rangeCall struct {
	path   string
	offset int64
	length int64
}

// rangeAnswer is one scripted reply to a ranged read. An answer with no field
// set is an empty successful reply, which is how a node reports end of file.
type rangeAnswer struct {
	content []byte
	// respError is a daemon-reported failure (Success=false); local route only.
	respError string
	// err is a transport failure returned instead of a reply.
	err error
	// storagePath makes the remote route stage its answer in storage
	// instead of inlining the bytes.
	storagePath string
	// windowFill, when set, replaces content with exactly as many copies of
	// the byte as the requested window asked for, so the node always answers
	// the read in full.
	windowFill byte
}

// rangeRecorder replies to ranged reads from a script and records every
// request, so tests can assert on the chunk walk the service performs.
type rangeRecorder struct {
	mu      sync.Mutex
	answers []rangeAnswer
	calls   []rangeCall
}

func (r *rangeRecorder) take(path string, offset, length int64) rangeAnswer {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, rangeCall{path: path, offset: offset, length: length})

	if len(r.calls) > len(r.answers) {
		return rangeAnswer{content: []byte{}}
	}

	answer := r.answers[len(r.calls)-1]
	if answer.windowFill != 0 {
		answer.content = bytes.Repeat([]byte{answer.windowFill}, int(length))
	}

	return answer
}

func (r *rangeRecorder) recorded() []rangeCall {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.calls)
}

// gatewayRead adapts the script to the local (gateway) route.
func (r *rangeRecorder) gatewayRead() func(
	context.Context, uint64, string, int64, int64,
) (*proto.FileReadResponse, error) {
	return func(_ context.Context, _ uint64, path string, offset, length int64) (*proto.FileReadResponse, error) {
		answer := r.take(path, offset, length)

		switch {
		case answer.err != nil:
			return nil, answer.err
		case answer.respError != "":
			return &proto.FileReadResponse{Success: false, Error: answer.respError}, nil
		}

		return &proto.FileReadResponse{Success: true, Content: answer.content}, nil
	}
}

// dispatchRead adapts the script to the remote (dispatcher) route.
func (r *rangeRecorder) dispatchRead() func(
	context.Context, uint64, string, int64, int64,
) (*FileReadResult, error) {
	return func(_ context.Context, _ uint64, path string, offset, length int64) (*FileReadResult, error) {
		answer := r.take(path, offset, length)
		if answer.err != nil {
			return nil, answer.err
		}

		if answer.storagePath != "" {
			return &FileReadResult{StoragePath: answer.storagePath}, nil
		}

		return &FileReadResult{Content: answer.content}, nil
	}
}

func TestFileService_DownloadStreamRange(t *testing.T) {
	t.Parallel()

	const (
		nodeID   uint64 = 61
		workPath        = "/srv/games"
		filePath        = "/srv/games/logs/big.log"
		relPath         = "logs/big.log"
	)

	type setup struct {
		isConnected         bool
		isConnectedAnywhere bool
		answers             []rangeAnswer
		storageContent      []byte
	}

	tests := []struct {
		name   string
		setup  setup
		offset uint64
		length uint64
		// wantOpenError is what DownloadStreamRange itself returns; the reader
		// is nil then and no route is touched.
		wantOpenError string
		// wantError is what surfaces from the reader while streaming.
		wantError string
		wantErrIs error
		wantBytes string
		wantCalls []rangeCall
	}{
		{
			name:          notConnectedCaseName,
			setup:         setup{},
			offset:        100,
			length:        10,
			wantOpenError: "daemon not connected",
			wantErrIs:     ErrDaemonNotConnected,
		},
		{
			name: "bounded_window_reaches_the_node_verbatim",
			setup: setup{
				isConnected: true,
				answers:     []rangeAnswer{{content: []byte("0123456789")}},
			},
			offset:    100,
			length:    10,
			wantBytes: "0123456789",
			wantCalls: []rangeCall{{path: relPath, offset: 100, length: 10}},
		},
		{
			name: "unbounded_read_asks_for_a_full_chunk",
			setup: setup{
				isConnected: true,
				answers:     []rangeAnswer{{content: []byte("short")}},
			},
			offset:    0,
			length:    0,
			wantBytes: "short",
			wantCalls: []rangeCall{{path: relPath, offset: 0, length: rangeChunkSize}},
		},
		{
			name: "empty_first_answer_ends_the_stream_without_error",
			setup: setup{
				isConnected: true,
				answers:     []rangeAnswer{{content: []byte{}}},
			},
			offset:    7,
			length:    0,
			wantBytes: "",
			wantCalls: []rangeCall{{path: relPath, offset: 7, length: rangeChunkSize}},
		},
		{
			name: "gateway_transport_error_surfaces_through_the_reader",
			setup: setup{
				isConnected: true,
				answers:     []rangeAnswer{{err: errors.New("stream down")}},
			},
			length:    0,
			wantError: "gateway ranged read: stream down",
			wantCalls: []rangeCall{{path: relPath, offset: 0, length: rangeChunkSize}},
		},
		{
			name: "missing_file_maps_to_ErrFileNotFound",
			setup: setup{
				isConnected: true,
				answers: []rangeAnswer{
					{respError: "openat servers/q2/big.log: no such file or directory"},
				},
			},
			length:    12,
			wantError: "file not found",
			wantErrIs: ErrFileNotFound,
			wantCalls: []rangeCall{{path: relPath, offset: 0, length: 12}},
		},
		{
			name: "unrecognised_daemon_error_keeps_its_raw_text",
			setup: setup{
				isConnected: true,
				answers:     []rangeAnswer{{respError: "quota manager offline"}},
			},
			length:    12,
			wantError: "gateway ranged read: quota manager offline",
			wantCalls: []rangeCall{{path: relPath, offset: 0, length: 12}},
		},
		{
			name: "remote_route_returns_inline_content",
			setup: setup{
				isConnectedAnywhere: true,
				answers:             []rangeAnswer{{content: []byte("abcd")}},
			},
			offset:    8,
			length:    4,
			wantBytes: "abcd",
			wantCalls: []rangeCall{{path: relPath, offset: 8, length: 4}},
		},
		{
			name: "remote_route_reads_a_staged_answer_from_storage",
			setup: setup{
				isConnectedAnywhere: true,
				answers:             []rangeAnswer{{storagePath: "transfers/r1/range"}},
				storageContent:      []byte("staged-range-bytes"),
			},
			length:    0,
			wantBytes: "staged-range-bytes",
			wantCalls: []rangeCall{{path: relPath, offset: 0, length: rangeChunkSize}},
		},
		{
			name: "remote_route_reports_a_missing_staged_answer",
			setup: setup{
				isConnectedAnywhere: true,
				answers:             []rangeAnswer{{storagePath: "transfers/gone/range"}},
			},
			length:    0,
			wantError: "reading transferred range from storage",
			wantCalls: []rangeCall{{path: relPath, offset: 0, length: rangeChunkSize}},
		},
		{
			name: "remote_transport_error_surfaces_through_the_reader",
			setup: setup{
				isConnectedAnywhere: true,
				answers:             []rangeAnswer{{err: errors.New("session gone")}},
			},
			length:    0,
			wantError: "dispatched ranged read: session gone",
			wantCalls: []rangeCall{{path: relPath, offset: 0, length: rangeChunkSize}},
		},
		{
			name: "huge_offset_is_clamped_to_the_int64_ceiling",
			setup: setup{
				isConnected: true,
				answers:     []rangeAnswer{{content: []byte("x")}},
			},
			offset:    math.MaxUint64,
			length:    0,
			wantBytes: "x",
			wantCalls: []rangeCall{{path: relPath, offset: math.MaxInt64, length: rangeChunkSize}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			ctx := testContext(t)
			s := setupFileService(t)
			s.registry.setConnected(nodeID, tt.setup.isConnected)
			s.registry.connectedAnywhere[nodeID] = tt.setup.isConnectedAnywhere

			var stagedPath string
			if len(tt.setup.answers) > 0 {
				stagedPath = tt.setup.answers[0].storagePath
			}
			if tt.setup.storageContent != nil {
				require.NoError(t, s.storage.Write(ctx, stagedPath, tt.setup.storageContent))
			}

			gatewayRec := &rangeRecorder{}
			dispatchRec := &rangeRecorder{}

			// The route the service must pick for this connectivity state; the
			// other recorder has to stay empty.
			remote := !tt.setup.isConnected && tt.setup.isConnectedAnywhere
			usedRec, idleRec := gatewayRec, dispatchRec
			if remote {
				usedRec, idleRec = dispatchRec, gatewayRec
			}
			usedRec.answers = tt.setup.answers

			s.gateway.requestFileRead = gatewayRec.gatewayRead()
			s.dispatcher.dispatchFileRead = dispatchRec.dispatchRead()

			// ACT
			rc, openErr := s.service.DownloadStreamRange(
				ctx, testNode(uint(nodeID), workPath), filePath, tt.offset, tt.length,
			)

			var (
				got     []byte
				readErr error
			)
			if openErr == nil && rc != nil {
				got, readErr = io.ReadAll(rc)
				_ = rc.Close()
			}

			// ASSERT
			if tt.wantOpenError != "" {
				require.Error(t, openErr)
				assert.Nil(t, rc, "reader must be nil when the range cannot be opened")
				assert.Contains(t, openErr.Error(), tt.wantOpenError, "error message mismatch")
				require.ErrorIs(t, openErr, tt.wantErrIs, "the connectivity sentinel must survive")
				assert.Empty(t, gatewayRec.recorded(), "the local route must stay untouched")
				assert.Empty(t, dispatchRec.recorded(), "the remote route must stay untouched")

				return
			}

			require.NoError(t, openErr, "a resolvable route must yield a reader, not an error")
			require.NotNil(t, rc)
			assert.Equal(t, tt.wantCalls, usedRec.recorded(), "ranged read requests mismatch")
			assert.Empty(t, idleRec.recorded(), "the route that does not own the session must stay untouched")

			if tt.wantError != "" {
				require.Error(t, readErr, "the streaming failure must surface at read time")
				assert.Contains(t, readErr.Error(), tt.wantError, "error message mismatch")

				if tt.wantErrIs != nil {
					require.ErrorIs(t, readErr, tt.wantErrIs, "daemon failure must be classified")
					assert.NotContains(t, readErr.Error(), "servers/", "client-facing text must not carry node paths")
				}

				return
			}

			require.NoError(t, readErr)
			assert.Equal(t, tt.wantBytes, string(got), "streamed bytes mismatch")

			if stagedPath != "" {
				assert.False(t, s.storage.Exists(ctx, stagedPath),
					"a staged range must be dropped from storage once it has been read")
			}
		})
	}
}

func TestFileService_DownloadStreamRange_keepsReadingWhileChunksComeBackFull(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const nodeID uint64 = 71
	s.registry.setConnected(nodeID, true)

	rec := &rangeRecorder{answers: []rangeAnswer{
		{windowFill: 'A'},
		{content: []byte("END")},
	}}
	s.gateway.requestFileRead = rec.gatewayRead()

	// ACT
	rc, err := s.service.DownloadStreamRange(ctx, testNode(71, "/srv"), "/srv/big.bin", 1000, 0)
	require.NoError(t, err)

	got, readErr := io.ReadAll(rc)
	_ = rc.Close()

	// ASSERT
	require.NoError(t, readErr)
	require.Equal(t, rangeChunkSize+len("END"), len(got), "streamed length mismatch")
	assert.Equal(t, rangeChunkSize, bytes.Count(got, []byte{'A'}), "the whole first chunk must reach the reader")
	assert.Equal(t, "END", string(got[rangeChunkSize:]), "the trailing short chunk must be appended in order")
	assert.Equal(t, []rangeCall{
		{path: "big.bin", offset: 1000, length: rangeChunkSize},
		{path: "big.bin", offset: 1000 + rangeChunkSize, length: rangeChunkSize},
	}, rec.recorded(), "an unbounded range walks forward one full chunk at a time")
}

func TestFileService_DownloadStreamRange_boundedWindowSpansChunks(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const nodeID uint64 = 72
	s.registry.setConnected(nodeID, true)

	rec := &rangeRecorder{answers: []rangeAnswer{
		{windowFill: 'A'},
		{windowFill: 'B'},
	}}
	s.gateway.requestFileRead = rec.gatewayRead()

	// ACT
	rc, err := s.service.DownloadStreamRange(
		ctx, testNode(72, "/srv"), "/srv/big.bin", 42, rangeChunkSize+3,
	)
	require.NoError(t, err)

	got, readErr := io.ReadAll(rc)
	_ = rc.Close()

	// ASSERT
	require.NoError(t, readErr)
	require.Equal(t, rangeChunkSize+3, len(got), "the window must be delivered in full and no further")
	assert.Equal(t, rangeChunkSize, bytes.Count(got, []byte{'A'}), "the first chunk must reach the reader whole")
	assert.Equal(t, "BBB", string(got[rangeChunkSize:]), "the remainder must follow the first chunk")
	assert.Equal(t, []rangeCall{
		{path: "big.bin", offset: 42, length: rangeChunkSize},
		{path: "big.bin", offset: 42 + rangeChunkSize, length: 3},
	}, rec.recorded(), "the last read must ask only for the bytes still missing from the window")
}

func TestFileService_DownloadStreamRange_cancelledContextStopsBeforeTheFirstRead(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx, cancel := context.WithCancel(testContext(t))
	s := setupFileService(t)
	const nodeID uint64 = 73
	s.registry.setConnected(nodeID, true)

	rec := &rangeRecorder{answers: []rangeAnswer{{content: []byte("never-read")}}}
	s.gateway.requestFileRead = rec.gatewayRead()
	cancel()

	// ACT
	rc, err := s.service.DownloadStreamRange(ctx, testNode(73, "/srv"), "/srv/big.bin", 0, 0)
	require.NoError(t, err)

	got, readErr := io.ReadAll(rc)
	_ = rc.Close()

	// ASSERT
	require.ErrorIs(t, readErr, context.Canceled, "the cancellation cause must reach the consumer")
	assert.Empty(t, got, "no bytes must be delivered from a cancelled range")
	assert.Empty(t, rec.recorded(), "a cancelled range must not reach the node at all")
}

func TestFileService_DownloadStreamRange_closedReaderStopsTheProducer(t *testing.T) {
	t.Parallel()

	// ARRANGE
	ctx := testContext(t)
	s := setupFileService(t)
	const nodeID uint64 = 74
	s.registry.setConnected(nodeID, true)

	// The node answers only once the consumer is gone, so the chunk it
	// returns has nowhere left to be written.
	readerClosed := make(chan struct{})
	rec := &rangeRecorder{answers: []rangeAnswer{{windowFill: 'A'}, {windowFill: 'B'}}}
	scripted := rec.gatewayRead()
	s.gateway.requestFileRead = func(
		ctx context.Context, node uint64, path string, offset, length int64,
	) (*proto.FileReadResponse, error) {
		<-readerClosed

		return scripted(ctx, node, path, offset, length)
	}

	rc, err := s.service.DownloadStreamRange(ctx, testNode(74, "/srv"), "/srv/big.bin", 0, 0)
	require.NoError(t, err)

	// ACT
	require.NoError(t, rc.Close())
	close(readerClosed)

	// ASSERT
	require.Eventually(t, func() bool {
		return len(rec.recorded()) >= 1
	}, time.Second, 10*time.Millisecond, "the chunk already in flight must still be requested")
	require.Never(t, func() bool {
		return len(rec.recorded()) > 1
	}, 200*time.Millisecond, 20*time.Millisecond, "a range whose reader went away must stop pulling chunks")
}
