package postgres_test

import (
	"os"
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/gameap/gameap/internal/repositories/postgres"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestPluginStorageRepository(t *testing.T) {
	testPostgresDSN := os.Getenv("TEST_POSTGRES_DSN")
	if testPostgresDSN == "" {
		t.Skip("Skipping PostgreSQL tests because TEST_POSTGRES_DSN is not set")
	}

	suite.Run(t, repotesting.NewPluginStorageRepositorySuite(
		func(t *testing.T) repositories.PluginStorageRepository {
			t.Helper()

			return postgres.NewPluginStorageRepository(SetupTestDB(t, testPostgresDSN))
		},
	))
}

func TestPluginStorageRepositoryScopeCollapse(t *testing.T) {
	testPostgresDSN := os.Getenv("TEST_POSTGRES_DSN")
	if testPostgresDSN == "" {
		t.Skip("Skipping PostgreSQL tests because TEST_POSTGRES_DSN is not set")
	}

	db := SetupTestDB(t, testPostgresDSN)

	repotesting.RunPluginStorageScopeCollapseTests(t, db, func(db base.DB) repositories.PluginStorageRepository {
		return postgres.NewPluginStorageRepository(db)
	})
}
