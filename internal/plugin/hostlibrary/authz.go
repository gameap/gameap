package hostlibrary

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/plugin/sdk/authz"
	"github.com/samber/lo"
	"github.com/tetratelabs/wazero"
)

// RBACChecker is the read-only slice of the panel's RBAC service that the
// gameap-authz module exposes. Declared here rather than importing
// *rbac.RBAC so the host library stays testable with a stub.
type RBACChecker interface {
	Can(ctx context.Context, userID uint, abilities []domain.AbilityName) (bool, error)
	CanOneOf(ctx context.Context, userID uint, abilities []domain.AbilityName) (bool, error)
	CanForEntity(
		ctx context.Context,
		userID uint,
		entityType domain.EntityType,
		entityID uint,
		abilities []domain.AbilityName,
	) (bool, error)
	CanAnyForEntity(
		ctx context.Context,
		userID uint,
		entityType domain.EntityType,
		entityID uint,
		abilities []domain.AbilityName,
	) (bool, error)
	GetRoles(ctx context.Context, userID uint) ([]string, error)
}

type AuthzServiceImpl struct {
	rbac RBACChecker
}

func NewAuthzService(rbac RBACChecker) *AuthzServiceImpl {
	return &AuthzServiceImpl{rbac: rbac}
}

func (s *AuthzServiceImpl) Can(
	ctx context.Context,
	req *authz.CanRequest,
) (*authz.CanResponse, error) {
	allowed, err := s.rbac.Can(ctx, uint(req.UserId), abilityNamesFromStrings(req.Abilities))

	return canResponse(allowed, err), nil
}

func (s *AuthzServiceImpl) CanOneOf(
	ctx context.Context,
	req *authz.CanRequest,
) (*authz.CanResponse, error) {
	allowed, err := s.rbac.CanOneOf(ctx, uint(req.UserId), abilityNamesFromStrings(req.Abilities))

	return canResponse(allowed, err), nil
}

func (s *AuthzServiceImpl) CanForEntity(
	ctx context.Context,
	req *authz.CanForEntityRequest,
) (*authz.CanResponse, error) {
	entityType, err := requiredEntityTypeFromProto(req.EntityType)
	if err != nil {
		return canResponse(false, err), nil
	}

	allowed, err := s.rbac.CanForEntity(
		ctx,
		uint(req.UserId),
		entityType,
		uint(req.EntityId),
		abilityNamesFromStrings(req.Abilities),
	)

	return canResponse(allowed, err), nil
}

func (s *AuthzServiceImpl) CanAnyForEntity(
	ctx context.Context,
	req *authz.CanForEntityRequest,
) (*authz.CanResponse, error) {
	entityType, err := requiredEntityTypeFromProto(req.EntityType)
	if err != nil {
		return canResponse(false, err), nil
	}

	allowed, err := s.rbac.CanAnyForEntity(
		ctx,
		uint(req.UserId),
		entityType,
		uint(req.EntityId),
		abilityNamesFromStrings(req.Abilities),
	)

	return canResponse(allowed, err), nil
}

func (s *AuthzServiceImpl) GetUserRoles(
	ctx context.Context,
	req *authz.GetUserRolesRequest,
) (*authz.GetUserRolesResponse, error) {
	roles, err := s.rbac.GetRoles(ctx, uint(req.UserId))
	if err != nil {
		return &authz.GetUserRolesResponse{Error: new(err.Error())}, nil
	}

	return &authz.GetUserRolesResponse{Roles: roles}, nil
}

// canResponse keeps the failure shape uniform: a failed check is never
// reported as a plain denial, so a plugin cannot mistake a database outage
// for "the user may not do this".
func canResponse(allowed bool, err error) *authz.CanResponse {
	if err != nil {
		return &authz.CanResponse{Allowed: false, Error: new(err.Error())}
	}

	return &authz.CanResponse{Allowed: allowed}
}

func abilityNamesFromStrings(names []string) []domain.AbilityName {
	return lo.Map(names, func(name string, _ int) domain.AbilityName {
		return domain.AbilityName(name)
	})
}

type AuthzHostLibrary struct {
	impl *AuthzServiceImpl
}

func NewAuthzHostLibrary(rbac RBACChecker) *AuthzHostLibrary {
	return &AuthzHostLibrary{
		impl: NewAuthzService(rbac),
	}
}

func (l *AuthzHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return authz.Instantiate(ctx, r, l.impl)
}
