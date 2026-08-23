package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/pkg/errors"
)

var pluginFields = []string{
	"id",
	"name",
	"version",
	"description",
	"author",
	"api_version",
	"filename",
	"source",
	"homepage",
	"checksum",
	"required_permissions",
	"allowed_permissions",
	"status",
	"priority",
	"generation",
	"category",
	"dependencies",
	"config",
	"config_schema",
	"installed_at",
	"last_loaded_at",
	"last_error",
	"last_error_at",
	"created_at",
	"updated_at",
}

type PluginRepository struct {
	db base.DB
}

func NewPluginRepository(db base.DB) *PluginRepository {
	return &PluginRepository{
		db: db,
	}
}

func (r *PluginRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Plugin, error) {
	builder := sq.Select(pluginFields...).
		From(base.PluginsTable)

	return r.find(ctx, builder, order, pagination)
}

func (r *PluginRepository) Find(
	ctx context.Context,
	filter *filters.FindPlugin,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Plugin, error) {
	builder := sq.Select(pluginFields...).
		From(base.PluginsTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *PluginRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Plugin, error) {
	if len(order) > 0 {
		for _, o := range order {
			builder = builder.OrderBy(o.String())
		}
	} else {
		builder = builder.OrderBy("priority DESC", "name ASC")
	}

	if pagination != nil {
		if pagination.Limit == 0 {
			pagination.Limit = filters.DefaultLimit
		}

		builder = builder.Limit(pagination.Limit).Offset(pagination.Offset)
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to build query")
	}

	rows, err := r.db.QueryContext(ctx, query, args...) //nolint:sqlclosecheck
	if err != nil {
		return nil, errors.WithMessage(err, "failed to execute query")
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.ErrorContext(ctx, "failed to close rows stream", "query", query, "err", err)
		}
	}(rows)

	var plugins []domain.Plugin

	for rows.Next() {
		var plugin *domain.Plugin
		plugin, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		plugins = append(plugins, *plugin)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return plugins, nil
}

func (r *PluginRepository) Save(ctx context.Context, plugin *domain.Plugin) error {
	plugin.UpdatedAt = new(time.Now())

	jsonFields, err := r.marshalJSONFields(plugin)
	if err != nil {
		return err
	}

	exists, err := r.Exists(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}})
	if err != nil {
		return errors.WithMessage(err, "failed to check plugin existence")
	}

	if exists {
		return r.update(ctx, plugin, jsonFields)
	}

	if plugin.CreatedAt == nil || plugin.CreatedAt.IsZero() {
		plugin.CreatedAt = new(time.Now())
	}

	return r.insert(ctx, plugin, jsonFields)
}

type pluginJSONFields struct {
	requiredPermissions []byte
	allowedPermissions  []byte
	dependencies        []byte
	config              []byte
}

func (r *PluginRepository) marshalJSONFields(plugin *domain.Plugin) (*pluginJSONFields, error) {
	requiredPermissionsJSON, err := json.Marshal(plugin.RequiredPermissions)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal required_permissions")
	}

	allowedPermissionsJSON, err := json.Marshal(plugin.AllowedPermissions)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal allowed_permissions")
	}

	dependenciesJSON, err := json.Marshal(plugin.Dependencies)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal dependencies")
	}

	configJSON, err := json.Marshal(plugin.Config)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to marshal config")
	}

	return &pluginJSONFields{
		requiredPermissions: requiredPermissionsJSON,
		allowedPermissions:  allowedPermissionsJSON,
		dependencies:        dependenciesJSON,
		config:              configJSON,
	}, nil
}

func (r *PluginRepository) insert(
	ctx context.Context,
	plugin *domain.Plugin,
	jsonFields *pluginJSONFields,
) error {
	if plugin.ID == 0 {
		return errors.New("plugin ID is required")
	}

	query, args, err := sq.Insert(base.PluginsTable).
		Columns(pluginFields...).
		Values(
			plugin.ID,
			plugin.Name,
			plugin.Version,
			plugin.Description,
			plugin.Author,
			plugin.APIVersion,
			plugin.Filename,
			plugin.Source,
			plugin.Homepage,
			plugin.Checksum,
			jsonFields.requiredPermissions,
			jsonFields.allowedPermissions,
			plugin.Status,
			plugin.Priority,
			plugin.Generation,
			plugin.Category,
			jsonFields.dependencies,
			jsonFields.config,
			plugin.ConfigSchema,
			plugin.InstalledAt,
			plugin.LastLoadedAt,
			plugin.LastError,
			plugin.LastErrorAt,
			plugin.CreatedAt,
			plugin.UpdatedAt,
		).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	return nil
}

func (r *PluginRepository) update(
	ctx context.Context,
	plugin *domain.Plugin,
	jsonFields *pluginJSONFields,
) error {
	query, args, err := sq.Update(base.PluginsTable).
		Set("name", plugin.Name).
		Set("version", plugin.Version).
		Set("description", plugin.Description).
		Set("author", plugin.Author).
		Set("api_version", plugin.APIVersion).
		Set("filename", plugin.Filename).
		Set("source", plugin.Source).
		Set("homepage", plugin.Homepage).
		Set("checksum", plugin.Checksum).
		Set("required_permissions", jsonFields.requiredPermissions).
		Set("allowed_permissions", jsonFields.allowedPermissions).
		Set("status", plugin.Status).
		Set("priority", plugin.Priority).
		Set("generation", plugin.Generation).
		Set("category", plugin.Category).
		Set("dependencies", jsonFields.dependencies).
		Set("config", jsonFields.config).
		Set("config_schema", plugin.ConfigSchema).
		Set("installed_at", plugin.InstalledAt).
		Set("last_loaded_at", plugin.LastLoadedAt).
		Set("last_error", plugin.LastError).
		Set("last_error_at", plugin.LastErrorAt).
		Set("updated_at", plugin.UpdatedAt).
		Where(sq.Eq{"id": plugin.ID}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	return nil
}

func (r *PluginRepository) UpdateLoadState(
	ctx context.Context,
	id domain.Uint64ID,
	state domain.PluginLoadState,
) error {
	query, args, err := sq.Update(base.PluginsTable).
		Set("status", state.Status).
		Set("last_error", state.LastError).
		Set("last_error_at", state.LastErrorAt).
		Set("last_loaded_at", state.LastLoadedAt).
		Set("generation", state.Generation).
		Set("config_schema", state.ConfigSchema).
		Set("updated_at", time.Now()).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return errors.WithMessage(err, "failed to read update result")
	}

	if affected == 0 {
		return repositories.ErrPluginNotFound
	}

	return nil
}

func (r *PluginRepository) Delete(ctx context.Context, id domain.Uint64ID) error {
	query, args, err := sq.Delete(base.PluginsTable).
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	return nil
}

func (r *PluginRepository) Exists(ctx context.Context, filter *filters.FindPlugin) (bool, error) {
	query, args, err := sq.Select("1").
		From(base.PluginsTable).
		Where(r.filterToSq(filter)).
		Limit(1).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return false, errors.WithMessage(err, "failed to build query")
	}

	var exists int
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}

		return false, errors.WithMessage(err, "failed to execute query")
	}

	return true, nil
}

func (r *PluginRepository) scan(row base.Scanner) (*domain.Plugin, error) {
	var plugin domain.Plugin
	var requiredPermissionsJSON, allowedPermissionsJSON, dependenciesJSON, configJSON []byte

	err := row.Scan(
		&plugin.ID,
		&plugin.Name,
		&plugin.Version,
		&plugin.Description,
		&plugin.Author,
		&plugin.APIVersion,
		&plugin.Filename,
		&plugin.Source,
		&plugin.Homepage,
		&plugin.Checksum,
		&requiredPermissionsJSON,
		&allowedPermissionsJSON,
		&plugin.Status,
		&plugin.Priority,
		&plugin.Generation,
		&plugin.Category,
		&dependenciesJSON,
		&configJSON,
		&plugin.ConfigSchema,
		&plugin.InstalledAt,
		&plugin.LastLoadedAt,
		&plugin.LastError,
		&plugin.LastErrorAt,
		&plugin.CreatedAt,
		&plugin.UpdatedAt,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	if len(requiredPermissionsJSON) > 0 {
		err = json.Unmarshal(requiredPermissionsJSON, &plugin.RequiredPermissions)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to unmarshal required_permissions")
		}
	}

	if len(allowedPermissionsJSON) > 0 {
		err = json.Unmarshal(allowedPermissionsJSON, &plugin.AllowedPermissions)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to unmarshal allowed_permissions")
		}
	}

	if len(dependenciesJSON) > 0 {
		err = json.Unmarshal(dependenciesJSON, &plugin.Dependencies)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to unmarshal dependencies")
		}
	}

	if len(configJSON) > 0 {
		err = json.Unmarshal(configJSON, &plugin.Config)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to unmarshal config")
		}
	}

	return &plugin, nil
}

func (r *PluginRepository) filterToSq(filter *filters.FindPlugin) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 4)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	if len(filter.Names) > 0 {
		and = append(and, sq.Eq{"name": filter.Names})
	}

	if len(filter.Statuses) > 0 {
		and = append(and, sq.Eq{"status": filter.Statuses})
	}

	if len(filter.Categories) > 0 {
		and = append(and, sq.Eq{"category": filter.Categories})
	}

	return and
}
