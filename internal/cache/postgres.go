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

const postgresKVStoreTable = "kv_store"

const postgresDatabaseKeyPrefix = "cache:"

const postgresCreateTableQuery = `
CREATE TABLE IF NOT EXISTS kv_store (
  key VARCHAR(128) PRIMARY KEY,
  value TEXT NOT NULL,
  expires_at TIMESTAMP NULL DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_kv_store_key_hash ON kv_store USING HASH (key);
CREATE INDEX IF NOT EXISTS idx_kv_store_expires ON kv_store (expires_at) WHERE expires_at IS NOT NULL;
`

type PostgreSQL struct {
	db     base.DB
	prefix string
}

func NewPostgreSQL(db base.DB) *PostgreSQL {
	cache := &PostgreSQL{
		db:     db,
		prefix: postgresDatabaseKeyPrefix,
	}

	if err := cache.ensureTable(context.TODO()); err != nil {
		panic(
			fmt.Errorf(
				"failed to ensure kv_store table: %w \n"+
					"You can manually create the table by executing the following SQL statement:\n %s",
				err,
				postgresCreateTableQuery,
			),
		)
	}

	return cache
}

func (c *PostgreSQL) ensureTable(ctx context.Context) error {
	// Check if table exists
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = $1
		)
	`
	err := c.db.QueryRowContext(ctx, query, postgresKVStoreTable).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if table exists: %w", err)
	}

	if exists {
		return nil
	}

	// Table doesn't exist, create it
	_, err = c.db.ExecContext(ctx, postgresCreateTableQuery)
	if err != nil {
		return fmt.Errorf("failed to create kv_store table: %w", err)
	}

	return nil
}

func (c *PostgreSQL) buildKey(key string) string {
	var sb strings.Builder
	sb.Grow(len(c.prefix) + len(key))
	sb.WriteString(c.prefix)
	sb.WriteString(key)

	return sb.String()
}

func (c *PostgreSQL) Get(ctx context.Context, key string) (any, error) {
	fullKey := c.buildKey(key)

	query, args, err := sq.Select("value", "expires_at").
		From(postgresKVStoreTable).
		Where(sq.Eq{"key": fullKey}).
		PlaceholderFormat(sq.Dollar).
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

func (c *PostgreSQL) Set(ctx context.Context, key string, value any, options ...Option) error {
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

	// PostgreSQL UPSERT using ON CONFLICT
	query := `
		INSERT INTO ` + postgresKVStoreTable + ` (key, value, expires_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    expires_at = EXCLUDED.expires_at
	`

	_, err = c.db.ExecContext(ctx, query, fullKey, string(valueJSON), expiresAt)
	if err != nil {
		return fmt.Errorf("failed to set cache value: %w", err)
	}

	return nil
}

func (c *PostgreSQL) Delete(ctx context.Context, key string) error {
	fullKey := c.buildKey(key)

	query, args, err := sq.Delete(postgresKVStoreTable).
		Where(sq.Eq{"key": fullKey}).
		PlaceholderFormat(sq.Dollar).
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

func (c *PostgreSQL) Clear(ctx context.Context) error {
	builder := sq.Delete(postgresKVStoreTable)

	if c.prefix != "" {
		var sb strings.Builder
		sb.WriteString(c.prefix)
		sb.WriteString("%")
		builder = builder.Where(sq.Like{"key": sb.String()})
	}

	query, args, err := builder.PlaceholderFormat(sq.Dollar).ToSql()
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
func (c *PostgreSQL) CleanupExpired(ctx context.Context) error {
	query, args, err := sq.Delete(postgresKVStoreTable).
		Where(sq.And{
			sq.NotEq{"expires_at": nil},
			sq.Lt{"expires_at": time.Now()},
		}).
		PlaceholderFormat(sq.Dollar).
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
