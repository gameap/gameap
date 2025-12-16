package putserverperms

import "github.com/gameap/gameap/internal/domain"

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
