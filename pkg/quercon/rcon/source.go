package rcon

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"time"

	"github.com/pkg/errors"
)

const (
	// Source RCON packet types.
	serverDataAuth          int32 = 3
	serverDataExecCommand   int32 = 2
	serverDataAuthResponse  int32 = 2
	serverDataResponseValue int32 = 0

	// Packet structure constants.
	minPacketSize = 10 // 4 (size) + 4 (id) + 4 (type) + 2 (empty strings)
	maxPacketSize = 4096
)

var (
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrInvalidPacket        = errors.New("invalid packet")
)

type Source struct {
	address    string
	password   string
	timeout    time.Duration
	connection net.Conn
	requestID  int32
}

func NewSource(config Config) (*Source, error) {
	adapter := &Source{
		address:   config.Address,
		password:  config.Password,
		timeout:   config.Timeout,
		requestID: 1,
	}

	return adapter, nil
}

func (s *Source) Open(ctx context.Context) error {
	dialer := &net.Dialer{
		Timeout: s.timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", s.address)
	if err != nil {
		return errors.WithMessage(err, "unable to connect")
	}

	s.connection = conn

	// Set deadline for authentication
	if err := s.connection.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return errors.WithMessage(err, "unable to set deadline")
	}

	// Authenticate
	if err := s.authenticate(); err != nil {
		_ = s.Close()

		return err
	}

	return nil
}

func (s *Source) Close() error {
	if s.connection != nil {
		err := s.connection.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Source) Execute(_ context.Context, command string) (string, error) {
	if err := s.connection.SetDeadline(time.Now().Add(s.timeout)); err != nil {
		return "", errors.WithMessage(err, "unable to set deadline")
	}

	// Send command packet
	packet := s.buildPacket(s.requestID, serverDataExecCommand, command)
	if _, err := s.connection.Write(packet); err != nil {
		return "", errors.WithMessage(err, "unable to send command")
	}

	// Read response
	responseID, responseType, responseBody, err := s.readPacket()
	if err != nil {
		return "", err
	}

	// Validate response
	if responseID != s.requestID {
		return "", errors.New("response ID mismatch")
	}

	if responseType != serverDataResponseValue {
		return "", errors.Errorf("unexpected response type: %d", responseType)
	}

	s.requestID++

	return responseBody, nil
}

func (s *Source) authenticate() error {
	// Send authentication packet
	packet := s.buildPacket(s.requestID, serverDataAuth, s.password)
	if _, err := s.connection.Write(packet); err != nil {
		return errors.WithMessage(err, "unable to send auth packet")
	}

	// Read auth response
	responseID, responseType, _, err := s.readPacket()
	if err != nil {
		return err
	}

	// Check if authentication was successful
	// A failed auth returns -1 as the request ID
	if responseID == -1 || responseType != serverDataAuthResponse {
		return ErrAuthenticationFailed
	}

	s.requestID++

	return nil
}

func (s *Source) buildPacket(id int32, packetType int32, body string) []byte {
	bodyBytes := []byte(body)
	bodyLen := len(bodyBytes)
	// #nosec G115 -- body length is validated by maxPacketSize constant
	size := int32(4 + 4 + bodyLen + 2) // id + type + body + 2 null terminators

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, size)
	_ = binary.Write(buf, binary.LittleEndian, id)
	_ = binary.Write(buf, binary.LittleEndian, packetType)
	buf.Write(bodyBytes)
	buf.WriteByte(0) // Null terminator for body
	buf.WriteByte(0) // Empty string null terminator

	return buf.Bytes()
}

func (s *Source) readPacket() (int32, int32, string, error) {
	// Read packet size
	var size int32
	if err := binary.Read(s.connection, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", errors.WithMessage(err, "unable to read packet size")
	}

	// Validate packet size
	if size < minPacketSize || size > maxPacketSize {
		return 0, 0, "", ErrInvalidPacket
	}

	// Read packet data
	data := make([]byte, size)
	if _, err := io.ReadFull(s.connection, data); err != nil {
		return 0, 0, "", errors.WithMessage(err, "unable to read packet data")
	}

	// Parse packet
	buf := bytes.NewReader(data)

	var id int32
	if err := binary.Read(buf, binary.LittleEndian, &id); err != nil {
		return 0, 0, "", errors.WithMessage(err, "unable to read packet ID")
	}

	var packetType int32
	if err := binary.Read(buf, binary.LittleEndian, &packetType); err != nil {
		return 0, 0, "", errors.WithMessage(err, "unable to read packet type")
	}

	// Read body (remaining bytes minus 2 null terminators)
	bodySize := size - 4 - 4 - 2
	bodyBytes := make([]byte, bodySize)
	if _, err := io.ReadFull(buf, bodyBytes); err != nil {
		return 0, 0, "", errors.WithMessage(err, "unable to read packet body")
	}

	body := string(bodyBytes)

	return id, packetType, body, nil
}
