package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/postgres"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/gameap/gameap/internal/services"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestRBACRepository(t *testing.T) {
	testPostgresDSN := os.Getenv("TEST_POSTGRES_DSN")

	if testPostgresDSN == "" {
		t.Skip("Skipping PostgreSQL tests because TEST_POSTGRES_DSN is not set")
	}

	suite.Run(t, repotesting.NewRBACRepositorySuite(
		func(t *testing.T) (repositories.RBACRepository, func(ctx context.Context, t *testing.T, name string) domain.Role, func(ctx context.Context, t *testing.T, ability domain.Ability) uint) {
			t.Helper()

			db := SetupTestDB(t, testPostgresDSN)
			tm := services.NewNilTransactionManager()
			repo := postgres.NewRBACRepository(db, tm)

			createRoleFunc := func(ctx context.Context, t *testing.T, name string) domain.Role {
				t.Helper()

				role := domain.Role{
					Name:  name,
					Title: lo.ToPtr(name + " Title"),
					Level: lo.ToPtr(uint(1)),
					Scope: lo.ToPtr(1),
				}

				query := "INSERT INTO roles (name, title, level, scope) VALUES ($1, $2, $3, $4) RETURNING id"
				err := db.QueryRowContext(ctx, query, role.Name, role.Title, role.Level, role.Scope).Scan(&role.ID)
				require.NoError(t, err)

				return role
			}

			createAbilityFunc := func(ctx context.Context, t *testing.T, ability domain.Ability) uint {
				t.Helper()

				var id uint
				err := db.QueryRowContext(ctx,
					"INSERT INTO abilities (name, title, entity_id, entity_type, only_owned, options, scope) VALUES ($1, $2, $3, $4, $5, NULL, NULL) RETURNING id",
					ability.Name, ability.Title, ability.EntityID, ability.EntityType, ability.OnlyOwned).Scan(&id)
				require.NoError(t, err)

				return id
			}

			return repo, createRoleFunc, createAbilityFunc
		},
	))
}
