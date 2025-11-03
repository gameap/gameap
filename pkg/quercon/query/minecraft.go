package query

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/text/encoding/charmap"
)

const (
	minecraftChallengePacket = "\xFE\xFD\x09\x10\x20\x30\x40"
	minecraftQueryPacketFmt  = "\xFE\xFD\x00\x10\x20\x30\x40%s\x00\x00\x00\x00"
	minecraftMaxPacketSize   = 4096
)

func queryMinecraft(ctx context.Context, host string, port int) (*Result, error) {
	result := &Result{
		Online:    false,
		QueryTime: time.Now(),
	}

	address := fmt.Sprintf("%s:%d", host, port)

	// Create UDP connection
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return result, errors.Wrap(err, "failed to create UDP connection")
	}
	defer func() {
		_ = conn.Close()
	}()

	// Set deadline
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(defaultTimeout)
	}
	err = conn.SetDeadline(deadline)
	if err != nil {
		return result, errors.Wrap(err, "failed to set deadline")
	}

	// Send challenge packet
	_, err = conn.Write([]byte(minecraftChallengePacket))
	if err != nil {
		return result, errors.Wrap(err, "failed to send challenge packet")
	}

	// Receive challenge response
	challengeResponse := make([]byte, minecraftMaxPacketSize)
	n, err := conn.Read(challengeResponse)
	if err != nil {
		return result, errors.Wrap(err, "failed to read challenge response")
	}
	challengeResponse = challengeResponse[:n]

	// Parse challenge
	challenge, err := parseMinecraftChallenge(challengeResponse)
	if err != nil {
		return result, errors.Wrap(err, "failed to parse challenge")
	}

	// Send query packet
	queryPacket := fmt.Sprintf(minecraftQueryPacketFmt, string(challenge))
	_, err = conn.Write([]byte(queryPacket))
	if err != nil {
		return result, errors.Wrap(err, "failed to send query packet")
	}

	// Receive query response
	buffer := make([]byte, minecraftMaxPacketSize)
	n, err = conn.Read(buffer)
	if err != nil {
		return result, errors.Wrap(err, "failed to read query response")
	}

	packet := buffer[:n]

	// Validate packet
	if len(packet) < 16 {
		return result, errors.New("query response too short")
	}

	// Check response type (should be 0x00)
	if packet[0] != 0x00 {
		return result, errors.Errorf("invalid response type: expected 0x00, got 0x%02x", packet[0])
	}

	// Verify session ID matches (bytes 1-4 should match our request)
	// Skip session ID validation for now and just extract the data

	// Skip response header: type (1 byte) + session ID (4 bytes) + padding (11 bytes) = 16 bytes
	responseData := packet[16:]

	if len(responseData) == 0 {
		return result, errors.New("no data in query response")
	}

	// Parse response
	err = parseMinecraftResponse(responseData, result)
	if err != nil {
		return result, errors.Wrap(err, "failed to parse response")
	}

	result.Online = true

	return result, nil
}

// parseMinecraftChallenge extracts the challenge from the response and encodes it.
func parseMinecraftChallenge(response []byte) ([]byte, error) {
	if len(response) < 5 {
		return nil, errors.New("challenge response too short")
	}

	// Skip header (5 bytes) and get challenge string
	challengeStr := string(response[5:])
	challengeStr = strings.TrimRight(challengeStr, "\x00")

	// Parse challenge number
	challengeNum, err := strconv.ParseInt(challengeStr, 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse challenge number")
	}

	// Encode challenge as 4-byte binary (big-endian)
	challenge := make([]byte, 4)
	// #nosec G115 - challengeNum is from server response, overflow is acceptable
	binary.BigEndian.PutUint32(challenge, uint32(challengeNum))

	return challenge, nil
}

// parseMinecraftResponse parses the server response.
func parseMinecraftResponse(data []byte, result *Result) error {
	// Split response into sections: server info and players/teams
	sections := bytes.Split(data, []byte{0x00, 0x00, 0x01})

	if len(sections) == 0 {
		return errors.New("invalid response format")
	}

	// Parse server details (first section)
	parseMinecraftServerDetails(sections[0], result)

	// Parse players and teams (second section if exists)
	if len(sections) > 1 && len(sections[1]) > 0 {
		// Don't fail if player parsing fails, some servers may not return player info
		parseMinecraftPlayers(sections[1], result)
	}

	return nil
}

// parseMinecraftServerDetails parses server information.
func parseMinecraftServerDetails(data []byte, result *Result) {
	reader := bytes.NewReader(data)
	decoder := charmap.ISO8859_1.NewDecoder()

	for {
		// Read key
		key, err := readNullTerminatedString(reader)
		if err != nil || key == "" {
			break
		}

		// Read value
		value, err := readNullTerminatedString(reader)
		if err != nil {
			break
		}

		// Convert to UTF-8
		valueBytes, err := decoder.Bytes([]byte(value))
		valueUTF8 := value // fallback to original
		if err == nil {
			valueUTF8 = string(valueBytes)
		}

		// Map common fields
		switch key {
		case "hostname":
			result.Name = valueUTF8
		case "mapname", "map":
			result.Map = valueUTF8
		case "numplayers":
			result.PlayersNum, _ = strconv.Atoi(valueUTF8)
		case "maxplayers":
			result.MaxPlayersNum, _ = strconv.Atoi(valueUTF8)
		}
	}
}

// parseMinecraftPlayers parses player information.
func parseMinecraftPlayers(data []byte, result *Result) {
	// Player data format: player_\x00name1\x00name2\x00...\x00\x00
	// Split by double null to separate from next section
	reader := bytes.NewReader(data)
	decoder := charmap.ISO8859_1.NewDecoder()

	var playerNames []string

	// Read until we hit double null or EOF
	for {
		name, err := readNullTerminatedString(reader)
		if err != nil || name == "" {
			break
		}

		// Skip header strings like "player_" or "score_"
		if strings.HasSuffix(name, "_") {
			continue
		}

		// Convert to UTF-8
		nameBytes, err := decoder.Bytes([]byte(name))
		nameUTF8 := name // fallback to original
		if err == nil {
			nameUTF8 = string(nameBytes)
		}

		playerNames = append(playerNames, nameUTF8)
	}

	if len(playerNames) == 0 {
		return
	}

	// Initialize players
	result.Players = make([]ResultPlayer, len(playerNames))
	for i, name := range playerNames {
		result.Players[i].Name = name
		result.Players[i].Score = 0 // Minecraft doesn't always provide scores in query
	}
}

// readNullTerminatedString reads a null-terminated string from a reader.
func readNullTerminatedString(reader *bytes.Reader) (string, error) {
	var result []byte

	for {
		b, err := reader.ReadByte()
		if err != nil {
			return string(result), err
		}
		if b == 0x00 {
			break
		}
		result = append(result, b)
	}

	return string(result), nil
}
