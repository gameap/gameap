package testing

import (
	"context"
	"database/sql"
	"testing"

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

	repo := newRepo(db)

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
		require.NoError(t, repo.Save(ctx, &domain.Plugin{
			ID:                 seed.id,
			Name:               seed.name,
			Version:            "1.0.0",
			APIVersion:         "1",
			AllowedPermissions: seed.allowed,
			Status:             domain.PluginStatusActive,
		}))
	}

	_, err = provider.UpTo(ctx, grantRuntimePermissionsVersion)
	require.NoError(t, err)

	granted := []domain.PluginPermission{
		domain.PluginPermissionManageServers,
		domain.PluginPermissionNodeCommands,
		domain.PluginPermissionListenEvents,
	}

	for _, seed := range seeds {
		plugins, err := repo.Find(ctx, filters.FindPluginByIDs(seed.id), nil, nil)
		require.NoError(t, err)
		require.Len(t, plugins, 1, seed.name)

		for _, permission := range granted {
			assert.Truef(t, plugins[0].HasPermission(permission), "%s must hold %s", seed.name, permission)
		}

		for _, permission := range seed.allowed {
			assert.Truef(t, plugins[0].HasPermission(permission), "%s must keep %s", seed.name, permission)
		}

		seen := make(map[domain.PluginPermission]int)
		for _, permission := range plugins[0].AllowedPermissions {
			seen[permission]++
			assert.Equalf(t, 1, seen[permission], "%s holds %s twice", seed.name, permission)
		}
	}
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
