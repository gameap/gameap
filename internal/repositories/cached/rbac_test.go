package cached_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/cached"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
)

func TestRBACRepository(t *testing.T) {
	suite.Run(t, repotesting.NewRBACRepositorySuite(
		func(_ *testing.T) (repositories.RBACRepository, func(ctx context.Context, t *testing.T, name string) domain.Role, func(ctx context.Context, t *testing.T, ability domain.Ability) uint) {
			repo := cached.NewRBACRepository(
				inmemory.NewRBACRepository(),
				cache.NewInMemory(),
				5*time.Minute,
			)

			var nextRoleID uint

			createRoleFunc := func(ctx context.Context, t *testing.T, name string) domain.Role {
				t.Helper()

				nextRoleID++
				role := domain.Role{
					ID:    nextRoleID,
					Name:  name,
					Title: lo.ToPtr(name + " Title"),
					Level: lo.ToPtr(uint(1)),
					Scope: lo.ToPtr(1),
				}

				err := repo.SaveRole(ctx, &role)
				if err != nil {
					t.Fatalf("failed to save role: %v", err)
				}

				return role
			}

			var nextAbilityID uint

			createAbilityFunc := func(ctx context.Context, t *testing.T, ability domain.Ability) uint {
				t.Helper()

				nextAbilityID++
				ability.ID = nextAbilityID

				role := domain.Role{
					ID:    1,
					Name:  "test",
					Title: lo.ToPtr("Test Role"),
					Level: lo.ToPtr(uint(1)),
					Scope: lo.ToPtr(1),
				}

				err := repo.SaveRole(ctx, &role)
				if err != nil {
					t.Fatalf("failed to save role: %v", err)
				}

				err = repo.Allow(ctx, role.ID, domain.EntityTypeEmpty, []domain.Ability{ability})
				if err != nil {
					t.Fatalf("failed to create ability: %v", err)
				}

				return ability.ID
			}

			return repo, createRoleFunc, createAbilityFunc
		},
	))
}

func TestRBACRepositoryWithRedisCache(t *testing.T) {
	testRedisAddr := os.Getenv("TEST_REDIS_ADDR")

	if testRedisAddr == "" {
		t.Skip("Skipping Redis tests because TEST_REDIS_ADDR is not set")
	}

	redisCache, err := cache.NewRedis(testRedisAddr, "", 0)
	if err != nil {
		t.Fatalf("failed to connect to Redis at %s: %v", testRedisAddr, err)
	}

	suite.Run(t, repotesting.NewRBACRepositorySuite(
		func(_ *testing.T) (repositories.RBACRepository, func(ctx context.Context, t *testing.T, name string) domain.Role, func(ctx context.Context, t *testing.T, ability domain.Ability) uint) {
			repo := cached.NewRBACRepository(
				inmemory.NewRBACRepository(),
				redisCache,
				5*time.Minute,
			)

			var nextRoleID uint

			createRoleFunc := func(ctx context.Context, t *testing.T, name string) domain.Role {
				t.Helper()

				nextRoleID++
				role := domain.Role{
					ID:    nextRoleID,
					Name:  name,
					Title: lo.ToPtr(name + " Title"),
					Level: lo.ToPtr(uint(1)),
					Scope: lo.ToPtr(1),
				}

				err := repo.SaveRole(ctx, &role)
				if err != nil {
					t.Fatalf("failed to save role: %v", err)
				}

				return role
			}

			var nextAbilityID uint

			createAbilityFunc := func(ctx context.Context, t *testing.T, ability domain.Ability) uint {
				t.Helper()

				nextAbilityID++
				ability.ID = nextAbilityID

				role := domain.Role{
					ID:    1,
					Name:  "test",
					Title: lo.ToPtr("Test Role"),
					Level: lo.ToPtr(uint(1)),
					Scope: lo.ToPtr(1),
				}

				err := repo.SaveRole(ctx, &role)
				if err != nil {
					t.Fatalf("failed to save role: %v", err)
				}

				err = repo.Allow(ctx, role.ID, domain.EntityTypeEmpty, []domain.Ability{ability})
				if err != nil {
					t.Fatalf("failed to create ability: %v", err)
				}

				return ability.ID
			}

			return repo, createRoleFunc, createAbilityFunc
		},
	))
}
