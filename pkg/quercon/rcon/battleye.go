package rcon

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	// battlEyeHeaderSize covers 'B', 'E', the CRC32 and the 0xFF terminator that precedes
	// the payload.
	battlEyeHeaderSize = 7

	battlEyePacketLogin         = 0x00
	battlEyePacketCommand       = 0x01
	battlEyePacketServerMessage = 0x02

	battlEyeLoginSucceeded = 0x01
)

var (
	// ErrInvalidBattlEyePacket marks a datagram that is not a well-formed BattlEye packet.
	// It is never returned to the caller: the read loops skip such datagrams, because a UDP
	// socket can receive unrelated traffic.
	ErrInvalidBattlEyePacket = errors.New("invalid battleye packet")

	// ErrBattlEyeIncompleteResponse is returned when the parts of a multi-packet reply do not
	// all arrive within the timeout.
	ErrBattlEyeIncompleteResponse = errors.New("battleye response is incomplete")
)

// BattlEye implements the BattlEye RCon protocol used by Arma 2, Arma 3, DayZ, SCUM and
// Arma Reforger. Unlike the id Tech protocols it is a real session: the client logs in, then
// numbers its commands, and the server can push unsolicited messages at any time.
//
// The protocol also requires an empty command packet at least every 45 seconds to keep a login
// alive. No keepalive is implemented here on purpose — the panel opens a connection, runs one
// command and closes it within a couple of seconds per request. Anyone reusing a client across
// requests (for example by wiring pool.go) must add one, and must also stop relying on
// drainStaleDatagrams, which discards unacknowledged server messages.
type BattlEye struct {
	address    string
	password   string
	timeout    time.Duration
	connection net.Conn
	sequence   byte
}

func NewBattlEye(config Config) (*BattlEye, error) {
	adapter := &BattlEye{
		address:  config.Address,
		password: config.Password,
		timeout:  config.Timeout,
	}

	return adapter, nil
}

func (b *BattlEye) Open(ctx context.Context) error {
	dialer := &net.Dialer{
		Timeout: b.timeout,
	}

	conn, err := dialer.DialContext(ctx, "udp", b.address)
	if err != nil {
		return errors.WithMessage(err, "unable to connect")
	}

	b.connection = conn

	if err := b.login(); err != nil {
		_ = conn.Close()
		b.connection = nil

		return err
	}

	return nil
}

func (b *BattlEye) Close() error {
	if b.connection != nil {
		err := b.connection.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *BattlEye) Execute(_ context.Context, command string) (string, error) {
	drainStaleDatagrams(b.connection)

	sequence := b.sequence
	b.sequence++

	payload := make([]byte, 0, 2+len(command))
	payload = append(payload, battlEyePacketCommand, sequence)
	payload = append(payload, command...)

	if _, err := b.connection.Write(buildBattlEyePacket(payload)); err != nil {
		return "", err
	}

	return b.readCommandResponse(sequence)
}

// login sends the password and waits for the acknowledgement. The server may already be pushing
// messages, and a UDP socket can pick up unrelated traffic, so anything that is not a login
// answer is skipped rather than treated as a protocol error.
func (b *BattlEye) login() error {
	payload := make([]byte, 0, 1+len(b.password))
	payload = append(payload, battlEyePacketLogin)
	payload = append(payload, b.password...)

	if _, err := b.connection.Write(buildBattlEyePacket(payload)); err != nil {
		return err
	}

	deadline := time.Now().Add(b.timeout)

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errors.WithMessage(ErrBattlEyeIncompleteResponse, "no login answer")
		}

		datagram, err := readDatagram(b.connection, remaining)
		if err != nil {
			return errors.WithMessage(err, "battleye login failed, rcon may be disabled")
		}

		answer, parseErr := parseBattlEyePacket(datagram)
		if parseErr != nil || answer[0] != battlEyePacketLogin || len(answer) < 2 {
			continue
		}

		if answer[1] != battlEyeLoginSucceeded {
			return ErrAuthenticationFailed
		}

		return nil
	}
}

func (b *BattlEye) readCommandResponse(sequence byte) (string, error) {
	deadline := time.Now().Add(b.timeout)

	var reassembly battlEyeReassembly

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return "", ErrBattlEyeIncompleteResponse
		}

		datagram, err := readDatagram(b.connection, remaining)
		if err != nil {
			return "", err
		}

		payload, parseErr := parseBattlEyePacket(datagram)
		if parseErr != nil {
			continue
		}

		switch payload[0] {
		case battlEyePacketServerMessage:
			b.acknowledgeServerMessage(payload)
		case battlEyePacketCommand:
			if response, done := collectCommandPart(sequence, payload, &reassembly); done {
				return response, nil
			}
		}
	}
}

// acknowledgeServerMessage replies to a pushed message. The server deauthenticates a client that
// leaves them unacknowledged, and they arrive while a command response is still being awaited.
func (b *BattlEye) acknowledgeServerMessage(payload []byte) {
	if len(payload) < 2 {
		return
	}

	_, _ = b.connection.Write(buildBattlEyePacket([]byte{battlEyePacketServerMessage, payload[1]}))
}

// collectCommandPart folds one command answer into the response, reporting whether the response
// is now complete. Answers carrying another command's sequence number are stale and dropped.
func collectCommandPart(sequence byte, payload []byte, reassembly *battlEyeReassembly) (string, bool) {
	if len(payload) < 2 || payload[1] != sequence {
		return "", false
	}

	body := payload[2:]

	total, index, chunk, multipart := battlEyeMultipart(body)
	if !multipart {
		return strings.TrimSpace(string(body)), true
	}

	if reassembly.add(total, index, chunk) {
		return reassembly.join(), true
	}

	return "", false
}

// battlEyeMultipart recognises the sub-header a server puts in front of each part of a split
// response. The protocol gives no explicit flag, so this is a heuristic: a response body is
// ASCII console text and never starts with NUL, and a split always announces at least two parts
// with an index inside that range.
func battlEyeMultipart(body []byte) (total, index byte, chunk []byte, ok bool) {
	if len(body) < 3 || body[0] != 0x00 {
		return 0, 0, nil, false
	}

	if body[1] < 2 || body[2] >= body[1] {
		return 0, 0, nil, false
	}

	return body[1], body[2], body[3:], true
}

// battlEyeReassembly collects the parts of a split response. Parts may arrive out of order, so
// they are keyed by index and joined once all of them are present.
type battlEyeReassembly struct {
	parts map[byte][]byte
	total byte
	size  int
}

func (r *battlEyeReassembly) add(total, index byte, chunk []byte) bool {
	if r.parts == nil {
		r.parts = make(map[byte][]byte, total)
		r.total = total
	}

	if _, seen := r.parts[index]; !seen {
		r.parts[index] = chunk
		r.size += len(chunk)
	}

	return len(r.parts) >= int(r.total) || r.size > maxReassembledResponseSize
}

func (r *battlEyeReassembly) join() string {
	buffer := bytes.Buffer{}

	for index := range int(r.total) {
		// #nosec G115 - index is bounded by total, which is a byte
		buffer.Write(r.parts[byte(index)])
	}

	return strings.TrimSpace(buffer.String())
}

// buildBattlEyePacket wraps a payload: 'B', 'E', the little-endian CRC32 of everything that
// follows the checksum field, then the 0xFF terminator and the payload itself.
func buildBattlEyePacket(payload []byte) []byte {
	body := make([]byte, 0, 1+len(payload))
	body = append(body, 0xFF)
	body = append(body, payload...)

	packet := make([]byte, 0, battlEyeHeaderSize+len(payload))
	packet = append(packet, 'B', 'E')
	packet = binary.LittleEndian.AppendUint32(packet, crc32.ChecksumIEEE(body))
	packet = append(packet, body...)

	return packet
}

func parseBattlEyePacket(datagram []byte) ([]byte, error) {
	if len(datagram) <= battlEyeHeaderSize {
		return nil, errors.WithMessage(ErrInvalidBattlEyePacket, "datagram is too short")
	}

	if datagram[0] != 'B' || datagram[1] != 'E' {
		return nil, errors.WithMessage(ErrInvalidBattlEyePacket, "unexpected magic bytes")
	}

	if datagram[battlEyeHeaderSize-1] != 0xFF {
		return nil, errors.WithMessage(ErrInvalidBattlEyePacket, "missing payload terminator")
	}

	body := datagram[battlEyeHeaderSize-1:]
	if crc32.ChecksumIEEE(body) != binary.LittleEndian.Uint32(datagram[2:6]) {
		return nil, errors.WithMessage(ErrInvalidBattlEyePacket, "checksum mismatch")
	}

	return datagram[battlEyeHeaderSize:], nil
}
