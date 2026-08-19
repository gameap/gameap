package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"

	"github.com/gameap/gameap/internal/repositories/sqlite"
)

func TestPluginStorageRepository(t *testing.T) {
	suite.Run(t, repotesting.NewPluginStorageRepositorySuite(
		func(t *testing.T) repositories.PluginStorageRepository {
			t.Helper()

			return sqlite.NewPluginStorageRepository(SetupTestDB(t))
		},
	))
}

func TestPluginStorageRepositoryScopeCollapse(t *testing.T) {
	db := SetupTestDB(t)

	repotesting.RunPluginStorageScopeCollapseTests(t, db, func(db base.DB) repositories.PluginStorageRepository {
		return sqlite.NewPluginStorageRepository(db)
	})
}
