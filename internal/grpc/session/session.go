package session

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
)

// outboundBufferSize bounds the per-session queue drained by Enqueue's worker.
// It absorbs bursts while a daemon is briefly backpressured; once full, Enqueue
// drops rather than blocking its caller (see Enqueue).
const outboundBufferSize = 256

// ErrSessionClosed is returned by Enqueue after the session has been Closed.
var ErrSessionClosed = errors.New("session is closed")

// ErrOutboundFull is returned by Enqueue when the outbound queue is saturated —
// the daemon is too far behind to accept more asynchronously delivered messages.
var ErrOutboundFull = errors.New("session outbound queue is full")

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
	logger *slog.Logger

	// sendMu serializes Stream.Send: a gRPC ServerStream is not safe for
	// concurrent Send, and pubsub handlers, broadcasts and request senders
	// all push to the same stream.
	sendMu sync.Mutex

	// outbound + its lazily-started worker back Enqueue, the non-blocking
	// delivery path used by the shared pubsub goroutine so a slow daemon Send
	// cannot stall it (head-of-line blocking). The worker funnels back through
	// Send, so sendMu still guarantees a single concurrent Stream.Send.
	outbound   chan *proto.GatewayMessage
	done       chan struct{}
	workerOnce sync.Once
	closeOnce  sync.Once
	closed     atomic.Bool

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
		logger:       slog.Default(),
		outbound:     make(chan *proto.GatewayMessage, outboundBufferSize),
		done:         make(chan struct{}),
		lastPing:     time.Now(),
		pendingReqs:  make(map[string]chan *proto.DaemonMessage),
	}
}

// SetLogger overrides the session's logger. Optional; NewSession defaults to
// slog.Default(). Call before Enqueue is first used.
func (s *Session) SetLogger(logger *slog.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

// Send synchronously writes msg to the daemon stream, serialized by sendMu.
// Callers running on their own goroutine (request/response senders) use this
// directly; the shared pubsub delivery goroutine must use Enqueue instead.
func (s *Session) Send(msg *proto.GatewayMessage) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	return s.Stream.Send(msg)
}

// Enqueue hands msg to the session's outbound worker without blocking the
// caller on the network. It is the delivery path for the single pubsub
// goroutine: a stalled daemon backs up this session's bounded queue only, never
// the shared goroutine. Returns ErrOutboundFull if the queue is saturated or
// ErrSessionClosed after Close.
func (s *Session) Enqueue(msg *proto.GatewayMessage) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}

	s.workerOnce.Do(func() { go s.dispatchLoop() })

	select {
	case s.outbound <- msg:
		return nil
	case <-s.done:
		return ErrSessionClosed
	default:
		s.logger.Warn("daemon outbound queue full, dropping message",
			"node_id", s.NodeID,
		)

		return ErrOutboundFull
	}
}

func (s *Session) dispatchLoop() {
	for {
		select {
		case <-s.done:
			return
		case msg := <-s.outbound:
			if err := s.Send(msg); err != nil {
				s.logger.Debug("failed to send queued daemon message",
					"node_id", s.NodeID,
					"error", err,
				)
			}
		}
	}
}

// Close stops the outbound worker and fails every in-flight request waiter.
// Idempotent; invoked by the gateway when the daemon stream ends so neither the
// worker goroutine nor a request waiting on a response outlives the session:
// without this, a caller blocked in RequestFile*/RequestCommand* would hang on
// its own context deadline (or forever, for a background context) after the
// daemon disconnected.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.done)
		s.failPendingRequests()
	})
}

// failPendingRequests closes every registered response channel so its waiter
// unblocks immediately (a closed channel yields a nil response, which callers
// treat as "request cancelled"). Delete-under-lock keeps this race-free against
// ResolvePendingRequest / CancelPendingRequest, which also delete before close.
func (s *Session) failPendingRequests() {
	s.mu.Lock()
	for requestID, ch := range s.pendingReqs {
		delete(s.pendingReqs, requestID)
		close(ch)
	}
	s.mu.Unlock()
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
