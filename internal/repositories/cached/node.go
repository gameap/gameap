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

	_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
		return r.inner.FindAll(ctx, order, pagination)
	})

	if err != nil {
		return nil, errors.WithMessage(err, "failed to get or set cache for FindAll nodes")
	}

	data, err := cache.GetTyped[[]domain.Node](ctx, r.cache, key)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get typed cached data for FindAll nodes")
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

		_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})

		if err != nil {
			return nil, errors.WithMessage(err, "failed to get or set cache for Find node by API key")
		}

		data, err := cache.GetTyped[[]domain.Node](ctx, r.cache, key)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get typed cached data for Find node by API key")
		}

		return data, nil
	}

	// Special case: if searching by API token (auth use case), cache it with dedicated key
	if filter != nil && filter.GDaemonAPIToken != nil {
		key := r.keyBuilder.BuildKey("apitoken", *filter.GDaemonAPIToken)

		_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
			return r.inner.Find(ctx, filter, order, pagination)
		})

		if err != nil {
			return nil, errors.WithMessage(err, "failed to get or set cache for Find node by API token")
		}

		data, err := cache.GetTyped[[]domain.Node](ctx, r.cache, key)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get typed cached data for Find node by API token")
		}

		return data, nil
	}

	key := r.keyBuilder.BuildKey("find", filter, order, pagination)

	_, err := r.wrapper.GetOrSet(ctx, key, func() (any, error) {
		return r.inner.Find(ctx, filter, order, pagination)
	})

	if err != nil {
		return nil, errors.WithMessage(err, "failed to get or set cache for Find nodes")
	}

	data, err := cache.GetTyped[[]domain.Node](ctx, r.cache, key)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get typed cached data for Find nodes")
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

	// Invalidate cache for this node's API key
	if apiKey != "" {
		if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("apikey", apiKey)); err != nil {
			return errors.WithMessage(err, "failed to invalidate node cache by API key after save")
		}
	}

	// Invalidate cache for this node's API token
	if apiToken != "" {
		if err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("apitoken", apiToken)); err != nil {
			return errors.WithMessage(err, "failed to invalidate node cache by API token after save")
		}
	}

	if err := r.wrapper.InvalidatePattern(ctx, "node:find*"); err != nil {
		return errors.WithMessage(err, "failed to invalidate node find pattern cache after save")
	}

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

	if err := r.invalidateNodeCache(ctx, findErr, nodes); err != nil {
		return err
	}

	if err := r.wrapper.InvalidatePattern(ctx, "node:find*"); err != nil {
		return errors.WithMessage(err, "failed to invalidate node find pattern cache after delete")
	}

	return nil
}

func (r *NodeRepository) invalidateNodeCache(ctx context.Context, findErr error, nodes []domain.Node) error {
	if findErr != nil {
		// Unable to find node for cache invalidation, but this shouldn't fail the delete
		return nil //nolint:nilerr
	}

	if len(nodes) == 0 {
		return nil
	}

	node := nodes[0]
	if node.GdaemonAPIKey != "" {
		err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("apikey", node.GdaemonAPIKey))
		if err != nil {
			return errors.WithMessage(err, "failed to invalidate node cache by API key after delete")
		}
	}
	if node.GdaemonAPIToken != nil && *node.GdaemonAPIToken != "" {
		err := r.wrapper.Invalidate(ctx, r.keyBuilder.BuildKey("apitoken", *node.GdaemonAPIToken))
		if err != nil {
			return errors.WithMessage(err, "failed to invalidate node cache by API token after delete")
		}
	}

	return nil
}
