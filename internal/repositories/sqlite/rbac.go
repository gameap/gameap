package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/samber/lo"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

var (
	roleFieldsWithAlias       = addAliasToFields(base.RoleFields, "r")
	abilityFieldsWithAlias    = addAliasToFields(base.AbilityFields, "a")
	permissionFieldsWithAlias = addAliasToFields(base.PermissionFields, "p")
)

type RBACRepository struct {
	db base.DB
	tm base.TransactionManager
}

func NewRBACRepository(db base.DB, tm base.TransactionManager) *RBACRepository {
	return &RBACRepository{
		db: db,
		tm: tm,
	}
}

func (r *RBACRepository) GetRoles(ctx context.Context) ([]domain.Role, error) {
	query, args, err := sq.Select(base.RoleFields...).
		From(base.RolesTable).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build query")
	}

	rows, err := r.db.QueryContext(ctx, query, args...) //nolint:sqlclosecheck
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.ErrorContext(ctx, "failed to close rows stream", "query", query, "err", err)
		}
	}(rows)

	var roles []domain.Role

	for rows.Next() {
		var role domain.Role

		err = rows.Scan(
			&role.ID,
			&role.Name,
			&role.Title,
			&role.Level,
			&role.Scope,
			&role.CreatedAt,
			&role.UpdatedAt,
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan row")
		}

		roles = append(roles, role)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.Wrap(err, "rows iteration error")
	}

	return roles, nil
}

func (r *RBACRepository) GetRolesForEntity(
	ctx context.Context,
	entityID uint,
	entityType domain.EntityType,
) ([]domain.RestrictedRole, error) {
	selectFields := append(roleFieldsWithAlias, "ar.restricted_to_id", "ar.restricted_to_type") //nolint:gocritic

	query, args, err := sq.Select(selectFields...).
		From(base.RolesTable + " r").
		Join(base.AssignedRolesTable + " ar ON r.id = ar.role_id").
		Where(sq.Eq{"ar.entity_id": entityID, "ar.entity_type": entityType}).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build query")
	}

	rows, err := r.db.QueryContext(ctx, query, args...) //nolint:sqlclosecheck
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.ErrorContext(ctx, "failed to close rows stream", "query", query, "err", err)
		}
	}(rows)

	var roles []domain.RestrictedRole

	for rows.Next() {
		var role domain.RestrictedRole

		err = rows.Scan(
			&role.ID,
			&role.Name,
			&role.Title,
			&role.Level,
			&role.Scope,
			&role.CreatedAt,
			&role.UpdatedAt,
			&role.RestrictedToID,
			&role.RestrictedToType,
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan row")
		}

		roles = append(roles, role)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.Wrap(err, "rows iteration error")
	}

	return roles, nil
}

func (r *RBACRepository) GetPermissions(
	ctx context.Context,
	entityID uint,
	entityType domain.EntityType,
) ([]domain.Permission, error) {
	selectFields := append(permissionFieldsWithAlias, abilityFieldsWithAlias...) //nolint:gocritic

	query, args, err := sq.Select(selectFields...).
		From(base.PermissionsTable + " p").
		Join(base.AbilitiesTable + " a ON p.ability_id = a.id").
		Where(sq.Eq{"p.entity_id": entityID, "p.entity_type": entityType}).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build query")
	}

	rows, err := r.db.QueryContext(ctx, query, args...) //nolint:sqlclosecheck
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute query")
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.ErrorContext(ctx, "failed to close rows stream", "query", query, "err", err)
		}
	}(rows)

	var permissions []domain.Permission

	for rows.Next() {
		var permission domain.Permission
		var ability domain.Ability

		err = rows.Scan(
			&permission.ID,
			&permission.AbilityID,
			&permission.EntityID,
			&permission.EntityType,
			&permission.Forbidden,
			&permission.Scope,
			&ability.ID,
			&ability.Name,
			&ability.Title,
			&ability.EntityID,
			&ability.EntityType,
			&ability.OnlyOwned,
			&ability.Options,
			&ability.Scope,
			&ability.CreatedAt,
			&ability.UpdatedAt,
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to scan row")
		}

		permission.Ability = &ability
		permissions = append(permissions, permission)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.Wrap(err, "rows iteration error")
	}

	return permissions, nil
}

func (r *RBACRepository) Allow(
	ctx context.Context,
	entityID uint,
	entityType domain.EntityType,
	abilities []domain.Ability,
) error {
	return r.applyAbilities(ctx, entityID, entityType, abilities, false)
}

func (r *RBACRepository) Forbid(
	ctx context.Context,
	entityID uint,
	entityType domain.EntityType,
	abilities []domain.Ability,
) error {
	return r.applyAbilities(ctx, entityID, entityType, abilities, true)
}

//nolint:gocognit
func (r *RBACRepository) Revoke(
	ctx context.Context,
	entityID uint,
	entityType domain.EntityType,
	abilities []domain.Ability,
) error {
	return r.tm.Do(ctx, func(ctx context.Context) error {
		if len(abilities) == 0 {
			return nil
		}

		orConditions := sq.Or{}
		for _, ability := range abilities {
			andCondition := sq.And{
				sq.Eq{"name": ability.Name},
			}

			if ability.EntityID != nil {
				andCondition = append(andCondition, sq.Eq{"entity_id": ability.EntityID})
			} else {
				andCondition = append(andCondition, sq.Eq{"entity_id": nil})
			}

			if ability.EntityType != nil {
				andCondition = append(andCondition, sq.Eq{"entity_type": ability.EntityType})
			} else {
				andCondition = append(andCondition, sq.Eq{"entity_type": nil})
			}

			if ability.Scope != nil {
				andCondition = append(andCondition, sq.Eq{"scope": ability.Scope})
			} else {
				andCondition = append(andCondition, sq.Eq{"scope": nil})
			}

			orConditions = append(orConditions, andCondition)
		}

		selectQuery, selectArgs, err := sq.Select("id").
			From(base.AbilitiesTable).
			Where(orConditions).
			ToSql()
		if err != nil {
			return errors.Wrap(err, "failed to build select abilities query")
		}

		rows, err := r.db.QueryContext(ctx, selectQuery, selectArgs...) //nolint:sqlclosecheck
		if err != nil {
			return errors.Wrap(err, "failed to query abilities")
		}
		defer func(rows *sql.Rows) {
			err := rows.Close()
			if err != nil {
				slog.ErrorContext(ctx, "failed to close rows stream", "query", selectQuery, "err", err)
			}
		}(rows)

		abilityIDs := make([]uint, 0, len(abilities))
		for rows.Next() {
			var id uint

			err = rows.Scan(&id)
			if err != nil {
				return errors.Wrap(err, "failed to scan ability row")
			}

			abilityIDs = append(abilityIDs, id)
		}

		if err = rows.Err(); err != nil {
			return errors.Wrap(err, "rows iteration error")
		}

		if len(abilityIDs) == 0 {
			return nil
		}

		deleteQuery, deleteArgs, err := sq.Delete(base.PermissionsTable).
			Where(sq.And{
				sq.Eq{"ability_id": abilityIDs},
				sq.Eq{"entity_id": entityID},
				sq.Eq{"entity_type": entityType},
			}).
			ToSql()
		if err != nil {
			return errors.Wrap(err, "failed to build delete permissions query")
		}

		_, err = r.db.ExecContext(ctx, deleteQuery, deleteArgs...)
		if err != nil {
			return errors.Wrap(err, "failed to delete permissions")
		}

		return nil
	})
}

//nolint:gocognit
func (r *RBACRepository) applyAbilities(
	ctx context.Context,
	entityID uint,
	entityType domain.EntityType,
	abilities []domain.Ability,
	forbidden bool,
) error {
	return r.tm.Do(ctx, func(ctx context.Context) error {
		if len(abilities) == 0 {
			return nil
		}

		err := r.saveAbilities(ctx, abilities)
		if err != nil {
			return err
		}

		orConditions := sq.Or{}
		for _, ability := range abilities {
			andCondition := sq.And{
				sq.Eq{"name": ability.Name},
			}

			if ability.EntityID != nil {
				andCondition = append(andCondition, sq.Eq{"entity_id": ability.EntityID})
			} else {
				andCondition = append(andCondition, sq.Eq{"entity_id": nil})
			}

			if ability.EntityType != nil {
				andCondition = append(andCondition, sq.Eq{"entity_type": ability.EntityType})
			} else {
				andCondition = append(andCondition, sq.Eq{"entity_type": nil})
			}

			if ability.Scope != nil {
				andCondition = append(andCondition, sq.Eq{"scope": ability.Scope})
			} else {
				andCondition = append(andCondition, sq.Eq{"scope": nil})
			}

			orConditions = append(orConditions, andCondition)
		}

		selectQuery, selectArgs, err := sq.Select("id").
			From(base.AbilitiesTable).
			Where(orConditions).
			ToSql()
		if err != nil {
			return errors.Wrap(err, "failed to build select abilities query")
		}

		rows, err := r.db.QueryContext(ctx, selectQuery, selectArgs...) //nolint:sqlclosecheck
		if err != nil {
			return errors.Wrap(err, "failed to query abilities")
		}
		defer func(rows *sql.Rows) {
			err := rows.Close()
			if err != nil {
				slog.ErrorContext(ctx, "failed to close rows stream", "query", selectQuery, "err", err)
			}
		}(rows)

		abilityIDs := make([]uint, 0, len(abilities))
		for rows.Next() {
			var id uint

			err = rows.Scan(&id)
			if err != nil {
				return errors.Wrap(err, "failed to scan ability row")
			}

			abilityIDs = append(abilityIDs, id)
		}

		if err = rows.Err(); err != nil {
			return errors.Wrap(err, "rows iteration error")
		}

		if len(abilityIDs) == 0 {
			return errors.New("no abilities found after insert")
		}

		err = r.insertPermissions(ctx, abilityIDs, entityID, entityType, forbidden)
		if err != nil {
			return err
		}

		return nil
	})
}

type abilityUniqueKey struct {
	name       domain.AbilityName
	entityID   uint
	entityType domain.EntityType
	scope      int
}

func abilityUniqueKeyFromAbility(ability domain.Ability) abilityUniqueKey {
	k := abilityUniqueKey{
		name: ability.Name,
	}

	if ability.EntityID != nil {
		k.entityID = *ability.EntityID
	}
	if ability.EntityType != nil {
		k.entityType = *ability.EntityType
	}
	if ability.Scope != nil {
		k.scope = *ability.Scope
	}

	return k
}

//nolint:funlen
func (r *RBACRepository) saveAbilities(ctx context.Context, abilities []domain.Ability) error {
	if len(abilities) == 0 {
		return nil
	}

	orConditions := sq.Or{}
	for _, ability := range abilities {
		andCondition := sq.And{
			sq.Eq{"name": ability.Name},
		}

		if ability.EntityID != nil {
			andCondition = append(andCondition, sq.Eq{"entity_id": ability.EntityID})
		} else {
			andCondition = append(andCondition, sq.Eq{"entity_id": nil})
		}

		if ability.EntityType != nil {
			andCondition = append(andCondition, sq.Eq{"entity_type": ability.EntityType})
		} else {
			andCondition = append(andCondition, sq.Eq{"entity_type": nil})
		}

		if ability.Scope != nil {
			andCondition = append(andCondition, sq.Eq{"scope": ability.Scope})
		} else {
			andCondition = append(andCondition, sq.Eq{"scope": nil})
		}

		orConditions = append(orConditions, andCondition)
	}

	selectQuery, selectArgs, err := sq.Select("name", "entity_id", "entity_type", "scope").
		From(base.AbilitiesTable).
		Where(orConditions).
		ToSql()
	if err != nil {
		return errors.Wrap(err, "failed to build select existing abilities query")
	}

	rows, err := r.db.QueryContext(ctx, selectQuery, selectArgs...)
	if err != nil {
		return errors.Wrap(err, "failed to query existing abilities")
	}

	existingAbilities := make(map[abilityUniqueKey]struct{})
	for rows.Next() {
		var ability domain.Ability

		err = rows.Scan(&ability.Name, &ability.EntityID, &ability.EntityType, &ability.Scope)
		if err != nil {
			defer func() {
				err := rows.Close()
				if err != nil {
					slog.ErrorContext(ctx, "failed to close rows stream", "query", selectQuery, "err", err)
				}
			}()

			return errors.Wrap(err, "failed to scan existing ability row")
		}

		existingAbilities[abilityUniqueKeyFromAbility(ability)] = struct{}{}
	}

	err = rows.Close()
	if err != nil {
		return errors.Wrap(err, "failed to close rows")
	}

	if err = rows.Err(); err != nil {
		return errors.Wrap(err, "rows iteration error")
	}

	newAbilities := make([]domain.Ability, 0, len(abilities))
	for _, ability := range abilities {
		if _, exists := existingAbilities[abilityUniqueKeyFromAbility(ability)]; !exists {
			newAbilities = append(newAbilities, ability)
		}
	}

	if len(newAbilities) == 0 {
		return nil
	}

	insertAbilitiesQuery := sq.Insert(base.AbilitiesTable).
		Columns(
			"name",
			"title",
			"entity_id",
			"entity_type",
			"only_owned",
			"options",
			"scope",
			"created_at",
			"updated_at",
		)

	for _, ability := range newAbilities {
		insertAbilitiesQuery = insertAbilitiesQuery.Values(
			ability.Name,
			ability.Title,
			ability.EntityID,
			ability.EntityType,
			ability.OnlyOwned,
			ability.Options,
			ability.Scope,
			ability.CreatedAt,
			ability.UpdatedAt,
		)
	}

	query, args, err := insertAbilitiesQuery.ToSql()
	if err != nil {
		return errors.Wrap(err, "failed to build insert abilities query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to insert abilities")
	}

	return nil
}

func (r *RBACRepository) insertPermissions(
	ctx context.Context,
	abilityIDs []uint,
	entityID uint,
	entityType domain.EntityType,
	forbidden bool,
) error {
	if len(abilityIDs) == 0 {
		return nil
	}

	insertPermissionsQuery := sq.Insert(base.PermissionsTable).
		Columns(
			"ability_id",
			"entity_id",
			"entity_type",
			"forbidden",
			"scope",
		)

	for _, abilityID := range abilityIDs {
		insertPermissionsQuery = insertPermissionsQuery.Values(
			abilityID,
			entityID,
			entityType,
			forbidden,
			nil,
		)
	}

	permissionsQuery, permissionsArgs, err := insertPermissionsQuery.
		ToSql()
	if err != nil {
		return errors.Wrap(err, "failed to build insert permissions query")
	}

	_, err = r.db.ExecContext(ctx, permissionsQuery, permissionsArgs...)
	if err != nil {
		return errors.Wrap(err, "failed to insert permissions")
	}

	return nil
}

func (r *RBACRepository) AssignRolesForEntity(
	ctx context.Context,
	entityID uint,
	entityType domain.EntityType,
	roles []domain.RestrictedRole,
) error {
	if len(roles) == 0 {
		return nil
	}

	insertQuery := sq.Insert(base.AssignedRolesTable).
		Columns(
			"role_id",
			"entity_id",
			"entity_type",
			"restricted_to_id",
			"restricted_to_type",
			"scope",
		)

	for _, role := range roles {
		insertQuery = insertQuery.Values(
			role.ID,
			entityID,
			entityType,
			role.RestrictedToID,
			role.RestrictedToType,
			role.Scope,
		)
	}

	query, args, err := insertQuery.ToSql()
	if err != nil {
		return errors.Wrap(err, "failed to build insert query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to insert role assignments")
	}

	return nil
}

func (r *RBACRepository) SaveRole(ctx context.Context, role *domain.Role) error {
	query, args, err := sq.Insert(base.RolesTable).
		Columns(base.RoleFields...).
		Values(
			lo.EmptyableToPtr(role.ID),
			role.Name,
			role.Title,
			role.Level,
			role.Scope,
			role.CreatedAt,
			role.UpdatedAt,
		).
		Suffix("ON CONFLICT(id) DO UPDATE SET " +
			"name=EXCLUDED.name," +
			"title=EXCLUDED.title," +
			"level=EXCLUDED.level," +
			"scope=EXCLUDED.scope," +
			"updated_at=EXCLUDED.updated_at " +
			"RETURNING id").
		ToSql()
	if err != nil {
		return errors.Wrap(err, "failed to build query")
	}

	var returnedID uint
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&returnedID)
	if err != nil {
		return errors.Wrap(err, "failed to execute query")
	}

	role.ID = returnedID

	return nil
}

func (r *RBACRepository) ClearRolesForEntity(
	ctx context.Context,
	entityID uint,
	entityType domain.EntityType,
) error {
	query, args, err := sq.Delete(base.AssignedRolesTable).
		Where(sq.Eq{"entity_id": entityID, "entity_type": entityType}).
		ToSql()
	if err != nil {
		return errors.Wrap(err, "failed to build delete query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to delete role assignments")
	}

	return nil
}

func addAliasToFields(fields []string, alias string) []string {
	if len(fields) == 0 {
		return fields
	}

	aliasedFields := make([]string, len(fields))
	var builder strings.Builder

	for i, field := range fields {
		builder.Reset()
		builder.Grow(len(alias) + len(field) + 1)

		builder.WriteString(alias)
		builder.WriteByte('.')
		builder.WriteString(field)

		aliasedFields[i] = builder.String()
	}

	return aliasedFields
}
