package hostlibrary

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/rbac"
	"github.com/samber/lo"
	"github.com/tetratelabs/wazero"
)

// RBACManager is the write-capable slice of the panel's RBAC service. Going
// through the service (rather than straight to the repository) keeps its
// permission cache coherent and reuses the panel's own semantics, such as
// forbidding an ability that a role still grants.
type RBACManager interface {
	SetRolesToUser(ctx context.Context, userID uint, roleNames []string) error
	AllowUserAbilitiesForEntity(
		ctx context.Context,
		userID uint,
		entityID uint,
		entityType domain.EntityType,
		abilityNames []domain.AbilityName,
	) error
	RevokeOrForbidUserAbilitiesForEntity(
		ctx context.Context,
		userID uint,
		entityID uint,
		entityType domain.EntityType,
		abilityNames []domain.AbilityName,
	) error

	InvalidateUserCache(userID uint)
	InvalidateCache()
}

type RBACServiceImpl struct {
	pluginID uint64
	rbac     RBACManager
	repo     repositories.RBACRepository
	guard    *PluginGuard
}

func NewRBACService(
	pluginID uint64,
	manager RBACManager,
	repo repositories.RBACRepository,
	guard *PluginGuard,
) *RBACServiceImpl {
	return &RBACServiceImpl{
		pluginID: pluginID,
		rbac:     manager,
		repo:     repo,
		guard:    guard,
	}
}

// authorize gates every method of this module on the manage_rbac grant and
// the rbac rate limit. A denial and a failed check are both reported as "not
// allowed" with a message; the caller must not proceed in either case.
func (s *RBACServiceImpl) authorize(ctx context.Context, export string) (bool, string) {
	if msg := s.guard.Check(ctx, ModuleRBAC, export); msg != "" {
		return false, msg
	}

	return true, ""
}

// auditEntityWrite records a grant or revocation on an entity (user, server,
// ...) with the plugin as the actor.
func (s *RBACServiceImpl) auditEntityWrite(
	ctx context.Context,
	eventType audit.EventType,
	action string,
	entityType domain.EntityType,
	entityID uint64,
	err error,
	attrs ...slog.Attr,
) {
	s.guard.Audit(ctx, eventType, action, string(entityType), strconv.FormatUint(entityID, 10), err, attrs...)
}

// ===============================
// Service level
// ===============================

func (s *RBACServiceImpl) SetUserRoles(
	ctx context.Context,
	req *rbac.SetUserRolesRequest,
) (*rbac.Result, error) {
	if allowed, msg := s.authorize(ctx, "set_user_roles"); !allowed {
		return failure(msg), nil
	}

	err := s.rbac.SetRolesToUser(ctx, uint(req.UserId), req.RoleNames)
	s.auditEntityWrite(ctx, audit.EventPluginRBACGrant, "set_user_roles", domain.EntityTypeUser, req.UserId, err,
		slog.Any("roles", req.RoleNames))

	return resultFromError(err), nil
}

func (s *RBACServiceImpl) AllowUserAbilitiesForEntity(
	ctx context.Context,
	req *rbac.UserAbilitiesRequest,
) (*rbac.Result, error) {
	if allowed, msg := s.authorize(ctx, "allow_user_abilities_for_entity"); !allowed {
		return failure(msg), nil
	}

	entityType, err := requiredEntityTypeFromProto(req.EntityType)
	if err != nil {
		return failure(err.Error()), nil
	}

	err = s.rbac.AllowUserAbilitiesForEntity(
		ctx,
		uint(req.UserId),
		uint(req.EntityId),
		entityType,
		abilityNamesFromStrings(req.Abilities),
	)
	s.auditEntityWrite(ctx, audit.EventPluginRBACGrant, "allow_user_abilities", entityType, req.EntityId, err,
		slog.Uint64("user_id", req.UserId), slog.Any("abilities", req.Abilities))

	return resultFromError(err), nil
}

func (s *RBACServiceImpl) RevokeOrForbidUserAbilitiesForEntity(
	ctx context.Context,
	req *rbac.UserAbilitiesRequest,
) (*rbac.Result, error) {
	if allowed, msg := s.authorize(ctx, "revoke_or_forbid_user_abilities_for_entity"); !allowed {
		return failure(msg), nil
	}

	entityType, err := requiredEntityTypeFromProto(req.EntityType)
	if err != nil {
		return failure(err.Error()), nil
	}

	err = s.rbac.RevokeOrForbidUserAbilitiesForEntity(
		ctx,
		uint(req.UserId),
		uint(req.EntityId),
		entityType,
		abilityNamesFromStrings(req.Abilities),
	)
	s.auditEntityWrite(ctx, audit.EventPluginRBACRevoke, "revoke_user_abilities", entityType, req.EntityId, err,
		slog.Uint64("user_id", req.UserId), slog.Any("abilities", req.Abilities))

	return resultFromError(err), nil
}

// ===============================
// Repository level
// ===============================

func (s *RBACServiceImpl) GetRoles(
	ctx context.Context,
	_ *rbac.GetRolesRequest,
) (*rbac.GetRolesResponse, error) {
	if allowed, msg := s.authorize(ctx, "get_roles"); !allowed {
		return &rbac.GetRolesResponse{Error: new(msg)}, nil
	}

	roles, err := s.repo.GetRoles(ctx)
	if err != nil {
		return &rbac.GetRolesResponse{Error: new(err.Error())}, nil
	}

	return &rbac.GetRolesResponse{
		Roles: lo.Map(roles, func(role domain.Role, _ int) *rbac.Role {
			return roleToProto(role)
		}),
	}, nil
}

func (s *RBACServiceImpl) SaveRole(
	ctx context.Context,
	req *rbac.SaveRoleRequest,
) (*rbac.SaveRoleResponse, error) {
	if allowed, msg := s.authorize(ctx, "save_role"); !allowed {
		return &rbac.SaveRoleResponse{Success: false, Error: new(msg)}, nil
	}

	if req.Role == nil || req.Role.Name == "" {
		return &rbac.SaveRoleResponse{Success: false, Error: new("role name is required")}, nil
	}

	role := roleFromProto(req.Role)

	now := time.Now()
	if role.ID == 0 {
		role.CreatedAt = &now
	}
	role.UpdatedAt = &now

	err := s.repo.SaveRole(ctx, &role)
	s.guard.Audit(ctx, audit.EventPluginRBACRole, "save", "role", strconv.FormatUint(uint64(role.ID), 10), err,
		slog.String("role", role.Name))

	if err != nil {
		return &rbac.SaveRoleResponse{Success: false, Error: new(err.Error())}, nil
	}

	// A renamed or re-levelled role can change what its holders may do.
	s.rbac.InvalidateCache()

	return &rbac.SaveRoleResponse{Success: true, Role: roleToProto(role)}, nil
}

func (s *RBACServiceImpl) DeleteRole(
	ctx context.Context,
	req *rbac.DeleteRoleRequest,
) (*rbac.Result, error) {
	if allowed, msg := s.authorize(ctx, "delete_role"); !allowed {
		return failure(msg), nil
	}

	err := s.repo.DeleteRole(ctx, uint(req.Id))
	if err == nil {
		s.rbac.InvalidateCache()
	}

	s.guard.Audit(ctx, audit.EventPluginRBACRole, "delete", "role", strconv.FormatUint(req.Id, 10), err)

	return resultFromError(err), nil
}

func (s *RBACServiceImpl) GetPermissions(
	ctx context.Context,
	req *rbac.EntityRequest,
) (*rbac.GetPermissionsResponse, error) {
	if allowed, msg := s.authorize(ctx, "get_permissions"); !allowed {
		return &rbac.GetPermissionsResponse{Error: new(msg)}, nil
	}

	entityType, err := requiredEntityTypeFromProto(req.EntityType)
	if err != nil {
		return &rbac.GetPermissionsResponse{Error: new(err.Error())}, nil
	}

	permissions, err := s.repo.GetPermissions(ctx, uint(req.EntityId), entityType)
	if err != nil {
		return &rbac.GetPermissionsResponse{Error: new(err.Error())}, nil
	}

	return &rbac.GetPermissionsResponse{
		Permissions: lo.Map(permissions, func(permission domain.Permission, _ int) *rbac.Permission {
			return permissionToProto(permission)
		}),
	}, nil
}

func (s *RBACServiceImpl) GetRolesForEntity(
	ctx context.Context,
	req *rbac.EntityRequest,
) (*rbac.GetRolesForEntityResponse, error) {
	if allowed, msg := s.authorize(ctx, "get_roles_for_entity"); !allowed {
		return &rbac.GetRolesForEntityResponse{Error: new(msg)}, nil
	}

	entityType, err := requiredEntityTypeFromProto(req.EntityType)
	if err != nil {
		return &rbac.GetRolesForEntityResponse{Error: new(err.Error())}, nil
	}

	roles, err := s.repo.GetRolesForEntity(ctx, uint(req.EntityId), entityType)
	if err != nil {
		return &rbac.GetRolesForEntityResponse{Error: new(err.Error())}, nil
	}

	return &rbac.GetRolesForEntityResponse{
		Roles: lo.Map(roles, func(role domain.RestrictedRole, _ int) *rbac.RestrictedRole {
			return restrictedRoleToProto(role)
		}),
	}, nil
}

func (s *RBACServiceImpl) AssignRolesForEntity(
	ctx context.Context,
	req *rbac.AssignRolesRequest,
) (*rbac.Result, error) {
	if allowed, msg := s.authorize(ctx, "assign_roles_for_entity"); !allowed {
		return failure(msg), nil
	}

	entityType, err := requiredEntityTypeFromProto(req.EntityType)
	if err != nil {
		return failure(err.Error()), nil
	}

	roles := lo.Map(req.Roles, func(role *rbac.RestrictedRole, _ int) domain.RestrictedRole {
		return restrictedRoleFromProto(role)
	})

	err = s.repo.AssignRolesForEntity(ctx, uint(req.EntityId), entityType, roles)
	s.invalidateAfterWrite(err, entityType, uint(req.EntityId))
	s.auditEntityWrite(ctx, audit.EventPluginRBACGrant, "assign_roles", entityType, req.EntityId, err,
		slog.Int("roles", len(roles)))

	return resultFromError(err), nil
}

func (s *RBACServiceImpl) ClearRolesForEntity(
	ctx context.Context,
	req *rbac.EntityRequest,
) (*rbac.Result, error) {
	if allowed, msg := s.authorize(ctx, "clear_roles_for_entity"); !allowed {
		return failure(msg), nil
	}

	entityType, err := requiredEntityTypeFromProto(req.EntityType)
	if err != nil {
		return failure(err.Error()), nil
	}

	err = s.repo.ClearRolesForEntity(ctx, uint(req.EntityId), entityType)
	s.invalidateAfterWrite(err, entityType, uint(req.EntityId))
	s.auditEntityWrite(ctx, audit.EventPluginRBACRevoke, "clear_roles", entityType, req.EntityId, err)

	return resultFromError(err), nil
}

func (s *RBACServiceImpl) Allow(ctx context.Context, req *rbac.AbilitiesRequest) (*rbac.Result, error) {
	return s.applyAbilities(ctx, "allow", audit.EventPluginRBACGrant, req, s.repo.Allow)
}

func (s *RBACServiceImpl) Forbid(ctx context.Context, req *rbac.AbilitiesRequest) (*rbac.Result, error) {
	return s.applyAbilities(ctx, "forbid", audit.EventPluginRBACRevoke, req, s.repo.Forbid)
}

func (s *RBACServiceImpl) Revoke(ctx context.Context, req *rbac.AbilitiesRequest) (*rbac.Result, error) {
	return s.applyAbilities(ctx, "revoke", audit.EventPluginRBACRevoke, req, s.repo.Revoke)
}

type abilitiesFunc func(context.Context, uint, domain.EntityType, []domain.Ability) error

func (s *RBACServiceImpl) applyAbilities(
	ctx context.Context,
	export string,
	eventType audit.EventType,
	req *rbac.AbilitiesRequest,
	apply abilitiesFunc,
) (*rbac.Result, error) {
	if allowed, msg := s.authorize(ctx, export); !allowed {
		return failure(msg), nil
	}

	entityType, err := requiredEntityTypeFromProto(req.EntityType)
	if err != nil {
		return failure(err.Error()), nil
	}

	abilities := lo.Map(req.Abilities, func(ability *rbac.Ability, _ int) domain.Ability {
		return abilityFromProto(ability)
	})

	err = apply(ctx, uint(req.EntityId), entityType, abilities)
	s.invalidateAfterWrite(err, entityType, uint(req.EntityId))
	s.auditEntityWrite(ctx, eventType, export, entityType, req.EntityId, err,
		slog.Int("abilities", len(abilities)))

	return resultFromError(err), nil
}

// invalidateAfterWrite keeps the RBAC service's permission cache in step with
// writes that bypassed it. A change to one user only affects that user; a
// change to a role affects everyone holding it, so the whole cache goes.
func (s *RBACServiceImpl) invalidateAfterWrite(err error, entityType domain.EntityType, entityID uint) {
	if err != nil {
		return
	}

	if entityType == domain.EntityTypeUser {
		s.rbac.InvalidateUserCache(entityID)

		return
	}

	s.rbac.InvalidateCache()
}

// ===============================
// Conversions
// ===============================

func roleToProto(role domain.Role) *rbac.Role {
	return &rbac.Role{
		Id:    uint64(role.ID),
		Name:  role.Name,
		Title: role.Title,
		Level: uint64PtrFromUintPtr(role.Level),
		Scope: int32PtrFromIntPtr(role.Scope),
	}
}

func roleFromProto(role *rbac.Role) domain.Role {
	if role == nil {
		return domain.Role{}
	}

	return domain.Role{
		ID:    uint(role.Id),
		Name:  role.Name,
		Title: role.Title,
		Level: uintPtrFromUint64Ptr(role.Level),
		Scope: intPtrFromInt32Ptr(role.Scope),
	}
}

func restrictedRoleToProto(role domain.RestrictedRole) *rbac.RestrictedRole {
	return &rbac.RestrictedRole{
		Role:             roleToProto(role.Role),
		RestrictedToType: optionalEntityTypeToProto(role.RestrictedToType),
		RestrictedToId:   uint64PtrFromUintPtr(role.RestrictedToID),
	}
}

func restrictedRoleFromProto(role *rbac.RestrictedRole) domain.RestrictedRole {
	if role == nil {
		return domain.RestrictedRole{}
	}

	return domain.RestrictedRole{
		Role:             roleFromProto(role.Role),
		RestrictedToType: optionalEntityTypeFromProto(role.RestrictedToType),
		RestrictedToID:   uintPtrFromUint64Ptr(role.RestrictedToId),
	}
}

func abilityToProto(ability *domain.Ability) *rbac.Ability {
	if ability == nil {
		return nil
	}

	return &rbac.Ability{
		Name:       string(ability.Name),
		Title:      ability.Title,
		EntityType: optionalEntityTypeToProto(ability.EntityType),
		EntityId:   uint64PtrFromUintPtr(ability.EntityID),
		OnlyOwned:  ability.OnlyOwned,
		Scope:      int32PtrFromIntPtr(ability.Scope),
	}
}

func abilityFromProto(ability *rbac.Ability) domain.Ability {
	if ability == nil {
		return domain.Ability{}
	}

	now := time.Now()

	return domain.Ability{
		Name:       domain.AbilityName(ability.Name),
		Title:      ability.Title,
		EntityType: optionalEntityTypeFromProto(ability.EntityType),
		EntityID:   uintPtrFromUint64Ptr(ability.EntityId),
		OnlyOwned:  ability.OnlyOwned,
		Scope:      intPtrFromInt32Ptr(ability.Scope),
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
}

func permissionToProto(permission domain.Permission) *rbac.Permission {
	return &rbac.Permission{
		Id:         uint64(permission.ID),
		AbilityId:  uint64(permission.AbilityID),
		EntityType: optionalEntityTypeToProto(permission.EntityType),
		EntityId:   uint64PtrFromUintPtr(permission.EntityID),
		Forbidden:  permission.Forbidden,
		Ability:    abilityToProto(permission.Ability),
	}
}

func failure(message string) *rbac.Result {
	return &rbac.Result{Success: false, Error: new(message)}
}

func resultFromError(err error) *rbac.Result {
	if err != nil {
		return failure(err.Error())
	}

	return &rbac.Result{Success: true}
}

// ===============================
// Host library
// ===============================

type RBACHostLibrary struct {
	impl *RBACServiceImpl
}

func NewRBACHostLibrary(
	pluginID uint64,
	manager RBACManager,
	repo repositories.RBACRepository,
	guard *PluginGuard,
) *RBACHostLibrary {
	return &RBACHostLibrary{
		impl: NewRBACService(pluginID, manager, repo, guard),
	}
}

func (l *RBACHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return rbac.Instantiate(ctx, r, l.impl)
}

type RBACHostLibraryFactory struct {
	manager RBACManager
	repo    repositories.RBACRepository
	guard   *Guard
}

func NewRBACHostLibraryFactory(
	manager RBACManager,
	repo repositories.RBACRepository,
	guard *Guard,
) *RBACHostLibraryFactory {
	return &RBACHostLibraryFactory{
		manager: manager,
		repo:    repo,
		guard:   guard,
	}
}

func (f *RBACHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return NewRBACHostLibrary(pluginID, f.manager, f.repo, f.guard.For(pluginID))
}
