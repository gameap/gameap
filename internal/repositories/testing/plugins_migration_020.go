package testing

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	grantRuntimePermissionsVersion = 20
	versionBeforeRuntimeGrants     = grantRuntimePermissionsVersion - 1
)

// RunGrantRuntimePermissionsMigrationTest checks migration 020 on a real
// database: installations that predate the manage_servers / node_commands /
// listen_events gates are grandfathered the grants, whatever their row held
// before, and nothing already granted is lost. db must be empty; driver is
// the DATABASE_DRIVER value; newRepo builds the plugin repository over db.
// Rows are seeded with plain SQL because the repository writes columns that
// later migrations add; the rows are read back through the repository once
// the schema is fully migrated (no migration after 020 touches grants).
func RunGrantRuntimePermissionsMigrationTest(
	t *testing.T,
	db *sql.DB,
	driver string,
	newRepo func(*sql.DB) repositories.PluginRepository,
) {
	t.Helper()

	ctx := context.Background()

	provider, err := migrations.NewProvider(ctx, migrationContainer{
		db:  db,
		cfg: &config.Config{DatabaseDriver: driver},
	})
	require.NoError(t, err)

	_, err = provider.UpTo(ctx, versionBeforeRuntimeGrants)
	require.NoError(t, err)

	// Rows as installations from before the gates could have left them:
	// no grants at all, the "files" grant from 015, a deliberate grant that
	// must survive, and one that already holds a new grant (no duplicate).
	seeds := []struct {
		id      domain.Uint64ID
		name    string
		allowed []domain.PluginPermission
	}{
		{id: 201, name: "ungranted", allowed: nil},
		{id: 205, name: "empty-list", allowed: []domain.PluginPermission{}},
		{id: 202, name: "files-only", allowed: []domain.PluginPermission{domain.PluginPermissionFiles}},
		{id: 203, name: "rbac", allowed: []domain.PluginPermission{domain.PluginPermissionManageRBAC}},
		{id: 204, name: "already", allowed: []domain.PluginPermission{
			domain.PluginPermissionFiles, domain.PluginPermissionManageServers,
		}},
	}

	for _, seed := range seeds {
		seedPluginRow(ctx, t, db, driver, seed.id, seed.name, seed.allowed)
	}

	_, err = provider.UpTo(ctx, grantRuntimePermissionsVersion)
	require.NoError(t, err)

	_, err = provider.Up(ctx)
	require.NoError(t, err)

	repo := newRepo(db)

	for _, seed := range seeds {
		plugins, err := repo.Find(ctx, filters.FindPluginByIDs(seed.id), nil, nil)
		require.NoError(t, err)
		require.Len(t, plugins, 1, seed.name)

		// The complete set, not a containment check: an accidentally wider
		// grant (a permission the migration was never meant to hand out)
		// must fail here rather than reach an installation.
		assert.ElementsMatchf(t, expectedPermissions(driver, seed.allowed), plugins[0].AllowedPermissions,
			"%s must hold exactly the grandfathered permissions", seed.name)

		seen := make(map[domain.PluginPermission]int)
		for _, permission := range plugins[0].AllowedPermissions {
			seen[permission]++
			assert.Equalf(t, 1, seen[permission], "%s holds %s twice", seed.name, permission)
		}
	}
}

// seedPluginRow inserts a row the way an installation from before migration
// 020 would have stored it: only the columns of the 002 table, with the
// grants in the driver's own encoding (a text array on Postgres, JSON
// elsewhere) and NULL for "never granted".
func seedPluginRow(
	ctx context.Context,
	t *testing.T,
	db *sql.DB,
	driver string,
	id domain.Uint64ID,
	name string,
	allowed []domain.PluginPermission,
) {
	t.Helper()

	postgres := driver == "postgres" || driver == "pgx"

	var grants *string
	switch {
	case allowed == nil:
	case postgres:
		if len(allowed) > 0 {
			elements := make([]string, len(allowed))
			for i, permission := range allowed {
				elements[i] = string(permission)
			}

			grants = new("{" + strings.Join(elements, ",") + "}")
		}
	default:
		encoded, err := json.Marshal(allowed)
		require.NoError(t, err)

		grants = new(string(encoded))
	}

	builder := sq.Insert("plugins").
		Columns("id", "name", "version", "description", "author", "api_version", "allowed_permissions", "status").
		Values(id, name, "1.0.0", "", "", "1", grants, domain.PluginStatusActive)
	if postgres {
		builder = builder.PlaceholderFormat(sq.Dollar)
	}

	query, args, err := builder.ToSql()
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, query, args...)
	require.NoError(t, err)
}

// runtimeGrants are what 020 grandfathers onto every installation.
var runtimeGrants = []domain.PluginPermission{
	domain.PluginPermissionManageServers,
	domain.PluginPermissionNodeCommands,
	domain.PluginPermissionListenEvents,
}

// expectedPermissions answers the exact set a plugin must hold after 020.
// A row that held nothing also gains "files" on SQLite: migration 015
// compared the BLOB column against 'null' as text and missed those rows, so
// 020 repairs what the other drivers already granted back then.
func expectedPermissions(driver string, before []domain.PluginPermission) []domain.PluginPermission {
	expected := append([]domain.PluginPermission(nil), before...)

	for _, permission := range runtimeGrants {
		if !slices.Contains(expected, permission) {
			expected = append(expected, permission)
		}
	}

	if driver == "sqlite" && len(before) == 0 && !slices.Contains(expected, domain.PluginPermissionFiles) {
		expected = append(expected, domain.PluginPermissionFiles)
	}

	return expected
}

// migrationContainer is the slice of the application container the
// migrator reads (pkg/testcontainer would import the in-memory repositories,
// whose tests import this package).
type migrationContainer struct {
	db  *sql.DB
	cfg *config.Config
}

func (c migrationContainer) Config() *config.Config { return c.cfg }
func (c migrationContainer) DB() *sql.DB            { return c.db }
