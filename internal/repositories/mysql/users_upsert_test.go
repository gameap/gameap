package mysql_test

import (
	"context"
	"os"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserRepository_UpsertByUniqueKeyReturnsExistingID covers the MySQL-only
// upsert branch (`ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`): when an
// ID-less user collides on the unique login/email key, Save must resolve to the
// existing row and report its id via LastInsertId() — matching the pg/sqlite
// RETURNING id contract instead of leaving the id at 0.
//
// This is intentionally not in the shared UserRepository suite: SQLite and
// PostgreSQL only upsert on an `id` conflict, so an ID-less re-save with a
// duplicate login is rejected there rather than updating — a shared subtest
// would fail on those drivers.
func TestUserRepository_UpsertByUniqueKeyReturnsExistingID(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")

	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL tests because TEST_MYSQL_DSN is not set")
	}

	// ARRANGE — persist a first user and remember its assigned id.
	ctx := context.Background()
	repo := mysql.NewUserRepository(SetupTestDB(t, testMySQLDSN))

	original := &domain.User{
		Login:    "upsertkey",
		Email:    "upsertkey@example.com",
		Password: "hashedpassword",
		Name:     new("Original"),
	}
	require.NoError(t, repo.Save(ctx, original))
	require.NotZero(t, original.ID)
	existingID := original.ID

	// ACT — a fresh, ID-less user carrying the same unique login/email.
	fresh := &domain.User{
		ID:       0,
		Login:    "upsertkey",
		Email:    "upsertkey@example.com",
		Password: "rehashed",
		Name:     new("Rewritten"),
	}
	err := repo.Save(ctx, fresh)

	// ASSERT — the conflict resolves to the existing row's id, and no duplicate
	// is inserted.
	require.NoError(t, err)
	assert.Equal(t, existingID, fresh.ID,
		"an upsert on a unique-key conflict must return the existing row id, not 0")

	all, err := repo.Find(ctx, &filters.FindUser{Logins: []string{"upsertkey"}}, nil, nil)
	require.NoError(t, err)
	require.Len(t, all, 1, "an ID-less re-save on a unique key must update, not insert a duplicate")
	assert.Equal(t, existingID, all[0].ID)
	assert.Equal(t, "rehashed", all[0].Password, "the conflicting save must overwrite the existing row")
}
