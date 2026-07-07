package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
)

// Bridge connects the shared PubSub realtime channels to the local in-process
// Hub on a single API instance.
//
// In a multi-instance deployment events are published to a shared PubSub
// (Redis or Postgres) so any instance can observe them, but WebSocket
// connections live on exactly one instance. The Bridge subscribes to the
// realtime channel patterns once on startup and, for each incoming PubSub
// message, translates the channel name to a local topic and invokes
// Hub.Broadcast. This way each API instance only fans out to the WebSocket
// clients connected locally, while the source of the event can be anywhere.
//
// The Bridge owns no goroutines of its own; subscription delivery is driven
// by the underlying pubsub.Subscriber implementation.
type Bridge struct {
	hub    *Hub
	ps     pubsub.Subscriber
	logger *slog.Logger
}

// NewBridge constructs a Bridge that will deliver PubSub messages to the
// given Hub via the provided Subscriber.
//
// If logger is nil, slog.Default is used. The returned Bridge is inert until
// Start is called.
func NewBridge(hub *Hub, ps pubsub.Subscriber, logger *slog.Logger) *Bridge {
	if logger == nil {
		logger = slog.Default()
	}

	return &Bridge{
		hub:    hub,
		ps:     ps,
		logger: logger,
	}
}

// Start subscribes the Bridge to the realtime task, console, and attach
// channel patterns on the underlying PubSub.
//
// The call is non-blocking with respect to message delivery: pattern
// subscriptions are registered synchronously and any subsequent messages are
// dispatched to handleMessage by the Subscriber implementation. The provided
// ctx governs the lifetime of the subscriptions; cancelling it is the
// expected way to stop the Bridge during graceful shutdown. Returns the
// first subscription error encountered, in which case partial subscriptions
// from earlier patterns remain in effect until ctx is cancelled.
func (b *Bridge) Start(ctx context.Context) error {
	patterns := []string{
		channels.RealtimeTaskAll,
		channels.RealtimeConsoleAll,
		channels.RealtimeAttachAll,
	}

	for _, pattern := range patterns {
		if err := b.ps.Subscribe(ctx, pattern, b.handleMessage); err != nil {
			return err
		}
	}

	b.logger.Info("websocket bridge started", "patterns", patterns)

	return nil
}

func (b *Bridge) handleMessage(_ context.Context, msg *pubsub.Message) error {
	b.logger.Debug("bridge received pubsub message",
		"channel", msg.Channel,
		"type", msg.Type,
	)

	topic := ChannelToTopic(msg.Channel)
	if topic == "" {
		return nil
	}

	envelope, err := wrapPayload(msg.Type, msg.Payload)
	if err != nil {
		b.logger.Warn("failed to wrap message payload", "error", err)

		return nil
	}

	b.hub.Broadcast(topic, envelope)

	return nil
}

func wrapPayload(msgType string, payload []byte) ([]byte, error) {
	msg := NewOutboundMessage(msgType, json.RawMessage(payload))

	return json.Marshal(msg)
}

// ChannelToTopic converts a PubSub channel name into the corresponding Hub
// topic by stripping the global channels.Prefix.
//
// The Hub uses topic strings without the PubSub prefix so that local
// subscribers do not need to know about the underlying transport. An input
// that does not carry the prefix is returned unchanged.
func ChannelToTopic(channel string) string {
	return strings.TrimPrefix(channel, channels.Prefix)
}
