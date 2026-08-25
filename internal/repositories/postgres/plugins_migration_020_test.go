package postgres_test

import (
	"database/sql"
	"os"
	"testing"

	trmsql "github.com/avito-tech/go-transaction-manager/drivers/sql/v2"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/gameap/gameap/internal/repositories/postgres"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/require"
)

func TestPluginsMigration020GrantsRuntimePermissions(t *testing.T) {
	testPostgresDSN := os.Getenv("TEST_POSTGRES_DSN")
	if testPostgresDSN == "" {
		t.Skip("Skipping PostgreSQL tests because TEST_POSTGRES_DSN is not set")
	}

	db, err := sql.Open("pgx", testPostgresDSN)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, db.Close()) })

	clearTestDB(t, db)

	repotesting.RunGrantRuntimePermissionsMigrationTest(t, db, "postgres",
		func(db *sql.DB) repositories.PluginRepository {
			return postgres.NewPluginRepository(base.NewDBTxWrapper(db, trmsql.DefaultCtxGetter))
		})
}
