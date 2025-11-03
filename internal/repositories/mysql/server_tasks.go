package mysql

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/samber/lo"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

var (
	wrappedServerTaskFields = lo.Map(base.ServerTaskFields, func(s string, _ int) string {
		b := strings.Builder{}
		b.Grow(len(s) + 2)
		b.WriteByte('`')
		b.WriteString(s)
		b.WriteByte('`')

		return b.String()
	})
	qualifiedServerTaskFields = lo.Map(base.ServerTaskFields, func(s string, _ int) string {
		b := strings.Builder{}
		b.Grow(len(base.ServerTasksTable) + len(s) + 4)
		b.WriteString(base.ServerTasksTable)
		b.WriteString(".`")
		b.WriteString(s)
		b.WriteByte('`')

		return b.String()
	})
)

type ServerTaskRepository struct {
	db base.DB
}

func NewServerTaskRepository(db base.DB) *ServerTaskRepository {
	return &ServerTaskRepository{
		db: db,
	}
}

func (r *ServerTaskRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTask, error) {
	builder := sq.Select(wrappedServerTaskFields...).
		From(base.ServerTasksTable)

	return r.find(ctx, builder, order, pagination)
}

func (r *ServerTaskRepository) Find(
	ctx context.Context,
	filter *filters.FindServerTask,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTask, error) {
	useJoin := filter != nil && len(filter.NodeIDs) > 0

	var fields []string
	if useJoin {
		fields = qualifiedServerTaskFields
	} else {
		fields = wrappedServerTaskFields
	}

	builder := sq.Select(fields...).
		From(base.ServerTasksTable)

	// Add JOIN if filtering by NodeIDs
	if useJoin {
		builder = builder.
			Join(base.ServersTable + " ON " + base.ServerTasksTable + ".server_id = " + base.ServersTable + ".id").
			Where(sq.Eq{base.ServersTable + ".ds_id": filter.NodeIDs})
	}

	builder = builder.Where(r.filterToSq(filter, useJoin))

	return r.find(ctx, builder, order, pagination)
}

func (r *ServerTaskRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTask, error) {
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

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to build query")
	}

	rows, err := r.db.QueryContext(ctx, query, args...) //nolint:sqlclosecheck // closed in defer
	if err != nil {
		return nil, errors.WithMessage(err, "failed to execute query")
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.ErrorContext(ctx, "failed to close rows stream", "query", query, "err", err)
		}
	}(rows)

	var tasks []domain.ServerTask

	for rows.Next() {
		var task *domain.ServerTask
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

func (r *ServerTaskRepository) Save(ctx context.Context, task *domain.ServerTask) error {
	if task.UpdatedAt == nil || task.UpdatedAt.IsZero() {
		task.UpdatedAt = lo.ToPtr(time.Now())
	}

	if task.ID == 0 && (task.CreatedAt == nil || task.CreatedAt.IsZero()) {
		task.CreatedAt = lo.ToPtr(time.Now())
	}

	query, args, err := sq.Insert(base.ServerTasksTable).
		Columns(wrappedServerTaskFields...).
		Values(
			task.ID,
			task.Command,
			task.ServerID,
			task.Repeat,
			task.RepeatPeriod.Seconds(),
			task.Counter,
			task.ExecuteDate,
			task.Payload,
			task.CreatedAt,
			task.UpdatedAt,
		).
		Suffix("ON DUPLICATE KEY UPDATE " +
			"`command`=VALUES(`command`)," +
			"`server_id`=VALUES(`server_id`)," +
			"`repeat`=VALUES(`repeat`)," +
			"`repeat_period`=VALUES(`repeat_period`)," +
			"`counter`=VALUES(`counter`)," +
			"`execute_date`=VALUES(`execute_date`)," +
			"`payload`=VALUES(`payload`)," +
			"`updated_at`=VALUES(`updated_at`)").
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	if task.ID == 0 {
		lastID, err := result.LastInsertId()
		if err != nil {
			return errors.WithMessage(err, "failed to get last insert ID")
		}
		if lastID < 0 {
			return errors.New("invalid last insert ID")
		}
		task.ID = uint(lastID)
	}

	return nil
}

func (r *ServerTaskRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.ServerTasksTable).
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

func (r *ServerTaskRepository) scan(row base.Scanner) (*domain.ServerTask, error) {
	var task domain.ServerTask

	var repeatPeriodInSeconds int64

	err := row.Scan(
		&task.ID,
		&task.Command,
		&task.ServerID,
		&task.Repeat,
		&repeatPeriodInSeconds,
		&task.Counter,
		&task.ExecuteDate,
		&task.Payload,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	task.RepeatPeriod = time.Duration(repeatPeriodInSeconds) * time.Second

	return &task, nil
}

func (r *ServerTaskRepository) filterToSq(filter *filters.FindServerTask, useJoin bool) sq.Sqlizer {
	if filter == nil {
		return nil
	}
	and := make(sq.And, 0, 6)

	var idField, serverIDField string
	if useJoin {
		idField = base.ServerTasksTable + ".id"
		serverIDField = base.ServerTasksTable + ".server_id"
	} else {
		idField = "id"
		serverIDField = "server_id"
	}

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{idField: filter.IDs})
	}

	if len(filter.ServersIDs) > 0 {
		and = append(and, sq.Eq{serverIDField: filter.ServersIDs})
	}

	// NodeIDs is handled in the Find method via JOIN condition
	// No need to add it here since it's already in the WHERE clause

	return and
}
