//go:build wasip1

package main

import (
	"context"
	"log/slog"

	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/plugin/sdk/gamemods"
	"github.com/gameap/gameap/pkg/plugin/sdk/games"
)

func (p *ServerLoggerPlugin) GetSubscribedEvents(
	_ context.Context,
	_ *pluginproto.GetSubscribedEventsRequest,
) (*pluginproto.GetSubscribedEventsResponse, error) {
	return &pluginproto.GetSubscribedEventsResponse{
		Events: []pluginproto.EventType{
			pluginproto.EventType_EVENT_TYPE_SERVER_PRE_START,
			pluginproto.EventType_EVENT_TYPE_SERVER_POST_START,
			pluginproto.EventType_EVENT_TYPE_SERVER_PRE_STOP,
			pluginproto.EventType_EVENT_TYPE_SERVER_POST_STOP,
			pluginproto.EventType_EVENT_TYPE_SERVER_PRE_RESTART,
			pluginproto.EventType_EVENT_TYPE_SERVER_POST_RESTART,
			pluginproto.EventType_EVENT_TYPE_SERVER_PRE_INSTALL,
			pluginproto.EventType_EVENT_TYPE_SERVER_POST_INSTALL,
			pluginproto.EventType_EVENT_TYPE_SERVER_PRE_UPDATE,
			pluginproto.EventType_EVENT_TYPE_SERVER_POST_UPDATE,
			pluginproto.EventType_EVENT_TYPE_SERVER_PRE_REINSTALL,
			pluginproto.EventType_EVENT_TYPE_SERVER_POST_REINSTALL,
			pluginproto.EventType_EVENT_TYPE_SERVER_PRE_DELETE,
			pluginproto.EventType_EVENT_TYPE_SERVER_POST_DELETE,
			// Events a panel older than these values never raises; an older
			// panel ignores the unknown subscriptions.
			pluginproto.EventType_EVENT_TYPE_SERVER_SETTINGS_CHANGED,
			pluginproto.EventType_EVENT_TYPE_USER_CREATED,
			pluginproto.EventType_EVENT_TYPE_USER_DELETED,
			pluginproto.EventType_EVENT_TYPE_NODE_ONLINE,
			pluginproto.EventType_EVENT_TYPE_NODE_OFFLINE,
			pluginproto.EventType_EVENT_TYPE_PLUGIN_LOADED,
			pluginproto.EventType_EVENT_TYPE_PLUGIN_ERROR,
		},
	}, nil
}

func (p *ServerLoggerPlugin) HandleEvent(
	ctx context.Context,
	event *pluginproto.Event,
) (*pluginproto.EventResult, error) {
	serverEvent := event.GetServerEvent()
	if serverEvent == nil || serverEvent.Server == nil {
		return handleOtherEvent(event), nil
	}

	server := serverEvent.Server
	eventName := eventTypeName(event.Type)

	var gameName, gameEngine string
	gameResp, err := gamesRepo.GetGame(ctx, &games.GetGameRequest{Code: server.GameId})
	if err != nil {
		logger.Warn("Cannot get game info", slog.String("error", err.Error()))
	} else if gameResp.Found && gameResp.Game != nil {
		gameName = gameResp.Game.Name
		gameEngine = gameResp.Game.Engine
	} else {
		logger.Warn("Game not found", slog.String("game_id", server.GameId))
	}

	var gameModName string
	gameModResp, err := gameModRepo.GetGameMod(ctx, &gamemods.GetGameModRequest{Id: server.GameModId})
	if err != nil {
		logger.Warn("Cannot get game mod info", slog.String("error", err.Error()))
	} else if gameModResp.Found && gameModResp.GameMod != nil {
		gameModName = gameModResp.GameMod.Name
	} else {
		logger.Warn("Game mod not found", slog.Uint64("game_mod_id", server.GameModId))
	}

	logger.Info("Server event",
		slog.String("event_type", eventName),
		slog.Uint64("server_id", server.Id),
		slog.String("server_name", server.Name),
		slog.String("server_ip", server.ServerIp),
		slog.Int("server_port", int(server.ServerPort)),
		slog.String("game", gameName),
		slog.String("game_engine", gameEngine),
		slog.String("game_mod", gameModName),
	)

	eventCounter.Add(1)

	return &pluginproto.EventResult{Handled: true}, nil
}

func eventTypeName(eventType pluginproto.EventType) string {
	switch eventType {
	case pluginproto.EventType_EVENT_TYPE_SERVER_PRE_START:
		return "SERVER_PRE_START"
	case pluginproto.EventType_EVENT_TYPE_SERVER_POST_START:
		return "SERVER_POST_START"
	case pluginproto.EventType_EVENT_TYPE_SERVER_PRE_STOP:
		return "SERVER_PRE_STOP"
	case pluginproto.EventType_EVENT_TYPE_SERVER_POST_STOP:
		return "SERVER_POST_STOP"
	case pluginproto.EventType_EVENT_TYPE_SERVER_PRE_RESTART:
		return "SERVER_PRE_RESTART"
	case pluginproto.EventType_EVENT_TYPE_SERVER_POST_RESTART:
		return "SERVER_POST_RESTART"
	case pluginproto.EventType_EVENT_TYPE_SERVER_PRE_INSTALL:
		return "SERVER_PRE_INSTALL"
	case pluginproto.EventType_EVENT_TYPE_SERVER_POST_INSTALL:
		return "SERVER_POST_INSTALL"
	case pluginproto.EventType_EVENT_TYPE_SERVER_PRE_UPDATE:
		return "SERVER_PRE_UPDATE"
	case pluginproto.EventType_EVENT_TYPE_SERVER_POST_UPDATE:
		return "SERVER_POST_UPDATE"
	case pluginproto.EventType_EVENT_TYPE_SERVER_PRE_REINSTALL:
		return "SERVER_PRE_REINSTALL"
	case pluginproto.EventType_EVENT_TYPE_SERVER_POST_REINSTALL:
		return "SERVER_POST_REINSTALL"
	case pluginproto.EventType_EVENT_TYPE_SERVER_PRE_DELETE:
		return "SERVER_PRE_DELETE"
	case pluginproto.EventType_EVENT_TYPE_SERVER_POST_DELETE:
		return "SERVER_POST_DELETE"
	default:
		return "UNKNOWN"
	}
}

// handleOtherEvent logs the non-server payloads this plugin subscribes to.
func handleOtherEvent(event *pluginproto.Event) *pluginproto.EventResult {
	attrs := []any{
		slog.String("event_type", pluginproto.EventType_name[int32(event.GetType())]),
		slog.String("receiver", event.GetContext().GetPluginId()),
	}

	switch payload := event.GetPayload().(type) {
	case *pluginproto.Event_UserEvent:
		attrs = append(attrs,
			slog.Uint64("user_id", payload.UserEvent.GetUser().GetId()),
			slog.String("login", payload.UserEvent.GetUser().GetLogin()))
	case *pluginproto.Event_NodeEvent:
		attrs = append(attrs,
			slog.Uint64("node_id", payload.NodeEvent.GetNode().GetId()),
			slog.String("node", payload.NodeEvent.GetNode().GetName()),
			slog.String("instance_id", payload.NodeEvent.GetExtraData()["instance_id"]))
	case *pluginproto.Event_ServerSettingsEvent:
		attrs = append(attrs,
			slog.Uint64("server_id", payload.ServerSettingsEvent.GetServerId()),
			slog.String("changed_fields", payload.ServerSettingsEvent.GetExtraData()["changed_fields"]))
	case *pluginproto.Event_PluginEvent:
		attrs = append(attrs,
			slog.String("plugin", payload.PluginEvent.GetName()),
			slog.String("status", payload.PluginEvent.GetStatus()),
			slog.String("error", payload.PluginEvent.GetError()))
	default:
		return &pluginproto.EventResult{Handled: false}
	}

	logger.Info("Panel event", attrs...)
	eventCounter.Add(1)

	return &pluginproto.EventResult{Handled: true}
}
