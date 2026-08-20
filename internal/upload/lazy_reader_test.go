package upload

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lazyChunkReader is unexported, so this test lives in the production package
// (no _test suffix on the package). The test still exercises the reader as a
// black box through io.Reader semantics.

func TestLazyChunkReader_Read_LoadsFromStorageOnFirstRead(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storage := files.NewInMemoryFileManager()
	require.NoError(t, storage.Write(context.Background(), "chunk", []byte("hello")))
	r := newLazyChunkReader(context.Background(), storage, "chunk")

	// ACT
	got, err := io.ReadAll(r)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), got)
}

func TestLazyChunkReader_Read_ReturnsErrorWhenChunkMissing(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storage := files.NewInMemoryFileManager()
	r := newLazyChunkReader(context.Background(), storage, "missing")

	// ACT
	_, err := r.Read(make([]byte, 8))

	// ASSERT
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open chunk missing", "error must identify the failing path")
}

func TestLazyChunkReader_Read_ReturnsEOFAfterCloseAndDoesNotReopen(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storage := &openCounterStorage{StreamFileManager: files.NewInMemoryFileManager()}
	require.NoError(t, storage.Write(context.Background(), "chunk", []byte("data")))
	r := newLazyChunkReader(context.Background(), storage, "chunk")
	require.NoError(t, r.Close())

	// ACT
	n, err := r.Read(make([]byte, 4))

	// ASSERT
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, int64(0), storage.opens.Load(),
		"Close before any Read must keep ReadStream from being called")
}

func TestLazyChunkReader_Read_PartialReadAcrossMultipleCalls(t *testing.T) {
	t.Parallel()
	// ARRANGE — verify the lazy reader threads io.Reader semantics through to
	// the underlying ReadCloser, including short reads.
	storage := files.NewInMemoryFileManager()
	require.NoError(t, storage.Write(context.Background(), "chunk", []byte("0123456789")))
	r := newLazyChunkReader(context.Background(), storage, "chunk")

	// ACT — drain the reader with small buffers.
	buf := make([]byte, 3)
	collected := strings.Builder{}
	for {
		n, err := r.Read(buf)
		collected.Write(buf[:n])
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
	}

	// ASSERT
	assert.Equal(t, "0123456789", collected.String())
}

func TestLazyChunkReader_Read_ReleasesUnderlyingReadCloserOnEOF(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storage := &openCounterStorage{StreamFileManager: files.NewInMemoryFileManager()}
	require.NoError(t, storage.Write(context.Background(), "chunk", []byte("xy")))
	r := newLazyChunkReader(context.Background(), storage, "chunk")

	_, err := io.ReadAll(r)
	require.NoError(t, err)

	// ACT — Close after EOF must not double-close the already-released stream.
	closeErr := r.Close()

	// ASSERT
	require.NoError(t, closeErr)
	assert.Equal(t, int64(1), storage.opens.Load(),
		"underlying stream must be opened exactly once across the reader's life")
}

func TestLazyChunkReader_MultiReader_OpensStreamsSequentiallyNotInParallel(t *testing.T) {
	t.Parallel()
	// ARRANGE — three lazy readers wrapped in io.MultiReader. The contract we
	// verify is that we never hold more than one underlying ReadStream open at
	// a time (the reason lazyChunkReader exists in the first place).
	storage := &concurrencyTrackingStorage{StreamFileManager: files.NewInMemoryFileManager()}
	for _, name := range []string{"a", "b", "c"} {
		require.NoError(t, storage.Write(context.Background(), name, []byte(name+name)))
	}
	multi := io.MultiReader(
		newLazyChunkReader(context.Background(), storage, "a"),
		newLazyChunkReader(context.Background(), storage, "b"),
		newLazyChunkReader(context.Background(), storage, "c"),
	)

	// ACT
	got, err := io.ReadAll(multi)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, []byte("aabbcc"), got)
	assert.LessOrEqual(t, storage.maxOpen.Load(), int64(1),
		"only one underlying stream may be open at a time")
}

// openCounterStorage wraps an InMemoryFileManager and counts ReadStream calls.
type openCounterStorage struct {
	files.StreamFileManager

	opens atomic.Int64
}

func (s *openCounterStorage) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	s.opens.Add(1)

	return s.StreamFileManager.ReadStream(ctx, path)
}

// concurrencyTrackingStorage records the high-water mark of concurrently-open
// streams, used to prove the lazy reader never overlaps opens.
type concurrencyTrackingStorage struct {
	files.StreamFileManager

	current atomic.Int64
	maxOpen atomic.Int64
}

func (s *concurrencyTrackingStorage) ReadStream(ctx context.Context, path string) (io.ReadCloser, error) {
	rc, err := s.StreamFileManager.ReadStream(ctx, path)
	if err != nil {
		return nil, err
	}
	cur := s.current.Add(1)
	for {
		mx := s.maxOpen.Load()
		if cur <= mx || s.maxOpen.CompareAndSwap(mx, cur) {
			break
		}
	}

	return &trackedReadCloser{ReadCloser: rc, parent: s}, nil
}

type trackedReadCloser struct {
	io.ReadCloser

	parent *concurrencyTrackingStorage
	closed bool
}

func (t *trackedReadCloser) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	t.parent.current.Add(-1)

	return t.ReadCloser.Close()
}

// Compile-time guarantee that the wrapping types satisfy upload.Storage. If
// the Storage contract changes, this fails fast.
var _ Storage = (*openCounterStorage)(nil)
var _ Storage = (*concurrencyTrackingStorage)(nil)

// errReadCloser is a stream wrapper whose Read always returns the configured
// error; used to verify lazyChunkReader propagates underlying read failures.
type errReadCloser struct{ err error }

func (e *errReadCloser) Read([]byte) (int, error) { return 0, e.err }
func (e *errReadCloser) Close() error             { return nil }

var errFakeDiskFault = errors.New("fake disk fault")

func TestLazyChunkReader_Read_PropagatesUnderlyingReadError(t *testing.T) {
	t.Parallel()
	// ARRANGE
	storage := &errStreamStorage{err: errFakeDiskFault}
	r := newLazyChunkReader(context.Background(), storage, "chunk")

	// ACT
	_, err := r.Read(make([]byte, 1))

	// ASSERT
	require.Error(t, err)
	assert.True(t, errors.Is(err, errFakeDiskFault), "underlying read error must propagate")
}

// errStreamStorage opens successfully but every Read on the returned stream
// returns the configured error. It implements only the methods Storage
// actually invokes from the lazy reader, so unused calls panic.
type errStreamStorage struct{ err error }

func (errStreamStorage) Read(context.Context, string) ([]byte, error) { panic("unused") }
func (errStreamStorage) Write(context.Context, string, []byte) error  { panic("unused") }
func (errStreamStorage) WriteStream(context.Context, string, io.Reader) error {
	panic("unused")
}
func (s errStreamStorage) ReadStream(context.Context, string) (io.ReadCloser, error) {
	return &errReadCloser{err: s.err}, nil
}
func (errStreamStorage) Exists(context.Context, string) bool            { panic("unused") }
func (errStreamStorage) List(context.Context, string) ([]string, error) { panic("unused") }
func (errStreamStorage) Delete(context.Context, string) error           { panic("unused") }
func (errStreamStorage) DeleteByPrefix(context.Context, string) error   { panic("unused") }
