package pluginssh

import "sync"

// capturedStream collects the head of a remote stream up to a cap. Write never
// blocks and never fails: an error or a stall here would freeze the SSH channel
// window and hang the remote process, so overflow is simply dropped and the
// stream is flagged truncated.
type capturedStream struct {
	mu        sync.Mutex
	buf       []byte
	capacity  int
	total     uint64
	truncated bool
}

func newCapturedStream(capacity int) *capturedStream {
	return &capturedStream{capacity: capacity}
}

func (s *capturedStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.total += uint64(len(p))

	free := s.capacity - len(s.buf)
	if free <= 0 {
		if len(p) > 0 {
			s.truncated = true
		}

		return len(p), nil
	}

	if len(p) > free {
		s.buf = append(s.buf, p[:free]...)
		s.truncated = true

		return len(p), nil
	}

	s.buf = append(s.buf, p...)

	return len(p), nil
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
