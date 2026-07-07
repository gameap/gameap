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

type PluginStorageRepository struct {
	db base.DB
}

var pluginStorageFields = []string{
	"id", "plugin_id", "`key`", "entity_type", "entity_id", "payload", "created_at", "updated_at",
}

func NewPluginStorageRepository(db base.DB) *PluginStorageRepository {
	return &PluginStorageRepository{
		db: db,
	}
}

func (r *PluginStorageRepository) Find(
	ctx context.Context,
	filter *filters.FindPluginStorage,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.PluginStorageEntry, error) {
	builder := sq.Select(pluginStorageFields...).
		From(base.PluginStorageTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *PluginStorageRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.PluginStorageEntry, error) {
	if len(order) > 0 {
		for _, o := range order {
			builder = builder.OrderBy(o.String())
		}
	} else {
		builder = builder.OrderBy("id ASC")
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

	var entries []domain.PluginStorageEntry

	for rows.Next() {
		var entry *domain.PluginStorageEntry
		entry, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		entries = append(entries, *entry)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return entries, nil
}

func (r *PluginStorageRepository) Save(ctx context.Context, entry *domain.PluginStorageEntry) error {
	now := time.Now()
	entry.UpdatedAt = &now

	if entry.CreatedAt == nil || entry.CreatedAt.IsZero() {
		entry.CreatedAt = &now
	}

	if entry.ID != 0 {
		return r.update(ctx, entry)
	}

	query := `INSERT INTO ` + base.PluginStorageTable +
		` (plugin_id, ` + "`key`" + `, entity_type, entity_id, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE payload = VALUES(payload), updated_at = VALUES(updated_at)`

	result, err := r.db.ExecContext(ctx, query,
		entry.PluginID,
		entry.Key,
		entry.EntityType,
		entry.EntityID,
		entry.Payload,
		entry.CreatedAt,
		entry.UpdatedAt,
	)
	if err != nil {
		return errors.WithMessage(err, "failed to execute upsert query")
	}

	lastID, err := result.LastInsertId()
	if err != nil {
		return errors.WithMessage(err, "failed to get last insert ID")
	}
	if lastID > 0 {
		entry.ID = uint64(lastID)
	}

	return nil
}

func (r *PluginStorageRepository) update(ctx context.Context, entry *domain.PluginStorageEntry) error {
	query, args, err := sq.Update(base.PluginStorageTable).
		Set("payload", entry.Payload).
		Set("updated_at", entry.UpdatedAt).
		Where(sq.Eq{"id": entry.ID}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build update query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute update query")
	}

	return nil
}

func (r *PluginStorageRepository) Delete(ctx context.Context, id uint64) error {
	query, args, err := sq.Delete(base.PluginStorageTable).
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

func (r *PluginStorageRepository) DeleteByPlugin(ctx context.Context, pluginID uint64) error {
	query, args, err := sq.Delete(base.PluginStorageTable).
		Where(sq.Eq{"plugin_id": pluginID}).
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

func (r *PluginStorageRepository) DeleteByFilter(ctx context.Context, filter *filters.FindPluginStorage) error {
	// Reject a nil OR an all-empty filter: an empty filter renders a WHERE-less
	// DELETE that would wipe the whole table.
	if filter == nil ||
		(len(filter.IDs) == 0 && len(filter.PluginIDs) == 0 &&
			len(filter.Keys) == 0 && len(filter.EntityPairs) == 0) {
		return errors.New("a non-empty filter is required for DeleteByFilter")
	}

	query, args, err := sq.Delete(base.PluginStorageTable).
		Where(r.filterToSq(filter)).
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

func (r *PluginStorageRepository) scan(row base.Scanner) (*domain.PluginStorageEntry, error) {
	var entry domain.PluginStorageEntry

	err := row.Scan(
		&entry.ID,
		&entry.PluginID,
		&entry.Key,
		&entry.EntityType,
		&entry.EntityID,
		&entry.Payload,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &entry, nil
}

func (r *PluginStorageRepository) filterToSq(filter *filters.FindPluginStorage) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 4)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	if len(filter.PluginIDs) > 0 {
		and = append(and, sq.Eq{"plugin_id": filter.PluginIDs})
	}

	if len(filter.Keys) > 0 {
		and = append(and, sq.Eq{"`key`": filter.Keys})
	}

	if len(filter.EntityPairs) > 0 {
		or := make(sq.Or, 0, len(filter.EntityPairs))
		for _, pair := range filter.EntityPairs {
			pairAnd := sq.And{
				sq.Eq{"entity_type": pair.EntityType},
				sq.Eq{"entity_id": pair.EntityID},
			}
			or = append(or, pairAnd)
		}
		and = append(and, or)
	}

	return and
}
