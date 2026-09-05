package query

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a2sTestChallenge is the challenge number the fake servers hand out.
var a2sTestChallenge = []byte{0x11, 0x22, 0x33, 0x44}

func appendA2SString(dst []byte, s string) []byte {
	dst = append(dst, []byte(s)...)

	return append(dst, 0x00)
}

// buildA2SInfoResponse builds a Source-style A2S_INFO reply ('I') without the EDF section.
func buildA2SInfoResponse(name, gameMap string, players, maxPlayers byte) []byte {
	response := []byte{0xFF, 0xFF, 0xFF, 0xFF, 'I', 0x11}
	response = appendA2SString(response, name)
	response = appendA2SString(response, gameMap)
	response = appendA2SString(response, "cstrike")
	response = appendA2SString(response, "Counter-Strike")
	response = append(response, 0x0A, 0x00) // appid 10 (LE)
	response = append(response, players, maxPlayers, 0x00)
	response = append(response, 'd', 'l', 0x00, 0x01)

	return appendA2SString(response, "1.0.0.0")
}

// buildGoldSourceInfoResponse builds the GoldSource-style A2S_INFO reply ('m') a ReUnion server sends for
// ServerInfoAnswerType 1, and first for ServerInfoAnswerType 2.
func buildGoldSourceInfoResponse(address, name, gameMap string, players, maxPlayers byte) []byte {
	response := []byte{0xFF, 0xFF, 0xFF, 0xFF, 'm'}
	response = appendA2SString(response, address)
	response = appendA2SString(response, name)
	response = appendA2SString(response, gameMap)
	response = appendA2SString(response, "cstrike")
	response = appendA2SString(response, "Counter-Strike")
	response = append(response, players, maxPlayers)
	response = append(response, 47, 'd', 'l', 0x00) // protocol, server type, os, password
	response = append(response, 0x00)               // no mod info

	return append(response, 0x01, 0x00) // secure, bots
}

// buildA2SPlayerResponse builds an A2S_PLAYER reply ('D') with the given players.
func buildA2SPlayerResponse(players []ResultPlayer) []byte {
	response := []byte{0xFF, 0xFF, 0xFF, 0xFF, 'D', byte(len(players))}

	for index, player := range players {
		response = append(response, byte(index))
		response = appendA2SString(response, player.Name)
		response = binary.LittleEndian.AppendUint32(response, uint32(player.Score)) // #nosec G115 - test data
		response = append(response, 0x00, 0x00, 0x80, 0x3F)                         // duration 1.0 (float32 LE)
	}

	return response
}

// buildA2SChallengeResponse builds the S2C_CHALLENGE reply ('A') carrying the challenge number.
func buildA2SChallengeResponse(challenge []byte) []byte {
	return append([]byte{0xFF, 0xFF, 0xFF, 0xFF, 'A'}, challenge...)
}

// splitA2SResponse cuts a reply into Source-style split packets: -2 header, packet id, total, number, size.
func splitA2SResponse(payload []byte, parts int, packetID uint32) [][]byte {
	chunk := (len(payload) + parts - 1) / parts
	packets := make([][]byte, 0, parts)

	for i := range parts {
		start := i * chunk
		end := min(start+chunk, len(payload))

		packet := []byte{0xFE, 0xFF, 0xFF, 0xFF}
		packet = binary.LittleEndian.AppendUint32(packet, packetID)
		packet = append(packet, byte(parts), byte(i))
		packet = binary.LittleEndian.AppendUint16(packet, uint16(end-start)) // #nosec G115 - test data
		packet = append(packet, payload[start:end]...)

		packets = append(packets, packet)
	}

	return packets
}

// a2sFakeServerHandler answers A2S_INFO with infoReplies as they are and runs the A2S_PLAYER challenge
// handshake: a request carrying the -1 placeholder gets a challenge, a request carrying the challenge gets
// playerReplies.
func a2sFakeServerHandler(infoReplies, playerReplies [][]byte) func(request []byte) [][]byte {
	return func(request []byte) [][]byte {
		if len(request) < 9 {
			return nil
		}

		switch request[4] {
		case 'T':
			return infoReplies
		case 'U':
			if bytes.Equal(request[5:9], a2sTestChallenge) {
				return playerReplies
			}

			return [][]byte{buildA2SChallengeResponse(a2sTestChallenge)}
		}

		return nil
	}
}

var a2sTestPlayers = []ResultPlayer{
	{Name: "Player One", Score: 5},
	{Name: "Player Two", Score: 3},
}

func assertA2STestPlayers(t *testing.T, result *Result) {
	t.Helper()

	require.Len(t, result.Players, 2)
	assert.Equal(t, "Player One", result.Players[0].Name)
	assert.Equal(t, 5, result.Players[0].Score)
	assert.Equal(t, "Player Two", result.Players[1].Name)
	assert.Equal(t, 3, result.Players[1].Score)
}

func TestQuerySource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("queries_server_over_udp", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — Source-style info reply and a players reply sent without a challenge handshake,
		// the way HLTV and some GoldSource servers answer.
		port := fakeUDPServer(t, func(request []byte) [][]byte {
			if len(request) < 5 {
				return nil
			}

			switch request[4] {
			case 'T':
				return [][]byte{buildA2SInfoResponse("Source Server", "de_dust2", 2, 32)}
			case 'U':
				return [][]byte{buildA2SPlayerResponse(a2sTestPlayers)}
			}

			return nil
		})

		// ACT
		result, err := querySource(ctx, "localhost", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online, "server answered with valid responses, result must be online")
		assert.Equal(t, "Source Server", result.Name)
		assert.Equal(t, "de_dust2", result.Map)
		assert.Equal(t, 2, result.PlayersNum)
		assert.Equal(t, 32, result.MaxPlayersNum)
		assertA2STestPlayers(t, result)
	})

	t.Run("reunion_goldsource_answer_type", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — ServerInfoAnswerType = 1: only the GoldSource-style 'm' reply is sent.
		port := fakeUDPServer(t, a2sFakeServerHandler(
			[][]byte{buildGoldSourceInfoResponse("127.0.0.1:27015", "ReUnion Old Style", "de_inferno", 7, 24)},
			[][]byte{buildA2SPlayerResponse(a2sTestPlayers)},
		))

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online, "GoldSource-style info reply must be accepted")
		assert.Equal(t, "ReUnion Old Style", result.Name)
		assert.Equal(t, "de_inferno", result.Map)
		assert.Equal(t, 7, result.PlayersNum)
		assert.Equal(t, 24, result.MaxPlayersNum)
		assertA2STestPlayers(t, result)
	})

	t.Run("reunion_hybrid_answer_type_with_bugfix_packet", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — ServerInfoAnswerType = 2 with FixBuggedQuery: 'm', an empty players reply and 'I'
		// all answer the single info request. The empty players reply must not become the players result.
		port := fakeUDPServer(t, a2sFakeServerHandler(
			[][]byte{
				buildGoldSourceInfoResponse("127.0.0.1:27015", "ReUnion Hybrid", "de_nuke", 9, 32),
				buildA2SPlayerResponse(nil),
				buildA2SInfoResponse("ReUnion Hybrid", "de_nuke", 9, 32),
			},
			[][]byte{buildA2SPlayerResponse(a2sTestPlayers)},
		))

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online)
		assert.Equal(t, "ReUnion Hybrid", result.Name)
		assert.Equal(t, "de_nuke", result.Map)
		assert.Equal(t, 9, result.PlayersNum)
		assert.Equal(t, 32, result.MaxPlayersNum)
		assertA2STestPlayers(t, result)
	})

	t.Run("reunion_hybrid_answer_type_two_packets", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — ServerInfoAnswerType = 2 without the bugfix packet: 'm' followed by 'I'.
		port := fakeUDPServer(t, a2sFakeServerHandler(
			[][]byte{
				buildGoldSourceInfoResponse("127.0.0.1:27015", "ReUnion Hybrid", "de_train", 1, 20),
				buildA2SInfoResponse("ReUnion Hybrid", "de_train", 1, 20),
			},
			[][]byte{buildA2SPlayerResponse(a2sTestPlayers)},
		))

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online)
		assert.Equal(t, "ReUnion Hybrid", result.Name)
		assert.Equal(t, "de_train", result.Map)
		assert.Equal(t, 1, result.PlayersNum)
		assert.Equal(t, 20, result.MaxPlayersNum)
		assertA2STestPlayers(t, result)
	})

	t.Run("info_challenge_handshake", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — the server demands a challenge for A2S_INFO and answers only the request that carries it.
		port := fakeUDPServer(t, func(request []byte) [][]byte {
			switch {
			case len(request) == 25 && request[4] == 'T':
				return [][]byte{buildA2SChallengeResponse(a2sTestChallenge)}
			case len(request) == 29 && request[4] == 'T' && bytes.Equal(request[25:], a2sTestChallenge):
				return [][]byte{buildA2SInfoResponse("Challenged", "cs_assault", 3, 16)}
			case len(request) == 9 && request[4] == 'U':
				return [][]byte{buildA2SPlayerResponse(a2sTestPlayers)}
			}

			return nil
		})

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online)
		assert.Equal(t, "Challenged", result.Name)
		assert.Equal(t, "cs_assault", result.Map)
		assert.Equal(t, 3, result.PlayersNum)
		assert.Equal(t, 16, result.MaxPlayersNum)
		assertA2STestPlayers(t, result)
	})

	t.Run("split_player_reply_out_of_order", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — the players reply arrives as two Source-style split fragments, last fragment first.
		fragments := splitA2SResponse(buildA2SPlayerResponse(a2sTestPlayers), 2, 0x1234)

		port := fakeUDPServer(t, a2sFakeServerHandler(
			[][]byte{buildA2SInfoResponse("Split", "de_dust", 2, 64)},
			[][]byte{fragments[1], fragments[0]},
		))

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online)
		assertA2STestPlayers(t, result)
	})

	t.Run("compressed_split_reply_is_rejected", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — the high bit of the packet id marks a bzip2-compressed reply.
		fragments := splitA2SResponse(buildA2SPlayerResponse(a2sTestPlayers), 2, 0x80001234)

		port := fakeUDPServer(t, a2sFakeServerHandler(
			[][]byte{buildA2SInfoResponse("Compressed", "de_dust", 2, 64)},
			fragments,
		))

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query players", "error message mismatch")
		assert.Contains(t, err.Error(), "compressed split response is not supported", "error message mismatch")
		assert.True(t, result.Online, "info query succeeded, result must stay online")
		assert.Equal(t, "Compressed", result.Name)
		assert.Empty(t, result.Players)
	})

	t.Run("unexpected_packet_types_are_skipped", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — a rules reply precedes the info reply.
		port := fakeUDPServer(t, a2sFakeServerHandler(
			[][]byte{
				{0xFF, 0xFF, 0xFF, 0xFF, 'E', 0x00, 0x00},
				buildA2SInfoResponse("After Rules", "de_aztec", 4, 12),
			},
			[][]byte{buildA2SPlayerResponse(a2sTestPlayers)},
		))

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.NoError(t, err)
		assert.True(t, result.Online)
		assert.Equal(t, "After Rules", result.Name)
		assert.Equal(t, "de_aztec", result.Map)
		assertA2STestPlayers(t, result)
	})

	t.Run("unknown_response_type", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{{0xFF, 0xFF, 0xFF, 0xFF, 'Z', 0x00}}
		})

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query info", "error message mismatch")
		assert.Contains(t, err.Error(), "unexpected response type 0x5a", "error message mismatch")
		assert.False(t, result.Online)
	})

	t.Run("repeated_challenge_is_rejected", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — the server answers every players request with a challenge.
		port := fakeUDPServer(t, func(request []byte) [][]byte {
			if len(request) >= 5 && request[4] == 'T' {
				return [][]byte{buildA2SInfoResponse("Challenge Loop", "de_dust2", 2, 32)}
			}

			return [][]byte{buildA2SChallengeResponse(a2sTestChallenge)}
		})

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query players", "error message mismatch")
		assert.Contains(t, err.Error(), "server rejected the challenge it handed out", "error message mismatch")
		assert.True(t, result.Online, "info query succeeded, result must stay online")
		assert.Empty(t, result.Players)
	})

	t.Run("truncated_info_reply", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — the reply ends in the middle of the server name.
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{buildA2SInfoResponse("Truncated Server", "de_dust2", 2, 32)[:12]}
		})

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query info", "error message mismatch")
		assert.Contains(t, err.Error(), "failed to parse response", "error message mismatch")
		assert.Contains(t, err.Error(), "truncated response", "error message mismatch")
		assert.False(t, result.Online)
		assert.Empty(t, result.Name)
	})

	t.Run("truncated_player_reply_keeps_info", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — the players reply announces two players but carries only one.
		truncated := buildA2SPlayerResponse(a2sTestPlayers[:1])
		truncated[5] = 2

		port := fakeUDPServer(t, a2sFakeServerHandler(
			[][]byte{buildA2SInfoResponse("Short List", "de_dust2", 2, 32)},
			[][]byte{truncated},
		))

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query players", "error message mismatch")
		assert.Contains(t, err.Error(), "truncated response", "error message mismatch")
		assert.True(t, result.Online, "info query succeeded, result must stay online")
		assert.Equal(t, "Short List", result.Name)
		assert.Equal(t, 2, result.PlayersNum)
		assert.Empty(t, result.Players)
	})

	t.Run("malformed_response", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := fakeUDPServer(t, func(_ []byte) [][]byte {
			return [][]byte{[]byte("garbage")}
		})

		// ACT
		result, err := querySource(ctx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query info", "error message mismatch")
		assert.Contains(t, err.Error(), "invalid response header", "error message mismatch")
		assert.False(t, result.Online)
	})

	t.Run("player_query_timeout_keeps_info", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — info query is answered, player query is not; without a context deadline the
		// read gives up after the package default timeout.
		port := fakeUDPServer(t, func(request []byte) [][]byte {
			if len(request) >= 5 && request[4] == 'T' {
				return [][]byte{buildA2SInfoResponse("Info Only", "cs_office", 1, 16)}
			}

			return nil
		})

		// ACT
		result, err := querySource(ctx, "localhost", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query players", "error message mismatch")
		assert.Contains(t, err.Error(), "failed to read query response", "error message mismatch")
		require.NotNil(t, result)
		assert.True(t, result.Online, "info query succeeded, result must stay online")
		assert.Equal(t, "Info Only", result.Name)
		assert.Equal(t, "cs_office", result.Map)
		assert.Equal(t, 1, result.PlayersNum)
		assert.Equal(t, 16, result.MaxPlayersNum)
		assert.Empty(t, result.Players)
	})

	t.Run("honors_context_deadline", func(t *testing.T) {
		t.Parallel()
		// ARRANGE — the player query is never answered; the context deadline, not the package default
		// timeout, must end the wait.
		port := fakeUDPServer(t, func(request []byte) [][]byte {
			if len(request) >= 5 && request[4] == 'T' {
				return [][]byte{buildA2SInfoResponse("Deadline", "cs_office", 1, 16)}
			}

			return nil
		})

		shortCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()

		// ACT
		start := time.Now()
		result, err := querySource(shortCtx, "127.0.0.1", port)
		elapsed := time.Since(start)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query players", "error message mismatch")
		assert.Less(t, elapsed, defaultTimeout, "context deadline must cut the wait short")
		assert.True(t, result.Online)
		assert.Equal(t, "Deadline", result.Name)
	})

	t.Run("read_timeout_when_nobody_answers", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		port := closedUDPPort(t)
		shortCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()

		// ACT
		result, err := querySource(shortCtx, "127.0.0.1", port)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query info", "error message mismatch")
		assert.Contains(t, err.Error(), "failed to read query response", "error message mismatch")
		require.NotNil(t, result)
		assert.False(t, result.Online)
		assert.False(t, result.QueryTime.IsZero(), "QueryTime is set before the info call and must be populated")
		assert.Empty(t, result.Name)
		assert.Empty(t, result.Players)
	})

	t.Run("cancelled_context_is_honored", func(t *testing.T) {
		t.Parallel()
		// ARRANGE
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		// ACT
		result, err := querySource(cancelledCtx, "127.0.0.1", 1)

		// ASSERT
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		assert.Contains(t, err.Error(), "failed to query info", "error message mismatch")
		assert.Contains(t, err.Error(), "failed to create UDP connection", "error message mismatch")
		require.NotNil(t, result)
		assert.False(t, result.Online)
	})

	t.Run("dial_error_on_invalid_port", func(t *testing.T) {
		t.Parallel()
		// ACT
		result, err := querySource(ctx, "127.0.0.1", -1)

		// ASSERT
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to query info", "error message mismatch")
		assert.Contains(t, err.Error(), "failed to create UDP connection", "error message mismatch")
		require.NotNil(t, result)
		assert.False(t, result.Online)
	})
}
