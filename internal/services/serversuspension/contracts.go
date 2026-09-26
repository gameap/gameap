package serversuspension

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

// serverStopper queues the stop of a server: servercontrol.Service, so a
// suspension stops the server the same way the stop button does.
type serverStopper interface {
	Stop(ctx context.Context, server *domain.Server) (uint, error)
}

// configPusher sends the saved server to its daemon at once, so the daemon
// refuses starts from that moment instead of from the next task or reconnect.
type configPusher interface {
	PushServerConfig(ctx context.Context, serverID uint)
}
