//go:build wasip1

package main

import (
	"context"
	"log/slog"
	"sync/atomic"

	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/gamemods"
	"github.com/gameap/gameap/pkg/plugin/sdk/games"
	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
)

func main() {}

var (
	logger       *slog.Logger
	gamesRepo    games.GamesService
	gameModRepo  gamemods.GameModsService
	serversRepo  servers.ServersService
	eventCounter atomic.Uint64
)

func init() {
	logger = log.NewLogger()
	gamesRepo = games.NewGamesService()
	gameModRepo = gamemods.NewGameModsService()
	serversRepo = servers.NewServersService()
	pluginproto.RegisterPluginService(&ServerLoggerPlugin{})
}

type ServerLoggerPlugin struct{}

func (p *ServerLoggerPlugin) GetInfo(
	_ context.Context,
	_ *pluginproto.GetInfoRequest,
) (*pluginproto.PluginInfo, error) {
	return &pluginproto.PluginInfo{
		Id:          "fwgfo26jzwnm4",
		Name:        "Server Logger",
		Version:     "1.0.0",
		Description: "Logs server lifecycle events",
		Author:      "GameAP",
		ApiVersion:  "1",
	}, nil
}

func (p *ServerLoggerPlugin) Initialize(
	_ context.Context,
	_ *pluginproto.InitializeRequest,
) (*pluginproto.InitializeResponse, error) {
	return &pluginproto.InitializeResponse{
		Result: &pluginproto.Result{Success: true},
	}, nil
}

func (p *ServerLoggerPlugin) Shutdown(
	_ context.Context,
	_ *pluginproto.ShutdownRequest,
) (*pluginproto.ShutdownResponse, error) {
	return &pluginproto.ShutdownResponse{
		Result: &pluginproto.Result{Success: true},
	}, nil
}

func (p *ServerLoggerPlugin) GetServerAbilities(
	_ context.Context,
	_ *pluginproto.GetServerAbilitiesRequest,
) (*pluginproto.GetServerAbilitiesResponse, error) {
	return &pluginproto.GetServerAbilitiesResponse{
		Abilities: []*pluginproto.ServerAbility{
			{
				Name:  "view-logs",
				Title: "plugins.fwgfo26jzwnm4.abilities.view-logs",
			},
			{
				Name:  "export-logs",
				Title: "plugins.fwgfo26jzwnm4.abilities.export-logs",
			},
		},
	}, nil
}
