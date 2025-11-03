package getserverabilities

import "github.com/gameap/gameap/internal/domain"

type abilitiesResponse struct {
	GameServerCommon      bool `json:"game-server-common"`
	GameServerStart       bool `json:"game-server-start"`
	GameServerStop        bool `json:"game-server-stop"`
	GameServerRestart     bool `json:"game-server-restart"`
	GameServerPause       bool `json:"game-server-pause"`
	GameServerUpdate      bool `json:"game-server-update"`
	GameServerFiles       bool `json:"game-server-files"`
	GameServerTasks       bool `json:"game-server-tasks"`
	GameServerSettings    bool `json:"game-server-settings"`
	GameServerConsoleView bool `json:"game-server-console-view"`
	GameServerConsoleSend bool `json:"game-server-console-send"`
	GameServerRconConsole bool `json:"game-server-rcon-console"`
	GameServerRconPlayers bool `json:"game-server-rcon-players"`
}

func newAbilitiesResponse(abilities map[domain.AbilityName]bool) abilitiesResponse {
	return abilitiesResponse{
		GameServerCommon:      abilities[domain.AbilityNameGameServerCommon],
		GameServerStart:       abilities[domain.AbilityNameGameServerStart],
		GameServerStop:        abilities[domain.AbilityNameGameServerStop],
		GameServerRestart:     abilities[domain.AbilityNameGameServerRestart],
		GameServerPause:       abilities[domain.AbilityNameGameServerPause],
		GameServerUpdate:      abilities[domain.AbilityNameGameServerUpdate],
		GameServerFiles:       abilities[domain.AbilityNameGameServerFiles],
		GameServerTasks:       abilities[domain.AbilityNameGameServerTasks],
		GameServerSettings:    abilities[domain.AbilityNameGameServerSettings],
		GameServerConsoleView: abilities[domain.AbilityNameGameServerConsoleView],
		GameServerConsoleSend: abilities[domain.AbilityNameGameServerConsoleSend],
		GameServerRconConsole: abilities[domain.AbilityNameGameServerRconConsole],
		GameServerRconPlayers: abilities[domain.AbilityNameGameServerRconPlayers],
	}
}
