package locker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/pkg/errors"
)

// DBDialect selects the SQL flavor used by DBLocker.
type DBDialect string

const (
	DBDialectSQLite   DBDialect = "sqlite"
	DBDialectMySQL    DBDialect = "mysql"
	DBDialectPostgres DBDialect = "postgres"
)

// dbLockKeyPrefix keeps lock rows disjoint from the DB cache backends, which
// share the kv_store table under the "cache:" prefix. Their expired-row sweep
// also reclaims stale lock rows, which is harmless: an expired lock is free.
const dbLockKeyPrefix = "lock:"

// DDL matches internal/cache mysql/postgres backends byte-for-byte so either
// side can create the shared table first. SQLite has no cache backend, so the
// locker owns its shape there; expires_at is unix milliseconds because SQLite
// TEXT timestamps do not compare correctly in SQL.
const (
	dbLockerMySQLCreateTableQuery = `
CREATE TABLE IF NOT EXISTS kv_store (
  ` + "`key`" + ` VARCHAR(128) PRIMARY KEY,
  ` + "`value`" + ` MEDIUMBLOB NOT NULL,
  ` + "`expires_at`" + ` TIMESTAMP NULL DEFAULT NULL,
  INDEX idx_expires (` + "`expires_at`" + `)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
`

	dbLockerPostgresCreateTableQuery = `
CREATE UNLOGGED TABLE IF NOT EXISTS kv_store (
  key VARCHAR(128) PRIMARY KEY,
  value BYTEA NOT NULL,
  expires_at TIMESTAMPTZ NULL DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_kv_store_key_hash ON kv_store USING HASH (key);
CREATE INDEX IF NOT EXISTS idx_kv_store_expires ON kv_store (expires_at) WHERE expires_at IS NOT NULL;
`

	dbLockerSQLiteCreateTableQuery = `
CREATE TABLE IF NOT EXISTS kv_store (
  key TEXT PRIMARY KEY,
  value BLOB NOT NULL,
  expires_at INTEGER NULL DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_kv_store_expires ON kv_store (expires_at) WHERE expires_at IS NOT NULL;
`
)

// DBLocker implements distributed locking on top of the shared kv_store
// table. It works on any SQL backend and needs no extra infrastructure,
// which makes it the multi-instance fallback when Redis is not configured.
type DBLocker struct {
	db      base.DB
	dialect DBDialect
}

func NewDBLocker(db base.DB, dialect DBDialect) *DBLocker {
	l := &DBLocker{db: db, dialect: dialect}

	if err := l.ensureTable(context.TODO()); err != nil {
		panic(fmt.Errorf(
			"failed to ensure kv_store table: %w \n"+
				"You can manually create the table by executing the following SQL statement:\n %s",
			err,
			l.createTableQuery(),
		))
	}

	return l
}

func (l *DBLocker) createTableQuery() string {
	switch l.dialect {
	case DBDialectMySQL:
		return dbLockerMySQLCreateTableQuery
	case DBDialectPostgres:
		return dbLockerPostgresCreateTableQuery
	case DBDialectSQLite:
		return dbLockerSQLiteCreateTableQuery
	default:
		return dbLockerSQLiteCreateTableQuery
	}
}

func (l *DBLocker) ensureTable(ctx context.Context) error {
	_, err := l.db.ExecContext(ctx, l.createTableQuery())

	return err
}

// timeArg converts an instant to the column representation of the dialect.
func (l *DBLocker) timeArg(t time.Time) any {
	if l.dialect == DBDialectSQLite {
		return t.UnixMilli()
	}

	return t.UTC()
}

func (l *DBLocker) Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, error) {
	if ttl <= 0 {
		return nil, errors.New("ttl must be positive")
	}

	token, err := newToken()
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate lock token")
	}

	fullKey := dbLockKeyPrefix + key
	now := time.Now()

	acquired, err := l.tryAcquire(ctx, fullKey, token, now, now.Add(ttl))
	if err != nil {
		return nil, errors.WithMessage(err, "failed to acquire db lock")
	}

	if !acquired {
		return nil, ErrLocked
	}

	return &dbLock{owner: l, key: fullKey, token: token}, nil
}

func (l *DBLocker) tryAcquire(
	ctx context.Context,
	fullKey, token string,
	now, expiresAt time.Time,
) (bool, error) {
	if l.dialect == DBDialectMySQL {
		return l.tryAcquireMySQL(ctx, fullKey, token, now, expiresAt)
	}

	query := `INSERT INTO kv_store (key, value, expires_at) VALUES (?, ?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at
WHERE kv_store.expires_at IS NOT NULL AND kv_store.expires_at < ?`
	if l.dialect == DBDialectPostgres {
		query = `INSERT INTO kv_store (key, value, expires_at) VALUES ($1, $2, $3)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, expires_at = EXCLUDED.expires_at
WHERE kv_store.expires_at IS NOT NULL AND kv_store.expires_at < $4`
	}

	res, err := l.db.ExecContext(ctx, query, fullKey, []byte(token), l.timeArg(expiresAt), l.timeArg(now))
	if err != nil {
		return false, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return rows > 0, nil
}

// tryAcquireMySQL has no conditional upsert, so it steals an expired row
// first and only then inserts; each statement is atomic on its own, and the
// unique key makes concurrent inserts resolve to a single winner.
func (l *DBLocker) tryAcquireMySQL(
	ctx context.Context,
	fullKey, token string,
	now, expiresAt time.Time,
) (bool, error) {
	res, err := l.db.ExecContext(ctx,
		"UPDATE kv_store SET `value` = ?, `expires_at` = ? WHERE `key` = ? AND `expires_at` IS NOT NULL AND `expires_at` < ?",
		[]byte(token), l.timeArg(expiresAt), fullKey, l.timeArg(now))
	if err != nil {
		return false, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	if rows > 0 {
		return true, nil
	}

	res, err = l.db.ExecContext(ctx,
		"INSERT IGNORE INTO kv_store (`key`, `value`, `expires_at`) VALUES (?, ?, ?)",
		fullKey, []byte(token), l.timeArg(expiresAt))
	if err != nil {
		return false, err
	}

	rows, err = res.RowsAffected()
	if err != nil {
		return false, err
	}

	return rows > 0, nil
}

func (l *DBLocker) release(ctx context.Context, fullKey, token string) error {
	query := "DELETE FROM kv_store WHERE key = ? AND value = ?"

	switch l.dialect {
	case DBDialectMySQL:
		query = "DELETE FROM kv_store WHERE `key` = ? AND `value` = ?"
	case DBDialectPostgres:
		query = "DELETE FROM kv_store WHERE key = $1 AND value = $2"
	case DBDialectSQLite:
	}

	_, err := l.db.ExecContext(ctx, query, fullKey, []byte(token))

	return err
}

func (l *DBLocker) refresh(ctx context.Context, fullKey, token string, expiresAt time.Time) (bool, error) {
	query := "UPDATE kv_store SET expires_at = ? WHERE key = ? AND value = ?"

	switch l.dialect {
	case DBDialectMySQL:
		query = "UPDATE kv_store SET `expires_at` = ? WHERE `key` = ? AND `value` = ?"
	case DBDialectPostgres:
		query = "UPDATE kv_store SET expires_at = $1 WHERE key = $2 AND value = $3"
	case DBDialectSQLite:
	}

	res, err := l.db.ExecContext(ctx, query, l.timeArg(expiresAt), fullKey, []byte(token))
	if err != nil {
		return false, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return rows > 0, nil
}

type dbLock struct {
	mu       sync.Mutex
	owner    *DBLocker
	key      string
	token    string
	released bool
}

func (l *dbLock) Release(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.released {
		return nil
	}

	if err := l.owner.release(ctx, l.key, l.token); err != nil {
		return errors.WithMessage(err, "db lock release failed")
	}

	l.released = true

	return nil
}

func (l *dbLock) Refresh(ctx context.Context, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.released {
		return errors.New("lock already released")
	}

	if ttl <= 0 {
		return errors.New("ttl must be positive")
	}

	refreshed, err := l.owner.refresh(ctx, l.key, l.token, time.Now().Add(ttl))
	if err != nil {
		return errors.WithMessage(err, "db lock refresh failed")
	}

	if !refreshed {
		return ErrLockLost
	}

	return nil
}
