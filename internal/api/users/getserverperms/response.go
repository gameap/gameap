package getserverperms

import "github.com/gameap/gameap/internal/domain"

type PermissionResponse struct {
	Permission string `json:"permission"`
	Value      bool   `json:"value"`
	Name       string `json:"name"`
}

var abilityNameToDisplayName = map[domain.AbilityName]string{
	domain.AbilityNameGameServerCommon:      "Common Game Server Ability",
	domain.AbilityNameGameServerStart:       "Start Game Server",
	domain.AbilityNameGameServerStop:        "Stop Game Server",
	domain.AbilityNameGameServerRestart:     "Restart Game Server",
	domain.AbilityNameGameServerPause:       "Pause Game Server",
	domain.AbilityNameGameServerUpdate:      "Update Game Server",
	domain.AbilityNameGameServerFiles:       "Access to filemanager",
	domain.AbilityNameGameServerTasks:       "Access to task scheduler",
	domain.AbilityNameGameServerSettings:    "Access to settings",
	domain.AbilityNameGameServerConsoleView: "Access to read server console",
	domain.AbilityNameGameServerConsoleSend: "Access to send console commands",
	domain.AbilityNameGameServerRconConsole: "RCON console",
	domain.AbilityNameGameServerRconPlayers: "RCON players manage",
}

func NewPermissionResponse(abilityName domain.AbilityName, value bool) PermissionResponse {
	displayName := abilityNameToDisplayName[abilityName]
	if displayName == "" {
		displayName = string(abilityName)
	}

	return PermissionResponse{
		Permission: string(abilityName),
		Value:      value,
		Name:       displayName,
	}
}
