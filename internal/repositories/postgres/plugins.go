package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
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
		From(base.PluginsTable).
		PlaceholderFormat(sq.Dollar)

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
		Where(r.filterToSq(filter)).
		PlaceholderFormat(sq.Dollar)

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

	exists, err := r.Exists(ctx, &filters.FindPlugin{IDs: []domain.Uint64ID{plugin.ID}})
	if err != nil {
		return errors.WithMessage(err, "failed to check plugin existence")
	}

	if exists {
		return r.update(ctx, plugin)
	}

	if plugin.CreatedAt == nil || plugin.CreatedAt.IsZero() {
		plugin.CreatedAt = new(time.Now())
	}

	return r.insert(ctx, plugin)
}

func (r *PluginRepository) insert(ctx context.Context, plugin *domain.Plugin) error {
	if plugin.ID == 0 {
		return errors.New("plugin ID is required")
	}

	requiredPermissions := permissionsToPostgresArray(plugin.RequiredPermissions)
	allowedPermissions := permissionsToPostgresArray(plugin.AllowedPermissions)
	dependencies := stringsToPostgresArray(plugin.Dependencies)

	configJSON, err := json.Marshal(plugin.Config)
	if err != nil {
		return errors.WithMessage(err, "failed to marshal config")
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
			requiredPermissions,
			allowedPermissions,
			plugin.Status,
			plugin.Priority,
			plugin.Generation,
			plugin.Category,
			dependencies,
			configJSON,
			plugin.ConfigSchema,
			plugin.InstalledAt,
			plugin.LastLoadedAt,
			plugin.LastError,
			plugin.LastErrorAt,
			plugin.CreatedAt,
			plugin.UpdatedAt,
		).
		PlaceholderFormat(sq.Dollar).
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

func (r *PluginRepository) update(ctx context.Context, plugin *domain.Plugin) error {
	requiredPermissions := permissionsToPostgresArray(plugin.RequiredPermissions)
	allowedPermissions := permissionsToPostgresArray(plugin.AllowedPermissions)
	dependencies := stringsToPostgresArray(plugin.Dependencies)

	configJSON, err := json.Marshal(plugin.Config)
	if err != nil {
		return errors.WithMessage(err, "failed to marshal config")
	}

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
		Set("required_permissions", requiredPermissions).
		Set("allowed_permissions", allowedPermissions).
		Set("status", plugin.Status).
		Set("priority", plugin.Priority).
		Set("generation", plugin.Generation).
		Set("category", plugin.Category).
		Set("dependencies", dependencies).
		Set("config", configJSON).
		Set("config_schema", plugin.ConfigSchema).
		Set("installed_at", plugin.InstalledAt).
		Set("last_loaded_at", plugin.LastLoadedAt).
		Set("last_error", plugin.LastError).
		Set("last_error_at", plugin.LastErrorAt).
		Set("updated_at", plugin.UpdatedAt).
		Where(sq.Eq{"id": plugin.ID}).
		PlaceholderFormat(sq.Dollar).
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
		PlaceholderFormat(sq.Dollar).
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
		PlaceholderFormat(sq.Dollar).
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
		PlaceholderFormat(sq.Dollar).
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
	var requiredPermissionsStr, allowedPermissionsStr, dependenciesStr *string
	var configJSON []byte

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
		&requiredPermissionsStr,
		&allowedPermissionsStr,
		&plugin.Status,
		&plugin.Priority,
		&plugin.Generation,
		&plugin.Category,
		&dependenciesStr,
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

	plugin.RequiredPermissions = parsePostgresArrayToPermissions(requiredPermissionsStr)
	plugin.AllowedPermissions = parsePostgresArrayToPermissions(allowedPermissionsStr)
	plugin.Dependencies = parsePostgresArrayToStrings(dependenciesStr)

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

func permissionsToPostgresArray(permissions []domain.PluginPermission) *string {
	if len(permissions) == 0 {
		return nil
	}

	elements := make([]string, len(permissions))
	for i, p := range permissions {
		elements[i] = string(p)
	}

	return new("{" + strings.Join(elements, ",") + "}")
}

func stringsToPostgresArray(strs []string) *string {
	if len(strs) == 0 {
		return nil
	}

	escaped := make([]string, len(strs))
	for i, s := range strs {
		escaped[i] = escapePostgresArrayElement(s)
	}

	return new("{" + strings.Join(escaped, ",") + "}")
}

func parsePostgresArrayToPermissions(s *string) []domain.PluginPermission {
	strs := parsePostgresArrayToStrings(s)
	if len(strs) == 0 {
		return nil
	}

	result := make([]domain.PluginPermission, len(strs))
	for i, str := range strs {
		result[i] = domain.PluginPermission(str)
	}

	return result
}

func parsePostgresArrayToStrings(s *string) []string {
	if s == nil || *s == "" || *s == "{}" {
		return nil
	}

	trimmed := strings.TrimPrefix(strings.TrimSuffix(*s, "}"), "{")
	if trimmed == "" {
		return nil
	}

	var result []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, r := range trimmed {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
		case r == ',' && !inQuotes:
			result = append(result, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

func escapePostgresArrayElement(s string) string {
	if s == "" {
		return `""`
	}

	needsQuotes := strings.ContainsAny(s, `",{}\`)
	if !needsQuotes {
		return s
	}

	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)

	return `"` + escaped + `"`
}
