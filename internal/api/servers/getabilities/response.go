package getabilities

import (
	"maps"

	"github.com/gameap/gameap/internal/domain"
)

// ServersAbilitiesResponse represents the response structure for server abilities
// It maps server IDs to their respective abilities.
type ServersAbilitiesResponse map[uint]ServerAbilities

// ServerAbilities represents the abilities available for a specific server
// It maps ability names to boolean values indicating whether the user has that ability.
type ServerAbilities map[domain.AbilityName]bool

// NewServersAbilitiesResponse creates a new ServersAbilitiesResponse from a map of server abilities.
func NewServersAbilitiesResponse(abilities map[uint]map[domain.AbilityName]bool) ServersAbilitiesResponse {
	response := make(ServersAbilitiesResponse, len(abilities))

	for serverID, serverAbilities := range abilities {
		response[serverID] = make(ServerAbilities)
		maps.Copy(response[serverID], serverAbilities)
	}

	return response
}
