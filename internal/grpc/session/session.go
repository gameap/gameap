package session

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/gameap/gameap/pkg/proto"
)

type Stream interface {
	Send(*proto.GatewayMessage) error
	Recv() (*proto.DaemonMessage, error)
	Context() context.Context
}

type Session struct {
	NodeID       uint64
	Stream       Stream
	Version      string
	Capabilities []string
	ConnectedAt  time.Time

	cancel context.CancelFunc

	// sendMu serializes Stream.Send: a gRPC ServerStream is not safe for
	// concurrent Send, and pubsub handlers, broadcasts and request senders
	// all push to the same stream.
	sendMu sync.Mutex

	mu          sync.RWMutex
	lastPing    time.Time
	pendingReqs map[string]chan *proto.DaemonMessage
}

func NewSession(
	nodeID uint64, stream Stream, version string, capabilities []string, cancel context.CancelFunc,
) *Session {
	return &Session{
		NodeID:       nodeID,
		Stream:       stream,
		Version:      version,
		Capabilities: capabilities,
		ConnectedAt:  time.Now(),
		cancel:       cancel,
		lastPing:     time.Now(),
		pendingReqs:  make(map[string]chan *proto.DaemonMessage),
	}
}

func (s *Session) Send(msg *proto.GatewayMessage) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	return s.Stream.Send(msg)
}

func (s *Session) UpdateLastPing() {
	s.mu.Lock()
	s.lastPing = time.Now()
	s.mu.Unlock()
}

func (s *Session) LastPing() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.lastPing
}

func (s *Session) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *Session) RegisterPendingRequest(requestID string) chan *proto.DaemonMessage {
	ch := make(chan *proto.DaemonMessage, 1)
	s.mu.Lock()
	s.pendingReqs[requestID] = ch
	s.mu.Unlock()

	return ch
}

func (s *Session) ResolvePendingRequest(requestID string, msg *proto.DaemonMessage) bool {
	s.mu.Lock()
	ch, ok := s.pendingReqs[requestID]
	if ok {
		delete(s.pendingReqs, requestID)
	}
	s.mu.Unlock()

	if ok {
		select {
		case ch <- msg:
		default:
		}
		close(ch)
	}

	return ok
}

func (s *Session) HasCapability(capability string) bool {
	return slices.Contains(s.Capabilities, capability)
}

func (s *Session) CancelPendingRequest(requestID string) {
	s.mu.Lock()
	ch, ok := s.pendingReqs[requestID]
	if ok {
		delete(s.pendingReqs, requestID)
		close(ch)
	}
	s.mu.Unlock()
}
