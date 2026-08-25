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
//
// A broader grant answers for a narrower one (domain.PermissionSatisfied):
// a plugin holding "files" passes a "files_read" check.
type PluginPermissionChecker interface {
	Has(ctx context.Context, pluginID uint64, permission domain.PluginPermission) (bool, error)
}

// AllowAllPermissionChecker admits every check. It is wired in place of the
// real checker while PLUGIN_PERMISSIONS_ENFORCE is off, so plugins written
// before grants existed keep working while operators record theirs.
type AllowAllPermissionChecker struct{}

func (AllowAllPermissionChecker) Has(context.Context, uint64, domain.PluginPermission) (bool, error) {
	return true, nil
}

// RepositoryPermissionChecker reads grants from the plugin's database record.
// It is the source behind CachedPermissionChecker, which is what the host
// libraries and the event gate actually hold; used directly it reads the
// record on every question.
type RepositoryPermissionChecker struct {
	repo repositories.PluginRepository
}

func NewRepositoryPermissionChecker(repo repositories.PluginRepository) *RepositoryPermissionChecker {
	return &RepositoryPermissionChecker{repo: repo}
}

// Grants reads every permission the plugin holds. A plugin without a record
// holds nothing, which is not an error: an install writes the record before
// the grants, and an uninstall removes it while the module may still run.
func (c *RepositoryPermissionChecker) Grants(
	ctx context.Context,
	pluginID uint64,
) ([]domain.PluginPermission, error) {
	// Plugin ID 0 means the module was loaded without a database record —
	// a transient validation or dry-run load. Nothing is granted to it.
	if pluginID == 0 {
		return nil, nil
	}

	plugins, err := c.repo.Find(
		ctx,
		filters.FindPluginByIDs(domain.Uint64ID(pluginID)),
		nil,
		&filters.Pagination{Limit: 1},
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find plugin")
	}

	if len(plugins) == 0 {
		return nil, nil
	}

	return plugins[0].AllowedPermissions, nil
}

func (c *RepositoryPermissionChecker) Has(
	ctx context.Context,
	pluginID uint64,
	permission domain.PluginPermission,
) (bool, error) {
	permissions, err := c.Grants(ctx, pluginID)
	if err != nil {
		return false, err
	}

	return domain.PermissionSatisfied(permission, permissions), nil
}
