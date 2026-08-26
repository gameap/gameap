package plugin

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
)

// Statuses carried by PluginEventPayload.status.
const (
	pluginEventStatusActive      = "active"
	pluginEventStatusError       = "error"
	pluginEventStatusUnloaded    = "unloaded"
	pluginEventStatusUninstalled = "uninstalled"
)

// emitPluginEvent publishes a lifecycle transition of the plugin to the other
// plugins. Delivery is asynchronous, so it is safe to call while the
// plugin's lifecycle lock is held.
func (l *Loader) emitPluginEvent(
	ctx context.Context,
	eventType proto.EventType,
	plugin *domain.Plugin,
	info *proto.PluginInfo,
	trigger string,
	loadErr error,
) {
	if l.events == nil {
		return
	}

	event := pkgplugin.EventInfo{
		DBID:    uint64(plugin.ID),
		Name:    plugin.Name,
		Version: plugin.Version,
		Status:  pluginEventStatusActive,
	}

	if info != nil {
		event.Name = info.Name
		event.Version = info.Version
	}

	if loadErr != nil {
		event.Status = pluginEventStatusError
		event.Error = new(LoadErrorText(loadErr))
	}

	l.events.DispatchPluginEventAsync(ctx, eventType, event, map[string]string{"trigger": trigger})
}

// emitUnloaded publishes that the module for the row stopped running.
func (l *Loader) emitUnloaded(ctx context.Context, dbID domain.Uint64ID, info *proto.PluginInfo, trigger string) {
	if l.events == nil {
		return
	}

	event := pkgplugin.EventInfo{DBID: uint64(dbID), Status: pluginEventStatusUnloaded}
	if trigger == TriggerUninstall {
		event.Status = pluginEventStatusUninstalled
	}

	if info != nil {
		event.Name = info.Name
		event.Version = info.Version
	}

	l.events.DispatchPluginEventAsync(ctx, proto.EventType_EVENT_TYPE_PLUGIN_UNLOADED, event,
		map[string]string{"trigger": trigger})
}

// emitRuntimeError publishes a runtime disable recorded by the supervisor.
func (l *Loader) emitRuntimeError(ctx context.Context, plugin *domain.Plugin, reason string) {
	if l == nil || l.events == nil {
		return
	}

	l.events.DispatchPluginEventAsync(ctx, proto.EventType_EVENT_TYPE_PLUGIN_ERROR, pkgplugin.EventInfo{
		DBID:    uint64(plugin.ID),
		Name:    plugin.Name,
		Version: plugin.Version,
		Status:  pluginEventStatusError,
		Error:   new(reason),
	}, map[string]string{"trigger": "runtime_disable"})

	slog.DebugContext(ctx, "plugin error event published",
		slog.Uint64("plugin_id", uint64(plugin.ID)),
		slog.String("reason", reason))
}
