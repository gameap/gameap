package console

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gameap/gameap/internal/ws"
	"github.com/stretchr/testify/require"
)

// silentLogger returns a logger that discards everything. Tests use it to
// keep stdout clean while exercising production code that logs through slog.
func silentLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// dialedConsoleClient is a real ws.Client whose underlying socket is the
// server side of a WebSocket connection that the test holds the client side
// of. Modeled on metricsbase/pump_test.go's dialedClient.
type dialedConsoleClient struct {
	srvClient *ws.Client
	cliConn   *websocket.Conn
	httpSrv   *httptest.Server
	hub       *ws.Hub
}

// dialConsoleClient stands up an httptest server that accepts a WebSocket,
// wraps the server side in a ws.Client, dials it from the client side, and
// runs the client read/write pumps in the background. Cleanup tears
// everything down via t.Cleanup.
func dialConsoleClient(t *testing.T) *dialedConsoleClient {
	t.Helper()

	hub := ws.NewHub(silentLogger())

	type accepted struct {
		client *ws.Client
		err    error
	}
	acceptedCh := make(chan accepted, 1)

	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Accept(w, r, nil)
		if err != nil {
			acceptedCh <- accepted{nil, err}

			return
		}
		client := ws.NewClient(r.Context(), conn, hub, nil, silentLogger())
		hub.Register(client)
		acceptedCh <- accepted{client, nil}
		client.Run()
	}))

	dialCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http")
	cliConn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	got := <-acceptedCh
	require.NoError(t, got.err, "server-side accept must succeed")
	require.NotNil(t, got.client)

	d := &dialedConsoleClient{
		srvClient: got.client,
		cliConn:   cliConn,
		httpSrv:   httpSrv,
		hub:       hub,
	}
	t.Cleanup(func() {
		_ = cliConn.Close(websocket.StatusNormalClosure, "")
		got.client.Close()
		httpSrv.Close()
	})

	return d
}

// consoleFrame is the JSON envelope read off the wire.
type consoleFrame struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	Error   string          `json:"error,omitempty"`
	Ts      int64           `json:"ts"`
}

func readConsoleFrame(t *testing.T, c *websocket.Conn, timeout time.Duration) (consoleFrame, bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, data, err := c.Read(ctx)
	if err != nil {
		return consoleFrame{}, false
	}

	var f consoleFrame
	require.NoError(t, json.Unmarshal(data, &f))

	return f, true
}

// expectNoConsoleFrame asserts no frame arrives within timeout.
func expectNoConsoleFrame(t *testing.T, c *websocket.Conn, timeout time.Duration) {
	t.Helper()

	if extra, ok := readConsoleFrame(t, c, timeout); ok {
		t.Fatalf("expected no frame, got type=%q payload=%q", extra.Type, string(extra.Payload))
	}
}

// callMessageHandler invokes the raw inbound dispatch path so the handler
// closure runs as if a frame arrived. Tests use it to drive the handler
// without bouncing through the network read pump.
func callMessageHandler(t *testing.T, handler ws.MessageHandler, msgType string, payload any) {
	t.Helper()

	raw, err := json.Marshal(payload)
	require.NoError(t, err, "test payload must be JSON-marshallable")
	handler(context.Background(), &ws.InboundMessage{
		Type:    msgType,
		Payload: raw,
	})
}
