package ws

import (
	"log/slog"
	"sync"
)

// Hub is the in-process registry of connected WebSocket clients and the
// topic-based fan-out point for outbound messages on a single API instance.
//
// Each connected Client is registered against zero or more topics (for example,
// per-task or per-server console channels). When an event is broadcast to a
// topic, the Hub delivers it to every subscribed client without blocking on
// slow consumers (the Client owns its own bounded send buffer).
//
// The Hub itself is transport-agnostic: it does not talk to PubSub or gRPC
// directly. In a multi-instance deployment, the Bridge subscribes to the
// shared PubSub realtime channels and invokes Broadcast on the local Hub, so
// each API instance only fans out to the WebSocket clients connected to it.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
	topics  map[string]map[*Client]struct{}
	logger  *slog.Logger
}

// NewHub constructs a Hub with empty client and topic registries.
//
// If logger is nil, slog.Default is used. The returned Hub is ready for
// concurrent use; no background goroutines are started.
func NewHub(logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}

	return &Hub{
		clients: make(map[*Client]struct{}),
		topics:  make(map[string]map[*Client]struct{}),
		logger:  logger,
	}
}

// Register adds a client to the Hub and subscribes it to the given topics
// in a single locked operation.
//
// Use this when a WebSocket connection is first accepted: it inserts the
// client into the global client set and creates topic buckets as needed.
// Passing no topics registers the client without any subscriptions; topics
// can be added later with Subscribe. Safe for concurrent use.
func (h *Hub) Register(client *Client, topics ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[client] = struct{}{}
	for _, topic := range topics {
		if h.topics[topic] == nil {
			h.topics[topic] = make(map[*Client]struct{})
		}
		h.topics[topic][client] = struct{}{}
	}

	h.logger.Debug("client registered",
		"topics", topics,
		"total_clients", len(h.clients),
	)
}

// Subscribe adds the client to the subscriber set of each given topic.
//
// The client must already be Registered. New topic buckets are created on
// demand. Re-subscribing to a topic the client already belongs to is a
// no-op. Safe for concurrent use.
func (h *Hub) Subscribe(client *Client, topics ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, topic := range topics {
		if h.topics[topic] == nil {
			h.topics[topic] = make(map[*Client]struct{})
		}
		h.topics[topic][client] = struct{}{}
	}
}

// Unregister removes the client from the Hub and from every topic it was
// subscribed to.
//
// Topic buckets that become empty are deleted to avoid unbounded growth.
// Unregister does not close the client's send channel; the caller is
// responsible for the client lifecycle. Safe for concurrent use.
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.clients, client)
	for topic, clients := range h.topics {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.topics, topic)
		}
	}
}

// Broadcast delivers msg to every client currently subscribed to topic.
//
// Subscribers are snapshotted under a read lock and then fanned out without
// holding the Hub lock, so a slow Client cannot block other deliveries.
// Each Client has its own bounded send buffer and is responsible for
// dropping or disconnecting on overflow; Broadcast itself never blocks on
// send. Topics with no subscribers are a fast no-op.
//
// In multi-instance deployments this is typically invoked by the Bridge in
// response to a PubSub message, so each API instance only writes to the
// WebSocket clients connected locally. Safe for concurrent use.
func (h *Hub) Broadcast(topic string, msg []byte) {
	h.mu.RLock()
	clients := h.topics[topic]

	h.logger.Debug("hub broadcasting",
		"topic", topic,
		"subscriber_count", len(clients),
	)

	if len(clients) == 0 {
		h.mu.RUnlock()

		return
	}

	targets := make([]*Client, 0, len(clients))
	for c := range clients {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	for _, c := range targets {
		c.Send(msg)
	}
}

// TopicSubscriberCount returns the number of clients currently subscribed
// to topic on this Hub instance.
//
// The value is local to the API instance: in a multi-instance deployment
// it does not reflect subscribers connected to other instances. Useful for
// metrics and for deciding whether to skip work when no one is listening.
// Safe for concurrent use.
func (h *Hub) TopicSubscriberCount(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.topics[topic])
}

// Close closes every client currently registered with the Hub.
//
// The client set is snapshotted under the lock and Close is then invoked
// on each client without holding the Hub lock, so a stalled client cannot
// block shutdown of the others. Close does not clear the Hub's internal
// maps; the unregister path of each client is expected to remove it.
// Intended to be called during graceful API shutdown.
func (h *Hub) Close() {
	h.mu.Lock()
	clients := make([]*Client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	for _, c := range clients {
		c.Close()
	}
}
