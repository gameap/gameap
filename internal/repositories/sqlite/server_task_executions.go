package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/samber/lo"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

var wrappedServerTaskExecutionFields = lo.Map(
	base.ServerTaskExecutionFields,
	func(s string, _ int) string {
		b := strings.Builder{}
		b.Grow(len(s) + 2)
		b.WriteByte('`')
		b.WriteString(s)
		b.WriteByte('`')

		return b.String()
	},
)

type ServerTaskExecutionRepository struct {
	db base.DB
}

func NewServerTaskExecutionRepository(db base.DB) *ServerTaskExecutionRepository {
	return &ServerTaskExecutionRepository{db: db}
}

func (r *ServerTaskExecutionRepository) Create(
	ctx context.Context, exec *domain.ServerTaskExecution,
) error {
	now := time.Now()
	if exec.CreatedAt.IsZero() {
		exec.CreatedAt = now
	}
	if exec.UpdatedAt.IsZero() {
		exec.UpdatedAt = now
	}
	if exec.Status == "" {
		exec.Status = domain.ServerTaskExecutionStatusRunning
	}

	var finishedAt, outputInline, outputStoragePath, errorMessage any
	if exec.FinishedAt != nil {
		finishedAt = exec.FinishedAt.Format(time.RFC3339)
	}
	if exec.OutputInline != nil {
		outputInline = *exec.OutputInline
	}
	if exec.OutputStoragePath != nil {
		outputStoragePath = *exec.OutputStoragePath
	}
	if exec.ErrorMessage != nil {
		errorMessage = *exec.ErrorMessage
	}

	query := "INSERT OR IGNORE INTO " + base.ServerTaskExecutionsTable + " (" +
		"`execution_id`, `server_task_id`, `server_id`, `node_id`, `command`," +
		"`task_version`, `status`, `exit_code`, `error_message`," +
		"`started_at`, `finished_at`, `duration_ms`," +
		"`output_inline`, `output_storage_path`," +
		"`created_at`, `updated_at`" +
		") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	res, err := r.db.ExecContext(ctx, query,
		exec.ExecutionID, exec.ServerTaskID, exec.ServerID, exec.NodeID, exec.Command,
		exec.TaskVersion, exec.Status, exec.ExitCode, errorMessage,
		exec.StartedAt.Format(time.RFC3339), finishedAt, exec.DurationMS,
		outputInline, outputStoragePath,
		exec.CreatedAt.Format(time.RFC3339), exec.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return errors.WithMessage(err, "failed to insert execution")
	}

	if affected, _ := res.RowsAffected(); affected == 0 {
		return nil
	}

	lastID, err := res.LastInsertId()
	if err != nil {
		return errors.WithMessage(err, "failed to read last insert id")
	}
	if lastID < 0 {
		return errors.New("invalid last insert ID")
	}
	exec.ID = uint(lastID)

	return nil
}

func (r *ServerTaskExecutionRepository) UpdateFinish(
	ctx context.Context,
	executionID string,
	patch repositories.ServerTaskExecutionFinishPatch,
) error {
	if patch.FinishedAt.IsZero() {
		patch.FinishedAt = time.Now()
	}

	query := "UPDATE " + base.ServerTaskExecutionsTable + " SET " +
		"`status` = ?, " +
		"`exit_code` = COALESCE(?, `exit_code`), " +
		"`error_message` = COALESCE(?, `error_message`), " +
		"`finished_at` = ?, " +
		"`duration_ms` = COALESCE(?, `duration_ms`), " +
		"`output_inline` = COALESCE(?, `output_inline`), " +
		"`output_storage_path` = COALESCE(?, `output_storage_path`), " +
		"`updated_at` = ? " +
		"WHERE `execution_id` = ?"

	finishedAt := patch.FinishedAt.Format(time.RFC3339)
	updatedAt := finishedAt

	_, err := r.db.ExecContext(ctx, query,
		patch.Status,
		patch.ExitCode,
		patch.ErrorMessage,
		finishedAt,
		patch.DurationMS,
		patch.OutputInline,
		patch.OutputStoragePath,
		updatedAt,
		executionID,
	)
	if err != nil {
		return errors.WithMessage(err, "failed to update execution finish")
	}

	return nil
}

func (r *ServerTaskExecutionRepository) AppendOutputInline(
	ctx context.Context, executionID string, chunk []byte, maxBytes int,
) error {
	if len(chunk) == 0 {
		return nil
	}

	query := "UPDATE " + base.ServerTaskExecutionsTable + " SET " +
		"`output_inline` = substr(IFNULL(`output_inline`, '') || ?, -?), " +
		"`updated_at` = ? " +
		"WHERE `execution_id` = ?"

	if _, err := r.db.ExecContext(ctx, query, string(chunk), maxBytes,
		time.Now().Format(time.RFC3339), executionID); err != nil {
		return errors.WithMessage(err, "failed to append output")
	}

	return nil
}

func (r *ServerTaskExecutionRepository) FindAll(
	ctx context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTaskExecution, error) {
	builder := sq.Select(wrappedServerTaskExecutionFields...).
		From(base.ServerTaskExecutionsTable)

	return r.find(ctx, builder, order, pagination)
}

func (r *ServerTaskExecutionRepository) Find(
	ctx context.Context,
	filter *filters.FindServerTaskExecution,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTaskExecution, error) {
	builder := sq.Select(wrappedServerTaskExecutionFields...).
		From(base.ServerTaskExecutionsTable)

	if filter != nil {
		builder = builder.Where(r.filterToSq(filter))
	}

	return r.find(ctx, builder, order, pagination)
}

func (r *ServerTaskExecutionRepository) find(
	ctx context.Context,
	builder sq.SelectBuilder,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.ServerTaskExecution, error) {
	if len(order) > 0 {
		for _, o := range order {
			builder = builder.OrderBy(o.Field + " " + o.Direction.String())
		}
	} else {
		builder = builder.OrderBy("started_at DESC")
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
		if cErr := rows.Close(); cErr != nil {
			slog.ErrorContext(ctx, "failed to close rows", "err", cErr)
		}
	}(rows)

	var execs []domain.ServerTaskExecution

	for rows.Next() {
		exec, scanErr := r.scan(rows)
		if scanErr != nil {
			return nil, errors.WithMessage(scanErr, "failed to scan row")
		}

		execs = append(execs, *exec)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return execs, nil
}

func (r *ServerTaskExecutionRepository) MarkAbandoned(
	ctx context.Context,
	nodeID uint,
	keepExecutionIDs []string,
	reason string,
) (int, error) {
	now := time.Now().Format(time.RFC3339)

	builder := sq.Update(base.ServerTaskExecutionsTable).
		Set("status", domain.ServerTaskExecutionStatusFailed).
		Set("error_message", reason).
		Set("finished_at", now).
		Set("updated_at", now).
		Where(sq.Eq{"node_id": nodeID}).
		Where(sq.Eq{"status": domain.ServerTaskExecutionStatusRunning})

	if len(keepExecutionIDs) > 0 {
		builder = builder.Where(sq.NotEq{"execution_id": keepExecutionIDs})
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return 0, errors.WithMessage(err, "failed to build mark-abandoned query")
	}

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to mark abandoned")
	}

	n, _ := res.RowsAffected()

	return int(n), nil
}

func (r *ServerTaskExecutionRepository) DeleteOlderThan(
	ctx context.Context,
	status domain.ServerTaskExecutionStatus,
	before time.Time,
) (int, error) {
	res, err := r.db.ExecContext(ctx,
		"DELETE FROM "+base.ServerTaskExecutionsTable+
			" WHERE `status` = ? AND `finished_at` < ?",
		status, before.Format(time.RFC3339),
	)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to delete old executions")
	}

	n, _ := res.RowsAffected()

	return int(n), nil
}

func (r *ServerTaskExecutionRepository) scan(row base.Scanner) (*domain.ServerTaskExecution, error) {
	var exec domain.ServerTaskExecution

	var startedAtStr, createdAtStr, updatedAtStr string
	var finishedAtStr *string

	err := row.Scan(
		&exec.ID,
		&exec.ExecutionID,
		&exec.ServerTaskID,
		&exec.ServerID,
		&exec.NodeID,
		&exec.Command,
		&exec.TaskVersion,
		&exec.Status,
		&exec.ExitCode,
		&exec.ErrorMessage,
		&startedAtStr,
		&finishedAtStr,
		&exec.DurationMS,
		&exec.OutputInline,
		&exec.OutputStoragePath,
		&createdAtStr,
		&updatedAtStr,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	startedAt, err := base.ParseTime(startedAtStr)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse started_at")
	}
	exec.StartedAt = startedAt

	if finishedAtStr != nil && *finishedAtStr != "" {
		finishedAt, parseErr := base.ParseTime(*finishedAtStr)
		if parseErr != nil {
			return nil, errors.WithMessage(parseErr, "failed to parse finished_at")
		}
		exec.FinishedAt = &finishedAt
	}

	createdAt, err := base.ParseTime(createdAtStr)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse created_at")
	}
	exec.CreatedAt = createdAt

	updatedAt, err := base.ParseTime(updatedAtStr)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse updated_at")
	}
	exec.UpdatedAt = updatedAt

	return &exec, nil
}

func (r *ServerTaskExecutionRepository) filterToSq(filter *filters.FindServerTaskExecution) sq.Sqlizer {
	and := make(sq.And, 0, 8)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}
	if len(filter.ExecutionIDs) > 0 {
		and = append(and, sq.Eq{"execution_id": filter.ExecutionIDs})
	}
	if len(filter.ServerTaskIDs) > 0 {
		and = append(and, sq.Eq{"server_task_id": filter.ServerTaskIDs})
	}
	if len(filter.ServerIDs) > 0 {
		and = append(and, sq.Eq{"server_id": filter.ServerIDs})
	}
	if len(filter.NodeIDs) > 0 {
		and = append(and, sq.Eq{"node_id": filter.NodeIDs})
	}
	if len(filter.Statuses) > 0 {
		and = append(and, sq.Eq{"status": filter.Statuses})
	}
	if len(filter.NotStatuses) > 0 {
		and = append(and, sq.NotEq{"status": filter.NotStatuses})
	}
	if filter.StartedAfter != nil {
		and = append(and, sq.GtOrEq{"started_at": filter.StartedAfter.Format(time.RFC3339)})
	}
	if filter.StartedBefore != nil {
		and = append(and, sq.Lt{"started_at": filter.StartedBefore.Format(time.RFC3339)})
	}

	return and
}
