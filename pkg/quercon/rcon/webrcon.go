package rcon

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/pkg/errors"
)

const (
	// webRconName is the client name Rust shows in its RCON log.
	webRconName = "WebRcon"

	// webRconReadLimit bounds a single response. Rust answers some commands (e.g. a full
	// entity or convar dump) with several megabytes of text.
	webRconReadLimit = 16 << 20

	// webSocketCloseOpcode is the first byte of an unmasked final close frame.
	webSocketCloseOpcode = 0x88
)

// WebRcon speaks the WebSocket RCON of Rust (rcon.web 1, the default). The password goes in
// the URL path, every command is a JSON message, and the server answers with a message carrying
// the same Identifier. Messages with any other Identifier are console broadcasts and are skipped.
type WebRcon struct {
	address   string
	password  string
	timeout   time.Duration
	conn      *websocket.Conn
	requestID int
}

type webRconRequest struct {
	Identifier int    `json:"Identifier"`
	Message    string `json:"Message"`
	Name       string `json:"Name"`
}

type webRconResponse struct {
	Identifier int    `json:"Identifier"`
	Message    string `json:"Message"`
	Type       string `json:"Type"`
}

func NewWebRcon(config Config) (*WebRcon, error) {
	return &WebRcon{
		address:   config.Address,
		password:  config.Password,
		timeout:   config.Timeout,
		requestID: 1,
	}, nil
}

func (w *WebRcon) Open(ctx context.Context) error {
	ctx, cancel := w.withTimeout(ctx)
	defer cancel()

	sniffer := &firstByteSniffer{}

	conn, resp, err := websocket.Dial(ctx, w.url(), &websocket.DialOptions{
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				DialContext: sniffer.dialContext(w.timeout),
			},
		},
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		// Rust rejects a wrong password before the handshake: instead of an HTTP response it
		// writes a bare close frame and drops the connection.
		if sniffer.firstByte() == webSocketCloseOpcode {
			return ErrAuthenticationFailed
		}

		// The URL path is the password, and *url.Error prints the whole URL.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return errors.Wrap(urlErr.Err, "unable to connect")
		}

		return errors.WithMessage(err, "unable to connect")
	}

	conn.SetReadLimit(webRconReadLimit)
	w.conn = conn

	return nil
}

func (w *WebRcon) Close() error {
	if w.conn == nil {
		return nil
	}

	conn := w.conn
	w.conn = nil

	err := conn.Close(websocket.StatusNormalClosure, "")
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return errors.Wrap(err, "unable to close connection")
	}

	return nil
}

func (w *WebRcon) Execute(ctx context.Context, command string) (string, error) {
	if w.conn == nil {
		return "", errors.New("connection is not open")
	}

	ctx, cancel := w.withTimeout(ctx)
	defer cancel()

	id := w.requestID
	w.requestID++

	payload, err := json.Marshal(webRconRequest{
		Identifier: id,
		Message:    command,
		Name:       webRconName,
	})
	if err != nil {
		return "", errors.Wrap(err, "unable to encode command")
	}

	if err = w.conn.Write(ctx, websocket.MessageText, payload); err != nil {
		return "", errors.Wrap(err, "unable to send command")
	}

	for {
		_, data, err := w.conn.Read(ctx)
		if err != nil {
			return "", errors.Wrap(err, "unable to read response")
		}

		var response webRconResponse
		if err = json.Unmarshal(data, &response); err != nil {
			return "", errors.WithMessage(ErrInvalidPacket, err.Error())
		}

		if response.Identifier == id {
			return response.Message, nil
		}
	}
}

func (w *WebRcon) url() string {
	u := url.URL{
		Scheme: "ws",
		Host:   w.address,
		Path:   "/" + w.password,
	}

	return u.String()
}

func (w *WebRcon) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if w.timeout <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, w.timeout)
}

// firstByteSniffer remembers the first byte the server sent on the connection it dialed, so a
// failed handshake can be told apart from an authentication rejection.
type firstByteSniffer struct {
	value atomic.Int32
}

func (s *firstByteSniffer) dialContext(timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		return &sniffingConn{Conn: conn, sniffer: s}, nil
	}
}

func (s *firstByteSniffer) firstByte() int32 {
	return s.value.Load() - 1
}

type sniffingConn struct {
	net.Conn

	sniffer *firstByteSniffer
}

func (c *sniffingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		// Stored shifted by one so the zero value means "nothing read yet".
		c.sniffer.value.CompareAndSwap(0, int32(b[0])+1)
	}

	return n, err
}
