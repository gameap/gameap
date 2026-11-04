package rcon

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const webRconTestPassword = "secret"

// newRustWebRconServer imitates the WebSocket RCON of a Rust server: the password is the URL
// path, and every accepted connection is handed to handler.
func newRustWebRconServer(t *testing.T, handler func(t *testing.T, conn *websocket.Conn)) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+webRconTestPassword {
			http.Error(rw, "unexpected path "+r.URL.Path, http.StatusNotFound)

			return
		}

		conn, err := websocket.Accept(rw, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)

			return
		}
		defer func() { _ = conn.CloseNow() }()

		handler(t, conn)
	}))
	t.Cleanup(srv.Close)

	return strings.TrimPrefix(srv.URL, "http://")
}

func readWebRconRequest(t *testing.T, conn *websocket.Conn) webRconRequest {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), mockServerReadTimeout)
	defer cancel()

	_, data, err := conn.Read(ctx)
	require.NoError(t, err)

	var req webRconRequest
	require.NoError(t, json.Unmarshal(data, &req))

	return req
}

func writeWebRconMessage(t *testing.T, conn *websocket.Conn, id int, message string) {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"Message":    message,
		"Identifier": id,
		"Type":       "Generic",
		"Stacktrace": "",
	})
	require.NoError(t, err)
	require.NoError(t, conn.Write(context.Background(), websocket.MessageText, payload))
}

func newOpenWebRcon(t *testing.T, address, password string, timeout time.Duration) *WebRcon {
	t.Helper()

	client, err := NewWebRcon(Config{Address: address, Password: password, Timeout: timeout})
	require.NoError(t, err)
	require.NoError(t, client.Open(context.Background()))
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func TestWebRcon_Execute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		handler   func(t *testing.T, conn *websocket.Conn)
		timeout   time.Duration
		want      string
		wantError string
	}{
		{
			name: "response_with_matching_identifier",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				req := readWebRconRequest(t, conn)
				assert.Equal(t, "status", req.Message)
				assert.Equal(t, "WebRcon", req.Name)
				assert.Equal(t, 1, req.Identifier)

				writeWebRconMessage(t, conn, req.Identifier, "hostname: GameAP Rust server\n")
			},
			want: "hostname: GameAP Rust server\n",
		},
		{
			name: "console_broadcasts_before_the_response_are_skipped",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				req := readWebRconRequest(t, conn)

				writeWebRconMessage(t, conn, 0, "Saving complete")
				writeWebRconMessage(t, conn, -1, "NiK has spawned")
				writeWebRconMessage(t, conn, req.Identifier+100, "answer to another client")
				writeWebRconMessage(t, conn, req.Identifier, "[]")
			},
			want: "[]",
		},
		{
			name: "malformed_message_is_invalid_packet",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				readWebRconRequest(t, conn)

				require.NoError(t, conn.Write(context.Background(), websocket.MessageText, []byte("not json")))
			},
			wantError: "invalid packet",
		},
		{
			name: "connection_closed_before_the_response",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				readWebRconRequest(t, conn)

				_ = conn.Close(websocket.StatusGoingAway, "server shutdown")
			},
			wantError: "unable to read response",
		},
		{
			name: "no_response_within_timeout",
			handler: func(t *testing.T, conn *websocket.Conn) {
				t.Helper()
				readWebRconRequest(t, conn)

				ctx, cancel := context.WithTimeout(context.Background(), mockServerReadTimeout)
				defer cancel()
				_, _, _ = conn.Read(ctx)
			},
			timeout:   200 * time.Millisecond,
			wantError: "unable to read response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			timeout := tt.timeout
			if timeout == 0 {
				timeout = 5 * time.Second
			}

			address := newRustWebRconServer(t, tt.handler)
			client := newOpenWebRcon(t, address, "secret", timeout)

			got, err := client.Execute(context.Background(), "status")

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWebRcon_Execute_IdentifierIncrementsAcrossCalls(t *testing.T) {
	t.Parallel()

	address := newRustWebRconServer(t, func(t *testing.T, conn *websocket.Conn) {
		t.Helper()
		for range 3 {
			req := readWebRconRequest(t, conn)
			writeWebRconMessage(t, conn, req.Identifier, req.Message+"#"+strconv.Itoa(req.Identifier))
		}
	})
	client := newOpenWebRcon(t, address, "secret", 5*time.Second)

	for i, command := range []string{"a", "b", "c"} {
		got, err := client.Execute(context.Background(), command)
		require.NoError(t, err)
		assert.Equal(t, command+"#"+strconv.Itoa(i+1), got)
	}
}

func TestWebRcon_Open(t *testing.T) {
	t.Parallel()

	t.Run("rust_close_frame_instead_of_handshake_is_authentication_failure", func(t *testing.T) {
		t.Parallel()

		// A real Rust server answers a wrong password with a bare close frame (status 1000)
		// in place of the HTTP 101 response.
		srv := newScriptedTCPServer(t, func(conn net.Conn) {
			_ = conn.SetDeadline(time.Now().Add(mockServerReadTimeout))
			reader := bufio.NewReader(conn)
			for {
				line, err := reader.ReadString('\n')
				if err != nil || line == "\r\n" {
					break
				}
			}

			_, _ = conn.Write([]byte{0x88, 0x02, 0x03, 0xe8})
		})

		client, err := NewWebRcon(Config{Address: srv.addr, Password: "wrong", Timeout: 2 * time.Second})
		require.NoError(t, err)

		err = client.Open(context.Background())

		require.ErrorIs(t, err, ErrAuthenticationFailed)
	})

	t.Run("http_error_response_is_connection_error", func(t *testing.T) {
		t.Parallel()

		address := newRustWebRconServer(t, func(*testing.T, *websocket.Conn) {})
		client, err := NewWebRcon(Config{Address: address, Password: "other", Timeout: 2 * time.Second})
		require.NoError(t, err)

		err = client.Open(context.Background())

		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrAuthenticationFailed)
		assert.Contains(t, err.Error(), "unable to connect")
	})

	t.Run("nothing_listening_is_connection_error", func(t *testing.T) {
		t.Parallel()

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		address := listener.Addr().String()
		require.NoError(t, listener.Close())

		client, err := NewWebRcon(Config{Address: address, Password: "secret", Timeout: 2 * time.Second})
		require.NoError(t, err)

		err = client.Open(context.Background())

		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrAuthenticationFailed)
		assert.Contains(t, err.Error(), "unable to connect")
		assert.NotContains(t, err.Error(), "secret")
	})
}

func TestWebRcon_Execute_WithoutOpen(t *testing.T) {
	t.Parallel()

	client, err := NewWebRcon(Config{Address: "127.0.0.1:1", Password: "secret", Timeout: time.Second})
	require.NoError(t, err)

	_, err = client.Execute(context.Background(), "status")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection is not open")
}

func TestWebRcon_Close_IsIdempotent(t *testing.T) {
	t.Parallel()

	address := newRustWebRconServer(t, func(t *testing.T, conn *websocket.Conn) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), mockServerReadTimeout)
		defer cancel()
		_, _, _ = conn.Read(ctx)
	})
	client := newOpenWebRcon(t, address, "secret", 5*time.Second)

	require.NoError(t, client.Close())
	require.NoError(t, client.Close())
}

func TestNewClient_WebRcon(t *testing.T) {
	t.Parallel()

	client, err := NewClient(Config{Address: "127.0.0.1:28016", Password: "secret", Protocol: ProtocolWebRcon})

	require.NoError(t, err)
	assert.IsType(t, &WebRcon{}, client)
	assert.True(t, IsProtocolSupported(ProtocolWebRcon))
}
