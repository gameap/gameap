package testing

import (
	"context"
	"database/sql"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	serverUserUniqueVersion       = 23
	versionBeforeServerUserUnique = serverUserUniqueVersion - 1
)

// RunServerUserUniqueMigrationTest checks migration 023 on a real database:
// duplicate (user_id, server_id) pairs an installation accumulated while the
// table had no uniqueness are collapsed to one row each, distinct pairs all
// survive, and the new unique index refuses a plain duplicate insert -- the
// guarantee AttachUserServer's conflict-safe insert relies on. db must be
// empty; driver is the DATABASE_DRIVER value.
func RunServerUserUniqueMigrationTest(t *testing.T, db *sql.DB, driver string) {
	t.Helper()

	ctx := context.Background()

	provider, err := migrations.NewProvider(ctx, migrationContainer{
		db:  db,
		cfg: &config.Config{DatabaseDriver: driver},
	})
	require.NoError(t, err)

	_, err = provider.UpTo(ctx, versionBeforeServerUserUnique)
	require.NoError(t, err)

	seeds := [][2]int64{
		{7001, 1}, {7001, 1}, {7001, 1},
		{7001, 2},
		{7002, 1}, {7002, 1},
		{7003, 3},
	}
	for _, pair := range seeds {
		seedServerUserRow(ctx, t, db, driver, pair[0], pair[1])
	}

	_, err = provider.Up(ctx)
	require.NoError(t, err)

	wantPairs := [][2]int64{
		{7001, 1},
		{7001, 2},
		{7002, 1},
		{7003, 3},
	}
	assert.Equal(t, wantPairs, readServerUserPairs(ctx, t, db, driver),
		"duplicates must collapse to one row, distinct pairs must all survive")

	// From here on the index, not the application, holds the invariant.
	query, args, err := sq.Insert("server_user").
		Columns("user_id", "server_id").
		Values(7003, 3).
		PlaceholderFormat(driverPlaceholders(driver)).
		ToSql()
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, query, args...)
	require.Error(t, err, "the unique index must refuse a plain duplicate insert")
}

func seedServerUserRow(
	ctx context.Context,
	t *testing.T,
	db *sql.DB,
	driver string,
	userID int64,
	serverID int64,
) {
	t.Helper()

	query, args, err := sq.Insert("server_user").
		Columns("user_id", "server_id").
		Values(userID, serverID).
		PlaceholderFormat(driverPlaceholders(driver)).
		ToSql()
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, query, args...)
	require.NoErrorf(t, err, "seeding pair (%d, %d)", userID, serverID)
}

func readServerUserPairs(
	ctx context.Context,
	t *testing.T,
	db *sql.DB,
	driver string,
) [][2]int64 {
	t.Helper()

	query, _, err := sq.Select("user_id", "server_id").From("server_user").
		OrderBy("user_id", "server_id").
		PlaceholderFormat(driverPlaceholders(driver)).
		ToSql()
	require.NoError(t, err)

	rows, err := db.QueryContext(ctx, query)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	var pairs [][2]int64

	for rows.Next() {
		var userID, serverID int64
		require.NoError(t, rows.Scan(&userID, &serverID))

		pairs = append(pairs, [2]int64{userID, serverID})
	}

	require.NoError(t, rows.Err())

	return pairs
}
