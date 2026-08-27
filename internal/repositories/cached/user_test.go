package cached_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/cached"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestUserRepository(t *testing.T) {
	suite.Run(t, repotesting.NewUserRepositorySuite(
		func(_ *testing.T) repositories.UserRepository {
			return cached.NewUserRepository(
				inmemory.NewUserRepository(),
				cache.NewInMemory(),
				5*time.Minute,
			)
		},
	))
}

func TestUserRepositoryWithRedisCache(t *testing.T) {
	testRedisAddr := os.Getenv("TEST_REDIS_ADDR")

	if testRedisAddr == "" {
		t.Skip("Skipping Redis tests because TEST_REDIS_ADDR is not set")
	}

	redisCache, err := cache.NewRedis(testRedisAddr, "", 0)
	if err != nil {
		t.Fatalf("failed to connect to Redis at %s: %v", testRedisAddr, err)
	}

	suite.Run(t, repotesting.NewUserRepositorySuite(
		func(_ *testing.T) repositories.UserRepository {
			return cached.NewUserRepository(
				inmemory.NewUserRepository(),
				redisCache,
				5*time.Minute,
			)
		},
	))
}

// The cache keys for login and email are hashed from the values themselves, so
// every one of these covers a way a stale entry could outlive the write it
// should have been dropped by.

func newCachedUserRepo(t *testing.T) (repositories.UserRepository, cache.Cache) {
	t.Helper()

	c := cache.NewInMemory()

	return cached.NewUserRepository(inmemory.NewUserRepository(), c, 5*time.Minute), c
}

func TestUserRepositorySaveDropsStaleEmailEntry(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCachedUserRepo(t)

	user := &domain.User{Login: "alice", Email: "alice@example.com", Password: "old-hash"}
	require.NoError(t, repo.Save(ctx, user))

	warm, err := repo.Find(ctx, &filters.FindUser{Emails: []string{"alice@example.com"}}, nil, nil)
	require.NoError(t, err)
	require.Len(t, warm, 1)
	require.Equal(t, "old-hash", warm[0].Password)

	user.Password = "new-hash"
	require.NoError(t, repo.Save(ctx, user))

	found, err := repo.Find(ctx, &filters.FindUser{Emails: []string{"alice@example.com"}}, nil, nil)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "new-hash", found[0].Password,
		"a lookup by email must not keep serving the password hash from before the write")
}

func TestUserRepositorySaveDropsPreviousEmailEntry(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCachedUserRepo(t)

	user := &domain.User{Login: "alice", Email: "old@example.com", Password: "hash"}
	require.NoError(t, repo.Save(ctx, user))

	warm, err := repo.Find(ctx, &filters.FindUser{Emails: []string{"old@example.com"}}, nil, nil)
	require.NoError(t, err)
	require.Len(t, warm, 1)

	user.Email = "new@example.com"
	require.NoError(t, repo.Save(ctx, user))

	stale, err := repo.Find(ctx, &filters.FindUser{Emails: []string{"old@example.com"}}, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, stale,
		"the freed address must stop resolving to its previous owner, who could otherwise be "+
			"served to whoever claims it next")

	found, err := repo.Find(ctx, &filters.FindUser{Emails: []string{"new@example.com"}}, nil, nil)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, user.ID, found[0].ID)
}

func TestUserRepositorySaveDropsPreviousLoginEntry(t *testing.T) {
	ctx := context.Background()
	repo, _ := newCachedUserRepo(t)

	user := &domain.User{Login: "oldlogin", Email: "alice@example.com", Password: "hash"}
	require.NoError(t, repo.Save(ctx, user))

	warm, err := repo.Find(ctx, &filters.FindUser{Logins: []string{"oldlogin"}}, nil, nil)
	require.NoError(t, err)
	require.Len(t, warm, 1)

	user.Login = "newlogin"
	require.NoError(t, repo.Save(ctx, user))

	stale, err := repo.Find(ctx, &filters.FindUser{Logins: []string{"oldlogin"}}, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, stale, "the previous login must stop resolving to the user")
}

func TestUserRepositorySaveKeepsUnrelatedCacheEntries(t *testing.T) {
	ctx := context.Background()
	repo, c := newCachedUserRepo(t)

	// Pattern invalidation falls back to Clear on every non-Redis cache, and the
	// panel keeps SSO tickets and 2FA challenges in the same instance. A user
	// write must drop its own keys and nothing else.
	require.NoError(t, c.Set(ctx, "sso:ticket:abc", "payload"))

	require.NoError(t, repo.Save(ctx, &domain.User{Login: "alice", Email: "alice@example.com"}))

	got, err := c.Get(ctx, "sso:ticket:abc")
	require.NoError(t, err, "saving a user must not flush unrelated cache entries")
	assert.Equal(t, "payload", got)
}
