package rcon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quakePrintDatagram(body string) []byte {
	return []byte(header + quakeResponseKeyword + "\n" + body)
}

func openQuakeClient(t *testing.T, addr string, newClient func(Config) (*Quake, error)) *Quake {
	t.Helper()

	client, err := newClient(Config{Address: addr, Password: "secret", Timeout: 2 * time.Second})
	require.NoError(t, err)
	require.NoError(t, client.Open(context.Background()))
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func TestQuake_ExecuteSendsOutOfBandRconRequest(t *testing.T) {
	requests := make(chan string, 1)
	srv := newScriptedUDPServer(t, func(req []byte, _ int) [][]byte {
		requests <- string(req)

		return [][]byte{quakePrintDatagram("map: q3dm17\n")}
	})

	client := openQuakeClient(t, srv.addr, NewQuake3)

	got, err := client.Execute(context.Background(), "status")

	require.NoError(t, err)
	assert.Equal(t, "map: q3dm17", got)
	assert.Equal(t, header+"rcon secret status", <-requests,
		"request must be a connectionless packet carrying password and command")
}

func TestQuake_ExecuteReassemblesChunkedResponse(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
		return [][]byte{
			quakePrintDatagram("num score ping name\n  0    15   45 Play"),
			quakePrintDatagram("er^7\n  1    12   50 Other^7\n"),
		}
	})

	client := openQuakeClient(t, srv.addr, NewQuake3)

	got, err := client.Execute(context.Background(), "status")

	require.NoError(t, err)
	assert.Equal(t, "num score ping name\n  0    15   45 Player^7\n  1    12   50 Other^7", got,
		"chunks are byte continuations and must be joined without a separator")
}

func TestQuake_ExecuteRefusals(t *testing.T) {
	tests := []struct {
		name      string
		newClient func(Config) (*Quake, error)
		response  string
		wantErr   error
		wantOK    bool
	}{
		{
			name:      "quake3_bad_password",
			newClient: NewQuake3,
			response:  "Bad rconpassword.\n",
			wantErr:   ErrAuthenticationFailed,
		},
		{
			name:      "quake3_rcon_disabled",
			newClient: NewQuake3,
			response:  "No rconpassword set on the server.\n",
			wantErr:   ErrRconDisabled,
		},
		{
			name:      "quake2_bad_password",
			newClient: NewQuake2,
			response:  "Bad rcon_password.\n",
			wantErr:   ErrAuthenticationFailed,
		},
		{
			name:      "quake3_ignores_quake2_marker",
			newClient: NewQuake3,
			response:  "Bad rcon_password.\n",
			wantOK:    true,
		},
		{
			name:      "quake2_ignores_quake3_marker",
			newClient: NewQuake2,
			response:  "Bad rconpassword.\n",
			wantOK:    true,
		},
		{
			name:      "output_quoting_the_marker_is_not_a_refusal",
			newClient: NewQuake3,
			response:  "say Bad rconpassword.\n",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
				return [][]byte{quakePrintDatagram(tt.response)}
			})

			client := openQuakeClient(t, srv.addr, tt.newClient)

			got, err := client.Execute(context.Background(), "status")

			if tt.wantOK {
				require.NoError(t, err)
				assert.Equal(t, strings.TrimSpace(tt.response), got)

				return
			}

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Empty(t, got)
		})
	}
}

func TestQuake_ExecuteSkipsForeignDatagrams(t *testing.T) {
	tests := []struct {
		name     string
		datagram []byte
	}{
		{name: "no_out_of_band_header", datagram: []byte("print\nnot ours")},
		{name: "other_out_of_band_command", datagram: []byte(header + "statusResponse\n\\mapname\\q3dm17")},
		{name: "header_only", datagram: []byte(header)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
				return [][]byte{tt.datagram, quakePrintDatagram("real output")}
			})

			client := openQuakeClient(t, srv.addr, NewQuake3)

			got, err := client.Execute(context.Background(), "status")

			require.NoError(t, err)
			assert.Equal(t, "real output", got, "unusable datagram must be skipped, not fatal")
		})
	}
}

func TestQuake_ExecuteAcceptsPrintWithoutNewline(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
		return [][]byte{[]byte(header + quakeResponseKeyword + "output without newline")}
	})

	client := openQuakeClient(t, srv.addr, NewQuake3)

	got, err := client.Execute(context.Background(), "status")

	require.NoError(t, err)
	assert.Equal(t, "output without newline", got)
}

func TestQuake_ExecuteTimesOutWhenServerIsSilent(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
		return nil
	})

	client, err := NewQuake3(Config{Address: srv.addr, Password: "secret", Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	require.NoError(t, client.Open(context.Background()))
	defer func() { _ = client.Close() }()

	_, err = client.Execute(context.Background(), "status")

	require.Error(t, err)
	assert.True(t, isTimeoutError(err), "silence must surface as a timeout, got %v", err)
}

func TestQuake_OpenRejectsPasswordWithWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{name: "space", password: "two words"},
		{name: "tab", password: "with\ttab"},
		{name: "newline", password: "with\nnewline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewQuake3(Config{Address: "127.0.0.1:27960", Password: tt.password, Timeout: time.Second})
			require.NoError(t, err)

			err = client.Open(context.Background())

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPasswordContainsWhitespace)
			assert.Nil(t, client.connection, "no socket must be opened for an unusable password")
		})
	}
}

func TestQuake_ExecuteDropsStaleDatagramsFromPreviousCommand(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, idx int) [][]byte {
		if idx == 0 {
			return [][]byte{
				quakePrintDatagram("first"),
				quakePrintDatagram("late tail of the first command"),
			}
		}

		return [][]byte{quakePrintDatagram("second")}
	})

	client := openQuakeClient(t, srv.addr, NewQuake3)

	first, err := client.Execute(context.Background(), "status")
	require.NoError(t, err)
	assert.Contains(t, first, "first")

	second, err := client.Execute(context.Background(), "status")

	require.NoError(t, err)
	assert.Equal(t, "second", second, "a late datagram must not leak into the next response")
}

func TestQuake_ExecuteCapsReassembledResponse(t *testing.T) {
	if testing.Short() {
		t.Skip("streams ~1 MiB over loopback UDP")
	}

	chunk := strings.Repeat("A", 4000)
	datagrams := make([][]byte, 0, maxReassembledResponseSize/4000+2)
	for range maxReassembledResponseSize/4000 + 2 {
		datagrams = append(datagrams, quakePrintDatagram(chunk))
	}

	srv := newPacedScriptedUDPServer(t, 2*time.Millisecond, func(_ []byte, _ int) [][]byte {
		return datagrams
	})

	client := openQuakeClient(t, srv.addr, NewQuake3)

	got, err := client.Execute(context.Background(), "cvarlist")

	require.NoError(t, err)
	assert.Greater(t, len(got), maxReassembledResponseSize)
	assert.Less(t, len(got), maxReassembledResponseSize+maxResponseSize,
		"reassembly must stop right after crossing the cap")
}

func TestQuake_CloseWithoutOpen(t *testing.T) {
	client, err := NewQuake2(Config{Address: "127.0.0.1:27910", Password: "secret"})
	require.NoError(t, err)

	assert.NoError(t, client.Close())
}
