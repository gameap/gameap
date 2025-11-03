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
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

type PersonalAccessTokenRepository struct {
	mu     sync.RWMutex
	tokens map[uint]*domain.PersonalAccessToken
	nextID uint32
}

func NewPersonalAccessTokenRepository() *PersonalAccessTokenRepository {
	return &PersonalAccessTokenRepository{
		tokens: make(map[uint]*domain.PersonalAccessToken),
	}
}

func (r *PersonalAccessTokenRepository) Find(
	_ context.Context,
	filter *filters.FindPersonalAccessToken,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.PersonalAccessToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == nil {
		filter = &filters.FindPersonalAccessToken{}
	}

	tokens := make([]domain.PersonalAccessToken, 0)

	idSet := make(map[uint]bool)
	for _, id := range filter.IDs {
		idSet[id] = true
	}

	tokenSet := make(map[string]bool)
	for _, t := range filter.Tokens {
		tokenSet[t] = true
	}

	tokenableTypeSet := make(map[string]bool)
	for _, tt := range filter.TokenableTypes {
		tokenableTypeSet[tt] = true
	}

	tokenableIDSet := make(map[uint]bool)
	for _, id := range filter.TokenableIDs {
		tokenableIDSet[id] = true
	}

	for _, token := range r.tokens {
		if len(filter.IDs) > 0 && !idSet[token.ID] {
			continue
		}
		if len(filter.Tokens) > 0 && !tokenSet[token.Token] {
			continue
		}
		if len(filter.TokenableTypes) > 0 && !tokenableTypeSet[string(token.TokenableType)] {
			continue
		}
		if len(filter.TokenableIDs) > 0 && !tokenableIDSet[token.TokenableID] {
			continue
		}
		tokens = append(tokens, *token)
	}

	r.sortTokens(tokens, order)

	return r.applyPagination(tokens, pagination), nil
}

func (r *PersonalAccessTokenRepository) Save(_ context.Context, token *domain.PersonalAccessToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if token.UpdatedAt == nil || token.UpdatedAt.IsZero() {
		token.UpdatedAt = lo.ToPtr(time.Now())
	}

	if token.ID == 0 && (token.CreatedAt == nil || token.CreatedAt.IsZero()) {
		token.CreatedAt = lo.ToPtr(time.Now())
	}

	if token.ID == 0 {
		r.create(token)

		return nil
	}

	return r.update(token)
}

func (r *PersonalAccessTokenRepository) Delete(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.tokens, id)

	return nil
}

func (r *PersonalAccessTokenRepository) UpdateLastUsedAt(
	_ context.Context,
	id uint,
	lastUsedAt time.Time,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	token, exists := r.tokens[id]
	if !exists {
		return nil
	}

	token.LastUsedAt = &lastUsedAt

	return nil
}

func (r *PersonalAccessTokenRepository) create(token *domain.PersonalAccessToken) {
	if token.ID == 0 {
		token.ID = uint(atomic.AddUint32(&r.nextID, 1))
	}

	r.tokens[token.ID] = &domain.PersonalAccessToken{
		ID:            token.ID,
		TokenableType: token.TokenableType,
		TokenableID:   token.TokenableID,
		Name:          token.Name,
		Token:         token.Token,
		Abilities:     token.Abilities,
		LastUsedAt:    token.LastUsedAt,
		CreatedAt:     token.CreatedAt,
		UpdatedAt:     token.UpdatedAt,
	}
}

func (r *PersonalAccessTokenRepository) update(token *domain.PersonalAccessToken) error {
	if _, exists := r.tokens[token.ID]; !exists {
		return errors.New("personal access token not found")
	}

	r.tokens[token.ID] = &domain.PersonalAccessToken{
		ID:            token.ID,
		TokenableType: token.TokenableType,
		TokenableID:   token.TokenableID,
		Name:          token.Name,
		Token:         token.Token,
		Abilities:     token.Abilities,
		LastUsedAt:    token.LastUsedAt,
		CreatedAt:     token.CreatedAt,
		UpdatedAt:     token.UpdatedAt,
	}

	return nil
}

//nolint:gocognit
func (r *PersonalAccessTokenRepository) sortTokens(tokens []domain.PersonalAccessToken, order []filters.Sorting) {
	if len(order) == 0 {
		return
	}

	sort.Slice(tokens, func(i, j int) bool {
		for _, o := range order {
			var cmpRes int
			switch o.Field {
			case "id":
				cmpRes = cmp.Compare(tokens[i].ID, tokens[j].ID)
			case "name":
				cmpRes = strings.Compare(tokens[i].Name, tokens[j].Name)
			case "tokenable_type":
				cmpRes = strings.Compare(string(tokens[i].TokenableType), string(tokens[j].TokenableType))
			case "tokenable_id":
				cmpRes = cmp.Compare(tokens[i].TokenableID, tokens[j].TokenableID)
			//nolint:gocritic
			case "last_used_at":
				if tokens[i].LastUsedAt == nil && tokens[j].LastUsedAt == nil {
					cmpRes = 0
				} else if tokens[i].LastUsedAt == nil {
					cmpRes = -1
				} else if tokens[j].LastUsedAt == nil {
					cmpRes = 1
				} else if tokens[i].LastUsedAt.Before(*tokens[j].LastUsedAt) {
					cmpRes = -1
				} else if tokens[i].LastUsedAt.After(*tokens[j].LastUsedAt) {
					cmpRes = 1
				}
			//nolint:gocritic
			case "created_at":
				if tokens[i].CreatedAt == nil && tokens[j].CreatedAt == nil {
					cmpRes = 0
				} else if tokens[i].CreatedAt == nil {
					cmpRes = -1
				} else if tokens[j].CreatedAt == nil {
					cmpRes = 1
				} else if tokens[i].CreatedAt.Before(*tokens[j].CreatedAt) {
					cmpRes = -1
				} else if tokens[i].CreatedAt.After(*tokens[j].CreatedAt) {
					cmpRes = 1
				}
			//nolint:gocritic
			case "updated_at":
				if tokens[i].UpdatedAt == nil && tokens[j].UpdatedAt == nil {
					cmpRes = 0
				} else if tokens[i].UpdatedAt == nil {
					cmpRes = -1
				} else if tokens[j].UpdatedAt == nil {
					cmpRes = 1
				} else if tokens[i].UpdatedAt.Before(*tokens[j].UpdatedAt) {
					cmpRes = -1
				} else if tokens[i].UpdatedAt.After(*tokens[j].UpdatedAt) {
					cmpRes = 1
				}
			default:
				continue
			}

			if cmpRes != 0 {
				if o.Direction == filters.SortDirectionDesc {
					return cmpRes > 0
				}

				return cmpRes < 0
			}
		}

		return false
	})
}

func (r *PersonalAccessTokenRepository) applyPagination(
	tokens []domain.PersonalAccessToken,
	pagination *filters.Pagination,
) []domain.PersonalAccessToken {
	if pagination == nil {
		return tokens
	}

	start := pagination.Offset
	if start > len(tokens) {
		return []domain.PersonalAccessToken{}
	}

	end := min(start+pagination.Limit, len(tokens))

	return tokens[start:end]
}
