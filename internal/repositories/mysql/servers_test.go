package mysql_test

import (
	"os"
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/mysql"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/gameap/gameap/internal/services"
	"github.com/stretchr/testify/suite"
)

func TestServerRepository(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")

	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL tests because TEST_MYSQL_DSN is not set")
	}

	suite.Run(t, repotesting.NewServerRepositorySuite(
		func(t *testing.T) repositories.ServerRepository {
			t.Helper()

			db := SetupTestDB(t, testMySQLDSN)
			tm := services.NewNilTransactionManager()

			return mysql.NewServerRepository(db, tm)
		},
	))
}
