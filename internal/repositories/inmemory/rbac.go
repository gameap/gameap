package inmemory

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/gameap/gameap/internal/domain"
	"github.com/samber/lo"
)

type RBACRepository struct {
	mu               sync.RWMutex
	roles            map[uint]*domain.Role
	assignedRoles    map[uint]*domain.AssignedRole
	abilities        map[uint]*domain.Ability
	permissions      map[uint]*domain.Permission
	nextRoleID       uint32
	nextAssignedID   uint32
	nextAbilityID    uint32
	nextPermissionID uint32
}

func NewRBACRepository() *RBACRepository {
	return &RBACRepository{
		roles:         make(map[uint]*domain.Role),
		assignedRoles: make(map[uint]*domain.AssignedRole),
		abilities:     make(map[uint]*domain.Ability),
		permissions:   make(map[uint]*domain.Permission),
	}
}

func (r *RBACRepository) GetRoles(_ context.Context) ([]domain.Role, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	roles := make([]domain.Role, 0, len(r.roles))

	for _, role := range r.roles {
		roles = append(roles, *role)
	}

	return roles, nil
}

func (r *RBACRepository) GetRolesForEntity(
	_ context.Context,
	entityID uint,
	entityType domain.EntityType,
) ([]domain.RestrictedRole, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var roles []domain.RestrictedRole

	// Find all assigned roles for the entity
	for _, assignedRole := range r.assignedRoles {
		if assignedRole.EntityID == entityID && assignedRole.EntityType == entityType {
			// Find the corresponding role
			if role, exists := r.roles[assignedRole.RoleID]; exists {
				restrictedRole := domain.RestrictedRole{
					Role:             *role,
					RestrictedToID:   assignedRole.RestrictedToID,
					RestrictedToType: assignedRole.RestrictedToType,
				}
				roles = append(roles, restrictedRole)
			}
		}
	}

	return roles, nil
}

func (r *RBACRepository) GetPermissions(
	_ context.Context,
	entityID uint,
	entityType domain.EntityType,
) ([]domain.Permission, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var permissions []domain.Permission

	// Find all permissions for the entity
	for _, permission := range r.permissions {
		if permission.EntityID != nil && *permission.EntityID == entityID &&
			permission.EntityType != nil && *permission.EntityType == entityType {
			// Create a copy of the permission
			permissionCopy := *permission

			// Find and attach the corresponding ability
			if ability, exists := r.abilities[permission.AbilityID]; exists {
				abilityCopy := *ability
				permissionCopy.Ability = &abilityCopy
			}

			permissions = append(permissions, permissionCopy)
		}
	}

	return permissions, nil
}

func (r *RBACRepository) SaveRole(_ context.Context, role *domain.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if role.ID == 0 {
		role.ID = uint(atomic.AddUint32(&r.nextRoleID, 1))
	}

	r.roles[role.ID] = &domain.Role{
		ID:        role.ID,
		Name:      role.Name,
		Title:     role.Title,
		Level:     role.Level,
		Scope:     role.Scope,
		CreatedAt: role.CreatedAt,
		UpdatedAt: role.UpdatedAt,
	}

	return nil
}

func (r *RBACRepository) SaveAssignedRole(_ context.Context, assignedRole *domain.AssignedRole) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if assignedRole.ID == 0 {
		assignedRole.ID = uint(atomic.AddUint32(&r.nextAssignedID, 1))
	}

	r.assignedRoles[assignedRole.ID] = &domain.AssignedRole{
		ID:               assignedRole.ID,
		RoleID:           assignedRole.RoleID,
		EntityID:         assignedRole.EntityID,
		EntityType:       assignedRole.EntityType,
		RestrictedToID:   assignedRole.RestrictedToID,
		RestrictedToType: assignedRole.RestrictedToType,
		Scope:            assignedRole.Scope,
	}

	return nil
}

func (r *RBACRepository) DeleteRole(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.roles, id)

	return nil
}

func (r *RBACRepository) DeleteAssignedRole(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.assignedRoles, id)

	return nil
}

func (r *RBACRepository) SaveAbility(_ context.Context, ability *domain.Ability) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ability.ID == 0 {
		ability.ID = uint(atomic.AddUint32(&r.nextAbilityID, 1))
	}

	r.abilities[ability.ID] = &domain.Ability{
		ID:         ability.ID,
		Name:       ability.Name,
		Title:      ability.Title,
		EntityID:   ability.EntityID,
		EntityType: ability.EntityType,
		OnlyOwned:  ability.OnlyOwned,
		Options:    ability.Options,
		Scope:      ability.Scope,
		CreatedAt:  ability.CreatedAt,
		UpdatedAt:  ability.UpdatedAt,
	}

	return nil
}

func (r *RBACRepository) SavePermission(_ context.Context, permission *domain.Permission) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if permission.ID == 0 {
		permission.ID = uint(atomic.AddUint32(&r.nextPermissionID, 1))
	}

	r.permissions[permission.ID] = &domain.Permission{
		ID:         permission.ID,
		AbilityID:  permission.AbilityID,
		EntityID:   permission.EntityID,
		EntityType: permission.EntityType,
		Forbidden:  permission.Forbidden,
		Scope:      permission.Scope,
	}

	return nil
}

func (r *RBACRepository) AssignAbilityToUser(_ context.Context, userID uint, abilityID uint) error {
	permission := &domain.Permission{
		AbilityID:  abilityID,
		EntityID:   &userID,
		EntityType: lo.ToPtr(domain.EntityTypeUser),
		Forbidden:  false,
	}

	return r.SavePermission(context.Background(), permission)
}

func (r *RBACRepository) AssignRolesForEntity(
	_ context.Context,
	entityID uint,
	entityType domain.EntityType,
	roles []domain.RestrictedRole,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create new role assignments
	for _, role := range roles {
		// Check if this exact assignment already exists
		exists := false
		for _, assignedRole := range r.assignedRoles {
			if assignedRole.EntityID == entityID &&
				assignedRole.EntityType == entityType &&
				assignedRole.RoleID == role.ID &&
				((assignedRole.RestrictedToID == nil && role.RestrictedToID == nil) ||
					(assignedRole.RestrictedToID != nil && role.RestrictedToID != nil &&
						*assignedRole.RestrictedToID == *role.RestrictedToID)) &&
				((assignedRole.RestrictedToType == nil && role.RestrictedToType == nil) ||
					(assignedRole.RestrictedToType != nil && role.RestrictedToType != nil &&
						*assignedRole.RestrictedToType == *role.RestrictedToType)) {
				exists = true

				break
			}
		}

		if !exists {
			assignedRole := &domain.AssignedRole{
				ID:               uint(atomic.AddUint32(&r.nextAssignedID, 1)),
				RoleID:           role.ID,
				EntityID:         entityID,
				EntityType:       entityType,
				RestrictedToID:   role.RestrictedToID,
				RestrictedToType: role.RestrictedToType,
				Scope:            role.Scope,
			}
			r.assignedRoles[assignedRole.ID] = assignedRole
		}
	}

	return nil
}

func (r *RBACRepository) ClearRolesForEntity(
	_ context.Context,
	entityID uint,
	entityType domain.EntityType,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Delete all role assignments for this entity
	for id, assignedRole := range r.assignedRoles {
		if assignedRole.EntityID == entityID && assignedRole.EntityType == entityType {
			delete(r.assignedRoles, id)
		}
	}

	return nil
}

func (r *RBACRepository) Allow(
	_ context.Context,
	entityID uint,
	entityType domain.EntityType,
	abilities []domain.Ability,
) error {
	return r.applyAbilities(entityID, entityType, abilities, false)
}

func (r *RBACRepository) Forbid(
	_ context.Context,
	entityID uint,
	entityType domain.EntityType,
	abilities []domain.Ability,
) error {
	return r.applyAbilities(entityID, entityType, abilities, true)
}

func (r *RBACRepository) Revoke(
	_ context.Context,
	entityID uint,
	entityType domain.EntityType,
	abilities []domain.Ability,
) error {
	if len(abilities) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Find ability IDs that match the input abilities
	abilityIDs := r.findAbilityIDs(abilities)
	if len(abilityIDs) == 0 {
		return nil
	}

	// Delete permissions matching the criteria
	for id, permission := range r.permissions {
		if permission.EntityID != nil && *permission.EntityID == entityID &&
			permission.EntityType != nil && *permission.EntityType == entityType {
			// Check if this permission's ability is in our list
			if slices.Contains(abilityIDs, permission.AbilityID) {
				delete(r.permissions, id)
			}
		}
	}

	return nil
}

func (r *RBACRepository) applyAbilities(
	entityID uint,
	entityType domain.EntityType,
	abilities []domain.Ability,
	forbidden bool,
) error {
	if len(abilities) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Step 1: Save abilities (create if they don't exist)
	for _, ability := range abilities {
		r.saveOrGetAbility(&ability)
	}

	// Step 2: Find ability IDs
	abilityIDs := r.findAbilityIDs(abilities)
	if len(abilityIDs) == 0 {
		return nil
	}

	// Step 3: Create permissions
	for _, abilityID := range abilityIDs {
		permission := &domain.Permission{
			ID:         uint(atomic.AddUint32(&r.nextPermissionID, 1)),
			AbilityID:  abilityID,
			EntityID:   &entityID,
			EntityType: &entityType,
			Forbidden:  forbidden,
			Scope:      nil,
		}
		r.permissions[permission.ID] = permission
	}

	return nil
}

// saveOrGetAbility saves an ability if it doesn't exist, or returns existing one.
func (r *RBACRepository) saveOrGetAbility(ability *domain.Ability) uint {
	// Check if ability already exists
	for _, existingAbility := range r.abilities {
		if r.abilitiesMatch(existingAbility, ability) {
			return existingAbility.ID
		}
	}

	// Create new ability
	newAbility := &domain.Ability{
		ID:         uint(atomic.AddUint32(&r.nextAbilityID, 1)),
		Name:       ability.Name,
		Title:      ability.Title,
		EntityID:   ability.EntityID,
		EntityType: ability.EntityType,
		OnlyOwned:  ability.OnlyOwned,
		Options:    ability.Options,
		Scope:      ability.Scope,
		CreatedAt:  ability.CreatedAt,
		UpdatedAt:  ability.UpdatedAt,
	}
	r.abilities[newAbility.ID] = newAbility

	return newAbility.ID
}

// findAbilityIDs finds IDs of abilities matching the input abilities.
func (r *RBACRepository) findAbilityIDs(abilities []domain.Ability) []uint {
	var abilityIDs []uint

	for _, ability := range abilities {
		for _, existingAbility := range r.abilities {
			if r.abilitiesMatch(existingAbility, &ability) {
				abilityIDs = append(abilityIDs, existingAbility.ID)

				break
			}
		}
	}

	return abilityIDs
}

// abilitiesMatch checks if two abilities match based on their unique key.
func (r *RBACRepository) abilitiesMatch(a, b *domain.Ability) bool {
	if a.Name != b.Name {
		return false
	}

	// Match entity_id
	if (a.EntityID == nil) != (b.EntityID == nil) {
		return false
	}

	if a.EntityID != nil && b.EntityID != nil && *a.EntityID != *b.EntityID {
		return false
	}

	// Match entity_type
	if (a.EntityType == nil) != (b.EntityType == nil) {
		return false
	}

	if a.EntityType != nil && b.EntityType != nil && *a.EntityType != *b.EntityType {
		return false
	}

	// Match scope
	if (a.Scope == nil) != (b.Scope == nil) {
		return false
	}

	if a.Scope != nil && b.Scope != nil && *a.Scope != *b.Scope {
		return false
	}

	return true
}
