package rcon

import (
	"bytes"
	"context"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	header = "\xff\xff\xff\xff"
	// maxResponseSize is the read buffer for a single UDP datagram. GoldSource can send RCON
	// replies up to the network MTU, so this is sized to the maximum UDP payload to avoid
	// truncating (and silently dropping) the tail of a datagram.
	maxResponseSize = 65535
	// responseIdleTimeout bounds the wait for follow-up datagrams of a multi-packet response.
	// GoldSource sends every part of a large RCON reply back-to-back right after the command,
	// so a short idle gap means the response is complete.
	responseIdleTimeout = 500 * time.Millisecond
)

var (
	ErrInvalidChallengeResponse = errors.New("invalid challenge response")
)

type GoldSource struct {
	address         string
	password        string
	timeout         time.Duration
	connection      net.Conn
	challengeNumber string
}

func NewGoldSource(config Config) (*GoldSource, error) {
	adapter := &GoldSource{
		address:  config.Address,
		password: config.Password,
		timeout:  config.Timeout,
	}

	return adapter, nil
}

func (g *GoldSource) Open(ctx context.Context) error {
	dialer := &net.Dialer{
		Timeout: g.timeout,
	}

	conn, err := dialer.DialContext(ctx, "udp", g.address)
	if err != nil {
		return errors.WithMessage(err, "unable to connect")
	}

	g.connection = conn

	if err := g.getChallengeNumber(); err != nil {
		_ = conn.Close()
		g.connection = nil

		return err
	}

	return nil
}

func (g *GoldSource) Close() error {
	if g.connection != nil {
		err := g.connection.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (g *GoldSource) Execute(_ context.Context, command string) (string, error) {
	// Drop late or unsolicited datagrams left over from previous commands so the first
	// read below belongs to this command's response (matters for reused pooled connections).
	g.drainStaleDatagrams()

	cmd := header + "rcon " + g.challengeNumber + " \"" + g.password + "\" " + command

	if _, err := g.connection.Write([]byte(cmd)); err != nil {
		return "", err
	}

	buffer := bytes.Buffer{}

	// The first datagram of the response gets the full configured timeout.
	part, err := g.readDatagram(g.timeout)
	if err != nil {
		return "", err
	}

	buffer.Write(part)

	// A large response arrives as several back-to-back datagrams; read until the socket
	// goes idle instead of polling the server with extra "continuation" rcon requests.
	for {
		part, err = g.readDatagram(responseIdleTimeout)
		if err != nil {
			if isTimeoutError(err) {
				break
			}

			return "", err
		}

		buffer.Write(part)
	}

	return strings.TrimSpace(buffer.String()), nil
}

func (g *GoldSource) getChallengeNumber() error {
	response, err := g.writeAndReadSocket(header + "challenge rcon")
	if err != nil {
		return err
	}

	parts := strings.Fields(string(response))
	if len(parts) < 3 {
		return ErrInvalidChallengeResponse
	}

	g.challengeNumber = parts[2]

	return nil
}

func (g *GoldSource) writeAndReadSocket(command string) ([]byte, error) {
	if _, err := g.connection.Write([]byte(command)); err != nil {
		return nil, err
	}

	return g.readDatagram(g.timeout)
}

// readDatagram reads a single UDP datagram and returns its body with the 5-byte GoldSource
// prefix (4-byte header plus the 'l' print type) and trailing NULs stripped. Datagrams that
// do not carry the GoldSource header, or carry no body at all, yield nil.
func (g *GoldSource) readDatagram(timeout time.Duration) ([]byte, error) {
	if timeout > 0 {
		_ = g.connection.SetReadDeadline(time.Now().Add(timeout))
	}

	buffer := make([]byte, maxResponseSize)

	n, err := g.connection.Read(buffer)
	if err != nil {
		return nil, err
	}

	if !bytes.HasPrefix(buffer[:n], []byte(header)) || n < 5 {
		return nil, nil //nolint:nilnil // "no usable body" is not an error here
	}

	return bytes.Trim(buffer[5:n], "\x00"), nil
}

// drainStaleDatagrams discards datagrams already sitting in the socket receive buffer
// (late replies to previous commands) so the next read starts from a clean slate.
func (g *GoldSource) drainStaleDatagrams() {
	buffer := make([]byte, maxResponseSize)

	for {
		_ = g.connection.SetReadDeadline(time.Now())

		if _, err := g.connection.Read(buffer); err != nil {
			return
		}
	}
}

func isTimeoutError(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr) && netErr.Timeout()
}
