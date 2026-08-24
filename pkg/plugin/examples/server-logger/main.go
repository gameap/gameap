//go:build wasip1

package main

import (
	"context"
	"log/slog"
	"sync/atomic"

	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk"
	"github.com/gameap/gameap/pkg/plugin/sdk/gamemods"
	"github.com/gameap/gameap/pkg/plugin/sdk/games"
	"github.com/gameap/gameap/pkg/plugin/sdk/host"
	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
	"github.com/gameap/gameap/pkg/plugin/sdk/servers"
)

func main() {}

var (
	logger       *slog.Logger
	gamesRepo    games.GamesService
	gameModRepo  gamemods.GameModsService
	serversRepo  servers.ServersService
	schedulerSvc scheduler.SchedulerService
	hostSvc      host.HostService
	eventCounter atomic.Uint64
)

func init() {
	logger = log.NewLogger()
	gamesRepo = games.NewGamesService()
	gameModRepo = gamemods.NewGameModsService()
	serversRepo = servers.NewServersService()
	schedulerSvc = scheduler.NewSchedulerService()
	hostSvc = host.NewHostService()
	pluginproto.RegisterPluginService(&ServerLoggerPlugin{})
	scheduler.RegisterScheduledTaskHandler(&scheduledTaskHandler{})
	// Registration alone adds the optional archive_events_handler_* exports;
	// the panel delivers events only for operations this plugin starts.
	nodefs.RegisterArchiveEventsHandler(&archiveEventsHandler{})
}

type ServerLoggerPlugin struct {
	sdk.EmptyPluginService
}

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
		// Event subscriptions are gated on listen_events; the install grants
		// exactly what is declared here.
		RequiredPermissions: []string{"listen_events"},
	}, nil
}

func (p *ServerLoggerPlugin) Initialize(
	ctx context.Context,
	_ *pluginproto.InitializeRequest,
) (*pluginproto.InitializeResponse, error) {
	registerStatsReportTask(ctx)
	reportStartup(ctx)

	return &pluginproto.InitializeResponse{
		Result: &pluginproto.Result{Success: true},
	}, nil
}

// reportStartup shows what gameap-host offers: the grants the operator gave
// this plugin and the host modules it may call.
func reportStartup(ctx context.Context) {
	grants, err := hostSvc.GetGrants(ctx, &host.GetGrantsRequest{})
	if err != nil {
		logger.Warn("Cannot read grants", slog.String("error", err.Error()))
	} else {
		logger.Info("Plugin grants", slog.Any("permissions", grants.GetPermissions()))
	}

	info, err := hostSvc.GetHostInfo(ctx, &host.GetHostInfoRequest{})
	if err != nil {
		logger.Warn("Cannot read host info", slog.String("error", err.Error()))
	} else {
		logger.Info("Host info",
			slog.String("panel_version", info.GetPanelVersion()),
			slog.Int("plugin_api_version", int(info.GetPluginApiVersion())),
			slog.String("instance_id", info.GetInstanceId()),
			slog.Int("modules", len(info.GetModules())),
		)
	}

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
