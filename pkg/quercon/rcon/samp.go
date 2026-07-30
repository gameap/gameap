package rcon

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"math"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	// sampHeaderSize is "SAMP" (4) + IPv4 (4) + port (2, little-endian) + opcode (1).
	sampHeaderSize = 11
	// sampRconOpcode carries a password and a console command.
	sampRconOpcode = 'x'
	// sampPingOpcode echoes its four-byte payload back, which is how a SA-MP client checks
	// that a server is answering at all.
	sampPingOpcode      = 'p'
	sampPingPayloadSize = 4
	// sampRefusedMessage is what open.mp answers on a rejected password. Legacy SA-MP
	// servers stay silent instead, so this is a best-effort signal.
	sampRefusedMessage = "Invalid RCON password."
)

var (
	// ErrSAMPRequiresIPv4 is returned when the server address does not resolve to IPv4. The
	// protocol embeds the four address octets in every packet, so IPv6 cannot be expressed.
	ErrSAMPRequiresIPv4 = errors.New("samp rcon requires an IPv4 server address")

	// ErrSAMPPingMismatch is returned when the server's ping reply does not echo our payload.
	ErrSAMPPingMismatch = errors.New("samp server did not echo the ping payload")

	// ErrSAMPFieldTooLong is returned when a password or command exceeds the 16-bit length
	// prefix the protocol uses.
	ErrSAMPFieldTooLong = errors.New("samp rcon field exceeds the maximum length")
)

// SAMP implements RCON for SA-MP and open.mp servers. The remote console shares the UDP query
// port and reuses the query framing: every packet starts with the eleven-byte header, and the
// server answers with one datagram per console output line.
//
// Most SA-MP console commands print nothing at all, and `players` prints nothing on an empty
// server, so silence after a command is a normal empty response rather than an error. Open
// therefore performs a ping handshake, which is what establishes that the server is reachable.
type SAMP struct {
	address      string
	password     string
	timeout      time.Duration
	connection   net.Conn
	packetHeader []byte
}

func NewSAMP(config Config) (*SAMP, error) {
	adapter := &SAMP{
		address:  config.Address,
		password: config.Password,
		timeout:  config.Timeout,
	}

	return adapter, nil
}

func (s *SAMP) Open(ctx context.Context) error {
	dialer := &net.Dialer{
		Timeout: s.timeout,
	}

	// "udp4" resolves and forces IPv4 in one step, so the octets below are the ones the
	// server itself sees.
	conn, err := dialer.DialContext(ctx, "udp4", s.address)
	if err != nil {
		return errors.WithMessage(err, "unable to connect")
	}

	s.connection = conn

	if err := s.buildPacketHeader(); err != nil {
		_ = conn.Close()
		s.connection = nil

		return err
	}

	if err := s.ping(); err != nil {
		_ = conn.Close()
		s.connection = nil

		return err
	}

	return nil
}

func (s *SAMP) Close() error {
	if s.connection != nil {
		err := s.connection.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *SAMP) Execute(_ context.Context, command string) (string, error) {
	drainStaleDatagrams(s.connection)

	packet, err := s.buildRconPacket(command)
	if err != nil {
		return "", err
	}

	if _, err := s.connection.Write(packet); err != nil {
		return "", err
	}

	response, err := readReassembledResponse(s.connection, s.timeout, s.datagramBody)
	if err != nil {
		// A command that produces no console output gets no datagrams at all. The ping
		// handshake in Open already proved the server answers, so silence is an empty result.
		if isTimeoutError(err) {
			return "", nil
		}

		return "", err
	}

	if strings.HasPrefix(response, sampRefusedMessage) {
		return "", errors.WithMessage(ErrAuthenticationFailed, sampRefusedMessage)
	}

	return response, nil
}

func (s *SAMP) buildPacketHeader() error {
	udpAddr, ok := s.connection.RemoteAddr().(*net.UDPAddr)
	if !ok {
		return errors.WithMessage(ErrSAMPRequiresIPv4, s.address)
	}

	ipv4 := udpAddr.IP.To4()
	if ipv4 == nil {
		return errors.WithMessage(ErrSAMPRequiresIPv4, udpAddr.IP.String())
	}

	header := make([]byte, sampHeaderSize)
	copy(header[0:4], "SAMP")
	copy(header[4:8], ipv4)
	// #nosec G115 - a resolved UDP port is always within the uint16 range
	binary.LittleEndian.PutUint16(header[8:10], uint16(udpAddr.Port))
	header[10] = sampRconOpcode

	s.packetHeader = header

	return nil
}

// ping exchanges the four-byte echo packet every SA-MP client sends before talking to a server.
// It is the only reachability check available: the RCON opcode itself cannot distinguish a
// silent server from a command with no output.
func (s *SAMP) ping() error {
	payload := make([]byte, sampPingPayloadSize)
	if _, err := rand.Read(payload); err != nil {
		return errors.Wrap(err, "failed to generate samp ping payload")
	}

	packet := make([]byte, 0, sampHeaderSize+sampPingPayloadSize)
	packet = append(packet, s.packetHeader...)
	packet[len(packet)-1] = sampPingOpcode
	packet = append(packet, payload...)

	if _, err := s.connection.Write(packet); err != nil {
		return err
	}

	response, err := readDatagram(s.connection, s.timeout)
	if err != nil {
		return errors.WithMessage(err, "samp ping failed")
	}

	if len(response) != sampHeaderSize+sampPingPayloadSize ||
		!bytes.Equal(response[sampHeaderSize:], payload) {
		return ErrSAMPPingMismatch
	}

	return nil
}

func (s *SAMP) buildRconPacket(command string) ([]byte, error) {
	if len(s.password) > math.MaxUint16 || len(command) > math.MaxUint16 {
		return nil, ErrSAMPFieldTooLong
	}

	packet := make([]byte, 0, sampHeaderSize+4+len(s.password)+len(command))
	packet = append(packet, s.packetHeader...)
	// #nosec G115 - length is bounds-checked above
	packet = binary.LittleEndian.AppendUint16(packet, uint16(len(s.password)))
	packet = append(packet, s.password...)
	// #nosec G115 - length is bounds-checked above
	packet = binary.LittleEndian.AppendUint16(packet, uint16(len(command)))
	packet = append(packet, command...)

	return packet, nil
}

// datagramBody unwraps one console line: the server echoes our eleven-byte header back and
// follows it with a 16-bit length and the text. A newline is appended because each datagram is
// a separate console line, unlike the byte-continuation chunks the id Tech protocols send.
func (s *SAMP) datagramBody(datagram []byte) []byte {
	const lengthSize = 2

	if len(datagram) < sampHeaderSize+lengthSize {
		return nil
	}

	if !bytes.Equal(datagram[:sampHeaderSize], s.packetHeader) {
		return nil
	}

	length := int(binary.LittleEndian.Uint16(datagram[sampHeaderSize : sampHeaderSize+lengthSize]))

	body := datagram[sampHeaderSize+lengthSize:]
	if length > len(body) {
		return nil
	}

	return append(body[:length:length], '\n')
}
