package sqlite_test

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/gameap/gameap/internal/services"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestRBACRepository(t *testing.T) {
	suite.Run(t, repotesting.NewRBACRepositorySuite(
		func(t *testing.T) (repositories.RBACRepository, func(ctx context.Context, t *testing.T, name string) domain.Role, func(ctx context.Context, t *testing.T, ability domain.Ability) uint) {
			t.Helper()

			db := SetupTestDB(t)
			tm := services.NewNilTransactionManager()
			repo := sqlite.NewRBACRepository(db, tm)

			createRoleFunc := func(ctx context.Context, t *testing.T, name string) domain.Role {
				t.Helper()

				role := domain.Role{
					Name:  name,
					Title: lo.ToPtr(name + " Title"),
					Level: lo.ToPtr(uint(1)),
					Scope: lo.ToPtr(1),
				}

				query := "INSERT INTO roles (name, title, level, scope) VALUES (?, ?, ?, ?) RETURNING id"
				err := db.QueryRowContext(ctx, query, role.Name, role.Title, role.Level, role.Scope).Scan(&role.ID)
				require.NoError(t, err)

				return role
			}

			createAbilityFunc := func(ctx context.Context, t *testing.T, ability domain.Ability) uint {
				t.Helper()

				var abilityID uint
				err := db.QueryRowContext(ctx,
					"INSERT INTO abilities (name, title, entity_id, entity_type, only_owned, options, scope) VALUES (?, ?, ?, ?, ?, NULL, NULL) RETURNING id",
					ability.Name, ability.Title, ability.EntityID, ability.EntityType, ability.OnlyOwned).Scan(&abilityID)
				require.NoError(t, err)

				return abilityID
			}

			return repo, createRoleFunc, createAbilityFunc
		},
	))
}
