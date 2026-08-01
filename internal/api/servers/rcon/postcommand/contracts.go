package postcommand

import "github.com/gameap/gameap/pkg/quercon/rcon"

// rconClientFactory builds the RCON client the handler talks to. It exists so tests can
// drive the handler without a live game server; production always uses rcon.NewClient.
type rconClientFactory func(config rcon.Config) (rcon.Client, error)
