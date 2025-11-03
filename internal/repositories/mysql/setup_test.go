package mysql_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/migrations"
	"github.com/gameap/gameap/pkg/testcontainer"
	_ "github.com/go-sql-driver/mysql" // MySQL driver
)

// SetupTestDB sets up and returns a test database connection for MySQL tests.
func SetupTestDB(t *testing.T, mysqlDSN string) *sql.DB {
	t.Helper()

	db, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		t.Fatalf("failed to open MySQL database: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})

	clearTestDB(t, db)

	err = migrations.Run(context.Background(), testcontainer.NewContainer(
		testcontainer.WithDB(db),
		testcontainer.WithConfig(&config.Config{
			DatabaseDriver: "mysql",
		}),
	))
	if err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return db
}

func clearTestDB(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx := context.Background()

	_, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0")
	if err != nil {
		t.Fatalf("failed to disable foreign key checks: %v", err)
	}
	defer func() {
		_, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1")
		if err != nil {
			t.Errorf("failed to enable foreign key checks: %v", err)
		}
	}()

	rows, err := db.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		t.Fatalf("failed to query tables: %v", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.Errorf("failed to close rows: %v", closeErr)
		}
	}()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Fatalf("failed to scan table name: %v", err)
		}
		tables = append(tables, tableName)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("error iterating tables: %v", err)
	}

	for _, table := range tables {
		_, err := db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table))
		if err != nil {
			t.Fatalf("failed to drop table %s: %v", table, err)
		}
	}
}
