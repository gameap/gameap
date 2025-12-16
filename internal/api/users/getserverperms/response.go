package getserverperms

import (
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/plugin"
)

type PermissionResponse struct {
	Permission string `json:"permission"`
	Value      bool   `json:"value"`
	Name       string `json:"name"`
}

func NewPermissionResponse(abilityName domain.AbilityName, value bool) PermissionResponse {
	displayName := "users." + string(abilityName)

	return PermissionResponse{
		Permission: string(abilityName),
		Value:      value,
		Name:       displayName,
	}
}

func NewPluginPermissionResponse(ability plugin.ServerAbility, value bool) PermissionResponse {
	displayName := ability.Title
	if displayName == "" {
		displayName = ability.Name
	}

	return PermissionResponse{
		Permission: ability.Name,
		Value:      value,
		Name:       displayName,
	}
}
