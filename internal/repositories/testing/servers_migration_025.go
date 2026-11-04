package testing

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	rustQueryPortVersion       = 25
	versionBeforeRustQueryPort = rustQueryPortVersion - 1
)

type rustQueryPortSeed struct {
	id         int64
	gameCode   string
	serverPort int64
	queryPort  *int64
	wantQuery  *int64
	why        string
}

// RunRustQueryPortMigrationTest checks migration 025 on a real database: Rust
// servers saved with the query port equal to the game port (or none at all)
// move to game port + 2, where Rust answers A2S, while deliberately chosen
// query ports and other engines stay as they were. db must be empty; driver is
// the DATABASE_DRIVER value.
func RunRustQueryPortMigrationTest(t *testing.T, db *sql.DB, driver string) {
	t.Helper()

	ctx := context.Background()

	provider, err := migrations.NewProvider(ctx, migrationContainer{
		db:  db,
		cfg: &config.Config{DatabaseDriver: driver},
	})
	require.NoError(t, err)

	_, err = provider.UpTo(ctx, versionBeforeRustQueryPort)
	require.NoError(t, err)

	seedGameRow(ctx, t, db, driver, "rust", "rust")
	seedGameRow(ctx, t, db, driver, "rustcustom", "Rust")
	seedGameRow(ctx, t, db, driver, "cstrike", "GoldSource")

	seeds := []rustQueryPortSeed{
		{
			id: 1, gameCode: "rust", serverPort: 28015, queryPort: new(int64(28015)), wantQuery: new(int64(28017)),
			why: "the old form saved the game port as the query port",
		},
		{
			id: 2, gameCode: "rust", serverPort: 28100, queryPort: nil, wantQuery: new(int64(28102)),
			why: "no query port means the game port to the query handler",
		},
		{
			id: 3, gameCode: "rust", serverPort: 28200, queryPort: new(int64(28300)), wantQuery: new(int64(28300)),
			why: "a query port set apart from the game port was chosen deliberately",
		},
		{
			id: 4, gameCode: "rustcustom", serverPort: 28400, queryPort: new(int64(28400)), wantQuery: new(int64(28402)),
			why: "the engine is matched case-insensitively",
		},
		{
			id: 5, gameCode: "cstrike", serverPort: 27015, queryPort: new(int64(27015)), wantQuery: new(int64(27015)),
			why: "GoldSource answers queries on the game port",
		},
		{
			id: 6, gameCode: "rust", serverPort: 65534, queryPort: new(int64(65534)), wantQuery: new(int64(65534)),
			why: "game port + 2 would leave the port range",
		},
	}
	for _, seed := range seeds {
		seedServerPortsRow(ctx, t, db, driver, seed)
	}

	_, err = provider.Up(ctx)
	require.NoError(t, err)

	got := readServerQueryPorts(ctx, t, db, driver)
	require.Len(t, got, len(seeds))

	for _, seed := range seeds {
		assert.Equalf(t, seed.wantQuery, got[seed.id], "server %d: %s", seed.id, seed.why)
	}
}

func seedGameRow(ctx context.Context, t *testing.T, db *sql.DB, driver, code, engine string) {
	t.Helper()

	query, args, err := sq.Insert("games").
		Columns("code", "name", "engine").
		Values(code, code, engine).
		PlaceholderFormat(driverPlaceholders(driver)).
		ToSql()
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, query, args...)
	require.NoErrorf(t, err, "seeding game %q", code)
}

func seedServerPortsRow(ctx context.Context, t *testing.T, db *sql.DB, driver string, seed rustQueryPortSeed) {
	t.Helper()

	query, args, err := sq.Insert("servers").
		Columns(
			"id", "uid", "name", "game_id", "ds_id", "game_mod_id",
			"server_ip", "server_port", "query_port", "dir",
		).
		Values(
			seed.id, fmt.Sprintf("00000000-0000-0000-0000-%012d", seed.id), "server", seed.gameCode, 1, 1,
			"127.0.0.1", seed.serverPort, seed.queryPort, "servers/test",
		).
		PlaceholderFormat(driverPlaceholders(driver)).
		ToSql()
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, query, args...)
	require.NoErrorf(t, err, "seeding server %d", seed.id)
}

func readServerQueryPorts(ctx context.Context, t *testing.T, db *sql.DB, driver string) map[int64]*int64 {
	t.Helper()

	query, _, err := sq.Select("id", "query_port").From("servers").
		PlaceholderFormat(driverPlaceholders(driver)).
		ToSql()
	require.NoError(t, err)

	rows, err := db.QueryContext(ctx, query)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	ports := make(map[int64]*int64)

	for rows.Next() {
		var (
			id        int64
			queryPort sql.NullInt64
		)
		require.NoError(t, rows.Scan(&id, &queryPort))

		if queryPort.Valid {
			ports[id] = new(queryPort.Int64)
		} else {
			ports[id] = nil
		}
	}

	require.NoError(t, rows.Err())

	return ports
}
