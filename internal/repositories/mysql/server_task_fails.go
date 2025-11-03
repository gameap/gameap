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
	wrappedServerTaskFailFields = lo.Map(base.ServerTaskFailFields, func(s string, _ int) string {
		b := strings.Builder{}
		b.Grow(len(s) + 2)
		b.WriteByte('`')
		b.WriteString(s)
		b.WriteByte('`')

		return b.String()
	})
)

type ServerTaskFailRepository struct {
	db base.DB
}

func NewServerTaskFailRepository(db base.DB) *ServerTaskFailRepository {
	return &ServerTaskFailRepository{
		db: db,
	}
}

func (r *ServerTaskFailRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTaskFail, error) {
	builder := sq.Select(wrappedServerTaskFailFields...).
		From(base.ServerTaskFailsTable)

	return r.find(ctx, builder, order, pagination)
}

func (r *ServerTaskFailRepository) Find(
	ctx context.Context,
	filter *filters.FindServerTaskFail,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTaskFail, error) {
	builder := sq.Select(wrappedServerTaskFailFields...).
		From(base.ServerTaskFailsTable).
		Where(r.filterToSq(filter))

	return r.find(ctx, builder, order, pagination)
}

func (r *ServerTaskFailRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTaskFail, error) {
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

	var taskFails []domain.ServerTaskFail

	for rows.Next() {
		var taskFail *domain.ServerTaskFail
		taskFail, err = r.scan(rows)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to scan row")
		}

		taskFails = append(taskFails, *taskFail)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return taskFails, nil
}

func (r *ServerTaskFailRepository) Save(ctx context.Context, taskFail *domain.ServerTaskFail) error {
	if taskFail.UpdatedAt == nil || taskFail.UpdatedAt.IsZero() {
		taskFail.UpdatedAt = lo.ToPtr(time.Now())
	}

	if taskFail.ID == 0 && (taskFail.CreatedAt == nil || taskFail.CreatedAt.IsZero()) {
		taskFail.CreatedAt = lo.ToPtr(time.Now())
	}

	query, args, err := sq.Insert(base.ServerTaskFailsTable).
		Columns(base.ServerTaskFailFields...).
		Values(
			taskFail.ID,
			taskFail.ServerTaskID,
			taskFail.Output,
			taskFail.CreatedAt,
			taskFail.UpdatedAt,
		).
		Suffix("ON DUPLICATE KEY UPDATE " +
			"server_task_id=VALUES(server_task_id)," +
			"output=VALUES(output)," +
			"updated_at=VALUES(updated_at)").
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build query")
	}

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute query")
	}

	if taskFail.ID == 0 {
		lastID, err := result.LastInsertId()
		if err != nil {
			return errors.WithMessage(err, "failed to get last insert ID")
		}
		if lastID < 0 {
			return errors.New("invalid last insert ID")
		}
		taskFail.ID = uint(lastID)
	}

	return nil
}

func (r *ServerTaskFailRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.ServerTaskFailsTable).
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

func (r *ServerTaskFailRepository) DeleteByServerTaskID(ctx context.Context, serverTaskID uint) error {
	query, args, err := sq.Delete(base.ServerTaskFailsTable).
		Where(sq.Eq{"server_task_id": serverTaskID}).
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

func (r *ServerTaskFailRepository) Count(ctx context.Context, filter *filters.FindServerTaskFail) (int, error) {
	query, args, err := sq.Select("COUNT(*)").
		From(base.ServerTaskFailsTable).
		Where(r.filterToSq(filter)).
		PlaceholderFormat(sq.Question).
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

func (r *ServerTaskFailRepository) scan(row base.Scanner) (*domain.ServerTaskFail, error) {
	var taskFail domain.ServerTaskFail

	err := row.Scan(
		&taskFail.ID,
		&taskFail.ServerTaskID,
		&taskFail.Output,
		&taskFail.CreatedAt,
		&taskFail.UpdatedAt,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	return &taskFail, nil
}

func (r *ServerTaskFailRepository) filterToSq(filter *filters.FindServerTaskFail) sq.Sqlizer {
	if filter == nil {
		return nil
	}

	and := make(sq.And, 0, 4)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}

	if len(filter.ServerTaskIDs) > 0 {
		and = append(and, sq.Eq{"server_task_id": filter.ServerTaskIDs})
	}

	if filter.CreatedAfter != nil {
		and = append(and, sq.GtOrEq{"created_at": filter.CreatedAfter})
	}

	if filter.CreatedBefore != nil {
		and = append(and, sq.LtOrEq{"created_at": filter.CreatedBefore})
	}

	return and
}
