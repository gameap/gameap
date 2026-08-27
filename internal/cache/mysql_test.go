package cache_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/repositories/base"
	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestMySQLCache(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")
	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL cache tests because TEST_MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", testMySQLDSN)
	require.NoError(t, err)
	defer db.Close()

	_, _ = db.Exec("DROP TABLE IF EXISTS kv_store")

	suite.Run(t, cache.NewCacheSuite(
		func(_ *testing.T) cache.Cache {
			return cache.NewMySQL(db)
		},
	))

	_, _ = db.Exec("DROP TABLE IF EXISTS kv_store")
}

func BenchmarkMySQLCache_Set(b *testing.B) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")
	if testMySQLDSN == "" {
		b.Skip("Skipping MySQL cache benchmarks because TEST_MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", testMySQLDSN)
	require.NoError(b, err)
	defer db.Close()

	_, _ = db.Exec("DROP TABLE IF EXISTS kv_store")

	c := cache.NewMySQL(db)
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		key := "bench_key_" + string(rune(b.N%1000))
		_ = c.Set(ctx, key, "benchmark_value")
	}
}

func BenchmarkMySQLCache_Get(b *testing.B) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")
	if testMySQLDSN == "" {
		b.Skip("Skipping MySQL cache benchmarks because TEST_MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", testMySQLDSN)
	require.NoError(b, err)
	defer db.Close()

	_, _ = db.Exec("DROP TABLE IF EXISTS kv_store")

	c := cache.NewMySQL(db)
	ctx := context.Background()

	for i := range 1000 {
		key := "bench_key_" + string(rune(i))
		_ = c.Set(ctx, key, "benchmark_value")
	}

	b.ResetTimer()
	for range b.N {
		key := "bench_key_" + string(rune(b.N%1000))
		_, _ = c.Get(ctx, key)
	}
}

func BenchmarkMySQLCache_Delete(b *testing.B) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")
	if testMySQLDSN == "" {
		b.Skip("Skipping MySQL cache benchmarks because TEST_MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", testMySQLDSN)
	require.NoError(b, err)
	defer db.Close()

	_, _ = db.Exec("DROP TABLE IF EXISTS kv_store")

	c := cache.NewMySQL(db)
	ctx := context.Background()

	b.ResetTimer()
	for i := range b.N {
		key := "bench_key_" + string(rune(i))
		_ = c.Set(ctx, key, "benchmark_value")
		_ = c.Delete(ctx, key)
	}
}

// overwriteOnDeleteDB runs a callback just before the first DELETE reaches the
// database, which is the one interleaving Pull cannot observe for itself: MySQL
// has no DELETE ... RETURNING, so the value is read by a separate statement and
// could be replaced before the delete lands.
type overwriteOnDeleteDB struct {
	base.DB

	overwrite func()
	fired     bool
}

func (d *overwriteOnDeleteDB) ExecContext(
	ctx context.Context, query string, args ...any,
) (sql.Result, error) {
	if !d.fired && strings.HasPrefix(strings.TrimSpace(query), "DELETE") {
		d.fired = true

		d.overwrite()
	}

	return d.DB.ExecContext(ctx, query, args...)
}

func TestMySQLCachePullDoesNotReturnOverwrittenValue(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")
	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL cache tests because TEST_MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", testMySQLDSN)
	require.NoError(t, err)

	defer db.Close()

	_, _ = db.Exec("DROP TABLE IF EXISTS kv_store")

	defer func() { _, _ = db.Exec("DROP TABLE IF EXISTS kv_store") }()

	ctx := context.Background()
	plain := cache.NewMySQL(db)
	require.NoError(t, plain.Set(ctx, "ticket", "first"))

	hooked := &overwriteOnDeleteDB{DB: db, overwrite: func() {
		_ = plain.Set(ctx, "ticket", "second")
	}}

	value, err := cache.NewMySQL(hooked).Pull(ctx, "ticket")

	require.True(t, hooked.fired, "the delete must have run for this to prove anything")
	require.ErrorIs(t, err, cache.ErrNotFound,
		"a value replaced between the read and the delete was never this caller's to hand back")
	assert.Nil(t, value)

	remaining, err := plain.Get(ctx, "ticket")
	require.NoError(t, err, "the replacement must survive: this Pull removed nothing")
	assert.Equal(t, "second", remaining)
}
