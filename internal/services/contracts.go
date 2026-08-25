package services

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
)

// NodeEventVeto is a plugin's answer to a synchronous node event. It mirrors
// the parts of plugin.EventDispatchResult this package acts on; the full type
// is not used because pkg/plugin reaches back into internal/services through
// its server control adapter.
type NodeEventVeto struct {
	Cancelled     bool
	CancelledBy   string
	CancelMessage string
}

// NodePluginDispatcher lets plugins veto a node deletion (NODE_PRE_DELETE) and
// learn about it afterwards (NODE_DELETED). The admin delete handler fires the
// same pair, so a node removed through a plugin is not invisible to the others.
type NodePluginDispatcher interface {
	DispatchNodeEvent(
		ctx context.Context,
		eventType pluginproto.EventType,
		node *domain.Node,
		extraData map[string]string,
	) NodeEventVeto
	DispatchNodeEventAsync(
		ctx context.Context,
		eventType pluginproto.EventType,
		node *domain.Node,
		extraData map[string]string,
	)
}
