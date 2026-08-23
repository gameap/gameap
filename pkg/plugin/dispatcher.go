package plugin

import (
	"context"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/idgen"
	"github.com/gameap/gameap/pkg/plugin/proto"
	domainproto "github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
)

// EventDispatchResult contains the result of dispatching an event.
type EventDispatchResult struct {
	// Cancelled indicates if the event was cancelled by a plugin.
	Cancelled bool
	// CancelledBy contains the plugin ID that cancelled the event.
	CancelledBy string
	// CancelMessage contains the cancellation message if any.
	CancelMessage string
	// HandledBy contains the list of plugin IDs that handled the event.
	HandledBy []string
	// ModifiedData contains any data modified by plugins.
	ModifiedData map[string]string
	// Errors contains any errors that occurred during dispatch.
	Errors []error
}

const (
	// defaultEventCallTimeout bounds a single plugin HandleEvent call.
	// On expiry the runtime closes the module, so the plugin is disabled
	// until it is reloaded.
	defaultEventCallTimeout = 10 * time.Second
	// asyncDispatchTimeout bounds the total delivery of one fire-and-forget
	// event across all subscribers.
	asyncDispatchTimeout = 60 * time.Second
	// maxAsyncDispatches caps concurrent background deliveries. A hung plugin
	// parks callers on its wrapper mutex (mutex waits ignore contexts), so
	// without a bound every incoming operation would grow the goroutine pile
	// until the per-call timeout disables the plugin.
	maxAsyncDispatches = 64
)

// SubscriptionGate decides whether a plugin that subscribes to events may
// receive them; it is consulted on every RefreshSubscriptions for plugins
// with a non-empty subscription list. The panel gates on the plugin's
// "listen_events" grant.
type SubscriptionGate func(ctx context.Context, plugin *LoadedPlugin) bool

// DispatcherOption tunes a Dispatcher.
type DispatcherOption func(*Dispatcher)

// WithSubscriptionGate installs the gate; nil admits every plugin.
func WithSubscriptionGate(gate SubscriptionGate) DispatcherOption {
	return func(d *Dispatcher) {
		d.gate = gate
	}
}

// WithDispatcherObserver reports delivery outcomes and drops to the observer.
func WithDispatcherObserver(observer Observer) DispatcherOption {
	return func(d *Dispatcher) {
		d.observer = observerOrNop(observer)
	}
}

// Dispatcher handles event dispatching to plugins.
type Dispatcher struct {
	mu              sync.RWMutex
	manager         *Manager
	subscriptions   map[proto.EventType][]*LoadedPlugin
	logger          *slog.Logger
	callTimeout     time.Duration
	asyncSlots      chan struct{}
	subscriptionsOK bool
	gate            SubscriptionGate
	observer        Observer
}

// NewDispatcher creates a new event dispatcher.
func NewDispatcher(manager *Manager, logger *slog.Logger, opts ...DispatcherOption) *Dispatcher {
	d := &Dispatcher{
		manager:       manager,
		subscriptions: make(map[proto.EventType][]*LoadedPlugin),
		logger:        logger,
		callTimeout:   defaultEventCallTimeout,
		asyncSlots:    make(chan struct{}, maxAsyncDispatches),
		observer:      NopObserver{},
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// RefreshSubscriptions queries all plugins for their subscribed events.
func (d *Dispatcher) RefreshSubscriptions(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.subscriptions = make(map[proto.EventType][]*LoadedPlugin)
	d.subscriptionsOK = false

	plugins := d.manager.GetPlugins()
	for _, plugin := range plugins {
		if !plugin.IsEnabled() {
			continue
		}

		resp, err := plugin.Instance.GetSubscribedEvents(ctx, &proto.GetSubscribedEventsRequest{})
		if err != nil {
			d.logger.Error("failed to get subscribed events",
				slog.String("plugin_id", plugin.Info.Id),
				slog.Any("error", err),
			)

			continue
		}

		if len(resp.Events) == 0 {
			continue
		}

		if d.gate != nil && !d.gate(ctx, plugin) {
			d.logger.Warn("plugin subscribes to events without the listen_events grant, subscriptions ignored",
				slog.String("plugin_id", plugin.Info.Id),
				slog.Int("events", len(resp.Events)),
			)

			continue
		}

		for _, eventType := range resp.Events {
			d.subscriptions[eventType] = append(d.subscriptions[eventType], plugin)
		}
	}

	d.subscriptionsOK = true

	return nil
}

// AsyncBacklog reports how many fire-and-forget deliveries are in flight or
// queued (bounded by maxAsyncDispatches).
func (d *Dispatcher) AsyncBacklog() int {
	return len(d.asyncSlots)
}

// Dispatch dispatches an event to all subscribed plugins. A nil dispatcher
// (plugins disabled) delivers nothing.
func (d *Dispatcher) Dispatch(ctx context.Context, event *proto.Event) *EventDispatchResult {
	result := &EventDispatchResult{
		HandledBy:    make([]string, 0),
		ModifiedData: make(map[string]string),
		Errors:       make([]error, 0),
	}

	if d == nil {
		return result
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	subscribers := d.subscriptions[event.Type]
	if len(subscribers) == 0 {
		return result
	}

	cancellable := isCancellableEvent(event.Type)

	for _, plugin := range subscribers {
		if !plugin.IsEnabled() || isPluginEventSubject(plugin, event) {
			continue
		}

		eventResult, err := d.handleEvent(ctx, plugin, withSubscriberContext(event, plugin))
		if err != nil {
			result.Errors = append(result.Errors, errors.Wrapf(
				err, "plugin %s failed to handle event", plugin.Info.Id,
			))

			d.logger.Error("plugin failed to handle event",
				slog.String("plugin_id", plugin.Info.Id),
				slog.Int("event_type", int(event.Type)),
				slog.Any("error", err),
			)

			d.observer.EventDispatched(event.Type, EventResultError)

			continue
		}

		if eventResult.Handled {
			result.HandledBy = append(result.HandledBy, plugin.Info.Id)
		}

		if cancellable && eventResult.ShouldCancel {
			result.Cancelled = true
			result.CancelledBy = plugin.Info.Id
			if eventResult.Message != nil {
				result.CancelMessage = *eventResult.Message
			}

			d.observer.EventDispatched(event.Type, EventResultCancelled)

			return result
		}

		if eventResult.Handled {
			d.observer.EventDispatched(event.Type, EventResultHandled)
		} else {
			d.observer.EventDispatched(event.Type, EventResultIgnored)
		}

		maps.Copy(result.ModifiedData, eventResult.ModifiedData)
	}

	return result
}

// handleEvent calls a single plugin with a per-call deadline. On expiry the
// module is closed by the runtime, so the plugin is disabled to stop routing
// further calls to a dead instance.
func (d *Dispatcher) handleEvent(
	ctx context.Context,
	plugin *LoadedPlugin,
	event *proto.Event,
) (*proto.EventResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, d.callTimeout)
	defer cancel()

	eventResult, err := plugin.Instance.HandleEvent(callCtx, event)
	// ErrPluginBusy means the wait for the call gate was abandoned — the
	// module is untouched, so the plugin must stay enabled.
	if err != nil && callCtx.Err() != nil && ctx.Err() == nil && !errors.Is(err, ErrPluginBusy) {
		plugin.DisableWithReason(EventTimeoutReason(event.Type))

		d.logger.Error("plugin event handler timed out, plugin disabled until reload",
			slog.String("plugin_id", plugin.Info.Id),
			slog.Int("event_type", int(event.Type)),
			slog.Duration("timeout", d.callTimeout),
		)
	}

	return eventResult, err
}

// DispatchServerEvent is a convenience method to dispatch a server event.
func (d *Dispatcher) DispatchServerEvent(
	ctx context.Context,
	eventType proto.EventType,
	server *domain.Server,
	extraData map[string]string,
) *EventDispatchResult {
	return d.Dispatch(ctx, buildServerEvent(ctx, eventType, server, extraData))
}

// DispatchServerEventAsync dispatches a server event in the background.
// The event payload is snapshotted synchronously; delivery errors are only
// logged by Dispatch.
func (d *Dispatcher) DispatchServerEventAsync(
	ctx context.Context,
	eventType proto.EventType,
	server *domain.Server,
	extraData map[string]string,
) {
	d.dispatchAsync(ctx, buildServerEvent(ctx, eventType, server, extraData))
}

// DispatchServerEventsAsync dispatches several server events for one operation
// in the background, delivering them sequentially in the given order (a single
// goroutine handles the whole batch).
func (d *Dispatcher) DispatchServerEventsAsync(
	ctx context.Context,
	eventTypes []proto.EventType,
	server *domain.Server,
	extraData map[string]string,
) {
	events := make([]*proto.Event, 0, len(eventTypes))
	for _, eventType := range eventTypes {
		events = append(events, buildServerEvent(ctx, eventType, server, extraData))
	}

	d.dispatchAsync(ctx, events...)
}

// DispatchTaskEvent is a convenience method to dispatch a task event.
func (d *Dispatcher) DispatchTaskEvent(
	ctx context.Context,
	eventType proto.EventType,
	taskID, nodeID uint,
	serverID *uint,
	taskType, status string,
	extraData map[string]string,
) *EventDispatchResult {
	return d.Dispatch(ctx, buildTaskEvent(ctx, eventType, taskID, nodeID, serverID, taskType, status, extraData))
}

// DispatchTaskEventAsync dispatches a task event in the background.
func (d *Dispatcher) DispatchTaskEventAsync(
	ctx context.Context,
	eventType proto.EventType,
	taskID, nodeID uint,
	serverID *uint,
	taskType, status string,
	extraData map[string]string,
) {
	d.dispatchAsync(ctx, buildTaskEvent(ctx, eventType, taskID, nodeID, serverID, taskType, status, extraData))
}

// DispatchUserEvent dispatches a user event synchronously; USER_PRE_DELETE
// may be cancelled by a plugin.
func (d *Dispatcher) DispatchUserEvent(
	ctx context.Context,
	eventType proto.EventType,
	user *domain.User,
	extraData map[string]string,
) *EventDispatchResult {
	return d.Dispatch(ctx, buildUserEvent(ctx, eventType, user, extraData))
}

// DispatchUserEventAsync dispatches a user event in the background.
func (d *Dispatcher) DispatchUserEventAsync(
	ctx context.Context,
	eventType proto.EventType,
	user *domain.User,
	extraData map[string]string,
) {
	if d == nil {
		return
	}

	d.dispatchAsync(ctx, buildUserEvent(ctx, eventType, user, extraData))
}

// DispatchNodeEvent dispatches a node event synchronously; NODE_PRE_DELETE
// may be cancelled by a plugin.
func (d *Dispatcher) DispatchNodeEvent(
	ctx context.Context,
	eventType proto.EventType,
	node *domain.Node,
	extraData map[string]string,
) *EventDispatchResult {
	return d.Dispatch(ctx, buildNodeEvent(ctx, eventType, node, extraData))
}

// DispatchNodeEventAsync dispatches a node event in the background.
func (d *Dispatcher) DispatchNodeEventAsync(
	ctx context.Context,
	eventType proto.EventType,
	node *domain.Node,
	extraData map[string]string,
) {
	if d == nil {
		return
	}

	d.dispatchAsync(ctx, buildNodeEvent(ctx, eventType, node, extraData))
}

// DispatchServerSettingsEventAsync publishes the server settings a save
// changed, in the background.
func (d *Dispatcher) DispatchServerSettingsEventAsync(
	ctx context.Context,
	serverID uint,
	settings []domain.ServerSetting,
	extraData map[string]string,
) {
	if d == nil {
		return
	}

	d.dispatchAsync(ctx, buildServerSettingsEvent(ctx, serverID, settings, extraData))
}

func buildUserEvent(
	ctx context.Context,
	eventType proto.EventType,
	user *domain.User,
	extraData map[string]string,
) *proto.Event {
	return &proto.Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Context:   buildEventContext(ctx),
		Payload: &proto.Event_UserEvent{
			UserEvent: &proto.UserEventPayload{
				User:      DomainUserToProto(user),
				ExtraData: extraData,
			},
		},
	}
}

func buildNodeEvent(
	ctx context.Context,
	eventType proto.EventType,
	node *domain.Node,
	extraData map[string]string,
) *proto.Event {
	return &proto.Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Context:   buildEventContext(ctx),
		Payload: &proto.Event_NodeEvent{
			NodeEvent: &proto.NodeEventPayload{
				Node:      DomainNodeToProto(node),
				ExtraData: extraData,
			},
		},
	}
}

func buildServerSettingsEvent(
	ctx context.Context,
	serverID uint,
	settings []domain.ServerSetting,
	extraData map[string]string,
) *proto.Event {
	converted := make([]*domainproto.ServerSetting, 0, len(settings))
	for _, setting := range settings {
		converted = append(converted, DomainServerSettingToProto(setting))
	}

	return &proto.Event{
		Type:      proto.EventType_EVENT_TYPE_SERVER_SETTINGS_CHANGED,
		Timestamp: time.Now().Unix(),
		Context:   buildEventContext(ctx),
		Payload: &proto.Event_ServerSettingsEvent{
			ServerSettingsEvent: &proto.ServerSettingsEventPayload{
				ServerId:  uint64(serverID),
				Settings:  converted,
				ExtraData: extraData,
			},
		},
	}
}

// EventInfo describes the plugin a PLUGIN_* event is about.
type EventInfo struct {
	DBID    uint64
	Name    string
	Version string
	Status  string
	Error   *string
}

// DispatchPluginEventAsync publishes a plugin lifecycle transition in the
// background; the plugin the event describes never receives it.
func (d *Dispatcher) DispatchPluginEventAsync(
	ctx context.Context,
	eventType proto.EventType,
	info EventInfo,
	extraData map[string]string,
) {
	if d == nil {
		return
	}

	d.dispatchAsync(ctx, buildPluginEvent(ctx, eventType, info, extraData))
}

func buildPluginEvent(
	ctx context.Context,
	eventType proto.EventType,
	info EventInfo,
	extraData map[string]string,
) *proto.Event {
	return &proto.Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Context:   buildEventContext(ctx),
		Payload: &proto.Event_PluginEvent{
			PluginEvent: &proto.PluginEventPayload{
				PluginId:  CompactPluginID(domain.Uint64ID(info.DBID)),
				Name:      info.Name,
				Version:   info.Version,
				Status:    info.Status,
				Error:     info.Error,
				ExtraData: extraData,
			},
		},
	}
}

func (d *Dispatcher) dispatchAsync(ctx context.Context, events ...*proto.Event) {
	if len(events) == 0 {
		return
	}

	select {
	case d.asyncSlots <- struct{}{}:
	default:
		// Fire-and-forget events are advisory; when the backlog is full
		// (plugins wedged for longer than the disable timeout can drain),
		// dropping the whole ordered batch is safer than blocking the
		// caller or queueing without bound.
		eventTypes := make([]int, 0, len(events))
		for _, event := range events {
			eventTypes = append(eventTypes, int(event.Type))
		}

		d.logger.Error("async plugin events dropped, dispatch backlog is full",
			slog.Any("event_types", eventTypes),
			slog.Int("capacity", cap(d.asyncSlots)),
		)

		for _, event := range events {
			d.observer.EventDispatched(event.Type, EventResultDropped)
		}

		return
	}

	bgCtx := context.WithoutCancel(ctx)

	go func() {
		defer func() { <-d.asyncSlots }()

		dispatchCtx, cancel := context.WithTimeout(bgCtx, asyncDispatchTimeout)
		defer cancel()

		for _, event := range events {
			d.Dispatch(dispatchCtx, event)
		}
	}()
}

// buildEventContext identifies the delivery and, when the triggering request
// was made by an authenticated user, that user — so a plugin can attribute
// what it does in response (and the panel audits it the same way).
func buildEventContext(ctx context.Context) *proto.PluginContext {
	eventCtx := &proto.PluginContext{
		RequestId: idgen.New(),
	}

	if session := auth.SessionFromContext(ctx); session.IsAuthenticated() {
		eventCtx.UserId = new(uint64(session.User.ID))
	}

	return eventCtx
}

func buildServerEvent(
	ctx context.Context,
	eventType proto.EventType,
	server *domain.Server,
	extraData map[string]string,
) *proto.Event {
	return &proto.Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Context:   buildEventContext(ctx),
		Payload: &proto.Event_ServerEvent{
			ServerEvent: &proto.ServerEventPayload{
				Server:    domainServerToProto(server),
				ExtraData: extraData,
			},
		},
	}
}

func buildTaskEvent(
	ctx context.Context,
	eventType proto.EventType,
	taskID, nodeID uint,
	serverID *uint,
	taskType, status string,
	extraData map[string]string,
) *proto.Event {
	payload := &proto.TaskEventPayload{
		TaskId:    uint64(taskID),
		NodeId:    uint64(nodeID),
		TaskType:  taskType,
		Status:    status,
		ExtraData: extraData,
	}
	if serverID != nil {
		sid := uint64(*serverID)
		payload.ServerId = &sid
	}

	return &proto.Event{
		Type:      eventType,
		Timestamp: time.Now().Unix(),
		Context:   buildEventContext(ctx),
		Payload: &proto.Event_TaskEvent{
			TaskEvent: payload,
		},
	}
}

// HasSubscribers returns true if there are any subscribers for the event type.
func (d *Dispatcher) HasSubscribers(eventType proto.EventType) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	subscribers := d.subscriptions[eventType]
	for _, p := range subscribers {
		if p.IsEnabled() {
			return true
		}
	}

	return false
}

// isCancellableEvent returns true if the event type supports cancellation.
func isCancellableEvent(eventType proto.EventType) bool {
	switch eventType {
	case proto.EventType_EVENT_TYPE_SERVER_PRE_START,
		proto.EventType_EVENT_TYPE_SERVER_PRE_STOP,
		proto.EventType_EVENT_TYPE_SERVER_PRE_RESTART,
		proto.EventType_EVENT_TYPE_SERVER_PRE_INSTALL,
		proto.EventType_EVENT_TYPE_SERVER_PRE_UPDATE,
		proto.EventType_EVENT_TYPE_SERVER_PRE_REINSTALL,
		proto.EventType_EVENT_TYPE_SERVER_PRE_DELETE,
		proto.EventType_EVENT_TYPE_USER_PRE_DELETE,
		proto.EventType_EVENT_TYPE_NODE_PRE_DELETE:
		return true
	default:
		return false
	}
}

// withSubscriberContext returns a shallow copy of the event whose context
// names the receiving plugin (the id it declared, as Initialize and Shutdown
// use); the payload is shared between subscribers.
func withSubscriberContext(event *proto.Event, plugin *LoadedPlugin) *proto.Event {
	if plugin.Info == nil {
		return event
	}

	eventCtx := &proto.PluginContext{PluginId: plugin.Info.Id}
	if event.Context != nil {
		eventCtx.RequestId = event.Context.RequestId
		eventCtx.UserId = event.Context.UserId
		eventCtx.Permissions = event.Context.Permissions
	}

	return &proto.Event{
		Type:      event.Type,
		Context:   eventCtx,
		Timestamp: event.Timestamp,
		Payload:   event.Payload,
	}
}

// isPluginEventSubject reports whether a PLUGIN_* event describes the plugin
// itself: a plugin never hears about its own lifecycle, its module was not
// running when it happened.
func isPluginEventSubject(plugin *LoadedPlugin, event *proto.Event) bool {
	payload := event.GetPluginEvent()
	if payload == nil {
		return false
	}

	if plugin.DBID != 0 && CompactPluginID(domain.Uint64ID(plugin.DBID)) == payload.PluginId {
		return true
	}

	return plugin.Info != nil && normalizePluginID(plugin.Info.Id) == normalizePluginID(payload.PluginId)
}
