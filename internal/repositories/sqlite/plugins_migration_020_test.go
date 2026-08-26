package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/require"
)

func TestPluginsMigration020GrantsRuntimePermissions(t *testing.T) {
	// A private in-memory database: the shared one used by the other tests
	// is already migrated past the version this test has to stop at.
	db, err := sql.Open("sqlite", "file:migration020?mode=memory&cache=shared")
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, db.Close()) })

	repotesting.RunGrantRuntimePermissionsMigrationTest(t, db, "sqlite",
		func(db *sql.DB) repositories.PluginRepository {
			return sqlite.NewPluginRepository(db)
		})
}
