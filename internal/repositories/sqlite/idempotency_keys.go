package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/pkg/errors"
)

// IdempotencyKeyRepository keeps expires_at as unix milliseconds: TEXT
// timestamps do not compare reliably in SQL.
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
		Where(sq.Gt{"expires_at": now.UnixMilli()}).
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

func (r *IdempotencyKeyRepository) Save(ctx context.Context, record *domain.IdempotencyKey) (bool, error) {
	headers, err := json.Marshal(nonNilIdempotencyHeaders(record.ResponseHeaders))
	if err != nil {
		return false, errors.Wrap(err, "failed to marshal response headers")
	}

	// The conflict update is conditional, so a row that still holds the key
	// is left alone and RETURNING yields nothing.
	query := `INSERT INTO ` + base.IdempotencyKeysTable + ` (
		user_id, key_hash, request_method, request_path, request_fingerprint,
		response_status, response_headers, response_body, created_at, expires_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (user_id, key_hash) DO UPDATE SET
		request_method = excluded.request_method,
		request_path = excluded.request_path,
		request_fingerprint = excluded.request_fingerprint,
		response_status = excluded.response_status,
		response_headers = excluded.response_headers,
		response_body = excluded.response_body,
		created_at = excluded.created_at,
		expires_at = excluded.expires_at
	WHERE ` + base.IdempotencyKeysTable + `.expires_at <= ?
	RETURNING id`

	var id uint64

	err = r.db.QueryRowContext(ctx, query,
		record.UserID,
		record.KeyHash,
		record.RequestMethod,
		record.RequestPath,
		record.RequestFingerprint,
		record.ResponseStatus,
		string(headers),
		nonNilIdempotencyBody(record.ResponseBody),
		record.CreatedAt.UTC().Format(time.RFC3339Nano),
		record.ExpiresAt.UnixMilli(),
		record.CreatedAt.UnixMilli(),
	).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}

		return false, errors.WithMessage(err, "failed to save idempotency key")
	}

	record.ID = id

	return true, nil
}

func (r *IdempotencyKeyRepository) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	query, args, err := sq.Delete(base.IdempotencyKeysTable).
		Where(sq.LtOrEq{"expires_at": now.UnixMilli()}).
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
	var headers, createdAt string
	var expiresAtMS int64

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
		&createdAt,
		&expiresAtMS,
	)
	if err != nil {
		return nil, err
	}

	record.CreatedAt, err = base.ParseTime(createdAt)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to parse created_at time")
	}

	record.ExpiresAt = time.UnixMilli(expiresAtMS).UTC()
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
