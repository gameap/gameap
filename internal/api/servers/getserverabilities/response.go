package getserverabilities

import "github.com/gameap/gameap/internal/domain"

type abilitiesResponse map[domain.AbilityName]bool

func newAbilitiesResponse(abilities map[domain.AbilityName]bool) abilitiesResponse {
	return abilities
}
