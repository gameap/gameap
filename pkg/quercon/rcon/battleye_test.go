package rcon

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// battlEyeDatagram frames a payload independently of buildBattlEyePacket, so a mistake in the
// production framing (byte order, CRC coverage) is actually caught instead of mirrored.
func battlEyeDatagram(payload ...byte) []byte {
	body := append([]byte{0xFF}, payload...)

	datagram := make([]byte, 0, 7+len(payload))
	datagram = append(datagram, 'B', 'E')

	checksum := make([]byte, 4)
	binary.LittleEndian.PutUint32(checksum, crc32.ChecksumIEEE(body))
	datagram = append(datagram, checksum...)

	return append(datagram, body...)
}

func battlEyeCommandDatagram(sequence byte, body string) []byte {
	return battlEyeDatagram(append([]byte{battlEyePacketCommand, sequence}, body...)...)
}

func battlEyeMultipartDatagram(sequence, total, index byte, chunk string) []byte {
	payload := append([]byte{battlEyePacketCommand, sequence, 0x00, total, index}, chunk...)

	return battlEyeDatagram(payload...)
}

var battlEyeLoginOK = battlEyeDatagram(battlEyePacketLogin, battlEyeLoginSucceeded)

// battlEyeServer answers the login and then delegates command handling to the caller.
func battlEyeServer(t *testing.T, onCommand func(req []byte, idx int) [][]byte) *scriptedUDPServer {
	t.Helper()

	return newScriptedUDPServer(t, func(req []byte, idx int) [][]byte {
		payload, err := parseBattlEyePacket(req)
		if err != nil {
			return nil
		}

		if payload[0] == battlEyePacketLogin {
			return [][]byte{battlEyeLoginOK}
		}

		return onCommand(req, idx)
	})
}

func openBattlEyeClient(t *testing.T, addr string) *BattlEye {
	t.Helper()

	client, err := NewBattlEye(Config{Address: addr, Password: "secret", Timeout: 2 * time.Second})
	require.NoError(t, err)
	require.NoError(t, client.Open(context.Background()))
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func TestBattlEye_BuildPacket(t *testing.T) {
	got := buildBattlEyePacket([]byte{battlEyePacketLogin, 'p', 'w'})

	require.Len(t, got, 7+3)
	assert.Equal(t, "BE", string(got[0:2]))
	assert.Equal(t, byte(0xFF), got[6], "the payload must be preceded by the 0xFF terminator")
	assert.Equal(t, crc32.ChecksumIEEE([]byte{0xFF, battlEyePacketLogin, 'p', 'w'}),
		binary.LittleEndian.Uint32(got[2:6]),
		"checksum covers the terminator and the payload, little-endian")
}

func TestBattlEye_ParsePacket(t *testing.T) {
	tests := []struct {
		name      string
		datagram  []byte
		want      []byte
		wantError string
	}{
		{
			name:     "valid_packet",
			datagram: battlEyeDatagram(battlEyePacketCommand, 7, 'o', 'k'),
			want:     []byte{battlEyePacketCommand, 7, 'o', 'k'},
		},
		{
			name:      "too_short",
			datagram:  []byte{'B', 'E', 0, 0, 0, 0, 0xFF},
			wantError: "datagram is too short",
		},
		{
			name:      "bad_magic",
			datagram:  append([]byte{'X', 'Y'}, battlEyeDatagram(battlEyePacketLogin, 1)[2:]...),
			wantError: "unexpected magic bytes",
		},
		{
			name: "missing_terminator",
			datagram: func() []byte {
				d := battlEyeDatagram(battlEyePacketLogin, 1)
				d[6] = 0x00

				return d
			}(),
			wantError: "missing payload terminator",
		},
		{
			name: "checksum_mismatch",
			datagram: func() []byte {
				d := battlEyeDatagram(battlEyePacketLogin, 1)
				d[2] ^= 0xFF

				return d
			}(),
			wantError: "checksum mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBattlEyePacket(tt.datagram)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidBattlEyePacket)
				assert.Contains(t, err.Error(), tt.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBattlEye_OpenSendsLoginPayload(t *testing.T) {
	requests := make(chan []byte, 1)
	srv := newScriptedUDPServer(t, func(req []byte, _ int) [][]byte {
		requests <- req

		return [][]byte{battlEyeLoginOK}
	})

	openBattlEyeClient(t, srv.addr)

	payload, err := parseBattlEyePacket(<-requests)
	require.NoError(t, err)
	assert.Equal(t, byte(battlEyePacketLogin), payload[0])
	assert.Equal(t, "secret", string(payload[1:]))
}

func TestBattlEye_OpenRejectsBadPassword(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
		return [][]byte{battlEyeDatagram(battlEyePacketLogin, 0x00)}
	})

	client, err := NewBattlEye(Config{Address: srv.addr, Password: "wrong", Timeout: time.Second})
	require.NoError(t, err)

	err = client.Open(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAuthenticationFailed)
	assert.Nil(t, client.connection)
}

func TestBattlEye_OpenSkipsCorruptDatagramBeforeLoginAnswer(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
		corrupt := battlEyeDatagram(battlEyePacketLogin, battlEyeLoginSucceeded)
		corrupt[3] ^= 0xFF

		return [][]byte{corrupt, battlEyeLoginOK}
	})

	client, err := NewBattlEye(Config{Address: srv.addr, Password: "secret", Timeout: time.Second})
	require.NoError(t, err)

	assert.NoError(t, client.Open(context.Background()))
	_ = client.Close()
}

func TestBattlEye_OpenFailsWhenServerIsSilent(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
		return nil
	})

	client, err := NewBattlEye(Config{Address: srv.addr, Password: "secret", Timeout: 300 * time.Millisecond})
	require.NoError(t, err)

	err = client.Open(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rcon may be disabled")
}

func TestBattlEye_ExecuteSingleAndEmptyResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "with_output", body: "Players on server:\n0 192.0.2.10:2304 45 abc(OK) Alice\n", want: "Players on server:\n0 192.0.2.10:2304 45 abc(OK) Alice"},
		{name: "without_output", body: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := battlEyeServer(t, func(req []byte, _ int) [][]byte {
				payload, _ := parseBattlEyePacket(req)

				return [][]byte{battlEyeCommandDatagram(payload[1], tt.body)}
			})

			client := openBattlEyeClient(t, srv.addr)

			got, err := client.Execute(context.Background(), "players")

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBattlEye_ExecuteReassemblesOutOfOrderMultipart(t *testing.T) {
	srv := battlEyeServer(t, func(req []byte, _ int) [][]byte {
		payload, _ := parseBattlEyePacket(req)
		seq := payload[1]

		return [][]byte{
			battlEyeMultipartDatagram(seq, 3, 2, "third"),
			battlEyeMultipartDatagram(seq, 3, 0, "first "),
			battlEyeMultipartDatagram(seq, 3, 1, "second "),
		}
	})

	client := openBattlEyeClient(t, srv.addr)

	got, err := client.Execute(context.Background(), "players")

	require.NoError(t, err)
	assert.Equal(t, "first second third", got, "parts must be ordered by index, not by arrival")
}

func TestBattlEye_ExecuteAcknowledgesServerMessages(t *testing.T) {
	acks := make(chan byte, 4)
	srv := newScriptedUDPServer(t, func(req []byte, _ int) [][]byte {
		payload, err := parseBattlEyePacket(req)
		if err != nil {
			return nil
		}

		switch payload[0] {
		case battlEyePacketLogin:
			return [][]byte{battlEyeLoginOK}
		case battlEyePacketServerMessage:
			acks <- payload[1]

			return nil
		default:
			return [][]byte{
				battlEyeDatagram(battlEyePacketServerMessage, 9, 'c', 'h', 'a', 't'),
				battlEyeCommandDatagram(payload[1], "done"),
			}
		}
	})

	client := openBattlEyeClient(t, srv.addr)

	got, err := client.Execute(context.Background(), "players")

	require.NoError(t, err)
	assert.Equal(t, "done", got, "a pushed message must not be mistaken for the command answer")
	assert.Equal(t, byte(9), <-acks, "pushed messages must be acknowledged or the server deauthenticates us")
}

func TestBattlEye_ExecuteIgnoresStaleSequence(t *testing.T) {
	srv := battlEyeServer(t, func(req []byte, _ int) [][]byte {
		payload, _ := parseBattlEyePacket(req)
		seq := payload[1]

		return [][]byte{
			battlEyeCommandDatagram(seq-1, "answer to a previous command"),
			battlEyeCommandDatagram(seq, "current answer"),
		}
	})

	client := openBattlEyeClient(t, srv.addr)

	got, err := client.Execute(context.Background(), "players")

	require.NoError(t, err)
	assert.Equal(t, "current answer", got)
}

func TestBattlEye_ExecuteIncrementsSequence(t *testing.T) {
	sequences := make(chan byte, 2)
	srv := battlEyeServer(t, func(req []byte, _ int) [][]byte {
		payload, _ := parseBattlEyePacket(req)
		sequences <- payload[1]

		return [][]byte{battlEyeCommandDatagram(payload[1], "ok")}
	})

	client := openBattlEyeClient(t, srv.addr)

	_, err := client.Execute(context.Background(), "players")
	require.NoError(t, err)
	_, err = client.Execute(context.Background(), "players")
	require.NoError(t, err)

	assert.Equal(t, byte(0), <-sequences)
	assert.Equal(t, byte(1), <-sequences)
}

func TestBattlEye_ExecuteTimesOutOnIncompleteMultipart(t *testing.T) {
	srv := battlEyeServer(t, func(req []byte, _ int) [][]byte {
		payload, _ := parseBattlEyePacket(req)

		return [][]byte{battlEyeMultipartDatagram(payload[1], 2, 0, "only the first part")}
	})

	client, err := NewBattlEye(Config{Address: srv.addr, Password: "secret", Timeout: 400 * time.Millisecond})
	require.NoError(t, err)
	require.NoError(t, client.Open(context.Background()))
	defer func() { _ = client.Close() }()

	_, err = client.Execute(context.Background(), "players")

	require.Error(t, err)
	assert.True(t, isTimeoutError(err) || strings.Contains(err.Error(), ErrBattlEyeIncompleteResponse.Error()),
		"an unfinished multipart response must not hang, got %v", err)
}

func TestBattlEye_MultipartDetection(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want bool
	}{
		{name: "valid_sub_header", body: []byte{0x00, 3, 1, 'x'}, want: true},
		{name: "plain_text_body", body: []byte("Players on server:"), want: false},
		{name: "single_part_announced", body: []byte{0x00, 1, 0, 'x'}, want: false},
		{name: "index_out_of_range", body: []byte{0x00, 2, 2, 'x'}, want: false},
		{name: "too_short", body: []byte{0x00, 2}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, ok := battlEyeMultipart(tt.body)

			assert.Equal(t, tt.want, ok)
		})
	}
}

func TestBattlEye_CloseWithoutOpen(t *testing.T) {
	client, err := NewBattlEye(Config{Address: "127.0.0.1:2302", Password: "secret"})
	require.NoError(t, err)

	assert.NoError(t, client.Close())
}
