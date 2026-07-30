package query

import (
	"context"
	"time"
)

const (
	defaultTimeout       = 1 * time.Second
	defaultMaxPacketSize = 14000
)

type Protocol string

const (
	ProtocolSource    Protocol = "source"
	ProtocolMinecraft Protocol = "minecraft"
	ProtocolGameSpy2  Protocol = "gamespy2"
	ProtocolGameSpy3  Protocol = "gamespy3"
	ProtocolQuake2    Protocol = "quake2"
	ProtocolQuake3    Protocol = "quake3"
	ProtocolSAMP      Protocol = "samp"
	ProtocolRakNet    Protocol = "raknet"
)

type Result struct {
	QueryTime     time.Time      `json:"query_time"`
	Online        bool           `json:"online"`
	Name          string         `json:"name,omitempty"`
	Map           string         `json:"map,omitempty"`
	PlayersNum    int            `json:"players_num,omitempty"`
	MaxPlayersNum int            `json:"max_players_num,omitempty"`
	Players       []ResultPlayer `json:"players,omitempty"`
}

type ResultPlayer struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

var queryProtocolFuncsMap = map[Protocol]func(ctx context.Context, host string, port int) (*Result, error){
	"source":    querySource,
	"minecraft": queryMinecraft,
	"gamespy2":  queryGameSpy2,
	"gamespy3":  queryGameSpy3,
	"quake2":    queryQuake2,
	"quake3":    queryQuake3,
	"samp":      querySAMP,
	"raknet":    queryRakNet,
}

// IsProtocolSupported reports whether the package implements the named query protocol. It reads
// the same registry Query dispatches on, so a caller validating a protocol name up front cannot
// disagree with what Query would actually do.
func IsProtocolSupported(protocol Protocol) bool {
	_, ok := queryProtocolFuncsMap[protocol]

	return ok
}

func Query(ctx context.Context, host string, port int, protocol Protocol) (*Result, error) {
	queryFunc, ok := queryProtocolFuncsMap[protocol]
	if !ok {
		return nil, NewUnsupportedQueryProtocolError(protocol)
	}

	return queryFunc(ctx, host, port)
}
