package mysql

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/pkg/errors"
)

type PluginSecretRepository struct {
	db base.DB
}

// key and value are reserved words in MySQL and have to be backtick-quoted
// everywhere they appear in a statement.
var pluginSecretFields = []string{
	"id", "plugin_id", "`key`", "`value`", "created_at", "updated_at",
}

func NewPluginSecretRepository(db base.DB) *PluginSecretRepository {
	return &PluginSecretRepository{
		db: db,
	}
}

func (r *PluginSecretRepository) Find(
	ctx context.Context,
	filter *filters.FindPluginSecret,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.PluginSecret, error) {
	builder := sq.Select(pluginSecretFields...).
		From(base.PluginSecretsTable).
		Where(r.filterToSq(filter))

	if len(order) > 0 {
		for _, o := range order {
			builder = builder.OrderBy("`" + o.Field + "` " + o.Direction.String())
		}
	} else {
		builder = builder.OrderBy("`key` ASC")
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

	var secrets []domain.PluginSecret

	for rows.Next() {
		var secret *domain.PluginSecret
		secret, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		secrets = append(secrets, *secret)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return secrets, nil
}

func (r *PluginSecretRepository) Upsert(ctx context.Context, secret *domain.PluginSecret) error {
	now := time.Now()
	secret.UpdatedAt = &now

	if secret.CreatedAt == nil || secret.CreatedAt.IsZero() {
		secret.CreatedAt = &now
	}

	query := "INSERT INTO " + base.PluginSecretsTable +
		" (plugin_id, `key`, `value`, created_at, updated_at)" +
		" VALUES (?, ?, ?, ?, ?)" +
		" ON DUPLICATE KEY UPDATE `value` = VALUES(`value`), updated_at = VALUES(updated_at)"

	_, err := r.db.ExecContext(ctx, query,
		secret.PluginID,
		secret.Key,
		secret.Value,
		secret.CreatedAt,
		secret.UpdatedAt,
	)
	if err != nil {
		return errors.WithMessage(err, "failed to execute upsert query")
	}

	// LastInsertId is unreliable on the duplicate-key update path, so the row
	// ID is always fetched by its natural key; created_at is read back too,
	// because the update path keeps the row's original value.
	selectQuery := "SELECT id, created_at FROM " + base.PluginSecretsTable +
		" WHERE plugin_id = ? AND `key` = ?"

	var returnedID uint64
	var storedCreatedAt *time.Time
	err = r.db.QueryRowContext(ctx, selectQuery, secret.PluginID, secret.Key).
		Scan(&returnedID, &storedCreatedAt)
	if err != nil {
		return errors.WithMessage(err, "failed to get secret ID after upsert")
	}
	secret.ID = returnedID

	if storedCreatedAt != nil {
		secret.CreatedAt = storedCreatedAt
	}

	return nil
}

func (r *PluginSecretRepository) Delete(ctx context.Context, pluginID domain.Uint64ID, key string) error {
	query, args, err := sq.Delete(base.PluginSecretsTable).
		Where(sq.Eq{"plugin_id": pluginID, "`key`": key}).
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

func (r *PluginSecretRepository) DeleteByPlugin(ctx context.Context, pluginID domain.Uint64ID) (int, error) {
	query, args, err := sq.Delete(base.PluginSecretsTable).
		Where(sq.Eq{"plugin_id": pluginID}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return 0, errors.WithMessage(err, "failed to build query")
	}

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to execute query")
	}

	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, errors.WithMessage(err, "failed to get affected rows")
	}

	return int(deleted), nil
}

func (r *PluginSecretRepository) CountByPlugin(ctx context.Context, pluginID domain.Uint64ID) (int, error) {
	query, args, err := sq.Select("COUNT(*)").
		From(base.PluginSecretsTable).
		Where(sq.Eq{"plugin_id": pluginID}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return 0, errors.WithMessage(err, "failed to build query")
	}

	var count int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, errors.WithMessage(err, "failed to execute query")
	}

	return count, nil
}

func (r *PluginSecretRepository) filterToSq(filter *filters.FindPluginSecret) sq.Sqlizer {
	conditions := sq.And{}

	if filter == nil {
		return conditions
	}

	if len(filter.PluginIDs) > 0 {
		conditions = append(conditions, sq.Eq{"plugin_id": filter.PluginIDs})
	}

	if len(filter.Keys) > 0 {
		conditions = append(conditions, sq.Eq{"`key`": filter.Keys})
	}

	return conditions
}

func (r *PluginSecretRepository) scan(row base.Scanner) (*domain.PluginSecret, error) {
	var secret domain.PluginSecret

	err := row.Scan(
		&secret.ID,
		&secret.PluginID,
		&secret.Key,
		&secret.Value,
		&secret.CreatedAt,
		&secret.UpdatedAt,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &secret, nil
}
