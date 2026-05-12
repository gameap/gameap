package sqlite

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
		From(base.ServerTasksTable).
		Where(sq.Eq{"deleted_at": nil})

	return r.find(ctx, builder, order, pagination, false)
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

	if useJoin {
		builder = builder.
			Join(base.ServersTable + " ON " + base.ServerTasksTable + ".server_id = " + base.ServersTable + ".id").
			Where(sq.Eq{base.ServersTable + ".ds_id": filter.NodeIDs})
	}

	builder = builder.Where(r.filterToSq(filter, useJoin))

	return r.find(ctx, builder, order, pagination, useJoin)
}

func (r *ServerTaskRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
	useJoin bool,
) ([]domain.ServerTask, error) {
	if len(order) > 0 {
		for _, o := range order {
			orderBy := o.Field
			if useJoin && !strings.Contains(orderBy, ".") {
				orderBy = base.ServerTasksTable + "." + orderBy
			}
			builder = builder.OrderBy(orderBy + " " + o.Direction.String())
		}
	} else {
		builder = builder.OrderBy(base.ServerTasksTable + ".id ASC")
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
	task.UpdatedAt = new(time.Now())

	if task.ID == 0 && (task.CreatedAt == nil || task.CreatedAt.IsZero()) {
		task.CreatedAt = new(time.Now())
	}

	if task.Version == 0 {
		task.Version = 1
	}
	if task.OverlapPolicy == "" {
		task.OverlapPolicy = domain.ServerTaskOverlapPolicySkip
	}
	if task.CatchupPolicy == "" {
		task.CatchupPolicy = domain.ServerTaskCatchupPolicySkip
	}

	var createdAtStr, updatedAtStr, deletedAtStr *string
	if task.CreatedAt != nil {
		createdAtStr = new(task.CreatedAt.Format(time.RFC3339))
	}
	if task.UpdatedAt != nil {
		updatedAtStr = new(task.UpdatedAt.Format(time.RFC3339))
	}
	if task.DeletedAt != nil {
		deletedAtStr = new(task.DeletedAt.Format(time.RFC3339))
	}

	executeDateStr := task.ExecuteDate.Format(time.RFC3339)

	query, args, err := sq.Insert(base.ServerTasksTable).
		Columns(wrappedServerTaskFields...).
		Values(
			lo.EmptyableToPtr(task.ID),
			task.Command,
			task.ServerID,
			task.NodeID,
			task.Name,
			task.Enabled,
			task.OverlapPolicy,
			task.CatchupPolicy,
			task.CreatedByUserID,
			task.Version,
			task.Timezone,
			task.Repeat,
			task.RepeatPeriod.Seconds(),
			task.Counter,
			executeDateStr,
			task.Payload,
			createdAtStr,
			updatedAtStr,
			deletedAtStr,
		).
		Suffix("ON CONFLICT(id) DO UPDATE SET " +
			"`command`=excluded.`command`," +
			"`server_id`=excluded.`server_id`," +
			"`node_id`=excluded.`node_id`," +
			"`name`=excluded.`name`," +
			"`enabled`=excluded.`enabled`," +
			"`overlap_policy`=excluded.`overlap_policy`," +
			"`catchup_policy`=excluded.`catchup_policy`," +
			"`created_by_user_id`=excluded.`created_by_user_id`," +
			"`version`=excluded.`version`," +
			"`timezone`=excluded.`timezone`," +
			"`repeat`=excluded.`repeat`," +
			"`repeat_period`=excluded.`repeat_period`," +
			"`counter`=excluded.`counter`," +
			"`execute_date`=excluded.`execute_date`," +
			"`payload`=excluded.`payload`," +
			"`updated_at`=excluded.`updated_at`," +
			"`deleted_at`=excluded.`deleted_at` " +
			"RETURNING id").
		ToSql()
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

func (r *ServerTaskRepository) BumpVersion(ctx context.Context, id uint) (uint64, error) {
	now := time.Now().Format(time.RFC3339)
	if _, err := r.db.ExecContext(ctx,
		"UPDATE "+base.ServerTasksTable+" SET `version` = `version` + 1, `updated_at` = ? WHERE `id` = ?",
		now, id,
	); err != nil {
		return 0, errors.WithMessage(err, "failed to bump version")
	}

	var version uint64
	if err := r.db.QueryRowContext(ctx,
		"SELECT `version` FROM "+base.ServerTasksTable+" WHERE `id` = ?", id,
	).Scan(&version); err != nil {
		return 0, errors.WithMessage(err, "failed to read bumped version")
	}

	return version, nil
}

func (r *ServerTaskRepository) SoftDelete(ctx context.Context, id uint) error {
	now := time.Now().Format(time.RFC3339)
	if _, err := r.db.ExecContext(ctx,
		"UPDATE "+base.ServerTasksTable+
			" SET `deleted_at` = ?, `version` = `version` + 1, `updated_at` = ? WHERE `id` = ?",
		now, now, id,
	); err != nil {
		return errors.WithMessage(err, "failed to soft-delete task")
	}

	return nil
}

func (r *ServerTaskRepository) Delete(ctx context.Context, id uint) error {
	query, args, err := sq.Delete(base.ServerTasksTable).
		Where(sq.Eq{"id": id}).
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
	var repeatPeriodSeconds float64
	var executeDateStr string
	var createdAtStr, updatedAtStr, deletedAtStr *string

	err := row.Scan(
		&task.ID,
		&task.Command,
		&task.ServerID,
		&task.NodeID,
		&task.Name,
		&task.Enabled,
		&task.OverlapPolicy,
		&task.CatchupPolicy,
		&task.CreatedByUserID,
		&task.Version,
		&task.Timezone,
		&task.Repeat,
		&repeatPeriodSeconds,
		&task.Counter,
		&executeDateStr,
		&task.Payload,
		&createdAtStr,
		&updatedAtStr,
		&deletedAtStr,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	task.RepeatPeriod = time.Duration(repeatPeriodSeconds) * time.Second

	executeDate, err := base.ParseTime(executeDateStr)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse execute_date time")
	}
	task.ExecuteDate = executeDate

	if createdAtStr != nil && *createdAtStr != "" {
		createdAt, parseErr := base.ParseTime(*createdAtStr)
		if parseErr != nil {
			return nil, errors.WithMessage(parseErr, "failed to parse created_at time")
		}
		task.CreatedAt = &createdAt
	}

	if updatedAtStr != nil && *updatedAtStr != "" {
		updatedAt, parseErr := base.ParseTime(*updatedAtStr)
		if parseErr != nil {
			return nil, errors.WithMessage(parseErr, "failed to parse updated_at time")
		}
		task.UpdatedAt = &updatedAt
	}

	if deletedAtStr != nil && *deletedAtStr != "" {
		deletedAt, parseErr := base.ParseTime(*deletedAtStr)
		if parseErr != nil {
			return nil, errors.WithMessage(parseErr, "failed to parse deleted_at time")
		}
		task.DeletedAt = &deletedAt
	}

	return &task, nil
}

func (r *ServerTaskRepository) filterToSq(filter *filters.FindServerTask, useJoin bool) sq.Sqlizer {
	and := make(sq.And, 0, 8)

	var idField, serverIDField, deletedAtField, enabledField string
	if useJoin {
		idField = base.ServerTasksTable + ".id"
		serverIDField = base.ServerTasksTable + ".server_id"
		deletedAtField = base.ServerTasksTable + ".deleted_at"
		enabledField = base.ServerTasksTable + ".enabled"
	} else {
		idField = "id"
		serverIDField = "server_id"
		deletedAtField = "deleted_at"
		enabledField = "enabled"
	}

	if filter == nil || !filter.IncludeDeleted {
		and = append(and, sq.Eq{deletedAtField: nil})
	}

	if filter != nil {
		if len(filter.IDs) > 0 {
			and = append(and, sq.Eq{idField: filter.IDs})
		}
		if len(filter.ServersIDs) > 0 {
			and = append(and, sq.Eq{serverIDField: filter.ServersIDs})
		}
		if filter.OnlyEnabled {
			and = append(and, sq.Eq{enabledField: 1})
		}
	}

	return and
}
