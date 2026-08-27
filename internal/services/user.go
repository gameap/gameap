package services

import (
	"context"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
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
	// Logins and emails are case-insensitive, but the repositories below compare
	// them exactly so the unique indexes stay usable. Canonicalising here keeps
	// that contract in one place: everything the repository sees is lowercase.
	if filter != nil {
		for i := range filter.Logins {
			filter.Logins[i] = strings.ToLower(filter.Logins[i])
		}

		for i := range filter.Emails {
			filter.Emails[i] = strings.ToLower(filter.Emails[i])
		}
	}

	return s.repo.Find(ctx, filter, order, pagination)
}

func (s *UserService) Save(
	ctx context.Context,
	user *domain.User,
) error {
	// Store the canonical form: see Find for why the repositories never fold case
	// themselves.
	user.Login = strings.ToLower(user.Login)
	user.Email = strings.ToLower(user.Email)

	if user.CreatedAt == nil || user.CreatedAt.IsZero() {
		user.CreatedAt = new(time.Now())
	}

	user.UpdatedAt = new(time.Now())

	return s.repo.Save(ctx, user)
}

func (s *UserService) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}
