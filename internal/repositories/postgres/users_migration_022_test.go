package postgres_test

import (
	"database/sql"
	"os"
	"testing"

	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	"github.com/stretchr/testify/require"
)

func TestUsersMigration022NormalizesLoginsAndEmails(t *testing.T) {
	testPostgresDSN := os.Getenv("TEST_POSTGRES_DSN")
	if testPostgresDSN == "" {
		t.Skip("Skipping PostgreSQL tests because TEST_POSTGRES_DSN is not set")
	}

	db, err := sql.Open("pgx", testPostgresDSN)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, db.Close()) })

	clearTestDB(t, db)

	// users_login_unique and users_email_unique compare case-sensitively here,
	// so an installation can already hold a case-only duplicate.
	repotesting.RunNormalizeUserIdentifiersMigrationTest(t, db, "postgres", true)
}
