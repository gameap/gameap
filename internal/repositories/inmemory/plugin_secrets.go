package inmemory

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
)

type pluginSecretKey struct {
	pluginID domain.Uint64ID
	key      string
}

type PluginSecretRepository struct {
	mu      sync.RWMutex
	secrets map[pluginSecretKey]*domain.PluginSecret
	nextID  atomic.Uint64
}

func NewPluginSecretRepository() *PluginSecretRepository {
	return &PluginSecretRepository{
		secrets: make(map[pluginSecretKey]*domain.PluginSecret),
	}
}

func (r *PluginSecretRepository) Find(
	_ context.Context,
	filter *filters.FindPluginSecret,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.PluginSecret, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []domain.PluginSecret

	for key, secret := range r.secrets {
		if !matchPluginSecret(filter, key) {
			continue
		}

		found = append(found, *secret)
	}

	sortPluginSecrets(found, order)

	return paginatePluginSecrets(found, pagination), nil
}

func (r *PluginSecretRepository) Upsert(_ context.Context, secret *domain.PluginSecret) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	secret.UpdatedAt = &now

	key := pluginSecretKey{pluginID: secret.PluginID, key: secret.Key}

	if existing, ok := r.secrets[key]; ok {
		secret.ID = existing.ID
		secret.CreatedAt = existing.CreatedAt
	} else {
		secret.ID = r.nextID.Add(1)
		if secret.CreatedAt == nil || secret.CreatedAt.IsZero() {
			secret.CreatedAt = &now
		}
	}

	saved := *secret
	r.secrets[key] = &saved

	return nil
}

func (r *PluginSecretRepository) Delete(_ context.Context, pluginID domain.Uint64ID, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.secrets, pluginSecretKey{pluginID: pluginID, key: key})

	return nil
}

func (r *PluginSecretRepository) DeleteByPlugin(_ context.Context, pluginID domain.Uint64ID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	deleted := 0

	for key := range r.secrets {
		if key.pluginID == pluginID {
			delete(r.secrets, key)
			deleted++
		}
	}

	return deleted, nil
}

func (r *PluginSecretRepository) CountByPlugin(_ context.Context, pluginID domain.Uint64ID) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0

	for key := range r.secrets {
		if key.pluginID == pluginID {
			count++
		}
	}

	return count, nil
}

func matchPluginSecret(filter *filters.FindPluginSecret, key pluginSecretKey) bool {
	if filter == nil {
		return true
	}

	if len(filter.PluginIDs) > 0 && !slices.Contains(filter.PluginIDs, key.pluginID) {
		return false
	}

	if len(filter.Keys) > 0 && !slices.Contains(filter.Keys, key.key) {
		return false
	}

	return true
}

func sortPluginSecrets(secrets []domain.PluginSecret, order []filters.Sorting) {
	field := "key"
	direction := filters.SortDirectionAsc

	if len(order) > 0 {
		field = order[0].Field
		direction = order[0].Direction
	}

	slices.SortFunc(secrets, func(a, b domain.PluginSecret) int {
		result := comparePluginSecrets(a, b, field)
		if direction == filters.SortDirectionDesc {
			return -result
		}

		return result
	})
}

func comparePluginSecrets(a, b domain.PluginSecret, field string) int {
	if field == "id" {
		return cmp.Compare(a.ID, b.ID)
	}

	if a.PluginID != b.PluginID {
		return cmp.Compare(a.PluginID, b.PluginID)
	}

	return cmp.Compare(a.Key, b.Key)
}

func paginatePluginSecrets(secrets []domain.PluginSecret, pagination *filters.Pagination) []domain.PluginSecret {
	if pagination == nil {
		return secrets
	}

	limit := pagination.Limit
	if limit == 0 {
		limit = filters.DefaultLimit
	}

	if pagination.Offset >= uint64(len(secrets)) {
		return nil
	}

	secrets = secrets[pagination.Offset:]

	if limit < uint64(len(secrets)) {
		secrets = secrets[:limit]
	}

	return secrets
}
