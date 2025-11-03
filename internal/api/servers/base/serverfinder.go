package base

import (
	"context"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

// ServerFinder is responsible for finding servers with proper access control.
type ServerFinder struct {
	serverRepo repositories.ServerRepository
	rbac       base.RBAC
}

func NewServerFinder(
	serverRepo repositories.ServerRepository,
	rbac base.RBAC,
) *ServerFinder {
	return &ServerFinder{
		serverRepo: serverRepo,
		rbac:       rbac,
	}
}

func (f *ServerFinder) FindUserServer(ctx context.Context, user *domain.User, serverID uint) (*domain.Server, error) {
	isAdmin, err := f.rbac.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to check admin permissions")
	}

	filter := &filters.FindServer{
		IDs: []uint{serverID},
	}

	if !isAdmin {
		filter.UserIDs = []uint{user.ID}
	}

	servers, err := f.serverRepo.Find(ctx, filter, nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find server")
	}

	if len(servers) == 0 {
		return nil, api.NewNotFoundError("server not found")
	}

	return &servers[0], nil
}
