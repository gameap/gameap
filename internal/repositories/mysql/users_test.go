package mysql_test

import (
	"os"
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/mysql"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestUserRepository(t *testing.T) {
	testMySQLDSN := os.Getenv("TEST_MYSQL_DSN")

	if testMySQLDSN == "" {
		t.Skip("Skipping MySQL tests because TEST_MYSQL_DSN is not set")
	}

	suite.Run(t, repotesting.NewUserRepositorySuite(
		func(_ *testing.T) repositories.UserRepository {
			return mysql.NewUserRepository(SetupTestDB(t, testMySQLDSN))
		},
	))
}
