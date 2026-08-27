package mysql_test

import (
	"database/sql"
	"os"
	"testing"

	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	_ "github.com/go-sql-driver/mysql" // MySQL driver
	"github.com/stretchr/testify/require"
)

func TestUsersMigration022NormalizesLoginsAndEmails(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")
	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL tests because TEST_MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", testMySQLDSN)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, db.Close()) })

	clearTestDB(t, db)

	// Migration 001 creates users with DEFAULT CHARSET=utf8mb4 and no COLLATE,
	// which pins utf8mb4's own default collation (case-insensitive) whatever
	// collation_server says, so users_login_unique and users_email_unique refuse
	// a case-only duplicate and the seed cannot contain one. An installation
	// upgraded from the legacy PHP panel keeps whichever collation that schema
	// was created with, and a case-sensitive one can hold such pairs -- which is
	// why 022 no longer folds unconditionally.
	repotesting.RunNormalizeUserIdentifiersMigrationTest(t, db, "mysql", false)
}
