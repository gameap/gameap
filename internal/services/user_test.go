package services

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserService_FindAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupRepo  func(*inmemory.UserRepository)
		order      []filters.Sorting
		pagination *filters.Pagination
		validate   func(t *testing.T, users []domain.User, err error)
	}{
		{
			name: "returns_all_users",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "bob", Email: "bob@example.com"})
			},
			validate: func(t *testing.T, users []domain.User, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, users, 2)
			},
		},
		{
			name:      "returns_empty_list_when_no_users",
			setupRepo: func(_ *inmemory.UserRepository) {},
			validate: func(t *testing.T, users []domain.User, err error) {
				t.Helper()
				require.NoError(t, err)
				assert.Empty(t, users)
			},
		},
		{
			name: "applies_pagination",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "bob", Email: "bob@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "charlie", Email: "charlie@example.com"})
			},
			pagination: &filters.Pagination{Limit: 2, Offset: 0},
			validate: func(t *testing.T, users []domain.User, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, users, 2)
			},
		},
		{
			name: "applies_sorting_by_login",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "charlie", Email: "charlie@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "bob", Email: "bob@example.com"})
			},
			order: []filters.Sorting{{Field: "login", Direction: filters.SortDirectionAsc}},
			validate: func(t *testing.T, users []domain.User, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, users, 3)
				assert.Equal(t, "alice", users[0].Login)
				assert.Equal(t, "bob", users[1].Login)
				assert.Equal(t, "charlie", users[2].Login)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := inmemory.NewUserRepository()
			tt.setupRepo(repo)

			service := NewUserService(repo)
			users, err := service.FindAll(context.Background(), tt.order, tt.pagination)

			tt.validate(t, users, err)
		})
	}
}

func TestUserService_Find(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupRepo func(*inmemory.UserRepository)
		filter    *filters.FindUser
		validate  func(t *testing.T, users []domain.User, err error)
	}{
		{
			name: "finds_by_login_case_insensitive",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "bob", Email: "bob@example.com"})
			},
			filter: &filters.FindUser{Logins: []string{"ALICE"}},
			validate: func(t *testing.T, users []domain.User, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, users, 1)
				assert.Equal(t, "alice", users[0].Login)
			},
		},
		{
			name: "finds_by_multiple_logins",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "bob", Email: "bob@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "charlie", Email: "charlie@example.com"})
			},
			filter: &filters.FindUser{Logins: []string{"alice", "charlie"}},
			validate: func(t *testing.T, users []domain.User, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, users, 2)
			},
		},
		{
			name: "finds_by_email",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "bob", Email: "bob@example.com"})
			},
			filter: &filters.FindUser{Emails: []string{"bob@example.com"}},
			validate: func(t *testing.T, users []domain.User, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, users, 1)
				assert.Equal(t, "bob", users[0].Login)
			},
		},
		{
			name: "finds_by_id",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "bob", Email: "bob@example.com"})
			},
			filter: &filters.FindUser{IDs: []uint{1}},
			validate: func(t *testing.T, users []domain.User, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, users, 1)
				assert.Equal(t, uint(1), users[0].ID)
			},
		},
		{
			name: "returns_empty_when_not_found",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
			},
			filter: &filters.FindUser{Logins: []string{"nonexistent"}},
			validate: func(t *testing.T, users []domain.User, err error) {
				t.Helper()
				require.NoError(t, err)
				assert.Empty(t, users)
			},
		},
		{
			name: "nil_filter_returns_all_users",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "bob", Email: "bob@example.com"})
			},
			filter: nil,
			validate: func(t *testing.T, users []domain.User, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, users, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := inmemory.NewUserRepository()
			tt.setupRepo(repo)

			service := NewUserService(repo)
			users, err := service.Find(context.Background(), tt.filter, nil, nil)

			tt.validate(t, users, err)
		})
	}
}

func TestUserService_Save(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupRepo func(*inmemory.UserRepository)
		user      *domain.User
		validate  func(t *testing.T, repo *inmemory.UserRepository, err error)
	}{
		{
			name:      "saves_new_user_with_lowercase_login",
			setupRepo: func(_ *inmemory.UserRepository) {},
			user:      &domain.User{Login: "ALICE", Email: "alice@example.com"},
			validate: func(t *testing.T, repo *inmemory.UserRepository, err error) {
				t.Helper()
				require.NoError(t, err)

				users, err := repo.Find(context.Background(), &filters.FindUser{Logins: []string{"alice"}}, nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
				assert.Equal(t, "alice", users[0].Login)
			},
		},
		{
			name:      "sets_created_at_for_new_user",
			setupRepo: func(_ *inmemory.UserRepository) {},
			user:      &domain.User{Login: "alice", Email: "alice@example.com"},
			validate: func(t *testing.T, repo *inmemory.UserRepository, err error) {
				t.Helper()
				require.NoError(t, err)

				users, err := repo.Find(context.Background(), &filters.FindUser{Logins: []string{"alice"}}, nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
				assert.NotNil(t, users[0].CreatedAt)
				assert.False(t, users[0].CreatedAt.IsZero())
			},
		},
		{
			name:      "sets_updated_at",
			setupRepo: func(_ *inmemory.UserRepository) {},
			user:      &domain.User{Login: "alice", Email: "alice@example.com"},
			validate: func(t *testing.T, repo *inmemory.UserRepository, err error) {
				t.Helper()
				require.NoError(t, err)

				users, err := repo.Find(context.Background(), &filters.FindUser{Logins: []string{"alice"}}, nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
				assert.NotNil(t, users[0].UpdatedAt)
				assert.False(t, users[0].UpdatedAt.IsZero())
			},
		},
		{
			name: "updates_existing_user",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{
					Login: "alice",
					Email: "alice@example.com",
					Name:  new("Alice"),
				})
			},
			user: &domain.User{ID: 1, Login: "Alice", Email: "alice@example.com", Name: new("Alice Updated")},
			validate: func(t *testing.T, repo *inmemory.UserRepository, err error) {
				t.Helper()
				require.NoError(t, err)

				users, err := repo.Find(context.Background(), &filters.FindUser{IDs: []uint{1}}, nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
				assert.Equal(t, "alice", users[0].Login)
				assert.Equal(t, "Alice Updated", lo.FromPtr(users[0].Name))
			},
		},
		{
			name:      "assigns_id_to_new_user",
			setupRepo: func(_ *inmemory.UserRepository) {},
			user:      &domain.User{Login: "alice", Email: "alice@example.com"},
			validate: func(t *testing.T, repo *inmemory.UserRepository, err error) {
				t.Helper()
				require.NoError(t, err)

				users, err := repo.Find(context.Background(), &filters.FindUser{Logins: []string{"alice"}}, nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
				assert.NotZero(t, users[0].ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := inmemory.NewUserRepository()
			tt.setupRepo(repo)

			service := NewUserService(repo)
			err := service.Save(context.Background(), tt.user)

			tt.validate(t, repo, err)
		})
	}
}

func TestUserService_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupRepo func(*inmemory.UserRepository)
		userID    uint
		validate  func(t *testing.T, repo *inmemory.UserRepository, err error)
	}{
		{
			name: "deletes_existing_user",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
				_ = repo.Save(context.Background(), &domain.User{Login: "bob", Email: "bob@example.com"})
			},
			userID: 1,
			validate: func(t *testing.T, repo *inmemory.UserRepository, err error) {
				t.Helper()
				require.NoError(t, err)

				users, err := repo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
				assert.Equal(t, "bob", users[0].Login)
			},
		},
		{
			name: "does_not_return_error_for_nonexistent_user",
			setupRepo: func(repo *inmemory.UserRepository) {
				_ = repo.Save(context.Background(), &domain.User{Login: "alice", Email: "alice@example.com"})
			},
			userID: 999,
			validate: func(t *testing.T, repo *inmemory.UserRepository, err error) {
				t.Helper()
				require.NoError(t, err)

				users, err := repo.FindAll(context.Background(), nil, nil)
				require.NoError(t, err)
				require.Len(t, users, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := inmemory.NewUserRepository()
			tt.setupRepo(repo)

			service := NewUserService(repo)
			err := service.Delete(context.Background(), tt.userID)

			tt.validate(t, repo, err)
		})
	}
}
