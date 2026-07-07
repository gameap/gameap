package metrics

import (
	"context"
	"time"

	"github.com/gameap/gameap/pkg/proto"
)

// Hub orchestrates per-node metrics polling, refcount tracking, live
// fan-out to local subscribers and history requests against the daemon.
type Hub interface {
	Start(ctx context.Context) error

	// Subscribe registers a subscriber for a node. The returned slice
	// contains replay entries (oldest → newest) for the requested window
	// and may be empty. Live samples flow through Subscription.Samples().
	Subscribe(
		ctx context.Context, nodeID uint64, replayWindow time.Duration,
	) (Subscription, []*proto.MetricsResponse, error)

	// GetHistory fetches recent metrics for the requested window
	// directly from the daemon. Used as a cold-start replay source when
	// the local ring does not cover the window.
	GetHistory(ctx context.Context, nodeID uint64, window time.Duration) (*proto.MetricsResponse, error)

	// Stop cancels the hub's background loops and releases per-node poll
	// cancels, stop-timers and subscriber channels. Called during graceful
	// shutdown so those resources do not outlive the serving phase.
	Stop()
}

// Subscription is what WS handlers consume. Samples are delivered in
// order. Close releases all resources and decrements refcount.
type Subscription interface {
	Samples() <-chan *proto.MetricsResponse
	Close()
}

// Registry abstracts the subset of session.Registry the Hub needs.
type Registry interface {
	InstanceID() string
	IsConnected(nodeID uint64) bool
	IsConnectedAnywhere(nodeID uint64) bool
	SendMetricsRequest(ctx context.Context, nodeID uint64, requestID string, req *proto.MetricsRequest) error
	ConnectedNodeIDs() []uint64
}

// HandlerWaiters is the subset of handlers.MetricsHandler that the Hub
// needs to register/cancel waiters.
type HandlerWaiters interface {
	RegisterPollWaiter(requestID string, nodeID uint64)
	RegisterRemoteWaiter(requestID string, nodeID uint64, requesterInstanceID string)
	CancelWaiter(requestID string)
}
