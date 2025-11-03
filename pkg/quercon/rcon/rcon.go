package rcon

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

var (
	ErrUnsupportedProtocol = errors.New("unsupported protocol")
)

type Protocol string

const (
	ProtocolSource  Protocol = "source"
	ProtocolGoldSrc Protocol = "goldsource"
)

type Config struct {
	Address  string
	Password string
	Protocol Protocol
	Timeout  time.Duration
}

type Player struct {
	ID    string
	Name  string
	Ping  string
	Score string
	Addr  string

	// Additional fields
	UniqID string
}

type Client interface {
	Open(ctx context.Context) error
	Close() error
	Execute(ctx context.Context, command string) (string, error)
}

func NewClient(config Config) (Client, error) {
	switch config.Protocol {
	case ProtocolGoldSrc:
		return NewGoldSource(config)
	case ProtocolSource:
		return NewSource(config)
	}

	return nil, ErrUnsupportedProtocol
}
