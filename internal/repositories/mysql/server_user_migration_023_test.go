package mysql_test

import (
	"database/sql"
	"os"
	"testing"

	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	_ "github.com/go-sql-driver/mysql" // MySQL driver
	"github.com/stretchr/testify/require"
)

func TestServerUserMigration023EnforcesUniquePairs(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")
	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL tests because TEST_MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", testMySQLDSN)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, db.Close()) })

	clearTestDB(t, db)

	repotesting.RunServerUserUniqueMigrationTest(t, db, "mysql")
}
