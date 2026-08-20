package ws

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccept_UpgradesConnection(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Accept(w, r, nil)
		if err != nil {
			t.Logf("accept error: %v", err)

			return
		}
		defer conn.CloseNow() //nolint:errcheck

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		_ = conn.Write(ctx, websocket.MessageText, []byte("upgraded"))
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dialCancel()

	conn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer conn.CloseNow() //nolint:errcheck

	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel()
	msgType, data, err := conn.Read(readCtx)
	require.NoError(t, err)
	assert.Equal(t, websocket.MessageText, msgType)
	assert.Equal(t, []byte("upgraded"), data)
}

func TestAccept_ClearsHTTPServerDeadlines(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	const httpTimeout = 100 * time.Millisecond
	const sleepBeforeWrite = 250 * time.Millisecond

	serverWriteErr := make(chan error, 1)
	server := &http.Server{
		ReadTimeout:       httpTimeout,
		WriteTimeout:      httpTimeout,
		ReadHeaderTimeout: httpTimeout,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, acceptErr := Accept(w, r, nil)
			if acceptErr != nil {
				serverWriteErr <- acceptErr

				return
			}
			defer conn.CloseNow() //nolint:errcheck

			// Sleep longer than the HTTP server's WriteTimeout. Without the
			// deadline-clearing inside Accept, the write below would fail
			// with i/o timeout. With it, the write succeeds.
			time.Sleep(sleepBeforeWrite)

			writeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			serverWriteErr <- conn.Write(writeCtx, websocket.MessageText, []byte("after-deadline"))
		}),
	}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()

	wsURL := "ws://" + listener.Addr().String()

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dialCancel()

	conn, resp, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer conn.CloseNow() //nolint:errcheck

	readCtx, readCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer readCancel()
	_, data, err := conn.Read(readCtx)
	require.NoError(t, err, "client read must succeed despite elapsed HTTP WriteTimeout")
	assert.Equal(t, []byte("after-deadline"), data)

	select {
	case writeErr := <-serverWriteErr:
		require.NoError(t, writeErr, "server-side WS write must not fail with i/o timeout")
	case <-time.After(3 * time.Second):
		t.Fatal("server handler did not report a result")
	}
}
