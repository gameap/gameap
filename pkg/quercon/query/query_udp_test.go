package query

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUDPServer answers datagrams on the wildcard loopback address: handler is
// invoked for every received datagram and returns the datagrams to send back.
// It returns the port the server is listening on. The socket is bound to the
// wildcard address so both "127.0.0.1" and "localhost" reach it.
func fakeUDPServer(t *testing.T, handler func(request []byte) [][]byte) int {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
	require.NoError(t, err)

	done := make(chan struct{})

	go func() {
		defer close(done)

		buffer := make([]byte, 65535)
		for {
			n, addr, readErr := conn.ReadFromUDP(buffer)
			if readErr != nil {
				return
			}

			for _, response := range handler(buffer[:n]) {
				_, _ = conn.WriteToUDP(response, addr)
			}
		}
	}()

	t.Cleanup(func() {
		_ = conn.Close()
		<-done
	})

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	require.True(t, ok, "UDP listener must have a UDP address")

	return addr.Port
}

// closedUDPPort returns a loopback port where nothing listens anymore, so query
// functions hit their read deadline against it.
func closedUDPPort(t *testing.T) int {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	require.True(t, ok, "UDP listener must have a UDP address")

	require.NoError(t, conn.Close())

	return addr.Port
}

func TestQueryQuake2(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("queries_server_over_udp", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{
				[]byte(quake2ResponseHeader + `\hostname\Q2 Server\mapname\q2dm1\maxplayers\16`),
			}
		})

		// ACT
		result, err := queryQuake2(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online, "server answered with a valid response, result must be online")
		assert.Equal(t, "Q2 Server", result.Name)
		assert.Equal(t, "q2dm1", result.Map)
	})

	t.Run("read_timeout_when_nobody_answers", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := closedUDPPort(t)
		shortCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()

		// ACT
		result, err := queryQuake2(shortCtx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read query response", "error message mismatch")
		assert.False(t, result.Online)
	})

	t.Run("malformed_response", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{[]byte("garbage")}
		})

		// ACT
		result, err := queryQuake2(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse response", "error message mismatch")
		assert.False(t, result.Online)
	})
}

func TestQueryQuake3(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("queries_server_over_udp", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{
				[]byte(quake3ResponseHeader +
					`\sv_hostname\Q3 Server\mapname\q3dm17\sv_maxclients\32` + "\n" +
					`5 0 "Player One"` + "\n"),
			}
		})

		// ACT
		result, err := queryQuake3(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online, "server answered with a valid response, result must be online")
		assert.Equal(t, "Q3 Server", result.Name)
		assert.Equal(t, "q3dm17", result.Map)
		assert.Equal(t, 32, result.MaxPlayersNum)
		require.Len(t, result.Players, 1)
		assert.Equal(t, "Player One", result.Players[0].Name)
		assert.Equal(t, 5, result.Players[0].Score)
	})

	t.Run("read_timeout_when_nobody_answers", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := closedUDPPort(t)
		shortCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()

		// ACT
		result, err := queryQuake3(shortCtx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read query response", "error message mismatch")
		assert.False(t, result.Online)
	})

	t.Run("malformed_response", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{[]byte("garbage")}
		})

		// ACT
		result, err := queryQuake3(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse response", "error message mismatch")
		assert.False(t, result.Online)
	})
}

func TestQueryGameSpy2(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("queries_server_over_udp", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{
				buildGameSpy2Response("GS2 Server", "de_dust2", 3, 32, []string{"Player1", "Player2"}),
			}
		})

		// ACT
		result, err := queryGameSpy2(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online, "server answered with a valid response, result must be online")
		assert.Equal(t, "GS2 Server", result.Name)
		assert.Equal(t, "de_dust2", result.Map)
		assert.Equal(t, 3, result.PlayersNum)
		assert.Equal(t, 32, result.MaxPlayersNum)
		require.Len(t, result.Players, 2)
		assert.Equal(t, "Player1", result.Players[0].Name)
		assert.Equal(t, "Player2", result.Players[1].Name)
	})

	t.Run("read_timeout_when_nobody_answers", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := closedUDPPort(t)
		shortCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()

		// ACT
		result, err := queryGameSpy2(shortCtx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read response", "error message mismatch")
		assert.False(t, result.Online)
	})

	t.Run("malformed_response", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{{0x01, 0x02, 0x03, 0x04, 0x05}}
		})

		// ACT
		result, err := queryGameSpy2(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse response", "error message mismatch")
		assert.False(t, result.Online)
	})
}

func TestQueryGameSpy3(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("queries_server_over_udp_with_challenge", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — the fake server implements the challenge-response handshake.
		port := fakeUDPServer(t, func(request []byte) [][]byte {
			if len(request) >= 3 && request[0] == 0xFE && request[1] == 0xFD && request[2] == 0x09 {
				return [][]byte{
					{0x09, 0x10, 0x20, 0x30, 0x40, '0', 0x00},
				}
			}

			payload := buildGameSpy3ResponseWithPlayers("GS3 Server", "strike_at_karkand", 2, 64, []string{"Player1", "Player2"})
			packet := append([]byte{0x00}, make([]byte, 15)...)
			packet = append(packet, payload...)

			return [][]byte{packet}
		})

		// ACT
		result, err := queryGameSpy3(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online, "server answered with a valid response, result must be online")
		assert.Equal(t, "GS3 Server", result.Name)
		assert.Equal(t, "strike_at_karkand", result.Map)
		assert.Equal(t, 2, result.PlayersNum)
		assert.Equal(t, 64, result.MaxPlayersNum)
		require.Len(t, result.Players, 2)
		assert.Equal(t, "Player1", result.Players[0].Name)
	})

	t.Run("read_timeout_when_nobody_answers", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := closedUDPPort(t)
		shortCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()

		// ACT
		result, err := queryGameSpy3(shortCtx, "localhost", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read challenge response", "error message mismatch")
		assert.False(t, result.Online)
	})

	t.Run("malformed_challenge_response", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{{0x01, 0x02}}
		})

		// ACT
		result, err := queryGameSpy3(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse challenge", "error message mismatch")
		assert.False(t, result.Online)
	})

	t.Run("assembles_split_packets", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — the payload is split into two packets carrying the splitnum marker.
		payload := buildGameSpy3ResponseWithPlayers("GS3 Split", "karkand", 1, 64, []string{"Player1"})
		half := len(payload) / 2

		splitPacket := func(packetID byte, data []byte) []byte {
			packet := make([]byte, 0, 16+len(data))
			packet = append(packet, 0x00, 0x10, 0x20, 0x30, 0x40)
			packet = append(packet, []byte("splitnum\x00")...)
			packet = append(packet, packetID, 0x00)
			packet = append(packet, data...)

			return packet
		}

		port := fakeUDPServer(t, func(request []byte) [][]byte {
			if len(request) >= 3 && request[0] == 0xFE && request[1] == 0xFD && request[2] == 0x09 {
				return [][]byte{
					{0x09, 0x10, 0x20, 0x30, 0x40, '0', 0x00},
				}
			}

			return [][]byte{
				splitPacket(0, payload[:half]),
				splitPacket(1, payload[half:]),
			}
		})

		// ACT
		result, err := queryGameSpy3(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online, "split packets reassembled into a valid response, result must be online")
		assert.Equal(t, "GS3 Split", result.Name)
		assert.Equal(t, "karkand", result.Map)
		assert.Equal(t, 64, result.MaxPlayersNum)
		require.Len(t, result.Players, 1)
		assert.Equal(t, "Player1", result.Players[0].Name)
	})
}

func TestQueryRakNet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("queries_server_over_udp", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{
				buildRakNetResponse("MCPE;My Server;486;1.19.0;5;20;12345;Overworld;Survival"),
			}
		})

		// ACT
		result, err := queryRakNet(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online, "server answered with a valid response, result must be online")
		assert.Equal(t, "My Server", result.Name)
		assert.Equal(t, 5, result.PlayersNum)
		assert.Equal(t, 20, result.MaxPlayersNum)
		assert.Equal(t, "Survival", result.Map)
	})

	t.Run("read_timeout_when_nobody_answers", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := closedUDPPort(t)
		shortCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()

		// ACT
		result, err := queryRakNet(shortCtx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read response", "error message mismatch")
		assert.False(t, result.Online)
	})

	t.Run("malformed_response", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{make([]byte, 40)}
		})

		// ACT
		result, err := queryRakNet(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse response", "error message mismatch")
		assert.False(t, result.Online)
	})
}

func TestQuerySAMP(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("queries_server_over_udp", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — the response must echo the query header (SAMP + ip + port + opcode).
		port := fakeUDPServer(t, func(request []byte) [][]byte {
			response := make([]byte, 0, 64)
			response = append(response, request[:11]...)
			response = append(response, 0x00)                         // password flag
			response = append(response, 0x0A, 0x00)                   // players: 10 (LE)
			response = append(response, 0x64, 0x00)                   // max players: 100 (LE)
			response = appendSAMPTestString(response, "SA-MP Server") // hostname
			response = appendSAMPTestString(response, "Freeroam")     // gametype
			response = appendSAMPTestString(response, "English")      // language

			return [][]byte{response}
		})

		// ACT
		result, err := querySAMP(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online, "server answered with a valid response, result must be online")
		assert.Equal(t, "SA-MP Server", result.Name)
		assert.Equal(t, "Freeroam", result.Map)
		assert.Equal(t, 10, result.PlayersNum)
		assert.Equal(t, 100, result.MaxPlayersNum)
	})

	t.Run("read_timeout_when_nobody_answers", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := closedUDPPort(t)
		shortCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()

		// ACT
		result, err := querySAMP(shortCtx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read response", "error message mismatch")
		assert.False(t, result.Online)
	})

	t.Run("malformed_response", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{[]byte("this is not a valid SAMP info response at all!!!!!")}
		})

		// ACT
		result, err := querySAMP(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse response", "error message mismatch")
		assert.False(t, result.Online)
	})
}

func appendSAMPTestString(dst []byte, s string) []byte {
	length := len(s)
	dst = append(dst, byte(length), byte(length>>8), byte(length>>16), byte(length>>24))

	return append(dst, []byte(s)...)
}

func TestQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("unsupported_protocol", func(t *testing.T) {
		t.Parallel()
		// ACT
		result, err := Query(ctx, "127.0.0.1", 27015, Protocol("bogus"))

		// ASSERT
		require.Error(t, err)
		assert.Equal(t, "unsupported query protocol: bogus", err.Error(), "error message mismatch")
		assert.Nil(t, result)
	})

	t.Run("delegates_to_protocol_query_func", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{
				[]byte(quake2ResponseHeader + `\hostname\Dispatched\mapname\q2dm2`),
			}
		})

		// ACT
		result, err := Query(ctx, "127.0.0.1", port, ProtocolQuake2)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online)
		assert.Equal(t, "Dispatched", result.Name)
		assert.Equal(t, "q2dm2", result.Map)
	})
}
