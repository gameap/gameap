package rcon

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoldSource_Open_AcceptsValidChallengeReply(t *testing.T) {
	srv := newScriptedUDPServer(t, func(req []byte, _ int) [][]byte {
		assert.Equal(t, header+"challenge rcon", string(req),
			"first datagram must be the GoldSource challenge request")

		return [][]byte{[]byte(header + "\x00challenge rcon 1234567\n")}
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolGoldSrc,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)

	require.NoError(t, g.Open(context.Background()))
	defer func() { _ = g.Close() }()

	assert.Equal(t, "1234567", g.challengeNumber,
		"challenge number must be the third whitespace-separated token from the body")
}

func TestGoldSource_Open_ReturnsErrorOnConnectFailure(t *testing.T) {
	g, err := NewGoldSource(Config{
		// Reserved-test address with port 0 — DialContext("udp") will fail.
		Address:  "256.256.256.256:0",
		Password: "secret",
		Protocol: ProtocolGoldSrc,
		Timeout:  500 * time.Millisecond,
	})
	require.NoError(t, err)

	err = g.Open(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to connect",
		"open must surface dial errors with the contractual prefix")
}

func TestGoldSource_getChallengeNumber_RejectsMalformedReplies(t *testing.T) {
	tests := []struct {
		name      string
		reply     []byte
		wantError string
	}{
		{
			name:      "less_than_three_parts",
			reply:     []byte(header + "\x00challenge_only"),
			wantError: ErrInvalidChallengeResponse.Error(),
		},
		{
			name:      "two_parts",
			reply:     []byte(header + "\x00challenge rcon"),
			wantError: ErrInvalidChallengeResponse.Error(),
		},
		{
			name:      "empty_body_after_header_strip",
			reply:     []byte(header + "X"),
			wantError: ErrInvalidChallengeResponse.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
				return [][]byte{tt.reply}
			})

			g, err := NewGoldSource(Config{
				Address:  srv.addr,
				Password: "secret",
				Protocol: ProtocolGoldSrc,
				Timeout:  2 * time.Second,
			})
			require.NoError(t, err)

			err = g.Open(context.Background())
			defer func() { _ = g.Close() }()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
			assert.Nil(t, g.connection, "a failed Open must release the dialed connection")
		})
	}
}

func TestGoldSource_getChallengeNumber_ShortReply_LeavesChallengeEmpty(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
		// 4 bytes total — a bare header with no challenge body.
		return [][]byte{[]byte("\xff\xff\xff\xff")}
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolGoldSrc,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)

	err = g.Open(context.Background())
	defer func() { _ = g.Close() }()

	require.Error(t, err, "a 4-byte reply has no challenge body and must error")
	assert.ErrorIs(t, err, ErrInvalidChallengeResponse)
}

func TestGoldSource_Execute_SinglePacketResponse(t *testing.T) {
	const password = "topsecret"
	const challenge = "424242"

	var requests atomic.Int32

	srv := newScriptedUDPServer(t, func(req []byte, idx int) [][]byte {
		requests.Add(1)

		switch idx {
		case 0:
			return [][]byte{[]byte(header + "\x00challenge rcon " + challenge + "\n")}
		case 1:
			expectedPrefix := header + "rcon " + challenge + " \"" + password + "\" status"
			assert.Equal(t, expectedPrefix, string(req),
				"command datagram must include challenge, quoted password and command")

			return [][]byte{[]byte(header + "\x00short response\n")}
		}

		return nil
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: password,
		Protocol: ProtocolGoldSrc,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, g.Open(context.Background()))
	defer func() { _ = g.Close() }()

	got, err := g.Execute(context.Background(), "status")
	require.NoError(t, err)
	assert.Equal(t, "short response", got,
		"single-packet body must be returned trimmed of trailing whitespace and nulls")
	assert.EqualValues(t, 2, requests.Load(),
		"a single-packet response must be fetched with exactly one rcon request")
}

func TestGoldSource_Execute_MultiPacketResponseReassembled(t *testing.T) {
	const challenge = "999"
	part1 := strings.Repeat("A", 300)
	const part2 = "tail\n"

	var requests atomic.Int32

	srv := newScriptedUDPServer(t, func(req []byte, idx int) [][]byte {
		requests.Add(1)

		switch idx {
		case 0:
			return [][]byte{[]byte(header + "\x00challenge rcon " + challenge + "\n")}
		case 1:
			assert.Contains(t, string(req), "status",
				"the command datagram must carry the user-supplied command")

			// GoldSource sends a large reply as several back-to-back datagrams to a
			// single request; no follow-up request is needed to receive them all.
			return [][]byte{
				[]byte(header + "\x00" + part1),
				[]byte(header + "\x00" + part2),
			}
		}

		return nil
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, g.Open(context.Background()))
	defer func() { _ = g.Close() }()

	got, err := g.Execute(context.Background(), "status")
	require.NoError(t, err)

	assert.Equal(t, part1+"tail", got,
		"all datagrams of the response must be reassembled in order")
	assert.EqualValues(t, 2, requests.Load(),
		"the whole multi-packet response must be fetched with a single rcon request")
}

func TestGoldSource_Execute_ShortIntermediatePacketDoesNotEndResponse(t *testing.T) {
	const challenge = "111"
	// A short datagram in the MIDDLE of a response must not terminate reassembly:
	// only an idle gap marks the end of the response. UDP may also deliver datagrams
	// out of order, so packet size is not a reliable end-of-response signal.
	const part1 = "abc"
	part2 := strings.Repeat("B", 300)

	srv := newScriptedUDPServer(t, func(_ []byte, idx int) [][]byte {
		switch idx {
		case 0:
			return [][]byte{[]byte(header + "\x00challenge rcon " + challenge + "\n")}
		case 1:
			return [][]byte{
				[]byte(header + "\x00" + part1),
				[]byte(header + "\x00" + part2),
			}
		}

		return nil
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, g.Open(context.Background()))
	defer func() { _ = g.Close() }()

	got, err := g.Execute(context.Background(), "status")
	require.NoError(t, err)
	assert.Equal(t, part1+part2, got,
		"a short intermediate datagram must not truncate the rest of the response")
}

func TestGoldSource_Execute_LateDatagramDoesNotPoisonNextCommand(t *testing.T) {
	const challenge = "222"

	srv := newScriptedUDPServer(t, func(_ []byte, idx int) [][]byte {
		switch idx {
		case 0:
			return [][]byte{[]byte(header + "\x00challenge rcon " + challenge + "\n")}
		case 1:
			return [][]byte{[]byte(header + "\x00first")}
		case 2:
			return [][]byte{[]byte(header + "\x00second")}
		}

		return nil
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, g.Open(context.Background()))
	defer func() { _ = g.Close() }()

	got, err := g.Execute(context.Background(), "first command")
	require.NoError(t, err)
	require.Equal(t, "first", got)

	// A stray datagram lands in the receive buffer between commands (a late reply to the
	// previous command on a reused connection). The next Execute must drain it instead of
	// mistaking it for its own response.
	injector, err := net.Dial("udp", g.connection.LocalAddr().String())
	require.NoError(t, err)
	_, err = injector.Write([]byte(header + "lSTALE"))
	require.NoError(t, err)
	require.NoError(t, injector.Close())

	time.Sleep(50 * time.Millisecond) // let the stray datagram reach the socket buffer

	got, err = g.Execute(context.Background(), "second command")
	require.NoError(t, err)
	assert.Equal(t, "second", got,
		"a stale datagram left over between commands must be drained, not returned")
}

func TestGoldSource_Execute_PropagatesWriteErrorWhenSocketClosed(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
		return [][]byte{[]byte(header + "\x00challenge rcon 5\n")}
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, g.Open(context.Background()))

	require.NoError(t, g.Close())

	_, err = g.Execute(context.Background(), "status")
	require.Error(t, err, "writing on a closed UDP connection must surface as an error")
}

func TestGoldSource_Close_NilConnection(t *testing.T) {
	g, err := NewGoldSource(Config{
		Address:  "127.0.0.1:0",
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  time.Second,
	})
	require.NoError(t, err)

	assert.NoError(t, g.Close(), "Close on a never-opened goldsource must be a no-op")
}

func TestGoldSource_Execute_LargeDatagramNotTruncated(t *testing.T) {
	const challenge = "777"
	// Non-whitespace, position-identifiable content larger than the old 1024-byte read buffer.
	// Before the fix this datagram was chopped to 1019 bytes and its tail was silently dropped.
	longBody := strings.Repeat("0123456789", 130) // 1300 bytes
	const tail = "END"

	srv := newScriptedUDPServer(t, func(_ []byte, idx int) [][]byte {
		switch idx {
		case 0:
			return [][]byte{[]byte(header + "\x00challenge rcon " + challenge + "\n")}
		case 1:
			return [][]byte{
				[]byte(header + "\x00" + longBody),
				[]byte(header + "\x00" + tail),
			}
		}

		return nil
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, g.Open(context.Background()))
	defer func() { _ = g.Close() }()

	got, err := g.Execute(context.Background(), "amxx plugins")
	require.NoError(t, err)
	assert.Equal(t, longBody+tail, got,
		"a datagram larger than the old 1024-byte buffer must be returned in full and joined with the next part")
}

func TestGoldSource_Execute_PreservesBoundaryWhitespace(t *testing.T) {
	const challenge = "555"
	// The first datagram ends with a space that lands exactly on the datagram boundary.
	// Trimming must happen once at the very end, not per chunk.
	const firstPart = "foo "
	const secondPart = "bar"

	srv := newScriptedUDPServer(t, func(_ []byte, idx int) [][]byte {
		switch idx {
		case 0:
			return [][]byte{[]byte(header + "\x00challenge rcon " + challenge + "\n")}
		case 1:
			return [][]byte{
				[]byte(header + "\x00" + firstPart),
				[]byte(header + "\x00" + secondPart),
			}
		}

		return nil
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, g.Open(context.Background()))
	defer func() { _ = g.Close() }()

	got, err := g.Execute(context.Background(), "status")
	require.NoError(t, err)
	assert.Equal(t, "foo bar", got,
		"whitespace on a datagram boundary must survive reassembly")
}

func TestGoldSource_Execute_IdleTimeoutTerminatesResponse(t *testing.T) {
	const challenge = "321"
	const part = "partial response"

	srv := newScriptedUDPServer(t, func(_ []byte, idx int) [][]byte {
		switch idx {
		case 0:
			return [][]byte{[]byte(header + "\x00challenge rcon " + challenge + "\n")}
		case 1:
			// No further datagrams follow: the read loop must stop on the idle timeout
			// and return what it already has instead of erroring or blocking forever.
			return [][]byte{[]byte(header + "\x00" + part)}
		}

		return nil
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  5 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, g.Open(context.Background()))
	defer func() { _ = g.Close() }()

	start := time.Now()
	got, err := g.Execute(context.Background(), "status")
	elapsed := time.Since(start)

	require.NoError(t, err,
		"an idle socket must be treated as end-of-response, not an error")
	assert.Equal(t, part, got, "the received part must be returned intact")
	assert.GreaterOrEqual(t, elapsed, responseIdleTimeout,
		"Execute must wait out the idle window for possible follow-up datagrams")
	assert.Less(t, elapsed, 3*time.Second,
		"the idle timeout, not the full configured timeout, must end the read loop")
}

func TestGoldSource_Open_TimesOutOnSilentServer(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) [][]byte {
		return nil // never answer the challenge request
	})

	const timeout = 300 * time.Millisecond

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  timeout,
	})
	require.NoError(t, err)

	start := time.Now()
	err = g.Open(context.Background())
	defer func() { _ = g.Close() }()

	require.Error(t, err, "a silent server must not hang Open forever")
	assert.Less(t, time.Since(start), timeout+time.Second,
		"Open must return around the configured Timeout, not block for seconds")
}

func TestGoldSource_Execute_CapsReassembledResponse(t *testing.T) {
	const challenge = "888"

	// Enough datagrams to exceed maxReassembledResponseSize with no idle gap.
	datagrams := make([][]byte, 0, maxReassembledResponseSize/4096+2)
	for range maxReassembledResponseSize/4096 + 2 {
		datagrams = append(datagrams, []byte(header+"\x00"+strings.Repeat("D", 4096)))
	}

	// The datagrams are paced: an unpaced burst overruns the kernel receive buffer (as small
	// as ~212 KiB on Linux) faster than the client can drain it, and the resulting packet
	// loss — not the client-side cap — would end the response.
	srv := newPacedScriptedUDPServer(t, 2*time.Millisecond, func(_ []byte, idx int) [][]byte {
		switch idx {
		case 0:
			return [][]byte{[]byte(header + "\x00challenge rcon " + challenge + "\n")}
		case 1:
			return datagrams
		}

		return nil
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	require.NoError(t, g.Open(context.Background()))
	defer func() { _ = g.Close() }()

	got, err := g.Execute(context.Background(), "cvarlist")
	require.NoError(t, err, "hitting the size cap must end the response normally, not error")
	assert.Greater(t, len(got), maxReassembledResponseSize,
		"the accumulated response must be returned once the cap is exceeded")
	assert.Less(t, len(got), maxReassembledResponseSize+maxResponseSize,
		"the response must stop growing right after the cap is exceeded")
}
