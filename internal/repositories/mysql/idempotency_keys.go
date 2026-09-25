package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/base"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
)

type IdempotencyKeyRepository struct {
	db base.DB
}

func NewIdempotencyKeyRepository(db base.DB) *IdempotencyKeyRepository {
	return &IdempotencyKeyRepository{
		db: db,
	}
}

func (r *IdempotencyKeyRepository) Find(
	ctx context.Context,
	userID uint,
	keyHash string,
	now time.Time,
) (*domain.IdempotencyKey, error) {
	query, args, err := sq.Select(base.IdempotencyKeyFields...).
		From(base.IdempotencyKeysTable).
		Where(sq.Eq{"user_id": userID, "key_hash": keyHash}).
		Where(sq.Gt{"expires_at": now.UTC()}).
		ToSql()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to build query")
	}

	record, err := r.scan(r.db.QueryRowContext(ctx, query, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, errors.WithMessage(err, "failed to find idempotency key")
	}

	return record, nil
}

// Save has no conditional upsert to lean on, so it first removes a row that
// has expired and then inserts; a duplicate key on the insert means a live
// row still holds the key.
func (r *IdempotencyKeyRepository) Save(ctx context.Context, record *domain.IdempotencyKey) (bool, error) {
	headers, err := json.Marshal(nonNilIdempotencyHeaders(record.ResponseHeaders))
	if err != nil {
		return false, errors.Wrap(err, "failed to marshal response headers")
	}

	_, err = r.db.ExecContext(ctx,
		"DELETE FROM "+base.IdempotencyKeysTable+" WHERE user_id = ? AND key_hash = ? AND expires_at <= ?",
		record.UserID, record.KeyHash, record.CreatedAt.UTC(),
	)
	if err != nil {
		return false, errors.WithMessage(err, "failed to delete expired idempotency key")
	}

	query, args, err := sq.Insert(base.IdempotencyKeysTable).
		Columns(
			"user_id", "key_hash", "request_method", "request_path", "request_fingerprint",
			"response_status", "response_headers", "response_body", "created_at", "expires_at",
		).
		Values(
			record.UserID,
			record.KeyHash,
			record.RequestMethod,
			record.RequestPath,
			record.RequestFingerprint,
			record.ResponseStatus,
			string(headers),
			nonNilIdempotencyBody(record.ResponseBody),
			record.CreatedAt.UTC(),
			record.ExpiresAt.UTC(),
		).
		ToSql()
	if err != nil {
		return false, errors.WithMessage(err, "failed to build query")
	}

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		var mysqlErr *mysqldriver.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrDupEntry {
			return false, nil
		}

		return false, errors.WithMessage(err, "failed to save idempotency key")
	}

	lastID, err := res.LastInsertId()
	if err != nil {
		return false, errors.WithMessage(err, "failed to read last insert id")
	}
	if lastID < 0 {
		return false, errors.New("invalid last insert ID")
	}

	record.ID = uint64(lastID)

	return true, nil
}

func (r *IdempotencyKeyRepository) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	query, args, err := sq.Delete(base.IdempotencyKeysTable).
		Where(sq.LtOrEq{"expires_at": now.UTC()}).
		ToSql()
	if err != nil {
		return 0, errors.WithMessage(err, "failed to build query")
	}

	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to delete expired idempotency keys")
	}

	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, errors.WithMessage(err, "failed to get affected rows")
	}

	return int(deleted), nil
}

func (r *IdempotencyKeyRepository) scan(row base.Scanner) (*domain.IdempotencyKey, error) {
	var record domain.IdempotencyKey
	var headers string

	err := row.Scan(
		&record.ID,
		&record.UserID,
		&record.KeyHash,
		&record.RequestMethod,
		&record.RequestPath,
		&record.RequestFingerprint,
		&record.ResponseStatus,
		&headers,
		&record.ResponseBody,
		&record.CreatedAt,
		&record.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}

	record.ResponseHeaders = map[string][]string{}

	if err = json.Unmarshal([]byte(headers), &record.ResponseHeaders); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response headers")
	}

	return &record, nil
}

func nonNilIdempotencyHeaders(headers map[string][]string) map[string][]string {
	if headers == nil {
		return map[string][]string{}
	}

	return headers
}

// nonNilIdempotencyBody keeps an empty body out of the NOT NULL column as NULL.
func nonNilIdempotencyBody(body []byte) []byte {
	if body == nil {
		return []byte{}
	}

	return body
}
