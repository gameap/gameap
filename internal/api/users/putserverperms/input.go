package putserverperms

import (
	"github.com/gameap/gameap/internal/domain"
	"github.com/pkg/errors"
)

type PermissionInput struct {
	Permission string `json:"permission"`
	Value      bool   `json:"value"`
}

type UpdatePermissionsInput []PermissionInput

func (input UpdatePermissionsInput) Validate() error {
	if len(input) == 0 {
		return errors.New("permissions array cannot be empty")
	}

	validPermissions := make(map[string]bool)
	for _, ability := range domain.ServersAbilities {
		validPermissions[string(ability)] = true
	}

	for idx, perm := range input {
		if perm.Permission == "" {
			return errors.Errorf("permission at index %d cannot be empty", idx)
		}

		if !validPermissions[perm.Permission] {
			return errors.Errorf("invalid permission name: %s", perm.Permission)
		}
	}

	return nil
}

func (input UpdatePermissionsInput) ToAbilities() ([]domain.AbilityName, []domain.AbilityName) {
	allow := make([]domain.AbilityName, 0, len(input))
	revoke := make([]domain.AbilityName, 0, len(input))

	for _, perm := range input {
		abilityName := domain.AbilityName(perm.Permission)

		if perm.Value {
			allow = append(allow, abilityName)
		} else {
			revoke = append(revoke, abilityName)
		}
	}

	return allow, revoke
}
