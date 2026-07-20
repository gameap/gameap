package rcon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoldSource_Open_AcceptsValidChallengeReply(t *testing.T) {
	srv := newScriptedUDPServer(t, func(req []byte, _ int) []byte {
		assert.Equal(t, header+"challenge rcon", string(req),
			"first datagram must be the GoldSource challenge request")

		return []byte(header + "\x00challenge rcon 1234567\n")
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
			srv := newScriptedUDPServer(t, func(_ []byte, _ int) []byte {
				return tt.reply
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
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) []byte {
		// 5 bytes total — writeAndReadSocket returns nil (n < 5 after the 5 leading bytes are consumed
		// is the documented "no body" case). Splitting "" by " " yields [""] (one part) → invalid challenge.
		return []byte("\xff\xff\xff\xff")
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

	srv := newScriptedUDPServer(t, func(req []byte, idx int) []byte {
		switch idx {
		case 0:
			return []byte(header + "\x00challenge rcon " + challenge + "\n")
		case 1:
			expectedPrefix := header + "rcon " + challenge + " \"" + password + "\" status"
			assert.Equal(t, expectedPrefix, string(req),
				"command datagram must include challenge, quoted password and command")

			return []byte(header + "\x00short response\n")
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
}

func TestGoldSource_Execute_MultiPartLoopTerminatesOnShortReply(t *testing.T) {
	const challenge = "999"
	longBody := strings.Repeat("A", maxSymbolsPerCommand+50) // forces a follow-up request

	srv := newScriptedUDPServer(t, func(req []byte, idx int) []byte {
		switch idx {
		case 0:
			return []byte(header + "\x00challenge rcon " + challenge + "\n")
		case 1:
			assert.Contains(t, string(req), "status",
				"first command datagram must carry the user-supplied command")

			return []byte(header + "\x00" + longBody)
		case 2:
			// Continuation request must omit the command but keep challenge + password.
			assert.NotContains(t, string(req), "status",
				"continuation datagram must drop the original command")
			assert.Contains(t, string(req), "rcon "+challenge,
				"continuation datagram must keep the challenge")

			return []byte(header + "\x00tail\n")
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

	assert.True(t, strings.HasPrefix(got, longBody),
		"aggregate response must begin with the first part returned by the server")
	assert.True(t, strings.HasSuffix(got, "tail"),
		"aggregate response must end with the final short reply")
}

func TestGoldSource_Execute_PropagatesWriteErrorWhenSocketClosed(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) []byte {
		return []byte(header + "\x00challenge rcon 5\n")
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

	srv := newScriptedUDPServer(t, func(_ []byte, idx int) []byte {
		switch idx {
		case 0:
			return []byte(header + "\x00challenge rcon " + challenge + "\n")
		case 1:
			return []byte(header + "\x00" + longBody)
		case 2:
			return []byte(header + "\x00" + tail)
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
	// First part exceeds maxSymbolsPerCommand so a continuation is fetched, and it ends with a
	// space that lands exactly on the datagram boundary. The old per-chunk TrimSpace ate it.
	firstPart := strings.Repeat("a", maxSymbolsPerCommand) + " foo "
	const secondPart = "bar"

	srv := newScriptedUDPServer(t, func(_ []byte, idx int) []byte {
		switch idx {
		case 0:
			return []byte(header + "\x00challenge rcon " + challenge + "\n")
		case 1:
			return []byte(header + "\x00" + firstPart)
		case 2:
			return []byte(header + "\x00" + secondPart)
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
	assert.Contains(t, got, "foo bar",
		"whitespace on a datagram boundary must survive reassembly")
}

func TestGoldSource_Execute_ContinuationTimeoutTerminatesLoop(t *testing.T) {
	const challenge = "321"
	firstPart := strings.Repeat("Z", maxSymbolsPerCommand+10) // forces a continuation read

	srv := newScriptedUDPServer(t, func(_ []byte, idx int) []byte {
		switch idx {
		case 0:
			return []byte(header + "\x00challenge rcon " + challenge + "\n")
		case 1:
			return []byte(header + "\x00" + firstPart)
		}
		// The continuation request (idx 2) gets no reply: the loop must time out and return what
		// it already has instead of erroring or blocking forever.
		return nil
	})

	g, err := NewGoldSource(Config{
		Address:  srv.addr,
		Password: "pw",
		Protocol: ProtocolGoldSrc,
		Timeout:  300 * time.Millisecond,
	})
	require.NoError(t, err)
	require.NoError(t, g.Open(context.Background()))
	defer func() { _ = g.Close() }()

	got, err := g.Execute(context.Background(), "status")
	require.NoError(t, err,
		"a continuation-read timeout must be treated as end-of-response, not an error")
	assert.Equal(t, firstPart, got, "the already-received part must be returned intact")
}

func TestGoldSource_Open_TimesOutOnSilentServer(t *testing.T) {
	srv := newScriptedUDPServer(t, func(_ []byte, _ int) []byte {
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
