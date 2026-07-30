package rcon

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampEchoPing answers the ping handshake by echoing the request verbatim, which is what a real
// server does. Returning nil for the ping makes Open fail, which some tests want.
func sampEchoPing(req []byte) []byte {
	return req
}

// sampConsoleLine frames one console output line the way a server does: the client's own
// eleven-byte header followed by a 16-bit length and the text.
func sampConsoleLine(reqHeader []byte, text string) []byte {
	datagram := make([]byte, 0, sampHeaderSize+2+len(text))
	datagram = append(datagram, reqHeader...)
	datagram = binary.LittleEndian.AppendUint16(datagram, uint16(len(text)))
	datagram = append(datagram, text...)

	return datagram
}

// sampRconServer scripts a server that completes the handshake and then answers RCON commands
// with the given console lines. It captures the last RCON request for assertions.
func sampRconServer(t *testing.T, lines func() []string, captured chan<- []byte) *scriptedUDPServer {
	t.Helper()

	return newScriptedUDPServer(t, func(req []byte, _ int) [][]byte {
		if len(req) == sampHeaderSize+sampPingPayloadSize && req[10] == sampPingOpcode {
			return [][]byte{sampEchoPing(req)}
		}

		if captured != nil {
			select {
			case captured <- req:
			default:
			}
		}

		header := req[:sampHeaderSize]
		out := make([][]byte, 0, 4)
		for _, line := range lines() {
			out = append(out, sampConsoleLine(header, line))
		}

		return out
	})
}

func openSAMPClient(t *testing.T, addr string) *SAMP {
	t.Helper()

	client, err := NewSAMP(Config{Address: addr, Password: "secret", Timeout: 2 * time.Second})
	require.NoError(t, err)
	require.NoError(t, client.Open(context.Background()))
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func TestSAMP_OpenCompletesPingHandshake(t *testing.T) {
	requests := make(chan []byte, 1)
	srv := newScriptedUDPServer(t, func(req []byte, _ int) [][]byte {
		requests <- req

		return [][]byte{sampEchoPing(req)}
	})

	client := openSAMPClient(t, srv.addr)

	got := <-requests
	require.Len(t, got, sampHeaderSize+sampPingPayloadSize)
	assert.Equal(t, "SAMP", string(got[0:4]))
	assert.Equal(t, byte(sampPingOpcode), got[10], "handshake must use the ping opcode")
	assert.Equal(t, byte(sampRconOpcode), client.packetHeader[10],
		"the stored header keeps the rcon opcode for later commands")
}

func TestSAMP_OpenRejectsPingMismatch(t *testing.T) {
	srv := newScriptedUDPServer(t, func(req []byte, _ int) [][]byte {
		mangled := append([]byte(nil), req...)
		mangled[len(mangled)-1] ^= 0xFF

		return [][]byte{mangled}
	})

	client, err := NewSAMP(Config{Address: srv.addr, Password: "secret", Timeout: time.Second})
	require.NoError(t, err)

	err = client.Open(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSAMPPingMismatch)
	assert.Nil(t, client.connection, "a failed handshake must not leave a socket behind")
}

func TestSAMP_OpenFailsWhenServerIsSilent(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
		return nil
	})

	client, err := NewSAMP(Config{Address: srv.addr, Password: "secret", Timeout: 300 * time.Millisecond})
	require.NoError(t, err)

	err = client.Open(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "samp ping failed")
}

func TestSAMP_ExecuteFramesRconRequest(t *testing.T) {
	requests := make(chan []byte, 1)
	srv := sampRconServer(t, func() []string { return []string{"ok"} }, requests)

	client := openSAMPClient(t, srv.addr)

	_, err := client.Execute(context.Background(), "gmx")
	require.NoError(t, err)

	got := <-requests
	assert.Equal(t, byte(sampRconOpcode), got[10])

	offset := sampHeaderSize
	passwordLen := int(binary.LittleEndian.Uint16(got[offset : offset+2]))
	offset += 2
	assert.Equal(t, "secret", string(got[offset:offset+passwordLen]))
	offset += passwordLen

	commandLen := int(binary.LittleEndian.Uint16(got[offset : offset+2]))
	offset += 2
	assert.Equal(t, "gmx", string(got[offset:offset+commandLen]))
}

func TestSAMP_ExecuteJoinsConsoleLinesWithNewlines(t *testing.T) {
	srv := sampRconServer(t, func() []string {
		return []string{"ID\tName\tPing\tIP", "0\tAlice\t42\t192.0.2.10", "1\tBob\t53\t192.0.2.11"}
	}, nil)

	client := openSAMPClient(t, srv.addr)

	got, err := client.Execute(context.Background(), "players")

	require.NoError(t, err)
	assert.Equal(t, "ID\tName\tPing\tIP\n0\tAlice\t42\t192.0.2.10\n1\tBob\t53\t192.0.2.11", got,
		"each datagram is a separate console line and must not be concatenated into one run-on string")
}

func TestSAMP_ExecuteTreatsSilenceAsEmptyOutput(t *testing.T) {
	srv := sampRconServer(t, func() []string { return nil }, nil)

	client, err := NewSAMP(Config{Address: srv.addr, Password: "secret", Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	require.NoError(t, client.Open(context.Background()))
	defer func() { _ = client.Close() }()

	got, err := client.Execute(context.Background(), "gmx")

	require.NoError(t, err, "most samp commands print nothing; silence is not an error")
	assert.Empty(t, got)
}

func TestSAMP_ExecuteDetectsInvalidPassword(t *testing.T) {
	srv := sampRconServer(t, func() []string { return []string{sampRefusedMessage} }, nil)

	client := openSAMPClient(t, srv.addr)

	got, err := client.Execute(context.Background(), "players")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthenticationFailed)
	assert.Empty(t, got)
}

func TestSAMP_ExecuteSkipsUnusableDatagrams(t *testing.T) {
	tests := []struct {
		name     string
		datagram func(header []byte) []byte
	}{
		{
			name:     "too_short_for_a_length_prefix",
			datagram: func(header []byte) []byte { return header },
		},
		{
			name: "foreign_header",
			datagram: func(header []byte) []byte {
				foreign := append([]byte(nil), header...)
				foreign[4] ^= 0xFF

				return sampConsoleLine(foreign, "not ours")
			},
		},
		{
			name: "declared_length_exceeds_datagram",
			datagram: func(header []byte) []byte {
				d := sampConsoleLine(header, "short")
				binary.LittleEndian.PutUint16(d[sampHeaderSize:sampHeaderSize+2], 9000)

				return d
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newScriptedUDPServer(t, func(req []byte, _ int) [][]byte {
				if len(req) == sampHeaderSize+sampPingPayloadSize && req[10] == sampPingOpcode {
					return [][]byte{sampEchoPing(req)}
				}

				header := req[:sampHeaderSize]

				return [][]byte{tt.datagram(header), sampConsoleLine(header, "real output")}
			})

			client := openSAMPClient(t, srv.addr)

			got, err := client.Execute(context.Background(), "players")

			require.NoError(t, err)
			assert.Equal(t, "real output", got, "an unusable datagram must be skipped, not fatal")
		})
	}
}

func TestSAMP_BuildRconPacketRejectsOversizedField(t *testing.T) {
	client, err := NewSAMP(Config{Address: "127.0.0.1:7777", Password: strings.Repeat("x", 70000)})
	require.NoError(t, err)
	client.packetHeader = make([]byte, sampHeaderSize)

	_, err = client.buildRconPacket("players")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSAMPFieldTooLong)
}

func TestSAMP_CloseWithoutOpen(t *testing.T) {
	client, err := NewSAMP(Config{Address: "127.0.0.1:7777", Password: "secret"})
	require.NoError(t, err)

	assert.NoError(t, client.Close())
}
