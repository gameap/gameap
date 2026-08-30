package sqlite_test

import (
	"database/sql"
	"testing"

	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/require"
)

func TestServerUserMigration023EnforcesUniquePairs(t *testing.T) {
	// A private in-memory database: the shared one used by the other tests
	// is already migrated past the version this test has to stop at.
	db, err := sql.Open("sqlite", "file:migration023?mode=memory&cache=shared")
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, db.Close()) })

	repotesting.RunServerUserUniqueMigrationTest(t, db, "sqlite")
}
