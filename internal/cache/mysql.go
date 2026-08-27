package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/repositories/base"
)

const kvStoreTable = "kv_store"

const databaseKeyPrefix = "cache:"

const createTableQuery = `
CREATE TABLE IF NOT EXISTS kv_store (
  ` + "`key`" + ` VARCHAR(128) PRIMARY KEY,
  ` + "`value`" + ` MEDIUMBLOB NOT NULL,
  ` + "`expires_at`" + ` TIMESTAMP NULL DEFAULT NULL,
  INDEX idx_expires (` + "`expires_at`" + `)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
`

type MySQL struct {
	db     base.DB
	prefix string
}

func NewMySQL(db base.DB) *MySQL {
	cache := &MySQL{
		db:     db,
		prefix: databaseKeyPrefix,
	}

	if err := cache.ensureTable(context.TODO()); err != nil {
		panic(
			fmt.Errorf(
				"failed to ensure kv_store table: %w \n"+
					"You can manually create the table by executing the following SQL statement:\n %s",
				err,
				createTableQuery,
			),
		)
	}

	return cache
}

func (c *MySQL) ensureTable(ctx context.Context) error {
	// Check if table exists using SHOW TABLES LIKE
	var tableName string
	query := fmt.Sprintf("SHOW TABLES LIKE '%s'", kvStoreTable)
	err := c.db.QueryRowContext(ctx, query).Scan(&tableName)
	if err == nil {
		// Table exists
		return nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check if table exists: %w", err)
	}

	// Table doesn't exist, create it
	_, err = c.db.ExecContext(ctx, createTableQuery)
	if err != nil {
		return fmt.Errorf("failed to create kv_store table: %w", err)
	}

	return nil
}

func (c *MySQL) buildKey(key string) string {
	var sb strings.Builder
	sb.Grow(len(c.prefix) + len(key))
	sb.WriteString(c.prefix)
	sb.WriteString(key)

	return sb.String()
}

func (c *MySQL) Get(ctx context.Context, key string) (any, error) {
	fullKey := c.buildKey(key)

	query, args, err := sq.Select("value", "expires_at").
		From(kvStoreTable).
		Where(sq.Eq{"`key`": fullKey}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	var valueJSON string
	var expiresAt sql.NullTime

	err = c.db.QueryRowContext(ctx, query, args...).Scan(&valueJSON, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("failed to query row: %w", err)
	}

	// Treat an expired row as absent. We deliberately do NOT delete it inline:
	// that turns every read of an expired key into a write (amplified under a
	// stampede of concurrent readers). The background CleanupExpired sweep
	// reclaims the row.
	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
		return nil, ErrNotFound
	}

	var value any
	if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return value, nil
}

// Pull returns a value and removes it. MySQL has no DELETE ... RETURNING, so it
// reads the payload and then claims the row with a DELETE: the row's removal is
// the atomic arbiter — of two concurrent redemptions exactly one sees
// RowsAffected == 1 and gets the value, the other gets ErrNotFound.
func (c *MySQL) Pull(ctx context.Context, key string) (any, error) {
	fullKey := c.buildKey(key)

	selectQuery, selectArgs, err := sq.Select("value", "expires_at").
		From(kvStoreTable).
		Where(sq.Eq{"`key`": fullKey}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	var valueJSON string
	var expiresAt sql.NullTime

	err = c.db.QueryRowContext(ctx, selectQuery, selectArgs...).Scan(&valueJSON, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("failed to query row: %w", err)
	}

	// Compare-and-delete on the value just read. MySQL has no DELETE ...
	// RETURNING (postgres Pull uses one, redis uses GETDEL), so without the value
	// in the predicate a Set landing between the two statements would have this
	// call return the old value while deleting the new one.
	deleteQuery, deleteArgs, err := sq.Delete(kvStoreTable).
		Where(sq.Eq{"`key`": fullKey, "`value`": valueJSON}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	result, err := c.db.ExecContext(ctx, deleteQuery, deleteArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to pull cache value: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to read rows affected: %w", err)
	}

	// Lost the race: another redemption removed the row, or a Set replaced the
	// value between the SELECT and the DELETE. Either way this caller removed
	// nothing and must not hand back what it read.
	if affected == 0 {
		return nil, ErrNotFound
	}

	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
		return nil, ErrNotFound
	}

	var value any
	if err := json.Unmarshal([]byte(valueJSON), &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return value, nil
}

func (c *MySQL) Set(ctx context.Context, key string, value any, options ...Option) error {
	opts := ApplyOptions(options...)
	fullKey := c.buildKey(key)

	valueJSON, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	var expiresAt sql.NullTime
	if opts.Expiration > 0 {
		expiresAt = sql.NullTime{
			Time:  time.Now().Add(opts.Expiration),
			Valid: true,
		}
	}

	query, args, err := sq.Insert(kvStoreTable).
		Columns("`key`", "`value`", "`expires_at`").
		Values(fullKey, string(valueJSON), expiresAt).
		Suffix("ON DUPLICATE KEY UPDATE `value`=VALUES(`value`), `expires_at`=VALUES(`expires_at`)").
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = c.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to set cache value: %w", err)
	}

	return nil
}

func (c *MySQL) Delete(ctx context.Context, key string) error {
	fullKey := c.buildKey(key)

	query, args, err := sq.Delete(kvStoreTable).
		Where(sq.Eq{"`key`": fullKey}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = c.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete cache value: %w", err)
	}

	return nil
}

func (c *MySQL) Clear(ctx context.Context) error {
	builder := sq.Delete(kvStoreTable)

	if c.prefix != "" {
		var sb strings.Builder
		sb.WriteString(c.prefix)
		sb.WriteString("%")
		builder = builder.Where(sq.Like{"`key`": sb.String()})
	}

	query, args, err := builder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = c.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	return nil
}

// CleanupExpired removes all expired entries from the cache.
func (c *MySQL) CleanupExpired(ctx context.Context) error {
	query, args, err := sq.Delete(kvStoreTable).
		Where(sq.And{
			sq.NotEq{"expires_at": nil},
			sq.Lt{"expires_at": time.Now()},
		}).
		PlaceholderFormat(sq.Question).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	_, err = c.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired cache entries: %w", err)
	}

	return nil
}
