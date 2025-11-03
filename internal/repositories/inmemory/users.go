package inmemory

import (
	"cmp"
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/samber/lo"
)

type UserRepository struct {
	mu     sync.RWMutex
	users  map[uint]*domain.User
	nextID uint32
}

func NewUserRepository() *UserRepository {
	return &UserRepository{
		users: make(map[uint]*domain.User),
	}
}

func (r *UserRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	users := make([]domain.User, 0, len(r.users))
	for _, user := range r.users {
		users = append(users, *user)
	}

	r.sortUsers(users, order)

	return r.applyPagination(users, pagination), nil
}

func (r *UserRepository) Find(
	_ context.Context,
	filter *filters.FindUser,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == nil {
		filter = &filters.FindUser{}
	}

	users := make([]domain.User, 0, len(r.users))

	idSet := make(map[uint]bool)
	for _, id := range filter.IDs {
		idSet[id] = true
	}

	loginSet := make(map[string]bool)
	for _, login := range filter.Logins {
		loginSet[login] = true
	}

	emailSet := make(map[string]bool)
	for _, email := range filter.Emails {
		emailSet[email] = true
	}

	for _, user := range r.users {
		if len(filter.IDs) > 0 && !idSet[user.ID] {
			continue
		}
		if len(filter.Logins) > 0 && !loginSet[user.Login] {
			continue
		}
		if len(filter.Emails) > 0 && !emailSet[user.Email] {
			continue
		}
		users = append(users, *user)
	}

	r.sortUsers(users, order)

	return r.applyPagination(users, pagination), nil
}

func (r *UserRepository) Save(_ context.Context, user *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if user.UpdatedAt == nil || user.UpdatedAt.IsZero() {
		user.UpdatedAt = lo.ToPtr(time.Now())
	}

	if user.ID == 0 && (user.CreatedAt == nil || user.CreatedAt.IsZero()) {
		user.CreatedAt = lo.ToPtr(time.Now())
	}

	if user.ID == 0 {
		user.ID = uint(atomic.AddUint32(&r.nextID, 1))
	}

	r.users[user.ID] = &domain.User{
		ID:            user.ID,
		Login:         user.Login,
		Email:         user.Email,
		Password:      user.Password,
		RememberToken: user.RememberToken,
		Name:          user.Name,
		CreatedAt:     user.CreatedAt,
		UpdatedAt:     user.UpdatedAt,
	}

	return nil
}

func (r *UserRepository) Delete(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.users, id)

	return nil
}

func (r *UserRepository) sortUsers(users []domain.User, order []filters.Sorting) {
	if len(order) == 0 {
		sort.Slice(users, func(i, j int) bool {
			return users[i].ID < users[j].ID
		})

		return
	}

	sort.Slice(users, func(i, j int) bool {
		for _, o := range order {
			cm := r.compareUsers(&users[i], &users[j], o.Field)
			if cm != 0 {
				if o.Direction == filters.SortDirectionDesc {
					return cm > 0
				}

				return cm < 0
			}
		}

		return false
	})
}

func (r *UserRepository) compareUsers(a, b *domain.User, field string) int {
	switch field {
	case "id":
		return cmp.Compare(a.ID, b.ID)
	case "login":
		return strings.Compare(a.Login, b.Login)
	case "email":
		return strings.Compare(a.Email, b.Email)
	case "name":
		if a.Name == nil && b.Name == nil {
			return 0
		}
		if a.Name == nil {
			return -1
		}
		if b.Name == nil {
			return 1
		}

		return strings.Compare(*a.Name, *b.Name)
	case "created_at":
		if a.CreatedAt == nil && b.CreatedAt == nil {
			return 0
		}
		if a.CreatedAt == nil {
			return -1
		}
		if b.CreatedAt == nil {
			return 1
		}
		if a.CreatedAt.Before(*b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(*b.CreatedAt) {
			return 1
		}

		return 0
	case "updated_at":
		if a.UpdatedAt == nil && b.UpdatedAt == nil {
			return 0
		}
		if a.UpdatedAt == nil {
			return -1
		}
		if b.UpdatedAt == nil {
			return 1
		}
		if a.UpdatedAt.Before(*b.UpdatedAt) {
			return -1
		}
		if a.UpdatedAt.After(*b.UpdatedAt) {
			return 1
		}

		return 0
	default:
		return 0
	}
}

func (r *UserRepository) applyPagination(users []domain.User, pagination *filters.Pagination) []domain.User {
	if pagination == nil {
		return users
	}

	limit := pagination.Limit
	if limit <= 0 {
		limit = filters.DefaultLimit
	}

	offset := max(pagination.Offset, 0)

	if offset >= len(users) {
		return []domain.User{}
	}

	end := min(offset+limit, len(users))

	return users[offset:end]
}
