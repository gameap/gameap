package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UserRepositorySuite struct {
	suite.Suite

	repo repositories.UserRepository

	fn func(t *testing.T) repositories.UserRepository
}

type userRepoSetupFunc func(t *testing.T) repositories.UserRepository

func NewUserRepositorySuite(fn userRepoSetupFunc) *UserRepositorySuite {
	return &UserRepositorySuite{
		fn: fn,
	}
}

func (s *UserRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *UserRepositorySuite) TestUserRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_user", func(t *testing.T) {
		user := &domain.User{
			Login:    "testuser",
			Email:    "test@example.com",
			Password: "hashedpassword",
			Name:     lo.ToPtr("Test User"),
		}

		err := s.repo.Save(ctx, user)
		require.NoError(t, err)
		assert.NotZero(t, user.ID)
		assert.NotNil(t, user.CreatedAt)
		assert.NotNil(t, user.UpdatedAt)
	})

	s.T().Run("update_existing_user", func(t *testing.T) {
		user := &domain.User{
			Login:    "updateuser",
			Email:    "update@example.com",
			Password: "hashedpassword",
			Name:     lo.ToPtr("Update User"),
		}

		err := s.repo.Save(ctx, user)
		require.NoError(t, err)
		originalID := user.ID

		user.Name = lo.ToPtr("Updated Name")
		user.Email = "updated@example.com"
		err = s.repo.Save(ctx, user)
		require.NoError(t, err)
		assert.Equal(t, originalID, user.ID)

		assert.Equal(t, "Updated Name", *user.Name)
		assert.Equal(t, "updated@example.com", user.Email)
	})
}

func (s *UserRepositorySuite) TestUserRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_existing_user", func(t *testing.T) {
		user := &domain.User{
			Login:    "deleteuser",
			Email:    "delete@example.com",
			Password: "hashedpassword",
			Name:     lo.ToPtr("Delete User"),
		}

		require.NoError(t, s.repo.Save(ctx, user))
		err := s.repo.Delete(ctx, user.ID)
		require.NoError(t, err)
	})

	s.T().Run("delete_non_existent_user", func(t *testing.T) {
		err := s.repo.Delete(ctx, 999)
		require.NoError(t, err)
	})

	s.T().Run("delete_and_try_to_find", func(t *testing.T) {
		// Create user to delete
		user := &domain.User{
			Login:    "deleteuser",
			Email:    "delete@example.com",
			Password: "hashedpassword",
			Name:     lo.ToPtr("Delete User"),
		}

		require.NoError(t, s.repo.Save(ctx, user))
		require.NotZero(t, user.ID)

		// Ensure user exists
		filter := &filters.FindUser{IDs: []uint{user.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, user.ID, results[0].ID)

		// Delete user
		err = s.repo.Delete(ctx, user.ID)
		require.NoError(t, err)

		filter = &filters.FindUser{IDs: []uint{user.ID}}
		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func (s *UserRepositorySuite) TestUserRepositorySaveWithTimestamps() {
	ctx := context.Background()

	s.T().Run("auto_set_timestamps_on_insert", func(t *testing.T) {
		user := &domain.User{
			Login:    "timestampuser",
			Email:    "timestamp@example.com",
			Password: "hashedpassword",
			Name:     lo.ToPtr("Timestamp User"),
		}

		beforeSave := time.Now()
		err := s.repo.Save(ctx, user)
		require.NoError(t, err)
		afterSave := time.Now()

		require.NotNil(t, user.CreatedAt)
		require.NotNil(t, user.UpdatedAt)
		assert.True(t, user.CreatedAt.After(beforeSave) || user.CreatedAt.Equal(beforeSave))
		assert.True(t, user.CreatedAt.Before(afterSave) || user.CreatedAt.Equal(afterSave))
		assert.True(t, user.UpdatedAt.After(beforeSave) || user.UpdatedAt.Equal(beforeSave))
		assert.True(t, user.UpdatedAt.Before(afterSave) || user.UpdatedAt.Equal(afterSave))
	})

	s.T().Run("update_timestamp_on_update", func(t *testing.T) {
		user := &domain.User{
			Login:    "updatetimestamp",
			Email:    "updatetimestamp@example.com",
			Password: "hashedpassword",
			Name:     lo.ToPtr("Update Timestamp User"),
		}

		require.NoError(t, s.repo.Save(ctx, user))
		originalUpdatedAt := *user.UpdatedAt

		time.Sleep(100 * time.Millisecond)

		user.Name = lo.ToPtr("Updated Name")
		require.NoError(t, s.repo.Save(ctx, user))

		assert.True(t, user.UpdatedAt.After(originalUpdatedAt) || user.UpdatedAt.Equal(originalUpdatedAt))
	})

	s.T().Run("preserve_custom_timestamps", func(t *testing.T) {
		customTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
		user := &domain.User{
			Login:     "customtime",
			Email:     "customtime@example.com",
			Password:  "hashedpassword",
			Name:      lo.ToPtr("Custom Time User"),
			CreatedAt: lo.ToPtr(customTime),
			UpdatedAt: lo.ToPtr(customTime),
		}

		err := s.repo.Save(ctx, user)
		require.NoError(t, err)

		assert.Equal(t, customTime, *user.CreatedAt)
	})
}

func (s *UserRepositorySuite) TestUserRepositoryFindAll() {
	ctx := context.Background()

	user1 := &domain.User{
		Login:    "findall1",
		Email:    "findall1@example.com",
		Password: "hashedpassword1",
		Name:     lo.ToPtr("FindAll User 1"),
	}
	user2 := &domain.User{
		Login:    "findall2",
		Email:    "findall2@example.com",
		Password: "hashedpassword2",
		Name:     lo.ToPtr("FindAll User 2"),
	}
	user3 := &domain.User{
		Login:    "findall3",
		Email:    "findall3@example.com",
		Password: "hashedpassword3",
		Name:     lo.ToPtr("FindAll User 3"),
	}

	require.NoError(s.T(), s.repo.Save(ctx, user1))
	require.NoError(s.T(), s.repo.Save(ctx, user2))
	require.NoError(s.T(), s.repo.Save(ctx, user3))

	s.T().Run("find_all_users", func(t *testing.T) {
		results, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_all_with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{Limit: 2, Offset: 0}

		results, err := s.repo.FindAll(ctx, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	s.T().Run("find_all_with_order_asc", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionAsc},
		}

		results, err := s.repo.FindAll(ctx, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 3)

		for i := 0; i < len(results)-1; i++ {
			assert.LessOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})

	s.T().Run("find_all_with_order_desc", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionDesc},
		}

		results, err := s.repo.FindAll(ctx, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 3)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})

	s.T().Run("find_all_with_pagination_and_order", func(t *testing.T) {
		pagination := &filters.Pagination{Limit: 1, Offset: 1}
		order := []filters.Sorting{
			{Field: "login", Direction: filters.SortDirectionAsc},
		}

		results, err := s.repo.FindAll(ctx, order, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})
}

func (s *UserRepositorySuite) TestUserRepositoryFind() {
	ctx := context.Background()

	user1 := &domain.User{
		Login:    "testfind1",
		Email:    "testfind1@example.com",
		Password: "hashedpassword1",
		Name:     lo.ToPtr("Test Find User 1"),
	}
	user2 := &domain.User{
		Login:    "testfind2",
		Email:    "testfind2@example.com",
		Password: "hashedpassword2",
		Name:     lo.ToPtr("Test Find User 2"),
	}
	user3 := &domain.User{
		Login:    "testfind3",
		Email:    "testfind3@example.com",
		Password: "hashedpassword3",
		Name:     lo.ToPtr("Test Find User 3"),
	}

	require.NoError(s.T(), s.repo.Save(ctx, user1))
	require.NoError(s.T(), s.repo.Save(ctx, user2))
	require.NoError(s.T(), s.repo.Save(ctx, user3))

	s.T().Run("find_by_single_id", func(t *testing.T) {
		filter := &filters.FindUser{IDs: []uint{user1.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, user1.ID, results[0].ID)
		assert.Equal(t, "testfind1", results[0].Login)
	})

	s.T().Run("find_by_multiple_ids", func(t *testing.T) {
		filter := &filters.FindUser{IDs: []uint{user1.ID, user3.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		ids := []uint{results[0].ID, results[1].ID}
		assert.Contains(t, ids, user1.ID)
		assert.Contains(t, ids, user3.ID)
	})

	s.T().Run("find_by_login", func(t *testing.T) {
		filter := &filters.FindUser{Logins: []string{"testfind2"}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "testfind2", results[0].Login)
		assert.Equal(t, "testfind2@example.com", results[0].Email)
	})

	s.T().Run("find_by_multiple_logins", func(t *testing.T) {
		filter := &filters.FindUser{Logins: []string{"testfind1", "testfind3"}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		logins := []string{results[0].Login, results[1].Login}
		assert.Contains(t, logins, "testfind1")
		assert.Contains(t, logins, "testfind3")
	})

	s.T().Run("find_by_email", func(t *testing.T) {
		filter := &filters.FindUser{Emails: []string{"testfind1@example.com"}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "testfind1@example.com", results[0].Email)
		assert.Equal(t, "testfind1", results[0].Login)
	})

	s.T().Run("find_by_multiple_emails", func(t *testing.T) {
		filter := &filters.FindUser{Emails: []string{"testfind2@example.com", "testfind3@example.com"}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		emails := []string{results[0].Email, results[1].Email}
		assert.Contains(t, emails, "testfind2@example.com")
		assert.Contains(t, emails, "testfind3@example.com")
	})

	s.T().Run("find_with_combined_filters", func(t *testing.T) {
		filter := &filters.FindUser{
			IDs:    []uint{user1.ID, user2.ID},
			Logins: []string{"testfind1"},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "testfind1", results[0].Login)
	})

	s.T().Run("find_with_nil_filter", func(t *testing.T) {
		results, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)
	})

	s.T().Run("find_non_existent", func(t *testing.T) {
		filter := &filters.FindUser{IDs: []uint{99999}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		pagination := &filters.Pagination{Limit: 1, Offset: 0}
		results, err := s.repo.Find(ctx, nil, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	s.T().Run("find_with_order", func(t *testing.T) {
		order := []filters.Sorting{
			{Field: "login", Direction: filters.SortDirectionAsc},
		}
		results, err := s.repo.Find(ctx, nil, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 3)

		for i := 0; i < len(results)-1; i++ {
			assert.LessOrEqual(t, results[i].Login, results[i+1].Login)
		}
	})

	s.T().Run("find_with_pagination_and_order", func(t *testing.T) {
		filter := &filters.FindUser{IDs: []uint{user1.ID, user2.ID, user3.ID}}
		pagination := &filters.Pagination{Limit: 2, Offset: 1}
		order := []filters.Sorting{
			{Field: "id", Direction: filters.SortDirectionAsc},
		}

		results, err := s.repo.Find(ctx, filter, order, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})
}

func (s *UserRepositorySuite) TestUserRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_user_lifecycle", func(t *testing.T) {
		user := &domain.User{
			Login:    "lifecycle",
			Email:    "lifecycle@example.com",
			Password: "hashedpassword",
			Name:     lo.ToPtr("Lifecycle User"),
		}

		err := s.repo.Save(ctx, user)
		require.NoError(t, err)
		assert.NotZero(t, user.ID)
		assert.NotNil(t, user.CreatedAt)
		assert.NotNil(t, user.UpdatedAt)

		filter := &filters.FindUser{IDs: []uint{user.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "lifecycle", results[0].Login)
		assert.Equal(t, "lifecycle@example.com", results[0].Email)

		user.Email = "updated@example.com"
		user.Name = lo.ToPtr("Updated User")
		err = s.repo.Save(ctx, user)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "updated@example.com", results[0].Email)
		assert.Equal(t, "Updated User", *results[0].Name)

		err = s.repo.Delete(ctx, user.ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("multiple_users_operations", func(t *testing.T) {
		users := []*domain.User{
			{Login: "multi1", Email: "multi1@example.com", Password: "pass1", Name: lo.ToPtr("Multi User 1")},
			{Login: "multi2", Email: "multi2@example.com", Password: "pass2", Name: lo.ToPtr("Multi User 2")},
			{Login: "multi3", Email: "multi3@example.com", Password: "pass3", Name: lo.ToPtr("Multi User 3")},
		}

		for _, user := range users {
			require.NoError(t, s.repo.Save(ctx, user))
		}

		filter := &filters.FindUser{
			Logins: []string{"multi1", "multi2", "multi3"},
		}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		require.NoError(t, s.repo.Delete(ctx, users[1].ID))

		results, err = s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		logins := []string{results[0].Login, results[1].Login}
		assert.Contains(t, logins, "multi1")
		assert.NotContains(t, logins, "multi2")
		assert.Contains(t, logins, "multi3")
	})
}
