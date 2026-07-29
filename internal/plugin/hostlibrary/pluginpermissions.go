package hostlibrary

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

// PluginPermissionChecker answers whether a plugin was granted a capability.
// Host libraries that expose privileged operations consult it on every call
// so an operator revoking a grant takes effect without a panel restart.
type PluginPermissionChecker interface {
	Has(ctx context.Context, pluginID uint64, permission domain.PluginPermission) (bool, error)
}

// RepositoryPermissionChecker reads grants from the plugin's database record.
// Deliberately uncached: these calls are not on a hot path, and a stale
// "allowed" answer is worse than an extra indexed lookup.
type RepositoryPermissionChecker struct {
	repo repositories.PluginRepository
}

func NewRepositoryPermissionChecker(repo repositories.PluginRepository) *RepositoryPermissionChecker {
	return &RepositoryPermissionChecker{repo: repo}
}

func (c *RepositoryPermissionChecker) Has(
	ctx context.Context,
	pluginID uint64,
	permission domain.PluginPermission,
) (bool, error) {
	// Plugin ID 0 means the module was loaded without a database record —
	// a transient validation or dry-run load. Nothing is granted to it.
	if pluginID == 0 {
		return false, nil
	}

	plugins, err := c.repo.Find(
		ctx,
		filters.FindPluginByIDs(domain.Uint64ID(pluginID)),
		nil,
		&filters.Pagination{Limit: 1},
	)
	if err != nil {
		return false, errors.WithMessage(err, "failed to find plugin")
	}

	if len(plugins) == 0 {
		return false, nil
	}

	return plugins[0].HasPermission(permission), nil
}
