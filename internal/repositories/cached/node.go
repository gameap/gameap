package cached

import (
	"context"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

// NodeRepository wraps NodeRepository with caching.
type NodeRepository struct {
	inner      repositories.NodeRepository
	cache      cache.Cache
	wrapper    *Wrapper
	keyBuilder CacheKeyBuilder
}

// NewNodeRepository creates a new cached node repository.
func NewNodeRepository(
	inner repositories.NodeRepository, cache cache.Cache, ttl time.Duration,
) *NodeRepository {
	keyBuilder := NewDefaultKeyBuilder("node")
	config := CacheConfig{
		TTL:                ttl,
		KeyBuilder:         keyBuilder,
		InvalidateOnSave:   true,
		InvalidateOnDelete: true,
	}

	return &NodeRepository{
		inner:      inner,
		cache:      cache,
		wrapper:    NewWrapper(cache, config),
		keyBuilder: keyBuilder,
	}
}

// FindAll retrieves all nodes with optional ordering and pagination.
func (r *NodeRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Node, error) {
	key := r.keyBuilder.BuildKey("findall", order, pagination)

	data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.Node, error) {
		return r.inner.FindAll(ctx, order, pagination)
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load FindAll nodes")
	}

	return data, nil
}

// Find retrieves nodes with filters.
func (r *NodeRepository) Find(
	ctx context.Context,
	filter *filters.FindNode,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Node, error) {
	// Special case: if searching by API key (auth use case), cache it with dedicated key
	if filter != nil && filter.GDaemonAPIKey != nil {
		key := r.keyBuilder.BuildKey("apikey", *filter.GDaemonAPIKey)

		data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.Node, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})
		if err != nil {
			return nil, errors.WithMessage(err, "failed to load Find node by API key")
		}

		return data, nil
	}

	// Special case: if searching by API token (auth use case), cache it with dedicated key
	if filter != nil && filter.GDaemonAPIToken != nil {
		key := r.keyBuilder.BuildKey("apitoken", *filter.GDaemonAPIToken)

		data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.Node, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})
		if err != nil {
			return nil, errors.WithMessage(err, "failed to load Find node by API token")
		}

		return data, nil
	}

	key := r.keyBuilder.BuildKey("find", filter, order, pagination)

	data, err := GetOrLoad(ctx, r.wrapper, key, func() ([]domain.Node, error) {
		return r.inner.Find(ctx, filter, order, pagination)
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to load Find nodes")
	}

	return data, nil
}

// Save creates or updates a node and invalidates cache.
func (r *NodeRepository) Save(ctx context.Context, node *domain.Node) error {
	// Store node API credentials before save (for cache invalidation)
	var apiKey, apiToken string
	if node.GdaemonAPIKey != "" {
		apiKey = node.GdaemonAPIKey
	}
	if node.GdaemonAPIToken != nil && *node.GdaemonAPIToken != "" {
		apiToken = *node.GdaemonAPIToken
	}

	err := r.inner.Save(ctx, node)
	if err != nil {
		return errors.WithMessage(err, "failed to save node")
	}

	// The write is committed; invalidation is best-effort (a cache hiccup must
	// not report the applied write as failed). Stale entries expire with TTL.
	if apiKey != "" {
		r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("apikey", apiKey))
	}

	if apiToken != "" {
		r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("apitoken", apiToken))
	}

	r.wrapper.InvalidatePatternBestEffort(ctx, "node:find*")

	return nil
}

// Delete removes a node and invalidates cache.
func (r *NodeRepository) Delete(ctx context.Context, id uint) error {
	// Try to get the node first to invalidate its cache
	filter := &filters.FindNode{IDs: []uint{id}}
	nodes, findErr := r.inner.Find(ctx, filter, nil, nil)

	err := r.inner.Delete(ctx, id)
	if err != nil {
		return errors.WithMessage(err, "failed to delete node")
	}

	r.invalidateNodeCache(ctx, findErr, nodes)
	r.wrapper.InvalidatePatternBestEffort(ctx, "node:find*")

	return nil
}

// UpdateGDaemonAPIToken atomically rotates the daemon API token and invalidates
// the affected cache entries.
func (r *NodeRepository) UpdateGDaemonAPIToken(
	ctx context.Context, nodeID uint, hashedToken string, updatedAt time.Time,
) error {
	// Capture the previous token hash before the rotation (the inner repo still
	// holds the pre-rotation row). Find caches auth lookups under
	// apitoken:<hash>; without invalidating the old entry the superseded token
	// keeps authenticating from cache until its TTL.
	var oldToken string
	prev, findErr := r.inner.Find(ctx, &filters.FindNode{IDs: []uint{nodeID}}, nil, nil)
	if findErr == nil && len(prev) > 0 && prev[0].GdaemonAPIToken != nil {
		oldToken = *prev[0].GdaemonAPIToken
	}

	if err := r.inner.UpdateGDaemonAPIToken(ctx, nodeID, hashedToken, updatedAt); err != nil {
		return errors.WithMessage(err, "failed to update node gdaemon api token")
	}

	if oldToken != "" && oldToken != hashedToken {
		r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("apitoken", oldToken))
	}

	r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("apitoken", hashedToken))
	r.wrapper.InvalidatePatternBestEffort(ctx, "node:find*")

	return nil
}

func (r *NodeRepository) invalidateNodeCache(ctx context.Context, findErr error, nodes []domain.Node) {
	if findErr != nil || len(nodes) == 0 {
		return
	}

	node := nodes[0]
	if node.GdaemonAPIKey != "" {
		r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("apikey", node.GdaemonAPIKey))
	}
	if node.GdaemonAPIToken != nil && *node.GdaemonAPIToken != "" {
		r.wrapper.InvalidateBestEffort(ctx, r.keyBuilder.BuildKey("apitoken", *node.GdaemonAPIToken))
	}
}
