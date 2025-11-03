package query

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"github.com/rumblefrog/go-a2s"
)

func querySource(_ context.Context, host string, port int) (*Result, error) {
	address := fmt.Sprintf("%s:%d", host, port)

	client, err := a2s.NewClient(
		address,
		a2s.SetMaxPacketSize(defaultMaxPacketSize),
		a2s.TimeoutOption(defaultTimeout),
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to create a2s client")
	}

	defer func(client *a2s.Client) {
		err = client.Close()
		if err != nil {
			slog.Error(errors.Wrap(err, "failed to close a2s client").Error())
		}
	}(client)

	result := &Result{
		Online:    false,
		QueryTime: time.Now(),
	}

	info, err := client.QueryInfo()
	if err != nil {
		return result, errors.Wrap(err, "failed to query info")
	}

	result.Name = info.Name
	result.Online = true
	result.Map = info.Map
	result.PlayersNum = int(info.Players)
	result.MaxPlayersNum = int(info.MaxPlayers)

	players, err := client.QueryPlayer()
	if err != nil {
		return result, errors.Wrap(err, "failed to query players")
	}

	if players != nil {
		result.Players = make([]ResultPlayer, 0, len(players.Players))

		for _, player := range players.Players {
			result.Players = append(result.Players, ResultPlayer{
				Name:  player.Name,
				Score: int(player.Score),
			})
		}
	}

	return result, nil
}
