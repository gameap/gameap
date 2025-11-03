package services

import (
	"context"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/samber/lo"
)

// UserService is sumular to UserRepository but for use cases.
// UserService implements UserRepository interface.
// It contains business logic.
type UserService struct {
	repo repositories.UserRepository
}

func NewUserService(repo repositories.UserRepository) *UserService {
	return &UserService{
		repo: repo,
	}
}

func (s *UserService) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.User, error) {
	return s.repo.FindAll(ctx, order, pagination)
}

func (s *UserService) Find(
	ctx context.Context,
	filter *filters.FindUser,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.User, error) {
	if filter != nil && len(filter.Logins) > 0 {
		// Normalize logins to lowercase. Logins are case-insensitive.
		// This is important for consistent querying.
		// Assuming the database stores logins in lowercase.
		for i := range filter.Logins {
			filter.Logins[i] = strings.ToLower(filter.Logins[i])
		}
	}

	return s.repo.Find(ctx, filter, order, pagination)
}

func (s *UserService) Save(
	ctx context.Context,
	user *domain.User,
) error {
	// Normalize login to lowercase. Logins are case-insensitive.
	user.Login = strings.ToLower(user.Login)

	if user.CreatedAt == nil || user.CreatedAt.IsZero() {
		user.CreatedAt = lo.ToPtr(time.Now())
	}

	user.UpdatedAt = lo.ToPtr(time.Now())

	return s.repo.Save(ctx, user)
}

func (s *UserService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}
