package mysql_test

import (
	"os"
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/mysql"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestServerTaskRepository(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")

	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL tests because TEST_MYSQL_DSN is not set")
	}

	suite.Run(t, repotesting.NewServerTaskRepositorySuite(
		func(_ *testing.T) repositories.ServerTaskRepository {
			return mysql.NewServerTaskRepository(SetupTestDB(t, testMySQLDSN))
		},
	))
}
