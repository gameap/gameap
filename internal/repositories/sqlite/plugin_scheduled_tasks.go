package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/pkg/errors"
)

type PluginScheduledTaskRepository struct {
	db base.DB
}

func NewPluginScheduledTaskRepository(db base.DB) *PluginScheduledTaskRepository {
	return &PluginScheduledTaskRepository{
		db: db,
	}
}

func (r *PluginScheduledTaskRepository) Upsert(ctx context.Context, task *domain.PluginScheduledTask) error {
	now := time.Now()
	task.UpdatedAt = &now

	if task.CreatedAt == nil || task.CreatedAt.IsZero() {
		task.CreatedAt = &now
	}

	createdAtStr := task.CreatedAt.Format(time.RFC3339Nano)
	updatedAtStr := task.UpdatedAt.Format(time.RFC3339Nano)

	upsertQuery := `INSERT INTO ` + base.PluginScheduledTasksTable +
		` (id, plugin_id, name, interval_ms, error_policy, max_retries,
			retry_delay_ms, max_jitter_ms, timeout_ms, created_at, updated_at)
		VALUES (NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (plugin_id, name)
		DO UPDATE SET interval_ms = excluded.interval_ms, error_policy = excluded.error_policy,
			max_retries = excluded.max_retries, retry_delay_ms = excluded.retry_delay_ms,
			max_jitter_ms = excluded.max_jitter_ms, timeout_ms = excluded.timeout_ms,
			updated_at = excluded.updated_at`

	_, err := r.db.ExecContext(ctx, upsertQuery,
		task.PluginID,
		task.Name,
		task.Interval.Milliseconds(),
		string(task.ErrorPolicy),
		task.MaxRetries,
		task.RetryDelay.Milliseconds(),
		task.MaxJitter.Milliseconds(),
		task.Timeout.Milliseconds(),
		createdAtStr,
		updatedAtStr,
	)
	if err != nil {
		return errors.WithMessage(err, "failed to execute upsert query")
	}

	selectQuery := `SELECT id, created_at FROM ` + base.PluginScheduledTasksTable +
		` WHERE plugin_id = ? AND name = ?`

	// The duplicate-key path keeps the row's original created_at, so the
	// persisted value is read back instead of trusting the local timestamp.
	var returnedID uint64
	var storedCreatedAtStr *string
	err = r.db.QueryRowContext(ctx, selectQuery, task.PluginID, task.Name).
		Scan(&returnedID, &storedCreatedAtStr)
	if err != nil {
		return errors.WithMessage(err, "failed to get task ID after upsert")
	}
	task.ID = returnedID

	if storedCreatedAtStr != nil && *storedCreatedAtStr != "" {
		storedCreatedAt, err := base.ParseTime(*storedCreatedAtStr)
		if err != nil {
			return errors.WithMessage(err, "failed to parse created_at time")
		}
		task.CreatedAt = &storedCreatedAt
	}

	return nil
}

func (r *PluginScheduledTaskRepository) Delete(
	ctx context.Context,
	pluginID domain.Uint64ID,
	name string,
) error {
	query, args, err := sq.Delete(base.PluginScheduledTasksTable).
		Where(sq.Eq{"plugin_id": pluginID, "name": name}).
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

func (r *PluginScheduledTaskRepository) DeleteByPlugin(
	ctx context.Context,
	pluginID domain.Uint64ID,
) (int, error) {
	query, args, err := sq.Delete(base.PluginScheduledTasksTable).
		Where(sq.Eq{"plugin_id": pluginID}).
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

func (r *PluginScheduledTaskRepository) FindAll(ctx context.Context) ([]domain.PluginScheduledTask, error) {
	builder := sq.Select(base.PluginScheduledTaskFields...).
		From(base.PluginScheduledTasksTable)

	return r.find(ctx, builder)
}

func (r *PluginScheduledTaskRepository) FindByPlugin(
	ctx context.Context,
	pluginID domain.Uint64ID,
) ([]domain.PluginScheduledTask, error) {
	builder := sq.Select(base.PluginScheduledTaskFields...).
		From(base.PluginScheduledTasksTable).
		Where(sq.Eq{"plugin_id": pluginID})

	return r.find(ctx, builder)
}

func (r *PluginScheduledTaskRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
) ([]domain.PluginScheduledTask, error) {
	query, args, err := builder.OrderBy("plugin_id ASC", "name ASC").ToSql()
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

	var tasks []domain.PluginScheduledTask

	for rows.Next() {
		var task *domain.PluginScheduledTask
		task, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		tasks = append(tasks, *task)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return tasks, nil
}

func (r *PluginScheduledTaskRepository) scan(row base.Scanner) (*domain.PluginScheduledTask, error) {
	var task domain.PluginScheduledTask
	var intervalMS, retryDelayMS, maxJitterMS, timeoutMS int64
	var createdAtStr, updatedAtStr *string

	err := row.Scan(
		&task.ID,
		&task.PluginID,
		&task.Name,
		&intervalMS,
		&task.ErrorPolicy,
		&task.MaxRetries,
		&retryDelayMS,
		&maxJitterMS,
		&timeoutMS,
		&createdAtStr,
		&updatedAtStr,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	task.Interval = time.Duration(intervalMS) * time.Millisecond
	task.RetryDelay = time.Duration(retryDelayMS) * time.Millisecond
	task.MaxJitter = time.Duration(maxJitterMS) * time.Millisecond
	task.Timeout = time.Duration(timeoutMS) * time.Millisecond

	if createdAtStr != nil && *createdAtStr != "" {
		createdAt, err := base.ParseTime(*createdAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse created_at time")
		}
		task.CreatedAt = &createdAt
	}

	if updatedAtStr != nil && *updatedAtStr != "" {
		updatedAt, err := base.ParseTime(*updatedAtStr)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to parse updated_at time")
		}
		task.UpdatedAt = &updatedAt
	}

	return &task, nil
}
