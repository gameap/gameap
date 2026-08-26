package rcon

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSource_buildPacket(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		id         int32
		packetType int32
		body       string
		wantSize   int32
		wantBody   string
	}{
		{
			name:       "auth_packet_with_password",
			id:         1,
			packetType: serverDataAuth,
			body:       "secret",
			wantSize:   int32(4 + 4 + len("secret") + 2),
			wantBody:   "secret",
		},
		{
			name:       "exec_command_packet",
			id:         42,
			packetType: serverDataExecCommand,
			body:       "status",
			wantSize:   int32(4 + 4 + len("status") + 2),
			wantBody:   "status",
		},
		{
			name:       "empty_body_yields_minimum_packet",
			id:         7,
			packetType: serverDataResponseValue,
			body:       "",
			wantSize:   minPacketSize,
			wantBody:   "",
		},
		{
			name:       "request_id_zero",
			id:         0,
			packetType: serverDataExecCommand,
			body:       "ping",
			wantSize:   int32(4 + 4 + len("ping") + 2),
			wantBody:   "ping",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Source{}

			packet := s.buildPacket(tt.id, tt.packetType, tt.body)

			require.GreaterOrEqual(t, len(packet), 4+int(minPacketSize),
				"packet must hold the size prefix plus the minimum payload")

			var size int32
			require.NoError(t, binary.Read(bytes.NewReader(packet[:4]), binary.LittleEndian, &size),
				"size prefix must decode as little-endian int32")
			assert.Equal(t, tt.wantSize, size, "size prefix must equal id+type+body+two nulls")

			assert.Equal(t, int(size), len(packet)-4, "remaining bytes must equal the declared size")

			payload := bytes.NewReader(packet[4:])

			var gotID int32
			require.NoError(t, binary.Read(payload, binary.LittleEndian, &gotID))
			assert.Equal(t, tt.id, gotID, "request id must round-trip via little-endian")

			var gotType int32
			require.NoError(t, binary.Read(payload, binary.LittleEndian, &gotType))
			assert.Equal(t, tt.packetType, gotType, "packet type must round-trip via little-endian")

			bodyBytes := make([]byte, len(tt.wantBody))
			_, err := io.ReadFull(payload, bodyBytes)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBody, string(bodyBytes), "body bytes must follow type field")

			tail := make([]byte, 2)
			_, err = io.ReadFull(payload, tail)
			require.NoError(t, err)
			assert.Equal(t, []byte{0x00, 0x00}, tail, "packet must end with two null terminators")
		})
	}
}

func TestSource_Open_Authenticate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		handler    func(t *testing.T, conn net.Conn)
		wantError  string
		afterCheck func(t *testing.T, s *Source)
	}{
		{
			name: "happy_path_authenticates_with_matching_response_id",
			handler: func(t *testing.T, conn net.Conn) {
				t.Helper()
				id, packetType, body, err := readSourcePacket(conn)
				require.NoError(t, err)
				assert.Equal(t, serverDataAuth, packetType, "first packet must be auth")
				assert.Equal(t, "secret", body, "body must contain the password")

				_, _ = conn.Write(buildSourcePacket(t, id, serverDataAuthResponse, ""))
			},
			afterCheck: func(t *testing.T, s *Source) {
				t.Helper()
				assert.Equal(t, int32(2), s.requestID,
					"requestID should advance from 1 to 2 after a successful auth")
			},
		},
		{
			name: "auth_failure_when_server_returns_minus_one_id",
			handler: func(t *testing.T, conn net.Conn) {
				t.Helper()
				_, _, _, err := readSourcePacket(conn)
				require.NoError(t, err)
				_, _ = conn.Write(buildSourcePacket(t, -1, serverDataAuthResponse, ""))
			},
			wantError: ErrAuthenticationFailed.Error(),
		},
		{
			name: "auth_failure_when_server_returns_wrong_packet_type",
			handler: func(t *testing.T, conn net.Conn) {
				t.Helper()
				id, _, _, err := readSourcePacket(conn)
				require.NoError(t, err)
				_, _ = conn.Write(buildSourcePacket(t, id, serverDataResponseValue, ""))
			},
			wantError: ErrAuthenticationFailed.Error(),
		},
		{
			name: "connection_closed_immediately_after_auth_send",
			handler: func(t *testing.T, conn net.Conn) {
				t.Helper()
				_ = conn.Close()
			},
			wantError: "unable to read packet size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := newScriptedTCPServer(t, func(c net.Conn) {
				tt.handler(t, c)
			})

			s, err := NewSource(Config{
				Address:  srv.addr,
				Password: "secret",
				Protocol: ProtocolSource,
				Timeout:  2 * time.Second,
			})
			require.NoError(t, err)

			err = s.Open(context.Background())
			defer func() { _ = s.Close() }()

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message must mention the failure cause")

				return
			}

			require.NoError(t, err)
			if tt.afterCheck != nil {
				tt.afterCheck(t, s)
			}
		})
	}
}

func TestSource_Execute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		handler    func(t *testing.T, conn net.Conn)
		command    string
		want       string
		wantError  string
		afterCheck func(t *testing.T, s *Source)
	}{
		{
			name: "returns_response_body_on_success",
			handler: func(t *testing.T, conn net.Conn) {
				t.Helper()
				authID, _, _, err := readSourcePacket(conn)
				require.NoError(t, err)
				_, _ = conn.Write(buildSourcePacket(t, authID, serverDataAuthResponse, ""))

				cmdID, packetType, body, err := readSourcePacket(conn)
				require.NoError(t, err)
				assert.Equal(t, serverDataExecCommand, packetType, "second packet must carry exec type")
				assert.Equal(t, "status", body)

				_, _ = conn.Write(buildSourcePacket(t, cmdID, serverDataResponseValue, "ok\n"))
			},
			command: "status",
			want:    "ok\n",
			afterCheck: func(t *testing.T, s *Source) {
				t.Helper()
				assert.Equal(t, int32(3), s.requestID,
					"requestID must advance after both auth and exec succeed")
			},
		},
		{
			name: "returns_error_on_response_id_mismatch",
			handler: func(t *testing.T, conn net.Conn) {
				t.Helper()
				authID, _, _, err := readSourcePacket(conn)
				require.NoError(t, err)
				_, _ = conn.Write(buildSourcePacket(t, authID, serverDataAuthResponse, ""))

				_, _, _, err = readSourcePacket(conn)
				require.NoError(t, err)
				_, _ = conn.Write(buildSourcePacket(t, 9999, serverDataResponseValue, "garbage"))
			},
			command:   "status",
			wantError: "response ID mismatch",
		},
		{
			name: "returns_error_on_unexpected_response_type",
			handler: func(t *testing.T, conn net.Conn) {
				t.Helper()
				authID, _, _, err := readSourcePacket(conn)
				require.NoError(t, err)
				_, _ = conn.Write(buildSourcePacket(t, authID, serverDataAuthResponse, ""))

				cmdID, _, _, err := readSourcePacket(conn)
				require.NoError(t, err)
				_, _ = conn.Write(buildSourcePacket(t, cmdID, 99, "weird"))
			},
			command:   "status",
			wantError: "unexpected response type: 99",
		},
		{
			name: "propagates_read_error_when_connection_drops",
			handler: func(t *testing.T, conn net.Conn) {
				t.Helper()
				authID, _, _, err := readSourcePacket(conn)
				require.NoError(t, err)
				_, _ = conn.Write(buildSourcePacket(t, authID, serverDataAuthResponse, ""))

				_, _, _, err = readSourcePacket(conn)
				require.NoError(t, err)
				_ = conn.Close()
			},
			command:   "status",
			wantError: "unable to read packet size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := newScriptedTCPServer(t, func(c net.Conn) {
				tt.handler(t, c)
			})

			s, err := NewSource(Config{
				Address:  srv.addr,
				Password: "secret",
				Protocol: ProtocolSource,
				Timeout:  2 * time.Second,
			})
			require.NoError(t, err)

			require.NoError(t, s.Open(context.Background()))
			defer func() { _ = s.Close() }()

			got, err := s.Execute(context.Background(), tt.command)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Empty(t, got, "no payload should be returned when execute fails")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			if tt.afterCheck != nil {
				tt.afterCheck(t, s)
			}
		})
	}
}

func TestSource_readPacket_RejectsInvalidSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		sizeHeader int32
	}{
		{
			name:       "size_below_minimum",
			sizeHeader: minPacketSize - 1,
		},
		{
			name:       "size_above_maximum",
			sizeHeader: maxPacketSize + 1,
		},
		{
			name:       "negative_size",
			sizeHeader: -42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := newScriptedTCPServer(t, func(conn net.Conn) {
				_, _, _, err := readSourcePacket(conn)
				if err != nil {
					return
				}
				buf := new(bytes.Buffer)
				_ = binary.Write(buf, binary.LittleEndian, tt.sizeHeader)
				_, _ = conn.Write(buf.Bytes())
			})

			s, err := NewSource(Config{
				Address:  srv.addr,
				Password: "secret",
				Protocol: ProtocolSource,
				Timeout:  2 * time.Second,
			})
			require.NoError(t, err)

			err = s.Open(context.Background())
			defer func() { _ = s.Close() }()

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidPacket,
				"out-of-range packet size must surface as ErrInvalidPacket")
		})
	}
}

func TestSource_readPacket_ShortBody(t *testing.T) {
	t.Parallel()
	srv := newScriptedTCPServer(t, func(conn net.Conn) {
		_, _, _, err := readSourcePacket(conn)
		if err != nil {
			return
		}
		// Announce 20 bytes but write only the 8-byte id+type header. ReadFull will EOF.
		buf := new(bytes.Buffer)
		_ = binary.Write(buf, binary.LittleEndian, int32(20))
		_ = binary.Write(buf, binary.LittleEndian, int32(1))
		_ = binary.Write(buf, binary.LittleEndian, serverDataAuthResponse)
		_, _ = conn.Write(buf.Bytes())
		_ = conn.Close()
	})

	s, err := NewSource(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)

	err = s.Open(context.Background())
	defer func() { _ = s.Close() }()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to read packet data",
		"a body shorter than the announced size must surface as a read-data error")
}

func TestNewSource_NoConnection(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())

	s, err := NewSource(Config{
		Address:  addr,
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  500 * time.Millisecond,
	})
	require.NoError(t, err)

	err = s.Open(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to connect",
		"dialing a closed listener must surface the connect error")
}

func TestSource_Close_NilConnection(t *testing.T) {
	t.Parallel()
	s, err := NewSource(Config{
		Address:  "127.0.0.1:0",
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  time.Second,
	})
	require.NoError(t, err)

	assert.NoError(t, s.Close(), "Close on a never-opened source must be a no-op")
}

func TestSource_Close_IsIdempotent_AfterOpen(t *testing.T) {
	t.Parallel()
	srv := newScriptedTCPServer(t, func(conn net.Conn) {
		id, _, _, err := readSourcePacket(conn)
		if err != nil {
			return
		}
		_, _ = conn.Write(buildSourcePacket(t, id, serverDataAuthResponse, ""))
		// Block here so the server-side connection isn't already gone when client closes.
		_, _ = conn.Read(make([]byte, 1))
	})

	s, err := NewSource(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, s.Open(context.Background()))

	require.NoError(t, s.Close(), "first Close must succeed")

	err = s.Close()
	require.Error(t, err, "second Close must surface the underlying use-of-closed-network error")
	assert.True(t,
		strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "use of closed"),
		"second Close error should mention the closed network connection: %v", err)
}

func TestSource_Execute_RequestIDIncrementsAcrossCalls(t *testing.T) {
	t.Parallel()
	var captured []int32
	var counter atomic.Int32

	srv := newScriptedTCPServer(t, func(conn net.Conn) {
		authID, _, _, err := readSourcePacket(conn)
		if err != nil {
			return
		}
		_, _ = conn.Write(buildSourcePacket(t, authID, serverDataAuthResponse, ""))

		for {
			id, _, _, err := readSourcePacket(conn)
			if err != nil {
				return
			}
			captured = append(captured, id)
			counter.Add(1)
			_, _ = conn.Write(buildSourcePacket(t, id, serverDataResponseValue, "ok"))
		}
	})

	s, err := NewSource(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, s.Open(context.Background()))
	defer func() { _ = s.Close() }()

	for range 3 {
		_, err := s.Execute(context.Background(), "ping")
		require.NoError(t, err)
	}

	require.Eventually(t, func() bool {
		return counter.Load() == 3
	}, time.Second, 10*time.Millisecond, "server should observe three command packets")

	require.Len(t, captured, 3)
	assert.Equal(t, int32(2), captured[0], "first exec uses request ID 2 (auth used 1)")
	assert.Equal(t, int32(3), captured[1])
	assert.Equal(t, int32(4), captured[2])
}

// readSourcePacket reads one Source RCON packet from the wire and returns its decoded fields.
// Used by the test handlers to assert what the client sent.
func readSourcePacket(conn net.Conn) (int32, int32, string, error) {
	var size int32
	if err := binary.Read(conn, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", err
	}

	data := make([]byte, size)
	if _, err := io.ReadFull(conn, data); err != nil {
		return 0, 0, "", err
	}

	buf := bytes.NewReader(data)

	var id int32
	if err := binary.Read(buf, binary.LittleEndian, &id); err != nil {
		return 0, 0, "", err
	}

	var packetType int32
	if err := binary.Read(buf, binary.LittleEndian, &packetType); err != nil {
		return 0, 0, "", err
	}

	body := make([]byte, size-4-4-2)
	if _, err := io.ReadFull(buf, body); err != nil {
		return 0, 0, "", err
	}

	return id, packetType, string(body), nil
}

// buildSourcePacket assembles a Source RCON packet ready to be written to the wire.
func buildSourcePacket(t *testing.T, id, packetType int32, body string) []byte {
	t.Helper()
	bodyBytes := []byte(body)
	size := int32(4 + 4 + len(bodyBytes) + 2)

	buf := new(bytes.Buffer)
	require.NoError(t, binary.Write(buf, binary.LittleEndian, size))
	require.NoError(t, binary.Write(buf, binary.LittleEndian, id))
	require.NoError(t, binary.Write(buf, binary.LittleEndian, packetType))
	buf.Write(bodyBytes)
	buf.WriteByte(0)
	buf.WriteByte(0)

	return buf.Bytes()
}
