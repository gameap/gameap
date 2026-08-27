package cached_test

import (
	"context"
	"os"
	"slices"
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

// upsertingUserRepo stands in for the MySQL user repository, whose upsert names
// no conflict target: an ID-less save whose login or email is already taken
// updates that row instead of inserting one. The in-memory repository always
// inserts, so it cannot reproduce this.
type upsertingUserRepo struct {
	users  []domain.User
	nextID uint
}

func (r *upsertingUserRepo) Save(_ context.Context, user *domain.User) error {
	if idx := r.indexOf(user); idx >= 0 {
		user.ID = r.users[idx].ID
		r.users[idx] = *user

		return nil
	}

	r.nextID++
	user.ID = r.nextID
	r.users = append(r.users, *user)

	return nil
}

func (r *upsertingUserRepo) indexOf(user *domain.User) int {
	for i, existing := range r.users {
		if user.ID != 0 {
			if existing.ID == user.ID {
				return i
			}

			continue
		}

		if existing.Login == user.Login || existing.Email == user.Email {
			return i
		}
	}

	return -1
}

// Find only has to serve single-field filters, which is all the cached
// decorator ever builds a key from.
func (r *upsertingUserRepo) Find(
	_ context.Context, filter *filters.FindUser, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.User, error) {
	var found []domain.User

	for _, user := range r.users {
		if slices.Contains(filter.IDs, user.ID) ||
			slices.Contains(filter.Logins, user.Login) ||
			slices.Contains(filter.Emails, user.Email) {
			found = append(found, user)
		}
	}

	return found, nil
}

func (r *upsertingUserRepo) FindAll(
	_ context.Context, _ []filters.Sorting, _ *filters.Pagination,
) ([]domain.User, error) {
	return r.users, nil
}

func (r *upsertingUserRepo) Delete(_ context.Context, id uint) error {
	r.users = slices.DeleteFunc(r.users, func(user domain.User) bool { return user.ID == id })

	return nil
}

func TestUserRepositorySaveDropsKeysOfRowOverwrittenByIDLessSave(t *testing.T) {
	tests := []struct {
		name     string
		save     *domain.User
		staleKey *filters.FindUser
		freshKey *filters.FindUser
	}{
		{
			name:     "login_collision_frees_the_old_email",
			save:     &domain.User{Login: "alice", Email: "new@example.com", Password: "new-hash"},
			staleKey: &filters.FindUser{Emails: []string{"alice@example.com"}},
			freshKey: &filters.FindUser{Emails: []string{"new@example.com"}},
		},
		{
			name:     "email_collision_frees_the_old_login",
			save:     &domain.User{Login: "newlogin", Email: "alice@example.com", Password: "new-hash"},
			staleKey: &filters.FindUser{Logins: []string{"alice"}},
			freshKey: &filters.FindUser{Logins: []string{"newlogin"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := cached.NewUserRepository(&upsertingUserRepo{}, cache.NewInMemory(), 5*time.Minute)

			require.NoError(t, repo.Save(ctx, &domain.User{
				Login: "alice", Email: "alice@example.com", Password: "old-hash",
			}))

			warm, err := repo.Find(ctx, tt.staleKey, nil, nil)
			require.NoError(t, err)
			require.Len(t, warm, 1)

			// No ID on the way in, but an identifier that is already taken: the
			// row above is updated rather than a new one inserted.
			require.NoError(t, repo.Save(ctx, tt.save))

			stale, err := repo.Find(ctx, tt.staleKey, nil, nil)
			require.NoError(t, err)
			assert.Empty(t, stale,
				"the identifier this save freed must stop resolving to its previous owner")

			fresh, err := repo.Find(ctx, tt.freshKey, nil, nil)
			require.NoError(t, err)
			require.Len(t, fresh, 1)
			assert.Equal(t, "new-hash", fresh[0].Password)
		})
	}
}
