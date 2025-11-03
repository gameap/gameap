package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/migrations"
	"github.com/gameap/gameap/pkg/testcontainer"
	_ "modernc.org/sqlite" // SQLite driver
)

func SetupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})

	err = migrations.Run(context.Background(), testcontainer.NewContainer(
		testcontainer.WithDB(db),
		testcontainer.WithConfig(&config.Config{
			DatabaseDriver: "sqlite",
		}),
	))
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}
