package rcon

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	header               = "\xff\xff\xff\xff"
	defaultBufferSize    = 1024
	maxSymbolsPerCommand = 256
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
	firstCommand := true

	buffer := bytes.Buffer{}
	buffer.Grow(defaultBufferSize)

	for {
		var cmd string
		if firstCommand {
			cmd = header + "rcon " + g.challengeNumber + " \"" + g.password + "\" " + command
		} else {
			cmd = header + "rcon " + g.challengeNumber + " \"" + g.password + "\""
		}

		cmdPartResult, err := g.writeAndReadSocket(cmd)
		if err != nil {
			return "", err
		}

		buffer.Write(cmdPartResult)

		if len(cmdPartResult) < maxSymbolsPerCommand {
			break
		}

		firstCommand = false
	}

	return strings.TrimSpace(buffer.String()), nil
}

func (g *GoldSource) getChallengeNumber() error {
	response, err := g.writeAndReadSocket(header + "challenge rcon")
	if err != nil {
		return err
	}

	parts := strings.Split(string(response), " ")
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

	reader := bufio.NewReader(g.connection)
	buffer := make([]byte, defaultBufferSize)

	n, err := reader.Read(buffer)
	if err != nil {
		return nil, err
	}

	if n < 5 {
		return nil, nil
	}

	return bytes.TrimSpace(
		bytes.Trim(buffer[5:n], "\x00"),
	), nil
}
