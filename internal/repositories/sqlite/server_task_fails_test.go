package sqlite_test

import (
	"testing"

	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/sqlite"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/stretchr/testify/suite"
)

func TestServerTaskFailRepository(t *testing.T) {
	suite.Run(t, repotesting.NewServerTaskFailRepositorySuite(
		func(t *testing.T) repositories.ServerTaskFailRepository {
			t.Helper()

			return sqlite.NewServerTaskFailRepository(SetupTestDB(t))
		},
	))
}
