package postgres

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/samber/lo"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

var (
	daemonTaskFieldsWithoutOutput = lo.Filter(base.DaemonTaskFields, func(field string, _ int) bool {
		return field != "output"
	})
)

type DaemonTaskRepository struct {
	db base.DB
}

func NewDaemonTaskRepository(db base.DB) *DaemonTaskRepository {
	return &DaemonTaskRepository{
		db: db,
	}
}

func (r *DaemonTaskRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.DaemonTask, error) {
	builder := sq.Select(daemonTaskFieldsWithoutOutput...).
		From(base.DaemonTasksTable)

	return r.find(ctx, builder, order, pagination, false)
}

func (r *DaemonTaskRepository) FindWithOutput(
	ctx context.Context,
	filter *filters.FindDaemonTask,
	order []filters.Sorting, pagination *filters.Pagination,
) ([]domain.DaemonTask, error) {
	builder := sq.Select(base.DaemonTaskFields...).
		From(base.DaemonTasksTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination, true)
}

func (r *DaemonTaskRepository) Find(
	ctx context.Context,
	filter *filters.FindDaemonTask,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.DaemonTask, error) {
	builder := sq.Select(daemonTaskFieldsWithoutOutput...).
		From(base.DaemonTasksTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination, false)
}

func (r *DaemonTaskRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
	withOutput bool,
) ([]domain.DaemonTask, error) {
	if len(order) > 0 {
		for _, o := range order {
			builder = builder.OrderBy(o.String())
		}
	} else {
		builder = builder.OrderBy("id ASC")
	}

	if pagination != nil {
		if pagination.Limit <= 0 {
			pagination.Limit = filters.DefaultLimit
		}

		if pagination.Offset < 0 {
			pagination.Offset = 0
		}

		builder = builder.Limit(uint64(pagination.Limit)).Offset(uint64(pagination.Offset))
	}

	query, args, err := builder.PlaceholderFormat(sq.Dollar).ToSql()
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

	var tasks []domain.DaemonTask

	for rows.Next() {
		var task *domain.DaemonTask

		if withOutput {
			task, err = r.scan(rows)
		} else {
			task, err = r.scanWithoutOutput(rows)
		}
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

func (r *DaemonTaskRepository) Save(ctx context.Context, task *domain.DaemonTask) error {
	if task.UpdatedAt == nil || task.UpdatedAt.IsZero() {
		task.UpdatedAt = lo.ToPtr(time.Now())
	}

	if task.ID == 0 && (task.CreatedAt == nil || task.CreatedAt.IsZero()) {
		task.CreatedAt = lo.ToPtr(time.Now())
	}

	builder := sq.Insert(base.DaemonTasksTable)

	if task.ID == 0 {
		builder = builder.
			Columns(
				"run_aft_id",
				"created_at",
				"updated_at",
				"dedicated_server_id",
				"server_id",
				"task",
				"data",
				"cmd",
				"output",
				"status",
			).
			Values(
				task.RunAftID,
				task.CreatedAt,
				task.UpdatedAt,
				task.DedicatedServerID,
				task.ServerID,
				task.Task,
				task.Data,
				task.Cmd,
				task.Output,
				task.Status,
			).
			Suffix("RETURNING id")
	} else {
		builder = builder.
			Columns(base.DaemonTaskFields...).
			Values(
				task.ID,
				task.RunAftID,
				task.CreatedAt,
				task.UpdatedAt,
				task.DedicatedServerID,
				task.ServerID,
				task.Task,
				task.Data,
				task.Cmd,
				task.Output,
				task.Status,
			).
			Suffix("ON CONFLICT(id) DO UPDATE SET " +
				"run_aft_id=excluded.run_aft_id," +
				"created_at=excluded.created_at," +
				"updated_at=excluded.updated_at," +
				"dedicated_server_id=excluded.dedicated_server_id," +
				"server_id=excluded.server_id," +
				"task=excluded.task," +
				"data=excluded.data," +
				"cmd=excluded.cmd," +
				"output=excluded.output," +
				"status=excluded.status " +
				"RETURNING id")
	}

	query, args, err := builder.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	var returnedID uint
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&returnedID)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	if task.ID == 0 {
		task.ID = returnedID
	}

	return nil
}

func (r *DaemonTaskRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.DaemonTasksTable).
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

func (r *DaemonTaskRepository) AppendOutput(ctx context.Context, id uint, output string) error {
	query, args, err := sq.Update(base.DaemonTasksTable).
		Set("output", sq.Expr("COALESCE(output, '') || ?", output)).
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

func (r *DaemonTaskRepository) Count(ctx context.Context, filter *filters.FindDaemonTask) (int, error) {
	query, args, err := sq.Select("COUNT(*)").
		From(base.DaemonTasksTable).
		Where(r.filterToSq(filter)).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return 0, errors.WithMessage(err, "failed to build query")
	}

	var count int
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to execute query")
	}

	return count, nil
}

func (r *DaemonTaskRepository) Exists(ctx context.Context, filter *filters.FindDaemonTask) (bool, error) {
	if filter == nil {
		return false, nil
	}

	innerQuery, args, err := sq.Select("1").
		From(base.DaemonTasksTable).
		Where(r.filterToSq(filter)).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return false, errors.WithMessage(err, "failed to build query")
	}

	query := "SELECT EXISTS(" + innerQuery + ")"

	var exists bool
	err = r.db.QueryRowContext(ctx, query, args...).Scan(&exists)
	if err != nil {
		return false, errors.WithMessage(err, "failed to execute query")
	}

	return exists, nil
}

func (r *DaemonTaskRepository) scanWithoutOutput(row base.Scanner) (*domain.DaemonTask, error) {
	var task domain.DaemonTask

	err := row.Scan(
		&task.ID,
		&task.RunAftID,
		&task.CreatedAt,
		&task.UpdatedAt,
		&task.DedicatedServerID,
		&task.ServerID,
		&task.Task,
		&task.Data,
		&task.Cmd,
		&task.Status,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &task, nil
}

func (r *DaemonTaskRepository) scan(row base.Scanner) (*domain.DaemonTask, error) {
	var task domain.DaemonTask

	err := row.Scan(
		&task.ID,
		&task.RunAftID,
		&task.CreatedAt,
		&task.UpdatedAt,
		&task.DedicatedServerID,
		&task.ServerID,
		&task.Task,
		&task.Data,
		&task.Cmd,
		&task.Output,
		&task.Status,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &task, nil
}

func (r *DaemonTaskRepository) filterToSq(filter *filters.FindDaemonTask) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 5)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	if len(filter.DedicatedServerIDs) > 0 {
		and = append(and, sq.Eq{"dedicated_server_id": filter.DedicatedServerIDs})
	}

	if len(filter.ServerIDs) > 0 {
		and = append(and, sq.Eq{"server_id": filter.ServerIDs})
	}

	if len(filter.Tasks) > 0 {
		and = append(and, sq.Eq{"task": filter.Tasks})
	}

	if len(filter.Statuses) > 0 {
		and = append(and, sq.Eq{"status": filter.Statuses})
	}

	return and
}
