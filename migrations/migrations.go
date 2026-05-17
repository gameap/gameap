package migrations

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/migrations/mysql"
	"github.com/gameap/gameap/migrations/postgres"
	"github.com/gameap/gameap/migrations/sqlite"
	"github.com/pkg/errors"
	"github.com/pressly/goose/v3"
)

//go:embed mysql/*.sql sqlite/*.sql postgres/*.sql
var migrationsFS embed.FS

func GetFS() embed.FS {
	return migrationsFS
}

type container interface {
	Config() *config.Config

	DB() *sql.DB
}

type migration struct {
	version int64
	upFN    func(ctx context.Context, tx *sql.Tx) error
	downFN  func(ctx context.Context, tx *sql.Tx) error
}

// List of SQLite-specific migrations in Go.
var sqliteMigrationsList = []migration{
	{version: 1, upFN: sqlite.Up001, downFN: sqlite.Down001},
	{version: 7, upFN: sqlite.Up007, downFN: sqlite.Down007},
	{version: 8, upFN: sqlite.Up008, downFN: sqlite.Down008},
}

// SqliteMigrations returns the list of SQLite-specific migrations in Go.
func SqliteMigrations(_ context.Context, _ container) (goose.Migrations, error) {
	m := make(goose.Migrations, 0, len(sqliteMigrationsList))

	for _, mig := range sqliteMigrationsList {
		m = append(m,
			goose.NewGoMigration(
				mig.version,
				&goose.GoFunc{RunTx: mig.upFN}, &goose.GoFunc{RunTx: mig.downFN},
			),
		)
	}

	return m, nil
}

// List of Postgres-specific migrations in Go.
var postgresMigrationsList = []migration{
	{version: 7, upFN: postgres.Up007, downFN: postgres.Down007},
	{version: 8, upFN: postgres.Up008, downFN: postgres.Down008},
}

func PostgresMigrations(_ context.Context, _ container) (goose.Migrations, error) {
	m := make(goose.Migrations, 0, len(postgresMigrationsList))

	for _, mig := range postgresMigrationsList {
		m = append(m,
			goose.NewGoMigration(
				mig.version,
				&goose.GoFunc{RunTx: mig.upFN}, &goose.GoFunc{RunTx: mig.downFN},
			),
		)
	}

	return m, nil
}

// List of MySQL-specific migrations in Go.
var mysqlMigrationsList = []migration{
	{version: 1, upFN: mysql.Up001, downFN: mysql.Down001},
	{version: 7, upFN: mysql.Up007, downFN: mysql.Down007},
	{version: 8, upFN: mysql.Up008, downFN: mysql.Down008},
}

func MySQLMigrations(_ context.Context, _ container) (goose.Migrations, error) {
	m := make(goose.Migrations, 0, len(mysqlMigrationsList))

	for _, mig := range mysqlMigrationsList {
		m = append(m,
			goose.NewGoMigration(
				mig.version,
				&goose.GoFunc{RunTx: mig.upFN}, &goose.GoFunc{RunTx: mig.downFN},
			),
		)
	}

	return m, nil
}

const (
	databaseDriverMySQL    = "mysql"
	databaseDriverPostgres = "postgres"
	databaseDriverPGX      = "pgx"
	databaseDriverSQLite   = "sqlite"
	databaseDriverInMemory = "inmemory"
)

var driverToDialectMap = map[string]goose.Dialect{
	databaseDriverMySQL:    goose.DialectMySQL,
	databaseDriverSQLite:   goose.DialectSQLite3,
	databaseDriverPostgres: goose.DialectPostgres,
	databaseDriverPGX:      goose.DialectPostgres,
}

var driverToFSDirMap = map[string]string{
	databaseDriverMySQL:    "mysql",
	databaseDriverSQLite:   "sqlite",
	databaseDriverPostgres: "postgres",
	databaseDriverPGX:      "postgres",
}

func Run(ctx context.Context, c container) error {
	if c.Config().DatabaseDriver == databaseDriverInMemory {
		return nil
	}

	migratorOptions := []goose.ProviderOption{
		goose.WithSlog(slog.Default()),
		goose.WithAllowOutofOrder(true),
	}

	switch c.Config().DatabaseDriver {
	case databaseDriverMySQL:
		mg, err := MySQLMigrations(ctx, c)
		if err != nil {
			return errors.Wrap(err, "failed to get mysql migrations")
		}

		migratorOptions = append(migratorOptions, goose.WithGoMigrations(mg...))
	case databaseDriverPostgres, databaseDriverPGX:
		mg, err := PostgresMigrations(ctx, c)
		if err != nil {
			return errors.Wrap(err, "failed to get postgres migrations")
		}

		migratorOptions = append(migratorOptions, goose.WithGoMigrations(mg...))
	case databaseDriverSQLite:
		mg, err := SqliteMigrations(ctx, c)
		if err != nil {
			return errors.Wrap(err, "failed to get sqlite migrations")
		}

		migratorOptions = append(migratorOptions, goose.WithGoMigrations(mg...))
	}

	dialect, ok := driverToDialectMap[c.Config().DatabaseDriver]
	if !ok {
		return errors.Errorf("unsupported database driver: %s", c.Config().DatabaseDriver)
	}

	dir, ok := driverToFSDirMap[c.Config().DatabaseDriver]
	if !ok {
		return errors.Errorf("unsupported database driver for migrations: %s", c.Config().DatabaseDriver)
	}

	migrationsSubFS, err := fs.Sub(GetFS(), dir)
	if err != nil {
		return errors.Wrap(err, "failed to create sub filesystem")
	}

	migrator, err := goose.NewProvider(
		dialect,
		c.DB(),
		migrationsSubFS,
		migratorOptions...,
	)
	if err != nil {
		return errors.Wrap(err, "failed to create migrator")
	}

	result, err := migrator.Up(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to run migrations")
	}

	for i := range result {
		fields := make([]any, 0, 3)

		if result[i].Source != nil {
			fields = append(fields, slog.String("path", result[i].Source.Path))
			fields = append(fields, slog.Int64("version", result[i].Source.Version))
		}

		fields = append(fields, slog.String("result", result[i].String()))

		slog.InfoContext(
			ctx,
			"Applied migration",
			fields...,
		)
	}

	return nil
}
