package getabilities

import (
	"encoding/json"

	"github.com/gameap/gameap/internal/domain"
)

type orderedAbility struct {
	Key   string
	Value string
}

type orderedGroup struct {
	Key       string
	Abilities []orderedAbility
}

type AbilitiesResponse struct {
	groups []orderedGroup
}

func (r AbilitiesResponse) MarshalJSON() ([]byte, error) {
	// Build JSON manually to preserve order
	buf := make([]byte, 0, 512)
	buf = append(buf, '{')

	for i, group := range r.groups {
		if i > 0 {
			buf = append(buf, ',')
		}

		// Marshal group key
		groupKeyJSON, err := json.Marshal(group.Key)
		if err != nil {
			return nil, err
		}
		buf = append(buf, groupKeyJSON...)
		buf = append(buf, ':')

		// Build abilities object manually to preserve order
		buf = append(buf, '{')
		for j, ability := range group.Abilities {
			if j > 0 {
				buf = append(buf, ',')
			}

			abilityKeyJSON, err := json.Marshal(ability.Key)
			if err != nil {
				return nil, err
			}
			abilityValueJSON, err := json.Marshal(ability.Value)
			if err != nil {
				return nil, err
			}

			buf = append(buf, abilityKeyJSON...)
			buf = append(buf, ':')
			buf = append(buf, abilityValueJSON...)
		}
		buf = append(buf, '}')
	}

	buf = append(buf, '}')

	return buf, nil
}

func newAbilitiesResponse(groupedAbilities domain.GroupedAbilities) AbilitiesResponse {
	// Define the order of groups
	groupOrder := []domain.PATAbilityGroup{
		domain.PATAbilityGroupServer,
		domain.PATAbilityGroupGDaemonTask,
	}

	groups := make([]orderedGroup, 0)

	for _, groupKey := range groupOrder {
		abilities, exists := groupedAbilities[groupKey]
		if !exists {
			continue
		}

		orderedAbilities := make([]orderedAbility, 0, len(abilities))
		for _, ability := range abilities {
			orderedAbilities = append(orderedAbilities, orderedAbility{
				Key:   string(ability.Ability),
				Value: ability.Description,
			})
		}

		groups = append(groups, orderedGroup{
			Key:       string(groupKey),
			Abilities: orderedAbilities,
		})
	}

	return AbilitiesResponse{groups: groups}
}
