package taskdispatcher

import (
	"context"

	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
)

// PluginEventDispatcher delivers daemon task lifecycle events to plugins.
type PluginEventDispatcher interface {
	DispatchTaskEventAsync(
		ctx context.Context,
		eventType pluginproto.EventType,
		taskID, nodeID uint,
		serverID *uint,
		taskType, status string,
		extraData map[string]string,
	)
}
