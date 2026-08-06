// White-box coverage for the DB startup ping retry (pingDBWithRetry): a
// transiently unavailable database must be retried with backoff within
// Config.DatabaseConnectTimeout, while context cancellation and an exhausted
// window must surface the last ping error. Uses go-sqlmock ping monitoring, so
// no live database handle is needed.

package application

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gameap/gameap/internal/config"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errDBShuttingDown    = errors.New("FATAL: the database system is shutting down")
	errConnectionRefused = errors.New("connect: connection refused")
)

func newPingMonitoredDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	return db, mock
}

func TestPingDBWithRetry_TransientFailures(t *testing.T) {
	db, mock := newPingMonitoredDB(t)
	mock.ExpectPing().WillReturnError(errDBShuttingDown)
	mock.ExpectPing().WillReturnError(errConnectionRefused)
	mock.ExpectPing()

	cfg := &config.Config{DatabaseConnectTimeout: 30 * time.Second}
	c := newMinimalContainer(cfg)

	err := c.pingDBWithRetry(db)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPingDBWithRetry_WindowExhausted(t *testing.T) {
	db, mock := newPingMonitoredDB(t)
	mock.ExpectPing().WillReturnError(errDBShuttingDown)

	cfg := &config.Config{DatabaseConnectTimeout: 100 * time.Millisecond}
	c := newMinimalContainer(cfg)

	start := time.Now()
	err := c.pingDBWithRetry(db)

	require.Error(t, err)
	assert.ErrorIs(t, err, errDBShuttingDown)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPingDBWithRetry_ContextCancelled(t *testing.T) {
	db, mock := newPingMonitoredDB(t)
	mock.ExpectPing().WillReturnError(errConnectionRefused)

	ctx, cancel := context.WithCancel(context.Background())
	cfg := &config.Config{DatabaseConnectTimeout: 30 * time.Second}
	c := NewContainer(cfg)
	c.context = ctx

	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	err := c.pingDBWithRetry(db)

	require.Error(t, err)
	assert.ErrorIs(t, err, errConnectionRefused)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestPingDBWithRetry_NilContext(t *testing.T) {
	db, mock := newPingMonitoredDB(t)
	mock.ExpectPing()

	cfg := &config.Config{DatabaseConnectTimeout: 30 * time.Second}
	c := NewContainer(cfg)

	err := c.pingDBWithRetry(db)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
