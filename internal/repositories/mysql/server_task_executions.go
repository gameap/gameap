package mysql

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
	"github.com/gameap/gameap/pkg/idgen"
	"github.com/google/uuid"
	"github.com/rs/xid"
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

	executionUUID := idgen.XIDToUUID(exec.ExecutionID).String()

	query := "INSERT IGNORE INTO " + base.ServerTaskExecutionsTable + " (" +
		"`execution_id`, `server_task_id`, `server_id`, `node_id`, `command`," +
		"`task_version`, `status`, `exit_code`, `error_message`," +
		"`started_at`, `finished_at`, `duration_ms`," +
		"`output_inline`, `output_storage_path`," +
		"`created_at`, `updated_at`" +
		") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

	res, err := r.db.ExecContext(ctx, query,
		executionUUID, exec.ServerTaskID, exec.ServerID, exec.NodeID, exec.Command,
		exec.TaskVersion, exec.Status, exec.ExitCode, exec.ErrorMessage,
		exec.StartedAt, exec.FinishedAt, exec.DurationMS,
		exec.OutputInline, exec.OutputStoragePath,
		exec.CreatedAt, exec.UpdatedAt,
	)
	if err != nil {
		return errors.WithMessage(err, "failed to insert execution")
	}

	if affected, _ := res.RowsAffected(); affected == 0 {
		// INSERT IGNORE skipped a duplicate — idempotent path.
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
	executionID xid.ID,
	patch repositories.ServerTaskExecutionFinishPatch,
) error {
	if patch.FinishedAt.IsZero() {
		patch.FinishedAt = time.Now()
	}

	executionUUID := idgen.XIDToUUID(executionID).String()

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

	_, err := r.db.ExecContext(ctx, query,
		patch.Status,
		patch.ExitCode,
		patch.ErrorMessage,
		patch.FinishedAt,
		patch.DurationMS,
		patch.OutputInline,
		patch.OutputStoragePath,
		patch.FinishedAt,
		executionUUID,
	)
	if err != nil {
		return errors.WithMessage(err, "failed to update execution finish")
	}

	return nil
}

func (r *ServerTaskExecutionRepository) AppendOutputInline(
	ctx context.Context, executionID xid.ID, chunk []byte, maxBytes int,
) error {
	if len(chunk) == 0 {
		return nil
	}

	executionUUID := idgen.XIDToUUID(executionID).String()

	query := "UPDATE " + base.ServerTaskExecutionsTable + " SET " +
		"`output_inline` = RIGHT(CONCAT(IFNULL(`output_inline`, ''), ?), ?), " +
		"`updated_at` = ? " +
		"WHERE `execution_id` = ?"

	if _, err := r.db.ExecContext(ctx, query, string(chunk), maxBytes, time.Now(), executionUUID); err != nil {
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
	keepExecutionIDs []xid.ID,
	reason string,
) (int, error) {
	now := time.Now()

	keepUUIDs := convertXIDsToUUIDs(keepExecutionIDs)

	builder := sq.Update(base.ServerTaskExecutionsTable).
		Set("status", domain.ServerTaskExecutionStatusFailed).
		Set("error_message", reason).
		Set("finished_at", now).
		Set("updated_at", now).
		Where(sq.Eq{"node_id": nodeID}).
		Where(sq.Eq{"status": domain.ServerTaskExecutionStatusRunning})

	if len(keepUUIDs) > 0 {
		builder = builder.Where(sq.NotEq{"execution_id": keepUUIDs})
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
		"DELETE FROM "+base.ServerTaskExecutionsTable+" WHERE `status` = ? AND `finished_at` < ?",
		status, before,
	)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to delete old executions")
	}

	n, _ := res.RowsAffected()

	return int(n), nil
}

func (r *ServerTaskExecutionRepository) scan(row base.Scanner) (*domain.ServerTaskExecution, error) {
	var exec domain.ServerTaskExecution
	var executionUUIDStr string

	err := row.Scan(
		&exec.ID,
		&executionUUIDStr,
		&exec.ServerTaskID,
		&exec.ServerID,
		&exec.NodeID,
		&exec.Command,
		&exec.TaskVersion,
		&exec.Status,
		&exec.ExitCode,
		&exec.ErrorMessage,
		&exec.StartedAt,
		&exec.FinishedAt,
		&exec.DurationMS,
		&exec.OutputInline,
		&exec.OutputStoragePath,
		&exec.CreatedAt,
		&exec.UpdatedAt,
	)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to scan row")
	}

	parsed, err := uuid.Parse(executionUUIDStr)
	if err != nil {
		return nil, errors.WithMessage(err, "parse execution uuid")
	}
	exec.ExecutionID = idgen.UUIDToXID(parsed)

	return &exec, nil
}

func convertXIDsToUUIDs(ids []xid.ID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, idgen.XIDToUUID(id).String())
	}

	return out
}

func (r *ServerTaskExecutionRepository) filterToSq(filter *filters.FindServerTaskExecution) sq.Sqlizer {
	and := make(sq.And, 0, 8)

	if len(filter.IDs) > 0 {
		and = append(and, sq.Eq{"id": filter.IDs})
	}
	if len(filter.ExecutionIDs) > 0 {
		and = append(and, sq.Eq{"execution_id": convertXIDsToUUIDs(filter.ExecutionIDs)})
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
		and = append(and, sq.GtOrEq{"started_at": *filter.StartedAfter})
	}
	if filter.StartedBefore != nil {
		and = append(and, sq.Lt{"started_at": *filter.StartedBefore})
	}

	return and
}
