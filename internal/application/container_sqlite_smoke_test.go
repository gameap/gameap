// Construction-path smoke coverage for the SQLite arms of the DI container.
// Every other container test either injects an already-open *sql.DB
// (newWiredContainer) or runs on the inmemory driver, so the real
// sql.Open + ping path in createDB and the SQLite branches of the task /
// DLQ repository factories are never entered, and neither are the memory
// branches of the cache, pub-sub and file-manager factories reached through a
// container that owns its own handle.
//
// This walks that graph once through the public accessors, plus the two ways
// createDB refuses to hand back a handle (bad DSN, unregistered driver). It is
// deliberately a smoke test: the per-repository behaviour lives in
// internal/repositories/sqlite, and the lazy-singleton sweep across the whole
// container lives in container_factories_test.go.

package application

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/files"
	pubsubmemory "github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recoverPanic runs fn and returns the value it panicked with, or nil if fn
// returned normally.
func recoverPanic(fn func()) (recovered any) {
	defer func() {
		recovered = recover()
	}()

	fn()

	return nil
}

// requirePanicError runs fn and returns the error it panicked with. The
// container reports unrecoverable wiring problems by panicking with an error
// rather than returning one, so recovering it is the only way to assert on the
// message.
func requirePanicError(t *testing.T, fn func()) error {
	t.Helper()

	recovered := recoverPanic(fn)
	require.NotNil(t, recovered, "the accessor was expected to panic")

	err, ok := recovered.(error)
	require.True(t, ok, "the panic value must be an error, got %T", recovered)

	return err
}

// TestContainer_SQLiteDriverSmoke drives a container that owns its own SQLite
// handle: DB() must open and ping it, the driver switches must pick the SQLite
// repositories and the memory cache / pub-sub / local file manager, every
// accessor must memoise, and Shutdown must close the graph without error.
func TestContainer_SQLiteDriverSmoke(t *testing.T) {
	t.Parallel()

	// ARRANGE
	cfg := sqliteWiredConfig(t)
	// A non-positive connect timeout makes pingDBWithRetry do a single
	// unbounded attempt, so a healthy database is reached without any backoff
	// sleep. Retry behaviour itself is covered in container_db_test.go.
	cfg.DatabaseConnectTimeout = 0

	c := newMinimalContainer(cfg)
	t.Cleanup(func() {
		if c.db != nil {
			_ = c.db.Close()
		}
	})

	// ACT
	db := c.DB()
	txDB := c.TransactionalDB()
	daemonTasks := c.DaemonTaskRepository()
	serverTasks := c.ServerTaskRepository()
	serverTaskExecutions := c.ServerTaskExecutionRepository()
	dlqRepo := c.DLQRepository()
	cacheStore := c.Cache()
	pubSub := c.PubSub()
	fileManager := c.FileManager()

	// ASSERT
	require.NotNil(t, db, "DB must open a handle for the sqlite driver")
	require.NoError(t, db.PingContext(context.Background()),
		"the handle DB() returns must be live, not merely non-nil")
	require.NotNil(t, txDB, "TransactionalDB must wrap the opened handle")

	require.IsType(t, &sqlite.DaemonTaskRepository{}, daemonTasks,
		"sqlite driver must build the sqlite daemon-task repository")
	require.IsType(t, &sqlite.ServerTaskRepository{}, serverTasks,
		"sqlite driver must build the sqlite server-task repository")
	require.IsType(t, &sqlite.ServerTaskExecutionRepository{}, serverTaskExecutions,
		"sqlite driver must build the sqlite server-task-execution repository")
	require.IsType(t, &sqlite.DLQRepository{}, dlqRepo,
		"sqlite driver must build the sqlite DLQ repository")

	assert.IsType(t, &cache.InMemory{}, cacheStore,
		"memory cache driver must build the in-memory cache")
	assert.IsType(t, &pubsubmemory.Memory{}, pubSub,
		"memory pub-sub driver must be handed back unwrapped while retry is disabled")
	assert.IsType(t, &files.LocalFileManager{}, fileManager,
		"local files driver must build the local file manager")

	// Every accessor is a lazy singleton: a second call must hand back what the
	// first one built instead of re-entering the factory.
	assert.Same(t, db, c.DB(), "DB must memoise the opened handle")
	assert.Same(t, txDB, c.TransactionalDB(), "TransactionalDB must memoise its wrapper")
	assert.Same(t, daemonTasks, c.DaemonTaskRepository(), "DaemonTaskRepository must memoise")
	assert.Same(t, serverTasks, c.ServerTaskRepository(), "ServerTaskRepository must memoise")
	assert.Same(t, serverTaskExecutions, c.ServerTaskExecutionRepository(),
		"ServerTaskExecutionRepository must memoise")
	assert.Same(t, dlqRepo, c.DLQRepository(), "DLQRepository must memoise")
	assert.Same(t, cacheStore, c.Cache(), "Cache must memoise")
	assert.Same(t, pubSub, c.PubSub(), "PubSub must memoise")
	assert.Same(t, fileManager, c.FileManager(), "FileManager must memoise")

	require.NoError(t, c.Shutdown(),
		"Shutdown must drain the late shutdown funcs registered by DB() without error")
}

// TestContainer_DB_PanicsWhenPingIsRefused pins the createDB error path a
// misconfigured DATABASE_URL takes: sql.Open is lazy, so the problem only
// surfaces on the ping, and DB() must turn that into a panic rather than hand
// back a dead handle.
func TestContainer_DB_PanicsWhenPingIsRefused(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// mode=rw stops SQLite from creating the file, and its parent directory
	// does not exist either, so the connection the ping opens is refused.
	cfg := &config.Config{
		DatabaseDriver:         databaseDriverSQLite,
		DatabaseURL:            "file:" + filepath.Join(t.TempDir(), "missing-dir", "panel.db") + "?mode=rw",
		DatabaseConnectTimeout: 0,
	}
	c := newMinimalContainer(cfg)

	// ACT
	err := requirePanicError(t, func() { c.DB() })

	// ASSERT
	assert.Contains(t, err.Error(), "failed to ping database",
		"an unreachable database must be reported as a ping error, not a connect error")
	assert.Contains(t, err.Error(), "unable to open database file",
		"the driver's own diagnosis must be preserved in the wrapped error")
}

// TestContainer_DB_PanicsWhenDriverIsUnknown pins the other createDB error
// path: an unregistered DATABASE_DRIVER is rejected by sql.Open itself, before
// any ping.
func TestContainer_DB_PanicsWhenDriverIsUnknown(t *testing.T) {
	t.Parallel()

	// ARRANGE
	cfg := &config.Config{
		DatabaseDriver: "no-such-driver",
		DatabaseURL:    "ignored-by-an-unregistered-driver",
	}
	c := newMinimalContainer(cfg)

	// ACT
	err := requirePanicError(t, func() { c.DB() })

	// ASSERT
	assert.Contains(t, err.Error(), "failed to connect to database",
		"an unregistered driver must be reported as a connect error, not a ping error")
	assert.Contains(t, err.Error(), `unknown driver "no-such-driver"`,
		"the rejected driver name must be preserved in the wrapped error")
}
