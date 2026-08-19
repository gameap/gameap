package postgres

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
	builder := sq.Select(base.PluginSecretFields...).
		From(base.PluginSecretsTable).
		Where(r.filterToSq(filter)).
		PlaceholderFormat(sq.Dollar)

	if len(order) > 0 {
		for _, o := range order {
			builder = builder.OrderBy(o.String())
		}
	} else {
		builder = builder.OrderBy("key ASC")
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

	query := `INSERT INTO ` + base.PluginSecretsTable +
		` (plugin_id, key, value, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (plugin_id, key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at
		RETURNING id, created_at`

	// created_at is read back because the conflict path keeps the row's
	// original value, not the local timestamp bound above.
	var returnedID uint64
	var storedCreatedAt *time.Time
	err := r.db.QueryRowContext(ctx, query,
		secret.PluginID,
		secret.Key,
		secret.Value,
		secret.CreatedAt,
		secret.UpdatedAt,
	).Scan(&returnedID, &storedCreatedAt)
	if err != nil {
		return errors.WithMessage(err, "failed to execute upsert query")
	}

	secret.ID = returnedID

	if storedCreatedAt != nil {
		secret.CreatedAt = storedCreatedAt
	}

	return nil
}

func (r *PluginSecretRepository) Delete(ctx context.Context, pluginID domain.Uint64ID, key string) error {
	query, args, err := sq.Delete(base.PluginSecretsTable).
		Where(sq.Eq{"plugin_id": pluginID, "key": key}).
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

func (r *PluginSecretRepository) DeleteByPlugin(ctx context.Context, pluginID domain.Uint64ID) (int, error) {
	query, args, err := sq.Delete(base.PluginSecretsTable).
		Where(sq.Eq{"plugin_id": pluginID}).
		PlaceholderFormat(sq.Dollar).
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
		PlaceholderFormat(sq.Dollar).
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
		conditions = append(conditions, sq.Eq{"key": filter.Keys})
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
