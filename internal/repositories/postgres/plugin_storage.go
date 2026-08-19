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

type PluginStorageRepository struct {
	db base.DB
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
	builder := sq.Select(base.PluginStorageFields...).
		From(base.PluginStorageTable).
		Where(r.filterToSq(filter)).
		PlaceholderFormat(sq.Dollar)

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

	// A global entry (no entity) never conflicts in the unique index — NULLs
	// are distinct there — so the upsert below would add a second row instead
	// of updating the first. Update whatever the scope already holds; insert
	// only when it holds nothing.
	existingID, err := r.findScopeID(ctx, entry)
	if err != nil {
		return err
	}

	if existingID != 0 {
		entry.ID = existingID

		err = r.updateScope(ctx, entry)
		if err != nil {
			return err
		}

		// Retry the cleanup an earlier save never reached — a cancelled context,
		// a crash between its two statements. Once a scope holds duplicates
		// every later save lands here, so this is the only path left to
		// collapse them.
		return r.deleteScopeBefore(ctx, entry)
	}

	query := `INSERT INTO ` + base.PluginStorageTable +
		` (plugin_id, key, entity_type, entity_id, payload, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (plugin_id, key, entity_type, entity_id)
		DO UPDATE SET payload = EXCLUDED.payload, updated_at = EXCLUDED.updated_at
		RETURNING id`

	var returnedID uint64
	err = r.db.QueryRowContext(ctx, query,
		entry.PluginID,
		entry.Key,
		entry.EntityType,
		entry.EntityID,
		entry.Payload,
		entry.CreatedAt,
		entry.UpdatedAt,
	).Scan(&returnedID)
	if err != nil {
		return errors.WithMessage(err, "failed to execute upsert query")
	}

	entry.ID = returnedID

	return r.deleteScopeBefore(ctx, entry)
}

// findScopeID returns the newest row id of the entry's scope
// (plugin, key, entity), or 0 when the scope is empty.
func (r *PluginStorageRepository) findScopeID(ctx context.Context, entry *domain.PluginStorageEntry) (uint64, error) {
	query, args, err := sq.Select("id").
		From(base.PluginStorageTable).
		Where(scopeEq(entry)).
		OrderBy("id DESC").
		Limit(1).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return 0, errors.WithMessage(err, "failed to build scope lookup query")
	}

	var id uint64
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, errors.WithMessage(err, "failed to execute scope lookup query")
	}

	return id, nil
}

// updateScope rewrites every row of the entry's scope. Normally that is one
// row; rows duplicated before the scope-aware save existed converge too.
func (r *PluginStorageRepository) updateScope(ctx context.Context, entry *domain.PluginStorageEntry) error {
	query, args, err := sq.Update(base.PluginStorageTable).
		Set("payload", entry.Payload).
		Set("updated_at", entry.UpdatedAt).
		Where(scopeEq(entry)).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build scope update query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute scope update query")
	}

	return nil
}

// deleteScopeBefore drops the rows a save racing this one left in the same
// scope. The lookup and the insert above are two statements, and the unique
// index does not close the gap between them for a global entry — NULLs never
// conflict there — so both saves insert. The newest row carries the newest
// payload and stays.
func (r *PluginStorageRepository) deleteScopeBefore(ctx context.Context, entry *domain.PluginStorageEntry) error {
	query, args, err := sq.Delete(base.PluginStorageTable).
		Where(scopeEq(entry)).
		Where(sq.Lt{"id": entry.ID}).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build scope cleanup query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute scope cleanup query")
	}

	return nil
}

// scopeEq matches the entry's (plugin, key, entity) scope; a nil entity part
// renders as IS NULL.
func scopeEq(entry *domain.PluginStorageEntry) sq.Eq {
	return sq.Eq{
		"plugin_id":   entry.PluginID,
		"key":         entry.Key,
		"entity_type": entry.EntityType,
		"entity_id":   entry.EntityID,
	}
}

func (r *PluginStorageRepository) update(ctx context.Context, entry *domain.PluginStorageEntry) error {
	query, args, err := sq.Update(base.PluginStorageTable).
		Set("payload", entry.Payload).
		Set("updated_at", entry.UpdatedAt).
		Where(sq.Eq{"id": entry.ID}).
		PlaceholderFormat(sq.Dollar).
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

func (r *PluginStorageRepository) DeleteByPlugin(ctx context.Context, pluginID uint64) error {
	query, args, err := sq.Delete(base.PluginStorageTable).
		Where(sq.Eq{"plugin_id": pluginID}).
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
		and = append(and, sq.Eq{"key": filter.Keys})
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
