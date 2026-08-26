package mysql_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/mysql"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/require"
)

func TestPluginsMigration020GrantsRuntimePermissions(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")
	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL tests because TEST_MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", testMySQLDSN)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, db.Close()) })

	clearTestDB(t, db)

	repotesting.RunGrantRuntimePermissionsMigrationTest(t, db, "mysql",
		func(db *sql.DB) repositories.PluginRepository {
			return mysql.NewPluginRepository(db)
		})
}
