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
}

func Query(ctx context.Context, host string, port int, protocol Protocol) (*Result, error) {
	queryFunc, ok := queryProtocolFuncsMap[protocol]
	if !ok {
		return nil, NewUnsupportedQueryProtocolError(protocol)
	}

	return queryFunc(ctx, host, port)
}
