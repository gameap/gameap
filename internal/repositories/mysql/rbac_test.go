package mysql_test

import (
	"context"
	"os"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/mysql"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/gameap/gameap/internal/services"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestRBACRepository(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")

	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL tests because TEST_MYSQL_DSN is not set")
	}

	suite.Run(t, repotesting.NewRBACRepositorySuite(
		func(t *testing.T) (repositories.RBACRepository, func(ctx context.Context, t *testing.T, name string) domain.Role, func(ctx context.Context, t *testing.T, ability domain.Ability) uint) {
			t.Helper()

			db := SetupTestDB(t, testMySQLDSN)
			tm := services.NewNilTransactionManager()
			repo := mysql.NewRBACRepository(db, tm)

			createRoleFunc := func(ctx context.Context, t *testing.T, name string) domain.Role {
				t.Helper()

				role := domain.Role{
					Name:  name,
					Title: lo.ToPtr(name + " Title"),
					Level: lo.ToPtr(uint(1)),
					Scope: lo.ToPtr(1),
				}

				query := "INSERT INTO roles (name, title, level, scope) VALUES (?, ?, ?, ?)"
				result, err := db.ExecContext(ctx, query, role.Name, role.Title, role.Level, role.Scope)
				require.NoError(t, err)

				id, err := result.LastInsertId()
				require.NoError(t, err)
				role.ID = uint(id)

				return role
			}

			createAbilityFunc := func(ctx context.Context, t *testing.T, ability domain.Ability) uint {
				t.Helper()

				result, err := db.ExecContext(ctx,
					"INSERT INTO abilities (name, title, entity_id, entity_type, only_owned, options, scope) VALUES (?, ?, ?, ?, ?, NULL, NULL)",
					ability.Name, ability.Title, ability.EntityID, ability.EntityType, ability.OnlyOwned)
				require.NoError(t, err)

				id, err := result.LastInsertId()
				require.NoError(t, err)

				return uint(id)
			}

			return repo, createRoleFunc, createAbilityFunc
		},
	))
}
