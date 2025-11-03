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
  ` + "`value`" + ` MEDIUMTEXT NOT NULL,
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

	// Check expiration
	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
		_ = c.Delete(ctx, key)

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
