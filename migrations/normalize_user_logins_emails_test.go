package migrations_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/migrations"
	"github.com/gameap/gameap/pkg/testcontainer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // SQLite driver
)

const (
	versionBeforeNormalize = 21
	normalizeVersion       = 22
)

// The postgres copy of 022 carries byte-identical SQL, so the collision skipping
// exercised here covers both. MySQL folds unconditionally because its collation
// already rules the collisions out.
func TestMigration022NormalizesUserLoginsAndEmails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	db, err := sql.Open("sqlite", "file:migration022?mode=memory&cache=shared")
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	provider, err := migrations.NewProvider(ctx, testcontainer.NewContainer(
		testcontainer.WithDB(db),
		testcontainer.WithConfig(&config.Config{DatabaseDriver: "sqlite"}),
	))
	require.NoError(t, err)

	_, err = provider.UpTo(ctx, versionBeforeNormalize)
	require.NoError(t, err)

	insert := func(id int, login, email string) {
		t.Helper()

		_, err = db.ExecContext(ctx,
			"INSERT INTO users (id, login, email, password) VALUES (?, ?, ?, ?)",
			id, login, email, "hash",
		)
		require.NoError(t, err)
	}

	insert(1, "MixedLogin", "Mixed@Example.COM")
	insert(2, "plainlogin", "plain@example.com")
	// Only differ by case: folding either one would break users_email_unique, so
	// 022 has to leave both alone rather than abort the upgrade.
	insert(3, "dupone", "Dup@Example.com")
	insert(4, "duptwo", "dup@example.com")

	_, err = provider.UpTo(ctx, normalizeVersion)
	require.NoError(t, err, "a case-only duplicate must not abort the migration")

	read := func(id int) (string, string) {
		t.Helper()

		var login, email string
		require.NoError(t, db.QueryRowContext(ctx,
			"SELECT login, email FROM users WHERE id = ?", id,
		).Scan(&login, &email))

		return login, email
	}

	login, email := read(1)
	assert.Equal(t, "mixedlogin", login)
	assert.Equal(t, "mixed@example.com", email)

	login, email = read(2)
	assert.Equal(t, "plainlogin", login)
	assert.Equal(t, "plain@example.com", email)

	_, email = read(3)
	assert.Equal(t, "Dup@Example.com", email, "the colliding row must be left as it was")

	_, email = read(4)
	assert.Equal(t, "dup@example.com", email)

	// What leaving the pair alone costs, stated outright. Before this migration
	// the LOWER(...) comparison matched both rows and the lowest id won; the
	// exact comparison the repositories now use reaches only the row that is
	// already lowercase, so id 3 stops being findable by that address.
	var matched []int

	rows, err := db.QueryContext(ctx,
		"SELECT id FROM users WHERE email = ? ORDER BY id", "dup@example.com")
	require.NoError(t, err)

	defer rows.Close()

	for rows.Next() {
		var id int
		require.NoError(t, rows.Scan(&id))

		matched = append(matched, id)
	}

	require.NoError(t, rows.Err())
	assert.Equal(t, []int{4}, matched,
		"a case-only duplicate resolves to the already-lowercase row, and only that one")
}
