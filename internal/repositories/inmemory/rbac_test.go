package inmemory

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
)

func TestRBACRepository(t *testing.T) {
	suite.Run(t, repotesting.NewRBACRepositorySuite(
		func(_ *testing.T) (repositories.RBACRepository, func(ctx context.Context, t *testing.T, name string) domain.Role, func(ctx context.Context, t *testing.T, ability domain.Ability) uint) {
			repo := NewRBACRepository()

			createRoleFunc := func(_ context.Context, t *testing.T, name string) domain.Role {
				t.Helper()

				roleID := atomic.AddUint32(&repo.nextRoleID, 1)
				role := domain.Role{
					ID:    uint(roleID),
					Name:  name,
					Title: lo.ToPtr(name + " Title"),
					Level: lo.ToPtr(uint(1)),
					Scope: lo.ToPtr(1),
				}

				repo.mu.Lock()
				repo.roles[role.ID] = &role
				repo.mu.Unlock()

				return role
			}

			createAbilityFunc := func(_ context.Context, t *testing.T, ability domain.Ability) uint {
				t.Helper()

				abilityID := atomic.AddUint32(&repo.nextAbilityID, 1)
				ability.ID = uint(abilityID)

				repo.mu.Lock()
				repo.abilities[ability.ID] = &ability
				repo.mu.Unlock()

				return ability.ID
			}

			return repo, createRoleFunc, createAbilityFunc
		},
	))
}
