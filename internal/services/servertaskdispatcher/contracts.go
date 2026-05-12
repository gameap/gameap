package servertaskdispatcher

import (
	"context"

	"github.com/gameap/gameap/pkg/proto"
)

// Sender abstracts the bidi session forwarding. Implemented by
// session.Registry: SendTask routes to the local stream when present,
// otherwise via pubsub to the API instance that owns the daemon's session.
type Sender interface {
	SendTask(ctx context.Context, nodeID uint64, msg *proto.GatewayMessage) error
}
