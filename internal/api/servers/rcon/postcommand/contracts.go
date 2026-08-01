package postcommand

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/quercon/rcon"
)

// rconClientFactory builds the RCON client the handler talks to. It exists so tests can
// drive the handler without a live game server; production always uses quercon.Resolver.RconClient.
type rconClientFactory func(ctx context.Context, game domain.Game, config rcon.Config) (rcon.Client, error)
