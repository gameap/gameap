package console

import (
	"context"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
)

// allowAllRBAC implements base.RBAC and answers `true` to every Can/CanForEntity
// query. Used by tests where the AbilityChecker should always succeed.
type allowAllRBAC struct{}

func (allowAllRBAC) Can(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return true, nil
}

func (allowAllRBAC) CanOneOf(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return true, nil
}

func (allowAllRBAC) CanForEntity(
	_ context.Context, _ uint, _ domain.EntityType, _ uint, _ []domain.AbilityName,
) (bool, error) {
	return true, nil
}

func (allowAllRBAC) GetRoles(_ context.Context, _ uint) ([]string, error) { return nil, nil }

func (allowAllRBAC) SetRolesToUser(_ context.Context, _ uint, _ []string) error { return nil }

func (allowAllRBAC) AdministrativeRoles(_ context.Context) ([]string, error) {
	return nil, nil
}

func (allowAllRBAC) AllowUserAbilitiesForEntity(
	_ context.Context, _ uint, _ uint, _ domain.EntityType, _ []domain.AbilityName,
) error {
	return nil
}

func (allowAllRBAC) RevokeOrForbidUserAbilitiesForEntity(
	_ context.Context, _ uint, _ uint, _ domain.EntityType, _ []domain.AbilityName,
) error {
	return nil
}

// denyAllRBAC implements base.RBAC and answers `false` to every Can/CanForEntity
// query. Used by tests where the AbilityChecker should always fail.
type denyAllRBAC struct{}

func (denyAllRBAC) Can(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return false, nil
}

func (denyAllRBAC) CanOneOf(_ context.Context, _ uint, _ []domain.AbilityName) (bool, error) {
	return false, nil
}

func (denyAllRBAC) CanForEntity(
	_ context.Context, _ uint, _ domain.EntityType, _ uint, _ []domain.AbilityName,
) (bool, error) {
	return false, nil
}

func (denyAllRBAC) GetRoles(_ context.Context, _ uint) ([]string, error) { return nil, nil }

func (denyAllRBAC) SetRolesToUser(_ context.Context, _ uint, _ []string) error { return nil }

func (denyAllRBAC) AdministrativeRoles(_ context.Context) ([]string, error) {
	return nil, nil
}

func (denyAllRBAC) AllowUserAbilitiesForEntity(
	_ context.Context, _ uint, _ uint, _ domain.EntityType, _ []domain.AbilityName,
) error {
	return nil
}

func (denyAllRBAC) RevokeOrForbidUserAbilitiesForEntity(
	_ context.Context, _ uint, _ uint, _ domain.EntityType, _ []domain.AbilityName,
) error {
	return nil
}

// Compile-time checks: both stubs must satisfy base.RBAC.
var (
	_ base.RBAC = allowAllRBAC{}
	_ base.RBAC = denyAllRBAC{}
)

// newAbilityCheckerWithRBAC constructs the project's real AbilityChecker
// against an arbitrary RBAC stub. Tests use it to wire up a Handler whose
// ability gate returns a known answer.
func newAbilityCheckerWithRBAC(r base.RBAC) *serversbase.AbilityChecker {
	return serversbase.NewAbilityChecker(r)
}
