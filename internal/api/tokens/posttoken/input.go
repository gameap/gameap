package posttoken

import (
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
)

var (
	ErrNameIsRequired      = api.NewValidationError("name field is required")
	ErrNameTooLong         = api.NewValidationError("name must not exceed 255 characters")
	ErrAbilitiesIsRequired = api.NewValidationError("abilities field is required")
	ErrAbilitiesEmpty      = api.NewValidationError("at least one ability must be provided")
	ErrInvalidAbility      = api.NewValidationError("invalid ability provided")
	ErrAbilitiesTooMany    = api.NewValidationError("too many abilities provided (maximum 100)")
	ErrDuplicateAbilities  = api.NewValidationError("duplicate abilities are not allowed")
)

const (
	maxNameLength     = 255
	maxAbilitiesCount = 100
)

type tokenInput struct {
	TokenName string   `json:"token_name"`
	Abilities []string `json:"abilities"`
}

func (r *tokenInput) Validate() error {
	if r.TokenName == "" {
		return ErrNameIsRequired
	}

	if len(r.TokenName) > maxNameLength {
		return ErrNameTooLong
	}

	if r.Abilities == nil {
		return ErrAbilitiesIsRequired
	}

	if len(r.Abilities) == 0 {
		return ErrAbilitiesEmpty
	}

	if len(r.Abilities) > maxAbilitiesCount {
		return ErrAbilitiesTooMany
	}

	if !r.validateAbilities() {
		return ErrInvalidAbility
	}

	if r.hasDuplicateAbilities() {
		return ErrDuplicateAbilities
	}

	return nil
}

func (r *tokenInput) validateAbilities() bool {
	for _, ability := range r.Abilities {
		if !domain.ValidateAbility(ability) {
			return false
		}
	}

	return true
}

func (r *tokenInput) hasDuplicateAbilities() bool {
	seen := make(map[string]bool)
	for _, ability := range r.Abilities {
		if seen[ability] {
			return true
		}
		seen[ability] = true
	}

	return false
}
