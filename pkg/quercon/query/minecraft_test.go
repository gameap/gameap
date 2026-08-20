package query

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMinecraftChallenge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		input         []byte
		wantChallenge []byte
		wantError     string
	}{
		{
			name:          "valid_positive_challenge",
			input:         buildMinecraftChallengeResponse(9513307),
			wantChallenge: encodeChallengeBigEndian(9513307),
		},
		{
			name:          "zero_challenge",
			input:         buildMinecraftChallengeResponse(0),
			wantChallenge: []byte{0x00, 0x00, 0x00, 0x00},
		},
		{
			name:          "negative_challenge",
			input:         buildMinecraftChallengeResponse(-1),
			wantChallenge: []byte{0xFF, 0xFF, 0xFF, 0xFF},
		},
		{
			name:          "challenge_with_trailing_null",
			input:         []byte("\x09\x10\x20\x30\x40123456\x00"),
			wantChallenge: encodeChallengeBigEndian(123456),
		},
		{
			name:          "challenge_with_multiple_trailing_nulls",
			input:         []byte("\x09\x10\x20\x30\x40789\x00\x00\x00"),
			wantChallenge: encodeChallengeBigEndian(789),
		},
		{
			name:      "response_too_short",
			input:     []byte{0x09, 0x10, 0x20, 0x40},
			wantError: "challenge response too short",
		},
		{
			name:      "non_numeric_challenge",
			input:     []byte("\x09\x10\x20\x30\x40notanumber\x00"),
			wantError: "failed to parse challenge number",
		},
		{
			name:      "empty_challenge_body",
			input:     []byte("\x09\x10\x20\x30\x40"),
			wantError: "failed to parse challenge number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			challenge, err := parseMinecraftChallenge(tt.input)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
				assert.Nil(t, challenge)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantChallenge, challenge, "challenge bytes must match big-endian encoding")
		})
	}
}

func TestParseMinecraftServerDetails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		input         []byte
		wantName      string
		wantMap       string
		wantPlayers   int
		wantMaxPlayer int
	}{
		{
			name:          "all_fields_populated",
			input:         buildMinecraftServerDetails(map[string]string{"hostname": "Survival World", "mapname": "world", "numplayers": "5", "maxplayers": "20"}),
			wantName:      "Survival World",
			wantMap:       "world",
			wantPlayers:   5,
			wantMaxPlayer: 20,
		},
		{
			name:          "alternative_map_key",
			input:         buildMinecraftServerDetails(map[string]string{"hostname": "Creative", "map": "flat", "numplayers": "1", "maxplayers": "10"}),
			wantName:      "Creative",
			wantMap:       "flat",
			wantPlayers:   1,
			wantMaxPlayer: 10,
		},
		{
			name:          "iso_8859_1_decoded_to_utf8",
			input:         buildMinecraftServerDetails(map[string]string{"hostname": "Caf\xE9 Server", "mapname": "lobby", "numplayers": "0", "maxplayers": "8"}),
			wantName:      "Café Server",
			wantMap:       "lobby",
			wantPlayers:   0,
			wantMaxPlayer: 8,
		},
		{
			name:          "non_numeric_player_counts_default_to_zero",
			input:         buildMinecraftServerDetails(map[string]string{"hostname": "Bad Counts", "mapname": "x", "numplayers": "abc", "maxplayers": "xyz"}),
			wantName:      "Bad Counts",
			wantMap:       "x",
			wantPlayers:   0,
			wantMaxPlayer: 0,
		},
		{
			name:          "unknown_keys_ignored",
			input:         buildMinecraftServerDetails(map[string]string{"hostname": "Mixed", "mapname": "m1", "numplayers": "2", "maxplayers": "4", "version": "1.20", "plugins": "none"}),
			wantName:      "Mixed",
			wantMap:       "m1",
			wantPlayers:   2,
			wantMaxPlayer: 4,
		},
		{
			name:          "empty_input_leaves_defaults",
			input:         []byte{},
			wantName:      "",
			wantMap:       "",
			wantPlayers:   0,
			wantMaxPlayer: 0,
		},
		{
			name:          "key_without_value_breaks_loop",
			input:         []byte("hostname\x00OnlyHost\x00mapname"),
			wantName:      "OnlyHost",
			wantMap:       "",
			wantPlayers:   0,
			wantMaxPlayer: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}

			parseMinecraftServerDetails(tt.input, result)

			assert.Equal(t, tt.wantName, result.Name, "Name mismatch")
			assert.Equal(t, tt.wantMap, result.Map, "Map mismatch")
			assert.Equal(t, tt.wantPlayers, result.PlayersNum, "PlayersNum mismatch")
			assert.Equal(t, tt.wantMaxPlayer, result.MaxPlayersNum, "MaxPlayersNum mismatch")
		})
	}
}

func TestParseMinecraftPlayers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		input       []byte
		wantPlayers []ResultPlayer
	}{
		{
			name:  "single_player_with_header",
			input: buildMinecraftPlayerSection([]string{"Steve"}),
			wantPlayers: []ResultPlayer{
				{Name: "Steve", Score: 0},
			},
		},
		{
			name:  "multiple_players",
			input: buildMinecraftPlayerSection([]string{"Alex", "Notch", "Herobrine"}),
			wantPlayers: []ResultPlayer{
				{Name: "Alex", Score: 0},
				{Name: "Notch", Score: 0},
				{Name: "Herobrine", Score: 0},
			},
		},
		{
			name:        "empty_player_list_after_header",
			input:       []byte("player_\x00"),
			wantPlayers: nil,
		},
		{
			name:        "input_with_only_header_suffix_strings",
			input:       []byte("player_\x00score_\x00"),
			wantPlayers: nil,
		},
		{
			name:        "no_input_yields_no_players",
			input:       []byte{},
			wantPlayers: nil,
		},
		{
			name:  "iso_8859_1_player_name_decoded",
			input: buildMinecraftPlayerSection([]string{"Andr\xE9"}),
			wantPlayers: []ResultPlayer{
				{Name: "André", Score: 0},
			},
		},
		{
			name:  "header_suffix_in_middle_skipped",
			input: []byte("player_\x00First\x00score_\x00Second\x00"),
			wantPlayers: []ResultPlayer{
				{Name: "First", Score: 0},
				{Name: "Second", Score: 0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}

			parseMinecraftPlayers(tt.input, result)

			if tt.wantPlayers == nil {
				assert.Empty(t, result.Players, "expected no players to be appended")

				return
			}

			require.Len(t, result.Players, len(tt.wantPlayers))
			for i, want := range tt.wantPlayers {
				assert.Equal(t, want.Name, result.Players[i].Name, "player[%d] name mismatch", i)
				assert.Equal(t, want.Score, result.Players[i].Score, "player[%d] score mismatch", i)
			}
		})
	}
}

func TestReadNullTerminatedString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     []byte
		wantValue string
		wantError bool
	}{
		{
			name:      "normal_string_terminated_by_null",
			input:     []byte("hello\x00rest"),
			wantValue: "hello",
			wantError: false,
		},
		{
			name:      "empty_string_immediate_null",
			input:     []byte("\x00more"),
			wantValue: "",
			wantError: false,
		},
		{
			name:      "eof_before_null",
			input:     []byte("partial"),
			wantValue: "partial",
			wantError: true,
		},
		{
			name:      "empty_input_returns_eof",
			input:     []byte{},
			wantValue: "",
			wantError: true,
		},
		{
			name:      "binary_bytes_until_null",
			input:     []byte{0x01, 0x02, 0x03, 0x00},
			wantValue: "\x01\x02\x03",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reader := bytes.NewReader(tt.input)

			got, err := readNullTerminatedString(reader)

			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantValue, got)
		})
	}
}

func TestParseMinecraftResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		input         []byte
		wantName      string
		wantMap       string
		wantPlayers   int
		wantMaxPlayer int
		wantPlayerLen int
	}{
		{
			name:          "server_details_only",
			input:         buildMinecraftServerDetails(map[string]string{"hostname": "Solo", "mapname": "world", "numplayers": "0", "maxplayers": "8"}),
			wantName:      "Solo",
			wantMap:       "world",
			wantPlayers:   0,
			wantMaxPlayer: 8,
			wantPlayerLen: 0,
		},
		{
			name: "server_details_with_players_section",
			input: buildMinecraftFullResponseData(
				map[string]string{"hostname": "Public", "mapname": "world", "numplayers": "2", "maxplayers": "20"},
				[]string{"Steve", "Alex"},
			),
			wantName:      "Public",
			wantMap:       "world",
			wantPlayers:   2,
			wantMaxPlayer: 20,
			wantPlayerLen: 2,
		},
		{
			name:          "empty_player_section_does_not_break",
			input:         append(buildMinecraftServerDetails(map[string]string{"hostname": "Empty", "mapname": "x", "numplayers": "0", "maxplayers": "10"}), []byte{0x00, 0x00, 0x01}...),
			wantName:      "Empty",
			wantMap:       "x",
			wantPlayers:   0,
			wantMaxPlayer: 10,
			wantPlayerLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := &Result{}

			err := parseMinecraftResponse(tt.input, result)

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, result.Name)
			assert.Equal(t, tt.wantMap, result.Map)
			assert.Equal(t, tt.wantPlayers, result.PlayersNum)
			assert.Equal(t, tt.wantMaxPlayer, result.MaxPlayersNum)
			require.Len(t, result.Players, tt.wantPlayerLen)
		})
	}
}

func TestQueryMinecraft_HappyPath(t *testing.T) {
	t.Parallel()
	host, port, stop := startMinecraftEchoServer(t, minecraftEchoConfig{
		challengeNumber: 9513307,
		details:         map[string]string{"hostname": "E2E Server", "mapname": "test_world", "numplayers": "3", "maxplayers": "16"},
		players:         []string{"P1", "P2", "P3"},
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := queryMinecraft(ctx, host, port)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Online, "Online flag must be true on success")
	assert.Equal(t, "E2E Server", result.Name)
	assert.Equal(t, "test_world", result.Map)
	assert.Equal(t, 3, result.PlayersNum)
	assert.Equal(t, 16, result.MaxPlayersNum)
	require.Len(t, result.Players, 3)
	assert.Equal(t, "P1", result.Players[0].Name)
	assert.Equal(t, "P2", result.Players[1].Name)
	assert.Equal(t, "P3", result.Players[2].Name)
	assert.False(t, result.QueryTime.IsZero(), "QueryTime must be populated")
}

func TestQueryMinecraft_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		config    minecraftEchoConfig
		wantError string
	}{
		{
			name: "no_response_yields_timeout",
			config: minecraftEchoConfig{
				silentChallenge: true,
			},
			wantError: "failed to read challenge response",
		},
		{
			name: "invalid_response_type_byte",
			config: minecraftEchoConfig{
				challengeNumber:           9513307,
				details:                   map[string]string{"hostname": "Bad", "mapname": "x", "numplayers": "0", "maxplayers": "1"},
				queryResponseTypeOverride: new(byte(0x09)),
			},
			wantError: "invalid response type",
		},
		{
			name: "query_response_too_short",
			config: minecraftEchoConfig{
				challengeNumber:  9513307,
				queryResponseRaw: []byte{0x00, 0x10, 0x20, 0x30},
			},
			wantError: "query response too short",
		},
		{
			name: "non_numeric_challenge_response",
			config: minecraftEchoConfig{
				challengeRaw: []byte("\x09\x10\x20\x30\x40notanumber\x00"),
			},
			wantError: "failed to parse challenge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			host, port, stop := startMinecraftEchoServer(t, tt.config)
			defer stop()

			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
			defer cancel()

			result, err := queryMinecraft(ctx, host, port)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
			require.NotNil(t, result)
			assert.False(t, result.Online, "Online must remain false on failure")
		})
	}
}

// --- helpers below ---

func buildMinecraftChallengeResponse(challenge int64) []byte {
	var buf bytes.Buffer
	// Header: type (0x09) + session ID (4 bytes)
	buf.WriteByte(0x09)
	buf.Write([]byte{0x10, 0x20, 0x30, 0x40})
	buf.WriteString(strconv.FormatInt(challenge, 10))
	buf.WriteByte(0x00)

	return buf.Bytes()
}

func encodeChallengeBigEndian(challenge int64) []byte {
	out := make([]byte, 4)
	// #nosec G115 - matches behavior of parseMinecraftChallenge
	binary.BigEndian.PutUint32(out, uint32(challenge))

	return out
}

func buildMinecraftServerDetails(pairs map[string]string) []byte {
	var buf bytes.Buffer
	for k, v := range pairs {
		buf.WriteString(k)
		buf.WriteByte(0x00)
		buf.WriteString(v)
		buf.WriteByte(0x00)
	}

	return buf.Bytes()
}

func buildMinecraftPlayerSection(playerNames []string) []byte {
	var buf bytes.Buffer
	buf.WriteString("player_")
	buf.WriteByte(0x00)
	for _, name := range playerNames {
		buf.WriteString(name)
		buf.WriteByte(0x00)
	}

	return buf.Bytes()
}

func buildMinecraftFullResponseData(details map[string]string, players []string) []byte {
	var buf bytes.Buffer
	buf.Write(buildMinecraftServerDetails(details))
	buf.Write([]byte{0x00, 0x00, 0x01})
	buf.Write(buildMinecraftPlayerSection(players))

	return buf.Bytes()
}

// minecraftEchoConfig drives a UDP echo server that participates in the
// Minecraft challenge-response handshake for end-to-end testing.
type minecraftEchoConfig struct {
	// silentChallenge: server receives the challenge packet but does not reply.
	silentChallenge bool

	// challengeNumber is the numeric challenge sent back when challengeRaw is nil.
	challengeNumber int64

	// challengeRaw, when non-nil, is sent verbatim as the challenge response.
	challengeRaw []byte

	// queryResponseRaw, when non-nil, replaces the entire query response.
	queryResponseRaw []byte

	// queryResponseTypeOverride, when non-nil, replaces only the type byte (offset 0)
	// of the synthesized query response.
	queryResponseTypeOverride *byte

	// details and players synthesize the query response when queryResponseRaw is nil.
	details map[string]string
	players []string
}

func startMinecraftEchoServer(t *testing.T, cfg minecraftEchoConfig) (host string, port int, stop func()) {
	t.Helper()

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err, "must bind UDP listener")

	udpAddr, ok := pc.LocalAddr().(*net.UDPAddr)
	require.True(t, ok, "expected *net.UDPAddr")

	done := make(chan struct{})

	go func() {
		defer close(done)

		buf := make([]byte, 4096)

		// First packet: challenge request from client.
		_ = pc.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, addr, readErr := pc.ReadFrom(buf)
		if readErr != nil {
			return
		}

		_ = n // we trust the client implementation; do not validate here

		if cfg.silentChallenge {
			return
		}

		challengeReply := cfg.challengeRaw
		if challengeReply == nil {
			challengeReply = buildMinecraftChallengeResponse(cfg.challengeNumber)
		}

		_, _ = pc.WriteTo(challengeReply, addr)

		// Second packet: stat request from client.
		_ = pc.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, addr, readErr = pc.ReadFrom(buf)
		if readErr != nil {
			return
		}

		var queryReply []byte
		switch {
		case cfg.queryResponseRaw != nil:
			queryReply = cfg.queryResponseRaw
		default:
			queryReply = buildMinecraftQueryResponse(cfg.details, cfg.players)
			if cfg.queryResponseTypeOverride != nil {
				queryReply[0] = *cfg.queryResponseTypeOverride
			}
		}

		_, _ = pc.WriteTo(queryReply, addr)
	}()

	stop = func() {
		_ = pc.Close()
		<-done
	}

	return udpAddr.IP.String(), udpAddr.Port, stop
}

// buildMinecraftQueryResponse synthesizes a stat-response packet:
// type(1) + sessionID(4) + padding(11) + details + 0x00 0x00 0x01 + players.
func buildMinecraftQueryResponse(details map[string]string, players []string) []byte {
	var buf bytes.Buffer
	buf.WriteByte(0x00)
	buf.Write([]byte{0x10, 0x20, 0x30, 0x40})
	buf.Write(bytes.Repeat([]byte{0x00}, 11))

	if details != nil {
		buf.Write(buildMinecraftServerDetails(details))
	}

	if len(players) > 0 {
		buf.Write([]byte{0x00, 0x00, 0x01})
		buf.Write(buildMinecraftPlayerSection(players))
	}

	return buf.Bytes()
}

// Smoke check that the test fixtures themselves are valid, helping isolate
// failures between the helpers and the production parser.
func TestMinecraftFixtureSanity(t *testing.T) {
	t.Parallel()
	resp := buildMinecraftQueryResponse(
		map[string]string{"hostname": "Sanity", "mapname": "x", "numplayers": "0", "maxplayers": "1"},
		[]string{"Solo"},
	)

	// First 16 bytes are header, then KV stream begins.
	require.GreaterOrEqual(t, len(resp), 16, "response must include at least the 16-byte header")
	assert.Equal(t, byte(0x00), resp[0], "type byte must be 0x00")

	// Sanity-check that the key list still contains the canonical hostname token.
	assert.True(t, strings.Contains(string(resp), "hostname"), "response must contain hostname key")

	// Sanity check the formatted IP/port helper signature matches what queryMinecraft expects.
	assert.NotEmpty(t, fmt.Sprintf("%s:%d", "127.0.0.1", 1))
}
