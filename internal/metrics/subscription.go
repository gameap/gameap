package metrics

import (
	"sync"

	"github.com/gameap/gameap/pkg/proto"
)

type subscription struct {
	hub    *hub
	nodeID uint64

	samplesCh chan *proto.MetricsResponse

	// mu is an RWMutex so concurrent deliver fan-out (read side) is not
	// serialized against itself — only against the exclusive channel close.
	// closed reports that samplesCh has been closed; unsubscribed guards Close
	// from running the unsubscribe path more than once.
	mu           sync.RWMutex
	closed       bool
	unsubscribed bool
}

func newSubscription(h *hub, nodeID uint64, bufferSize int) *subscription {
	return &subscription{
		hub:       h,
		nodeID:    nodeID,
		samplesCh: make(chan *proto.MetricsResponse, bufferSize),
	}
}

func (s *subscription) Samples() <-chan *proto.MetricsResponse {
	return s.samplesCh
}

func (s *subscription) Close() {
	s.mu.Lock()
	if s.unsubscribed {
		s.mu.Unlock()

		return
	}
	s.unsubscribed = true
	s.mu.Unlock()

	s.hub.unsubscribe(s)
}

// deliver attempts a non-blocking send. Drops on full buffer to keep
// the producer (pubsub fan-out) non-blocking; consumers are expected
// to drain promptly.
func (s *subscription) deliver(entry *proto.MetricsResponse) {
	// Hold the read lock across the non-blocking send so it cannot race
	// closeChannel (write lock) into a send-on-closed-channel panic, while
	// still letting concurrent deliver calls run in parallel.
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return
	}

	select {
	case s.samplesCh <- entry:
	default:
	}
}

func (s *subscription) closeChannel() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	close(s.samplesCh)
}
