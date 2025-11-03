package postgres_test

import (
	"os"
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/postgres"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestServerTaskRepository(t *testing.T) {
	testPostgresDSN := os.Getenv("TEST_POSTGRES_DSN")

	if testPostgresDSN == "" {
		t.Skip("Skipping PostgreSQL tests because TEST_POSTGRES_DSN is not set")
	}

	suite.Run(t, repotesting.NewServerTaskRepositorySuite(
		func(t *testing.T) repositories.ServerTaskRepository {
			t.Helper()

			return postgres.NewServerTaskRepository(SetupTestDB(t, testPostgresDSN))
		},
	))
}
