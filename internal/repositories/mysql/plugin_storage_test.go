package mysql_test

import (
	"os"
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/gameap/gameap/internal/repositories/mysql"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestPluginStorageRepository(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")
	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL tests because TEST_MYSQL_DSN is not set")
	}

	suite.Run(t, repotesting.NewPluginStorageRepositorySuite(
		func(t *testing.T) repositories.PluginStorageRepository {
			t.Helper()

			return mysql.NewPluginStorageRepository(SetupTestDB(t, testMySQLDSN))
		},
	))
}

func TestPluginStorageRepositoryScopeCollapse(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")
	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL tests because TEST_MYSQL_DSN is not set")
	}

	db := SetupTestDB(t, testMySQLDSN)

	repotesting.RunPluginStorageScopeCollapseTests(t, db, func(db base.DB) repositories.PluginStorageRepository {
		return mysql.NewPluginStorageRepository(db)
	})
}
