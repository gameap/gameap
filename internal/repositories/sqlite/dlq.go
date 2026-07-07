package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/dlq"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/pkg/errors"
)

type DLQRepository struct {
	db base.DB
}

var dlqSelectColumns = []string{
	"id", "channel", "original_message", "error",
	"attempt_count", "failed_at", "processed", "processed_at",
}

func NewDLQRepository(db base.DB) *DLQRepository {
	return &DLQRepository{
		db: db,
	}
}

func (r *DLQRepository) Push(ctx context.Context, msg *dlq.FailedMessage) error {
	originalMsgJSON, err := json.Marshal(msg.OriginalMsg)
	if err != nil {
		return errors.WithMessage(err, "failed to marshal original message")
	}

	failedAtStr := msg.FailedAt.Format(time.RFC3339Nano)

	query, args, err := sq.Insert(base.DLQTable).
		Columns("id", "channel", "original_message", "error", "attempt_count", "failed_at", "processed").
		Values(msg.ID, msg.Channel, originalMsgJSON, msg.Error, msg.AttemptCount, failedAtStr, msg.Processed).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build insert query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute insert query")
	}

	return nil
}

func (r *DLQRepository) Pop(ctx context.Context) (*dlq.FailedMessage, error) {
	query, args, err := sq.Select(dlqSelectColumns...).
		From(base.DLQTable).
		Where(sq.Eq{"processed": false}).
		OrderBy("failed_at ASC").
		Limit(1).
		ToSql()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to build select query")
	}

	row := r.db.QueryRowContext(ctx, query, args...)
	msg, err := r.scanRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, dlq.ErrEmpty
		}

		return nil, errors.WithMessage(err, "failed to scan row")
	}

	deleteQuery, deleteArgs, err := sq.Delete(base.DLQTable).
		Where(sq.Eq{"id": msg.ID}).
		ToSql()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to build delete query")
	}

	res, err := r.db.ExecContext(ctx, deleteQuery, deleteArgs...)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to execute delete query")
	}

	// The SELECT and DELETE are not one atomic statement, so two concurrent
	// consumers (or panel instances) can read the same row. Only the consumer
	// whose DELETE actually removes the row may deliver it; a zero-row delete
	// means another consumer already claimed it, so we report empty rather than
	// delivering the message twice.
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to read delete result")
	}
	if affected == 0 {
		return nil, dlq.ErrEmpty
	}

	return msg, nil
}

func (r *DLQRepository) List(ctx context.Context, limit, offset int) ([]dlq.FailedMessage, error) {
	query, args, err := sq.Select(dlqSelectColumns...).
		From(base.DLQTable).
		OrderBy("failed_at DESC").
		Limit(uint64(max(0, limit))).
		Offset(uint64(max(0, offset))).
		ToSql()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to build select query")
	}

	rows, err := r.db.QueryContext(ctx, query, args...) //nolint:sqlclosecheck
	if err != nil {
		return nil, errors.WithMessage(err, "failed to execute select query")
	}
	defer func(rows *sql.Rows) {
		if closeErr := rows.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "failed to close rows", "error", closeErr)
		}
	}(rows)

	var messages []dlq.FailedMessage
	for rows.Next() {
		msg, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, errors.WithMessage(scanErr, "failed to scan row")
		}
		messages = append(messages, *msg)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.WithMessage(err, "rows iteration error")
	}

	return messages, nil
}

func (r *DLQRepository) Count(ctx context.Context) (int, error) {
	query, args, err := sq.Select("COUNT(*)").
		From(base.DLQTable).
		Where(sq.Eq{"processed": false}).
		ToSql()
	if err != nil {
		return 0, errors.WithMessage(err, "failed to build count query")
	}

	var count int
	if err = r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, errors.WithMessage(err, "failed to execute count query")
	}

	return count, nil
}

func (r *DLQRepository) MarkProcessed(ctx context.Context, id string) error {
	processedAtStr := time.Now().Format(time.RFC3339Nano)

	query, args, err := sq.Update(base.DLQTable).
		Set("processed", true).
		Set("processed_at", processedAtStr).
		Where(sq.Eq{"id": id}).
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

func (r *DLQRepository) Delete(ctx context.Context, id string) error {
	query, args, err := sq.Delete(base.DLQTable).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build delete query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute delete query")
	}

	return nil
}

func (r *DLQRepository) Purge(ctx context.Context) error {
	query, args, err := sq.Delete(base.DLQTable).
		ToSql()
	if err != nil {
		return errors.WithMessage(err, "failed to build delete query")
	}

	_, err = r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.WithMessage(err, "failed to execute delete query")
	}

	return nil
}

func (r *DLQRepository) scanRow(row *sql.Row) (*dlq.FailedMessage, error) {
	var msg dlq.FailedMessage
	var originalMsgJSON []byte
	var failedAtStr string
	var processedAtStr *string

	err := row.Scan(
		&msg.ID,
		&msg.Channel,
		&originalMsgJSON,
		&msg.Error,
		&msg.AttemptCount,
		&failedAtStr,
		&msg.Processed,
		&processedAtStr,
	)
	if err != nil {
		return nil, err
	}

	msg.FailedAt, err = base.ParseTime(failedAtStr)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse failed_at time")
	}

	if processedAtStr != nil && *processedAtStr != "" {
		processedAt, parseErr := base.ParseTime(*processedAtStr)
		if parseErr != nil {
			return nil, errors.WithMessage(parseErr, "failed to parse processed_at time")
		}
		msg.ProcessedAt = &processedAt
	}

	msg.OriginalMsg = &pubsub.Message{}
	if err = json.Unmarshal(originalMsgJSON, msg.OriginalMsg); err != nil {
		return nil, errors.WithMessage(err, "failed to unmarshal original message")
	}

	return &msg, nil
}

func (r *DLQRepository) scanRows(rows *sql.Rows) (*dlq.FailedMessage, error) {
	var msg dlq.FailedMessage
	var originalMsgJSON []byte
	var failedAtStr string
	var processedAtStr *string

	err := rows.Scan(
		&msg.ID,
		&msg.Channel,
		&originalMsgJSON,
		&msg.Error,
		&msg.AttemptCount,
		&failedAtStr,
		&msg.Processed,
		&processedAtStr,
	)
	if err != nil {
		return nil, err
	}

	msg.FailedAt, err = base.ParseTime(failedAtStr)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse failed_at time")
	}

	if processedAtStr != nil && *processedAtStr != "" {
		processedAt, parseErr := base.ParseTime(*processedAtStr)
		if parseErr != nil {
			return nil, errors.WithMessage(parseErr, "failed to parse processed_at time")
		}
		msg.ProcessedAt = &processedAt
	}

	msg.OriginalMsg = &pubsub.Message{}
	if err = json.Unmarshal(originalMsgJSON, msg.OriginalMsg); err != nil {
		return nil, errors.WithMessage(err, "failed to unmarshal original message")
	}

	return &msg, nil
}
