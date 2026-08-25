package pluginssh

import "sync"

// capturedStream collects the head of a remote stream up to a cap. Write never
// blocks and never fails: an error or a stall here would freeze the SSH channel
// window and hang the remote process, so overflow is simply dropped and the
// stream is flagged truncated.
type capturedStream struct {
	mu            sync.Mutex
	buf           []byte
	capacity      int
	total         uint64
	truncated     bool
	transferLimit uint64
	onOverflow    func()
	overflowFired bool
}

// transferLimitMultiplier scales the panel's capture cap into the total the
// remote side may send at all. The capture cap protects memory; this one
// protects bandwidth and CPU — without it a discarded stream still lets
// "cat /dev/urandom" pump data at line rate for the whole exec timeout.
const transferLimitMultiplier = 8

func newCapturedStream(capacity int) *capturedStream {
	return &capturedStream{capacity: capacity}
}

func (s *capturedStream) Write(p []byte) (int, error) {
	s.mu.Lock()

	s.total += uint64(len(p))

	free := s.capacity - len(s.buf)
	switch {
	case free <= 0:
		if len(p) > 0 {
			s.truncated = true
		}
	case len(p) > free:
		s.buf = append(s.buf, p[:free]...)
		s.truncated = true
	default:
		s.buf = append(s.buf, p...)
	}

	overflow := s.takeOverflowLocked()
	s.mu.Unlock()

	if overflow != nil {
		overflow()
	}

	return len(p), nil
}

// armTransferLimit caps the total number of bytes the remote side may send on
// this stream. The callback fires exactly once and outside the stream lock;
// arming after the limit was already crossed fires immediately, which closes
// the gap between session start and operation registration.
func (s *capturedStream) armTransferLimit(limit uint64, onOverflow func()) {
	s.mu.Lock()
	s.transferLimit = limit
	s.onOverflow = onOverflow
	overflow := s.takeOverflowLocked()
	s.mu.Unlock()

	if overflow != nil {
		overflow()
	}
}

func (s *capturedStream) takeOverflowLocked() func() {
	if s.transferLimit == 0 || s.onOverflow == nil || s.overflowFired || s.total <= s.transferLimit {
		return nil
	}
	s.overflowFired = true

	return s.onOverflow
}

// slice returns a copy of the captured bytes from offset onwards, plus the
// offset to pass back next time.
func (s *capturedStream) slice(offset uint64) (chunk []byte, next uint64, truncated bool, total uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	captured := uint64(len(s.buf))
	if offset > captured {
		offset = captured
	}

	chunk = make([]byte, captured-offset)
	copy(chunk, s.buf[offset:])

	return chunk, captured, s.truncated, s.total
}
