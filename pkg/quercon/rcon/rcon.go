package rcon

import (
	"context"
	"time"

	"github.com/gameap/gameap/pkg/quercon/rcon/players"
	"github.com/pkg/errors"
)

var (
	ErrUnsupportedProtocol = errors.New("unsupported protocol")
)

type Protocol string

const (
	ProtocolSource   Protocol = "source"
	ProtocolGoldSrc  Protocol = "goldsource"
	ProtocolQuake2   Protocol = "quake2"
	ProtocolQuake3   Protocol = "quake3"
	ProtocolSAMP     Protocol = "samp"
	ProtocolBattlEye Protocol = "battleye"
)

// clientFactories is the single registry of supported protocols. Both NewClient and
// IsProtocolSupported read it, so a protocol cannot be runnable but unadvertised (or the
// reverse) — the two used to be independent switch statements that had to be kept in sync.
// The closures exist because the concrete constructors return concrete types.
var clientFactories = map[Protocol]func(Config) (Client, error){
	ProtocolSource:   func(c Config) (Client, error) { return NewSource(c) },
	ProtocolGoldSrc:  func(c Config) (Client, error) { return NewGoldSource(c) },
	ProtocolQuake2:   func(c Config) (Client, error) { return NewQuake2(c) },
	ProtocolQuake3:   func(c Config) (Client, error) { return NewQuake3(c) },
	ProtocolSAMP:     func(c Config) (Client, error) { return NewSAMP(c) },
	ProtocolBattlEye: func(c Config) (Client, error) { return NewBattlEye(c) },
}

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
	factory, ok := clientFactories[config.Protocol]
	if !ok {
		return nil, ErrUnsupportedProtocol
	}

	return factory(config)
}

func IsProtocolSupported(protocol Protocol) bool {
	_, ok := clientFactories[protocol]

	return ok
}

func IsPlayerManagementSupported(gameCode string) bool {
	return players.IsPlayerManagementSupported(gameCode)
}
