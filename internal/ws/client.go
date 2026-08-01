package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/coder/websocket"
)

// Default tuning parameters for a Client. The send buffer is bounded so a
// stalled WebSocket peer cannot grow memory without limit; the read limit
// caps an inbound frame size; the ping interval and pong timeout drive
// liveness detection over the WebSocket connection.
const (
	defaultSendBufferSize = 64
	defaultMaxMessageSize = 8192
	defaultPingInterval   = 30 * time.Second
	defaultPongTimeout    = 60 * time.Second
)

// MessageHandler is invoked by a Client for every well-formed inbound
// message that is not a transport-level ping.
//
// The handler runs on the Client's read pump goroutine, so it must not block
// for long; offload heavy work to a separate goroutine if needed. The ctx
// is the Client's context and is cancelled when the connection is closed.
type MessageHandler func(ctx context.Context, msg *InboundMessage)

// OutboundFilter transforms an already-encoded outbound frame before it is
// queued for delivery.
//
// It is the only place a Client can inspect traffic that reaches it through
// Hub.Broadcast, which hands over frames that were encoded elsewhere (the
// Bridge, from a PubSub payload). Returning the frame unchanged is the no-op
// behaviour; returning a different slice replaces what the peer receives.
// The filter runs on the caller's goroutine — the Hub fan-out or whoever
// calls Send — so it must be cheap and safe for concurrent use.
type OutboundFilter func(frame []byte) []byte

// Client represents a single connected WebSocket peer on this API instance.
//
// Each Client owns its own bounded send channel and a context that is
// cancelled when the connection is torn down. Inbound and outbound traffic
// are driven by two goroutines started by Run: the read pump decodes
// frames, answers application-level pings, and dispatches everything else
// to the MessageHandler; the write pump drains the send buffer onto the
// socket and emits periodic transport pings for liveness.
//
// The Client is tied to exactly one Hub on the local instance: the Hub
// fans out a Broadcast to every subscribed Client by calling Send, which
// is non-blocking and drops messages when the buffer is full. In a
// multi-instance deployment a Client is reachable only from the API
// instance it is connected to; cross-instance delivery is handled by the
// Bridge translating shared PubSub events into local Hub broadcasts.
type Client struct {
	conn    *websocket.Conn
	hub     *Hub
	send    chan []byte
	ctx     context.Context
	cancel  context.CancelFunc
	handler MessageHandler
	filter  OutboundFilter
	logger  *slog.Logger
}

// ClientConfig collects the per-connection liveness tuning knobs.
//
// PingInterval is the period between transport-level pings sent by the
// write pump. PongTimeout is the maximum time the peer may stay silent
// before the connection is considered dead.
type ClientConfig struct {
	PingInterval time.Duration
	PongTimeout  time.Duration
}

// NewClient constructs a Client wrapped around an already-accepted
// WebSocket connection.
//
// The provided ctx is wrapped with a cancel function exposed via Close and
// Done; cancelling either the parent ctx or calling Close terminates both
// pumps. If logger is nil, slog.Default is used. handler may be nil, in
// which case inbound non-ping messages are silently ignored until
// SetMessageHandler is called. NewClient does not start any goroutines;
// call Run to begin processing.
func NewClient(
	ctx context.Context,
	conn *websocket.Conn,
	hub *Hub,
	handler MessageHandler,
	logger *slog.Logger,
) *Client {
	ctx, cancel := context.WithCancel(ctx)

	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		conn:    conn,
		hub:     hub,
		send:    make(chan []byte, defaultSendBufferSize),
		ctx:     ctx,
		cancel:  cancel,
		handler: handler,
		logger:  logger,
	}
}

// SetMessageHandler installs handler as the callback for inbound messages.
//
// It may be called before Run to defer handler wiring, or at runtime to
// swap the handler. There is no synchronization with the read pump, so the
// caller must avoid swapping the handler while messages are being
// processed if that would race with handler-side state.
func (c *Client) SetMessageHandler(handler MessageHandler) {
	c.handler = handler
}

// SetOutboundFilter installs filter as the transform applied to every frame
// on its way to the peer, including frames fanned out by the Hub.
//
// There is no synchronization with the write path, so it must be installed
// before the Client is registered with the Hub and before Run — once either
// has happened another goroutine may already be calling Send. Passing nil
// clears the filter.
func (c *Client) SetOutboundFilter(filter OutboundFilter) {
	c.filter = filter
}

// Run starts the write pump in a new goroutine and blocks on the read pump
// in the calling goroutine.
//
// Run returns when the connection is closed (either side), the context is
// cancelled, or a fatal read error occurs. On exit the Client is
// automatically Unregistered from the Hub and the underlying connection is
// closed by the read pump. Intended to be invoked once per Client, usually
// from the HTTP handler that accepted the WebSocket upgrade.
func (c *Client) Run() {
	go c.writePump()
	c.readPump()
}

// Send enqueues msg onto the Client's bounded send buffer.
//
// The installed OutboundFilter, if any, is applied first. The call is
// non-blocking: if the buffer is full the message is dropped and a warning
// is logged, so a slow WebSocket peer cannot back-pressure the Hub. This is
// the entry point used by Hub.Broadcast for fan-out. Safe for concurrent
// use.
func (c *Client) Send(msg []byte) {
	if c.filter != nil {
		msg = c.filter(msg)
	}

	select {
	case c.send <- msg:
	default:
		c.logger.Warn("client send buffer full, dropping message", "msg_size", len(msg))
	}
}

// SendMessage marshals msg as JSON and enqueues the result via Send.
//
// Marshalling errors are logged and the message is silently dropped; the
// connection is not torn down, since a single bad payload should not kill
// the session. Safe for concurrent use.
func (c *Client) SendMessage(msg *OutboundMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Warn("failed to marshal outbound message", "error", err)

		return
	}

	c.Send(data)
}

// Close cancels the Client's context, which causes both pumps to terminate
// and the underlying WebSocket connection to be closed.
//
// Close is idempotent and safe for concurrent use. It does not block on
// the pumps draining; callers that need to wait should observe Done.
func (c *Client) Close() {
	c.cancel()
}

// Done returns a channel that is closed when the Client's context has been
// cancelled — i.e. when the connection is being or has been torn down.
//
// Useful for callers that want to observe disconnection without holding a
// reference to the cancel function.
func (c *Client) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.Unregister(c)
		c.cancel()
		c.conn.CloseNow() //nolint:errcheck
	}()

	c.conn.SetReadLimit(defaultMaxMessageSize)

	for {
		_, data, err := c.conn.Read(c.ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway {
				c.logger.Debug("client disconnected normally")
			} else {
				select {
				case <-c.ctx.Done():
				default:
					c.logger.Debug("read error", "error", err)
				}
			}

			return
		}

		var msg InboundMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logger.Debug("invalid message format", "error", err)

			continue
		}

		if msg.Type == TypePing {
			c.SendMessage(&OutboundMessage{
				Type:      TypePong,
				Timestamp: time.Now().Unix(),
			})

			continue
		}

		if c.handler != nil {
			c.handler(c.ctx, &msg)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(defaultPingInterval)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close(websocket.StatusGoingAway, "server shutting down")
	}()

	for {
		select {
		case <-c.ctx.Done():
			return

		case msg := <-c.send:
			ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
			err := c.conn.Write(ctx, websocket.MessageText, msg)
			cancel()

			if err != nil {
				c.logger.Debug("write error", "error", err)

				return
			}

		case <-ticker.C:
			ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
			err := c.conn.Ping(ctx)
			cancel()

			if err != nil {
				c.logger.Debug("ping failed", "error", err)

				return
			}
		}
	}
}
