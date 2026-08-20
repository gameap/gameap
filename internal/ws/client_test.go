package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_Defaults(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	client := NewClient(context.Background(), nil, hub, nil, nil)

	require.NotNil(t, client)
	assert.Same(t, hub, client.hub)
	assert.Nil(t, client.handler)
	assert.NotNil(t, client.logger, "nil logger must default to slog.Default")
	assert.Equal(t, defaultSendBufferSize, cap(client.send))
	require.NotNil(t, client.ctx)
	require.NotNil(t, client.cancel)

	select {
	case <-client.ctx.Done():
		t.Fatal("new client context must not be cancelled")
	default:
	}
}

func TestClient_Send_Enqueues(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)

	client.Send([]byte("hello"))

	require.Len(t, client.send, 1)
	assert.Equal(t, []byte("hello"), <-client.send)
}

func TestClient_Send_DropsOnFullBuffer(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)

	for range defaultSendBufferSize {
		client.Send([]byte("payload"))
	}

	done := make(chan struct{})
	go func() {
		client.Send([]byte("dropped"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Send must not block when buffer is full")
	}

	require.Len(t, client.send, defaultSendBufferSize)
	for range defaultSendBufferSize {
		assert.Equal(t, []byte("payload"), <-client.send)
	}
	assert.Empty(t, client.send, "no extra message should remain after draining")
}

func TestClient_Send_AppliesOutboundFilter(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)
	client.SetOutboundFilter(func(frame []byte) []byte {
		return bytes.ReplaceAll(frame, []byte("secret"), []byte("******"))
	})

	client.Send([]byte("password is secret"))

	require.Len(t, client.send, 1)
	assert.Equal(t, []byte("password is ******"), <-client.send)
}

func TestClient_SendMessage_AppliesOutboundFilter(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)
	client.SetOutboundFilter(func(frame []byte) []byte {
		return bytes.ReplaceAll(frame, []byte("secret"), []byte("******"))
	})

	client.SendMessage(NewOutboundMessage("console.output", map[string]any{"chunk": "secret"}))

	require.Len(t, client.send, 1)
	assert.Contains(t, string(<-client.send), `"chunk":"******"`)
}

func TestClient_Send_WithoutFilter_PassesThrough(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)

	client.Send([]byte("password is secret"))

	require.Len(t, client.send, 1)
	assert.Equal(t, []byte("password is secret"), <-client.send)
}

func TestClient_SetOutboundFilter_NilClearsFilter(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)
	client.SetOutboundFilter(func([]byte) []byte {
		return []byte("filtered")
	})
	client.SetOutboundFilter(nil)

	client.Send([]byte("original"))

	require.Len(t, client.send, 1)
	assert.Equal(t, []byte("original"), <-client.send)
}

func TestClient_SendMessage_MarshalsAndEnqueues(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)

	msg := &OutboundMessage{
		Type:      "task.status",
		Payload:   map[string]any{"state": "running"},
		Timestamp: 1700000000,
	}
	client.SendMessage(msg)

	require.Len(t, client.send, 1)
	data := <-client.send

	var got OutboundMessage
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, msg.Type, got.Type)
	assert.Equal(t, msg.Timestamp, got.Timestamp)
}

func TestClient_SendMessage_DropsOnMarshalError(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)

	msg := &OutboundMessage{
		Type:    "broken",
		Payload: make(chan int),
	}

	assert.NotPanics(t, func() {
		client.SendMessage(msg)
	})
	assert.Empty(t, client.send, "marshal failure must not enqueue anything")
}

func TestClient_Close_CancelsContext(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)

	client.Close()

	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("Done must close after Close()")
	}
}

func TestClient_Close_Idempotent(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)

	assert.NotPanics(t, func() {
		client.Close()
		client.Close()
		client.Close()
	})
}

func TestClient_Done_TracksParentContext(t *testing.T) {
	t.Parallel()
	parentCtx, cancel := context.WithCancel(context.Background())
	client := NewClient(parentCtx, nil, NewHub(nil), nil, nil)

	cancel()

	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("Done must close when the parent context is cancelled")
	}
}

func TestClient_SetMessageHandler(t *testing.T) {
	t.Parallel()
	client := NewClient(context.Background(), nil, NewHub(nil), nil, nil)
	require.Nil(t, client.handler)

	called := false
	client.SetMessageHandler(func(_ context.Context, _ *InboundMessage) {
		called = true
	})

	require.NotNil(t, client.handler)
	client.handler(context.Background(), &InboundMessage{})
	assert.True(t, called)
}

// ----- real WebSocket integration -----

const wsTestTopic = "test-topic"

type wsTestServer struct {
	url      string
	hub      *Hub
	server   *httptest.Server
	clientCh chan *Client
}

// newWSTestServer spins up an httptest.Server that upgrades incoming
// connections via ws.Accept, wraps them in a Client, registers the Client to
// wsTestTopic on the provided Hub, and runs the read/write pumps. The
// returned server's clientCh receives the server-side Client per upgrade.
func newWSTestServer(t *testing.T, hub *Hub, handler MessageHandler) *wsTestServer {
	t.Helper()

	clientCh := make(chan *Client, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Accept(w, r, nil)
		if err != nil {
			t.Logf("accept failed: %v", err)

			return
		}

		client := NewClient(r.Context(), conn, hub, handler, nil)
		hub.Register(client, wsTestTopic)
		clientCh <- client
		client.Run()
	}))

	t.Cleanup(srv.Close)

	return &wsTestServer{
		url:      "ws" + strings.TrimPrefix(srv.URL, "http"),
		hub:      hub,
		server:   srv,
		clientCh: clientCh,
	}
}

func dialWS(t *testing.T, ts *wsTestServer) *websocket.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, resp, err := websocket.Dial(ctx, ts.url, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = conn.CloseNow() })

	return conn
}

func writeJSON(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, conn.Write(ctx, websocket.MessageText, data))
}

func readMessage(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, data, err := conn.Read(ctx)
	require.NoError(t, err)

	return data
}

func TestClient_Run_AnswersApplicationPing(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	ts := newWSTestServer(t, hub, nil)

	conn := dialWS(t, ts)
	<-ts.clientCh

	writeJSON(t, conn, InboundMessage{Type: TypePing})

	data := readMessage(t, conn)
	var pong OutboundMessage
	require.NoError(t, json.Unmarshal(data, &pong))
	assert.Equal(t, TypePong, pong.Type)
	assert.NotZero(t, pong.Timestamp)
}

func TestClient_Run_DispatchesToHandler(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)

	got := make(chan *InboundMessage, 1)
	handler := func(_ context.Context, msg *InboundMessage) {
		got <- msg
	}

	ts := newWSTestServer(t, hub, handler)

	conn := dialWS(t, ts)
	<-ts.clientCh

	writeJSON(t, conn, InboundMessage{
		Type:    "task.subscribe",
		ID:      "req-7",
		Payload: json.RawMessage(`{"task_id":42}`),
	})

	select {
	case msg := <-got:
		assert.Equal(t, "task.subscribe", msg.Type)
		assert.Equal(t, "req-7", msg.ID)
		assert.JSONEq(t, `{"task_id":42}`, string(msg.Payload))
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not invoked in time")
	}
}

func TestClient_Run_IgnoresInvalidJSON(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)

	var handlerCalls int32
	var mu sync.Mutex
	handler := func(_ context.Context, _ *InboundMessage) {
		mu.Lock()
		handlerCalls++
		mu.Unlock()
	}

	ts := newWSTestServer(t, hub, handler)

	conn := dialWS(t, ts)
	<-ts.clientCh

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte("not json")))

	writeJSON(t, conn, InboundMessage{Type: TypePing})

	data := readMessage(t, conn)
	var pong OutboundMessage
	require.NoError(t, json.Unmarshal(data, &pong))
	assert.Equal(t, TypePong, pong.Type, "connection must stay alive after invalid JSON")

	mu.Lock()
	assert.Zero(t, handlerCalls, "handler must not be invoked for malformed payloads or pings")
	mu.Unlock()
}

func TestClient_Run_WritePumpDeliversHubBroadcast(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	ts := newWSTestServer(t, hub, nil)

	conn := dialWS(t, ts)
	<-ts.clientCh

	hub.Broadcast(wsTestTopic, []byte(`{"type":"task.status","ts":1}`))

	data := readMessage(t, conn)
	var msg OutboundMessage
	require.NoError(t, json.Unmarshal(data, &msg))
	assert.Equal(t, "task.status", msg.Type)
	assert.Equal(t, int64(1), msg.Timestamp)
}

func TestClient_Run_DisconnectUnregistersFromHub(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	ts := newWSTestServer(t, hub, nil)

	conn := dialWS(t, ts)
	<-ts.clientCh

	require.Equal(t, 1, hub.TopicSubscriberCount(wsTestTopic))

	require.NoError(t, conn.Close(websocket.StatusNormalClosure, "bye"))

	require.Eventually(t, func() bool {
		return hub.TopicSubscriberCount(wsTestTopic) == 0
	}, 2*time.Second, 10*time.Millisecond, "client must be unregistered after disconnect")
}

func TestClient_Run_ServerCloseTearsDownConnection(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	ts := newWSTestServer(t, hub, nil)

	conn := dialWS(t, ts)
	serverClient := <-ts.clientCh

	serverClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := conn.Read(ctx)
	require.Error(t, err, "client read must fail after server-side Close")

	require.Eventually(t, func() bool {
		return hub.TopicSubscriberCount(wsTestTopic) == 0
	}, 2*time.Second, 10*time.Millisecond)
}
