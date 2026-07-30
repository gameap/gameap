package rcon

import (
	"bytes"
	"context"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
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
	drainStaleDatagrams(g.connection)

	cmd := header + "rcon " + g.challengeNumber + " \"" + g.password + "\" " + command

	if _, err := g.connection.Write([]byte(cmd)); err != nil {
		return "", err
	}

	return readReassembledResponse(g.connection, g.timeout, goldSourceDatagramBody)
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

	datagram, err := readDatagram(g.connection, g.timeout)
	if err != nil {
		return nil, err
	}

	return goldSourceDatagramBody(datagram), nil
}

// goldSourceDatagramBody strips the 5-byte GoldSource prefix (4-byte header plus the 'l' print
// type) and trailing NULs. Datagrams that do not carry the GoldSource header, or carry no body
// at all, yield nil.
func goldSourceDatagramBody(datagram []byte) []byte {
	if !bytes.HasPrefix(datagram, []byte(header)) || len(datagram) < 5 {
		return nil
	}

	return bytes.Trim(datagram[5:], "\x00")
}
